package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/STRML/claude-cells/internal/daemon"
	"github.com/STRML/claude-cells/internal/workstream"
)

func TestWaitForDaemon_SocketExists(t *testing.T) {
	tmpDir := testShortDir(t)

	// Create the socket file immediately
	sockPath := filepath.Join(tmpDir, "daemon.sock")
	f, err := os.Create(sockPath)
	if err != nil {
		t.Fatalf("Failed to create socket file: %v", err)
	}
	f.Close()

	// waitForDaemon should return almost immediately
	start := time.Now()
	waitForDaemon(sockPath, 5*time.Second)
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("waitForDaemon took %v when socket already exists, expected <500ms", elapsed)
	}
}

func TestWaitForDaemon_SocketAppearsLater(t *testing.T) {
	tmpDir := testShortDir(t)

	sockPath := filepath.Join(tmpDir, "daemon.sock")

	// Create the socket after a delay
	go func() {
		time.Sleep(200 * time.Millisecond)
		f, _ := os.Create(sockPath)
		f.Close()
	}()

	start := time.Now()
	waitForDaemon(sockPath, 5*time.Second)
	elapsed := time.Since(start)

	if elapsed < 150*time.Millisecond {
		t.Errorf("waitForDaemon returned too quickly: %v", elapsed)
	}
	if elapsed > 2*time.Second {
		t.Errorf("waitForDaemon took too long: %v", elapsed)
	}
}

func TestWaitForDaemon_Timeout(t *testing.T) {
	tmpDir := testShortDir(t)

	sockPath := filepath.Join(tmpDir, "nonexistent-daemon.sock")

	start := time.Now()
	waitForDaemon(sockPath, 200*time.Millisecond)
	elapsed := time.Since(start)

	if elapsed < 150*time.Millisecond {
		t.Errorf("waitForDaemon returned too early: %v (timeout=200ms)", elapsed)
	}
	if elapsed > 1*time.Second {
		t.Errorf("waitForDaemon took too long: %v (timeout=200ms)", elapsed)
	}
}

func TestSendDaemonRequest_RetryOnConnectionRefused(t *testing.T) {
	tmpDir := testShortDir(t)
	sockPath := filepath.Join(tmpDir, "daemon.sock")

	// No daemon running — should fail after retries
	start := time.Now()
	_, err := sendDaemonRequestWithResponse(sockPath, "ping", nil, 1*time.Second)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error when daemon not running")
	}
	// With 3 retries at 500ms apart, should take at least 1 second
	if elapsed < 900*time.Millisecond {
		t.Errorf("retries completed too fast: %v (expected >=1s with 3 retries)", elapsed)
	}
}

func TestSendDaemonRequest_ConnectsAfterRetry(t *testing.T) {
	tmpDir := testShortDir(t)
	sockPath := filepath.Join(tmpDir, "daemon.sock")

	// Start a daemon after a short delay
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Small delay to simulate daemon starting up
		time.Sleep(600 * time.Millisecond)
		d := daemon.New(daemon.Config{
			SocketPath: sockPath,
		})
		d.Run(ctx)
	}()

	// Send a request — first attempt fails, retry succeeds
	resp, err := sendDaemonRequestWithResponse(sockPath, "ping", nil, 5*time.Second)
	cancel()
	wg.Wait()

	if err != nil {
		t.Fatalf("expected success after retry, got error: %v", err)
	}
	if !resp.OK {
		t.Errorf("expected OK response, got: %+v", resp)
	}
}

func TestSendDaemonRequest_PingResponse(t *testing.T) {
	tmpDir := testShortDir(t)
	sockPath := filepath.Join(tmpDir, "daemon.sock")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start daemon immediately
	d := daemon.New(daemon.Config{
		SocketPath: sockPath,
	})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.Run(ctx)
	}()

	// Wait for socket to appear
	waitForDaemon(sockPath, 5*time.Second)

	// Send a ping request
	resp, err := sendDaemonRequestWithResponse(sockPath, "ping", nil)
	if err != nil {
		t.Fatalf("ping error: %v", err)
	}
	if !resp.OK {
		t.Errorf("ping response not OK: %+v", resp)
	}

	cancel()
	wg.Wait()
}

func TestSendDaemonRequest_CreateWithoutHandler(t *testing.T) {
	tmpDir := testShortDir(t)
	sockPath := filepath.Join(tmpDir, "daemon.sock")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start daemon without OnCreate handler
	d := daemon.New(daemon.Config{
		SocketPath: sockPath,
	})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.Run(ctx)
	}()

	waitForDaemon(sockPath, 5*time.Second)

	// Send a create request — should fail because handler is nil
	params, _ := json.Marshal(map[string]string{
		"branch":  "test-branch",
		"prompt":  "test prompt",
		"runtime": "claude",
	})
	resp, err := sendDaemonRequestWithResponse(sockPath, "create", params)
	if err != nil {
		t.Fatalf("create request error: %v", err)
	}
	if resp.OK {
		t.Error("create should fail without handler configured")
	}
	if resp.Error == "" {
		t.Error("create error response should have error message")
	}

	cancel()
	wg.Wait()
}

func TestSendDaemonRequest_CreateWithHandler(t *testing.T) {
	tmpDir := testShortDir(t)
	sockPath := filepath.Join(tmpDir, "daemon.sock")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var createCalled bool
	var capturedBranch, capturedPrompt, capturedRuntime string
	var capturedSkipPane bool

	// Start daemon with OnCreate handler
	d := daemon.New(daemon.Config{
		SocketPath: sockPath,
		OnCreate: func(ctx context.Context, branch, prompt, runtime string, skipPane bool, opts daemon.CreateExtraOpts) (string, error) {
			createCalled = true
			capturedBranch = branch
			capturedPrompt = prompt
			capturedRuntime = runtime
			capturedSkipPane = skipPane
			return "ccells-test-project-test-branch", nil
		},
	})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.Run(ctx)
	}()

	waitForDaemon(sockPath, 5*time.Second)

	// Send a create request
	params, _ := json.Marshal(map[string]interface{}{
		"branch":    "test-branch",
		"prompt":    "fix the login bug",
		"runtime":   "claude",
		"skip_pane": true,
	})
	resp, err := sendDaemonRequestWithResponse(sockPath, "create", params, 30*time.Second)
	if err != nil {
		t.Fatalf("create request error: %v", err)
	}
	if !resp.OK {
		t.Errorf("create should succeed, got error: %s", resp.Error)
	}

	// Verify handler was called with correct params
	if !createCalled {
		t.Error("OnCreate handler was not called")
	}
	if capturedBranch != "test-branch" {
		t.Errorf("branch = %q, want %q", capturedBranch, "test-branch")
	}
	if capturedPrompt != "fix the login bug" {
		t.Errorf("prompt = %q, want %q", capturedPrompt, "fix the login bug")
	}
	if capturedRuntime != "claude" {
		t.Errorf("runtime = %q, want %q", capturedRuntime, "claude")
	}
	if !capturedSkipPane {
		t.Error("skipPane should be true")
	}

	// Verify response contains container name
	var data map[string]string
	if resp.Data != nil {
		json.Unmarshal(resp.Data, &data)
	}
	if data["container"] != "ccells-test-project-test-branch" {
		t.Errorf("container = %q, want %q", data["container"], "ccells-test-project-test-branch")
	}

	cancel()
	wg.Wait()
}

func TestSendDaemonRequest_ShutdownGraceful(t *testing.T) {
	tmpDir := testShortDir(t)
	sockPath := filepath.Join(tmpDir, "daemon.sock")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := daemon.New(daemon.Config{
		SocketPath: sockPath,
	})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.Run(ctx)
	}()

	waitForDaemon(sockPath, 5*time.Second)

	// Send shutdown request
	resp, err := sendDaemonRequestWithResponse(sockPath, "shutdown", nil)
	if err != nil {
		t.Fatalf("shutdown request error: %v", err)
	}
	if !resp.OK {
		t.Errorf("shutdown should succeed, got error: %s", resp.Error)
	}

	// Daemon should exit after shutdown
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Expected
	case <-time.After(5 * time.Second):
		t.Error("daemon should exit after shutdown request")
		cancel() // Clean up
	}
}

func TestDaemonStartupFlow_SocketReady(t *testing.T) {
	// This test verifies the complete startup flow:
	// 1. Daemon starts in goroutine
	// 2. waitForDaemon confirms socket is ready
	// 3. Client can successfully send requests
	// This is the same pattern used in cmd_up.go's runUp function.
	tmpDir := testShortDir(t)
	sockPath := filepath.Join(tmpDir, "daemon.sock")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start daemon in goroutine (same as cmd_up.go lines 117-122)
	var wg sync.WaitGroup
	wg.Add(1)
	d := daemon.New(daemon.Config{
		SocketPath: sockPath,
		OnCreate: func(ctx context.Context, branch, prompt, runtime string, skipPane bool, opts daemon.CreateExtraOpts) (string, error) {
			return "test-container", nil
		},
	})
	go func() {
		defer wg.Done()
		d.Run(ctx)
	}()

	// Wait for daemon socket (same as cmd_up.go line 127)
	waitForDaemon(sockPath, 5*time.Second)

	// Verify socket exists
	if _, err := os.Stat(sockPath); os.IsNotExist(err) {
		t.Fatal("daemon socket should exist after waitForDaemon")
	}

	// Simulate what the subprocess does: send a ping
	resp, err := sendDaemonRequestWithResponse(sockPath, "ping", nil)
	if err != nil {
		t.Fatalf("ping after waitForDaemon should succeed: %v", err)
	}
	if !resp.OK {
		t.Error("ping response should be OK")
	}

	// Simulate what the create dialog does: send a create request
	params, _ := json.Marshal(map[string]interface{}{
		"branch":    "my-feature",
		"prompt":    "implement login page",
		"runtime":   "claude",
		"skip_pane": true,
	})
	resp, err = sendDaemonRequestWithResponse(sockPath, "create", params, 30*time.Second)
	if err != nil {
		t.Fatalf("create after waitForDaemon should succeed: %v", err)
	}
	if !resp.OK {
		t.Errorf("create should succeed, got error: %s", resp.Error)
	}
	var data map[string]string
	if resp.Data != nil {
		json.Unmarshal(resp.Data, &data)
	}
	if data["container"] != "test-container" {
		t.Errorf("container = %q, want %q", data["container"], "test-container")
	}

	cancel()
	wg.Wait()

	// Socket should be cleaned up
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Error("daemon socket should be cleaned up after shutdown")
	}
}

func TestDaemonStartupFlow_CreateWithUntrackedFiles(t *testing.T) {
	tmpDir := testShortDir(t)
	sockPath := filepath.Join(tmpDir, "daemon.sock")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var capturedOpts daemon.CreateExtraOpts

	d := daemon.New(daemon.Config{
		SocketPath: sockPath,
		OnCreate: func(ctx context.Context, branch, prompt, runtime string, skipPane bool, opts daemon.CreateExtraOpts) (string, error) {
			capturedOpts = opts
			return "test-container-untracked", nil
		},
	})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.Run(ctx)
	}()

	waitForDaemon(sockPath, 5*time.Second)

	// Send create with untracked files (same as what dialog_create.go does)
	params, _ := json.Marshal(map[string]interface{}{
		"branch":          "fix-bug",
		"prompt":          "fix authentication",
		"runtime":         "claude",
		"skip_pane":       true,
		"copy_untracked":  true,
		"untracked_files": []string{"newfile.go", "readme.md"},
	})
	resp, err := sendDaemonRequestWithResponse(sockPath, "create", params, 30*time.Second)
	if err != nil {
		t.Fatalf("create error: %v", err)
	}
	if !resp.OK {
		t.Errorf("create should succeed, got error: %s", resp.Error)
	}

	// Verify untracked files were passed through
	if !capturedOpts.CopyUntracked {
		t.Error("CopyUntracked should be true")
	}
	if len(capturedOpts.UntrackedFiles) != 2 {
		t.Errorf("UntrackedFiles length = %d, want 2", len(capturedOpts.UntrackedFiles))
	}

	cancel()
	wg.Wait()
}

// testShortDir creates a temporary directory with a short path to avoid
// macOS Unix socket path length limits (104 chars).
func testShortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp/claude", "up-*")
	if err != nil {
		// Fall back to default temp dir
		dir, err = os.MkdirTemp("", "up-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// TestSendDaemonRequest_ConcurrentRequests verifies the daemon handles
// multiple concurrent requests without deadlocking or corrupting responses.
func TestSendDaemonRequest_ConcurrentRequests(t *testing.T) {
	tmpDir := testShortDir(t)
	sockPath := filepath.Join(tmpDir, "daemon.sock")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := daemon.New(daemon.Config{
		SocketPath: sockPath,
	})
	var daemonWg sync.WaitGroup
	daemonWg.Add(1)
	go func() {
		defer daemonWg.Done()
		d.Run(ctx)
	}()

	waitForDaemon(sockPath, 5*time.Second)

	// Send 10 concurrent ping requests
	const numRequests = 10
	var clientWg sync.WaitGroup
	errors := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		clientWg.Add(1)
		go func() {
			defer clientWg.Done()
			resp, err := sendDaemonRequestWithResponse(sockPath, "ping", nil)
			if err != nil {
				errors <- err
				return
			}
			if !resp.OK {
				errors <- net.UnknownNetworkError("ping not OK")
				return
			}
		}()
	}

	clientWg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent request error: %v", err)
	}

	cancel()
	daemonWg.Wait()
}

// --- Startup pane command tests ---
// These verify the critical startup logic: what command runs in the initial tmux pane.
// This is the entry point for creating workstreams on startup.

func TestDeterminePaneCommand_FirstTime(t *testing.T) {
	// No state file → welcome screen → chains to create dialog
	tmpDir := testShortDir(t)
	stateDir := filepath.Join(tmpDir, "state")
	os.MkdirAll(stateDir, 0755)

	cmd := determinePaneCommand("/usr/local/bin/ccells", stateDir)

	if cmd == "" {
		t.Fatal("first-time startup should have a pane command, got empty string")
	}
	if !strings.Contains(cmd, "welcome") {
		t.Errorf("first-time pane command should contain 'welcome', got: %s", cmd)
	}
	if !strings.Contains(cmd, "/usr/local/bin/ccells") {
		t.Errorf("pane command should contain ccells path, got: %s", cmd)
	}
	if !strings.Contains(cmd, "exec \"$SHELL\"") {
		t.Errorf("pane command should chain to exec $SHELL, got: %s", cmd)
	}
}

func TestDeterminePaneCommand_ReturningNoWorkstreams(t *testing.T) {
	// State file exists but empty workstreams → create dialog
	tmpDir := testShortDir(t)
	stateDir := filepath.Join(tmpDir, "state")
	os.MkdirAll(stateDir, 0755)

	// Create an empty state file
	if err := workstream.SaveState(stateDir, nil, 0, 0); err != nil {
		t.Fatalf("failed to create empty state: %v", err)
	}

	cmd := determinePaneCommand("/usr/local/bin/ccells", stateDir)

	if cmd == "" {
		t.Fatal("returning with 0 workstreams should have a pane command, got empty string")
	}
	if !strings.Contains(cmd, "create --interactive") {
		t.Errorf("returning pane command should contain 'create --interactive', got: %s", cmd)
	}
	if strings.Contains(cmd, "welcome") {
		t.Errorf("returning pane command should NOT contain 'welcome', got: %s", cmd)
	}
}

func TestDeterminePaneCommand_ReturningWithWorkstreams(t *testing.T) {
	// State file has workstreams → plain shell (panes restore separately)
	tmpDir := testShortDir(t)
	stateDir := filepath.Join(tmpDir, "state")
	os.MkdirAll(stateDir, 0755)

	// Create state with a workstream
	ws := workstream.New("test prompt")
	ws.BranchName = "test-branch"
	if err := workstream.SaveState(stateDir, []*workstream.Workstream{ws}, 0, 0); err != nil {
		t.Fatalf("failed to create state with workstream: %v", err)
	}

	cmd := determinePaneCommand("/usr/local/bin/ccells", stateDir)

	if cmd != "" {
		t.Errorf("with existing workstreams, pane command should be empty, got: %s", cmd)
	}
}

func TestDeterminePaneCommand_PathWithSpaces(t *testing.T) {
	// Verify the path is single-quoted for shell safety
	tmpDir := testShortDir(t)
	stateDir := filepath.Join(tmpDir, "state")
	os.MkdirAll(stateDir, 0755)

	cmd := determinePaneCommand("/path/with spaces/ccells", stateDir)

	if !strings.Contains(cmd, "'/path/with spaces/ccells'") {
		t.Errorf("path should be single-quoted, got: %s", cmd)
	}
}

// TestParseCommand_Welcome verifies that the "welcome" subcommand dispatches correctly.
// This was a real bug: "welcome" was missing from the parseCommand case list,
// causing the subprocess to print help text and exit instead of showing the welcome screen.
func TestParseCommand_Welcome(t *testing.T) {
	got := parseCommand([]string{"welcome"})
	if got != "welcome" {
		t.Errorf("parseCommand([welcome]) = %q, want %q", got, "welcome")
	}
}

// TestParseCommand_CreateInteractive verifies create with --interactive flag.
func TestParseCommand_CreateInteractive(t *testing.T) {
	got := parseCommand([]string{"create", "--interactive"})
	if got != "create" {
		t.Errorf("parseCommand([create, --interactive]) = %q, want %q", got, "create")
	}
}

// TestStartupFlowEndToEnd_FirstTime verifies the full first-time startup flow:
// 1. determinePaneCommand returns welcome command
// 2. parseCommand dispatches "welcome" correctly
// 3. Welcome dialog chains to create on Enter
// 4. Create dialog sends daemon request
// 5. Daemon returns container name
// Steps 3-5 are tested via the dialog and daemon tests respectively.
// This test ties steps 1-2 together.
func TestStartupFlowEndToEnd_FirstTime(t *testing.T) {
	tmpDir := testShortDir(t)
	stateDir := filepath.Join(tmpDir, "state")
	os.MkdirAll(stateDir, 0755)

	ccellsBin := "/usr/local/bin/ccells"

	// Step 1: Determine pane command for first-time user
	paneCmd := determinePaneCommand(ccellsBin, stateDir)
	if paneCmd == "" {
		t.Fatal("first-time startup should produce a pane command")
	}
	if !strings.Contains(paneCmd, "welcome") {
		t.Fatalf("first-time pane command should contain 'welcome', got: %s", paneCmd)
	}

	// Step 2: Verify the subprocess command dispatches correctly.
	// The pane runs: '<ccellsBin>' welcome; exec "$SHELL"
	// The subprocess parses args ["welcome"] and should dispatch to "welcome".
	got := parseCommand([]string{"welcome"})
	if got != "welcome" {
		t.Fatalf("parseCommand([welcome]) = %q, want 'welcome' — subprocess would dispatch to wrong handler", got)
	}

	// Steps 3-5 are verified by TestWelcomeDialog_EnterCreates (dialog_welcome_test.go)
	// and TestDaemonStartupFlow_SocketReady / TestSendDaemonRequest_CreateWithHandler (cmd_up_test.go).
}

// TestStartupFlowEndToEnd_Returning verifies the returning user startup flow:
// State exists with 0 workstreams → create dialog.
func TestStartupFlowEndToEnd_Returning(t *testing.T) {
	tmpDir := testShortDir(t)
	stateDir := filepath.Join(tmpDir, "state")
	os.MkdirAll(stateDir, 0755)

	// Save empty state
	if err := workstream.SaveState(stateDir, nil, 0, 0); err != nil {
		t.Fatalf("failed to create state: %v", err)
	}

	ccellsBin := "/usr/local/bin/ccells"

	// Step 1: Determine pane command
	paneCmd := determinePaneCommand(ccellsBin, stateDir)
	if paneCmd == "" {
		t.Fatal("returning with 0 workstreams should produce a pane command")
	}
	if !strings.Contains(paneCmd, "create --interactive") {
		t.Fatalf("returning pane command should contain 'create --interactive', got: %s", paneCmd)
	}

	// Step 2: Verify create --interactive dispatches correctly
	got := parseCommand([]string{"create", "--interactive"})
	if got != "create" {
		t.Fatalf("parseCommand([create, --interactive]) = %q, want 'create'", got)
	}
}
