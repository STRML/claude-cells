package gitproxy

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// mockExecutor is a test double for CommandExecutor.
type mockExecutor struct {
	mu         sync.Mutex
	response   *Response
	prResult   *PRCreateResult
	callCount  int
	lastOp     Operation
	lastArgs   []string
	lastWS     WorkstreamInfo
	executeFn  func(ctx context.Context, op Operation, args []string, ws WorkstreamInfo) (*Response, *PRCreateResult)
}

func (m *mockExecutor) Execute(ctx context.Context, op Operation, args []string, ws WorkstreamInfo) (*Response, *PRCreateResult) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callCount++
	m.lastOp = op
	m.lastArgs = args
	m.lastWS = ws

	if m.executeFn != nil {
		return m.executeFn(ctx, op, args, ws)
	}

	if m.response == nil {
		return &Response{ExitCode: 0}, m.prResult
	}
	return m.response, m.prResult
}

// TestNewServer tests creating a new server.
func TestNewServer(t *testing.T) {
	called := false
	server := NewServer(func(wsID string, prNumber int, prURL string) {
		called = true
	})

	if server == nil {
		t.Fatal("NewServer returned nil")
	}
	if server.sockets == nil {
		t.Error("sockets map not initialized")
	}
	if server.executor == nil {
		t.Error("executor not initialized")
	}
	if server.baseDir == "" {
		t.Error("baseDir not set")
	}

	// Verify callback is stored (not called yet)
	if called {
		t.Error("callback should not be called on creation")
	}
}

// TestStartSocket_Success tests starting a socket successfully.
func TestStartSocket_Success(t *testing.T) {
	server := NewServer(nil)
	tmpDir := t.TempDir()
	server.SetBaseDir(tmpDir)

	ws := WorkstreamInfo{
		ID:           "ws-123",
		Branch:       "feature/test",
		WorktreePath: "/tmp/test",
	}

	socketPath, err := server.StartSocket(context.Background(), "container-1", ws)
	if err != nil {
		t.Fatalf("StartSocket failed: %v", err)
	}
	defer server.StopSocket("container-1")

	if socketPath == "" {
		t.Error("socketPath is empty")
	}

	// Verify socket file exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Errorf("socket file does not exist: %s", socketPath)
	}

	// Verify socket path matches expected pattern
	expectedPath := filepath.Join(tmpDir, "container-1", "git.sock")
	if socketPath != expectedPath {
		t.Errorf("socketPath = %s, want %s", socketPath, expectedPath)
	}
}

// TestStartSocket_AlreadyExists tests that starting a socket for an existing container returns existing path.
func TestStartSocket_AlreadyExists(t *testing.T) {
	server := NewServer(nil)
	tmpDir := t.TempDir()
	server.SetBaseDir(tmpDir)

	ws := WorkstreamInfo{
		ID:     "ws-123",
		Branch: "feature/test",
	}

	path1, err := server.StartSocket(context.Background(), "container-1", ws)
	if err != nil {
		t.Fatalf("first StartSocket failed: %v", err)
	}
	defer server.StopSocket("container-1")

	// Starting again should return same path
	path2, err := server.StartSocket(context.Background(), "container-1", ws)
	if err != nil {
		t.Fatalf("second StartSocket failed: %v", err)
	}

	if path1 != path2 {
		t.Errorf("paths differ: %s vs %s", path1, path2)
	}
}

// TestStartSocket_UnwritableDir tests that starting a socket fails when directory is unwritable.
func TestStartSocket_UnwritableDir(t *testing.T) {
	// Skip if running as root (root can write anywhere)
	if os.Getuid() == 0 {
		t.Skip("skipping test when running as root")
	}

	// Create a directory then make it unwritable
	tmpDir := t.TempDir()
	unwritableDir := filepath.Join(tmpDir, "unwritable")
	if err := os.Mkdir(unwritableDir, 0555); err != nil {
		t.Fatalf("failed to create unwritable dir: %v", err)
	}

	server := NewServer(nil)
	server.SetBaseDir(unwritableDir)

	ws := WorkstreamInfo{
		ID:     "ws-123",
		Branch: "feature/test",
	}

	_, err := server.StartSocket(context.Background(), "container-1", ws)
	if err == nil {
		// Clean up if test fails
		server.StopSocket("container-1")
		t.Error("expected error for unwritable directory")
	}
}

// TestStopSocket_RemovesSocket tests that stopping a socket removes it.
func TestStopSocket_RemovesSocket(t *testing.T) {
	server := NewServer(nil)
	tmpDir := t.TempDir()
	server.SetBaseDir(tmpDir)

	ws := WorkstreamInfo{
		ID:     "ws-123",
		Branch: "feature/test",
	}

	socketPath, err := server.StartSocket(context.Background(), "container-1", ws)
	if err != nil {
		t.Fatalf("StartSocket failed: %v", err)
	}

	// Verify socket exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Fatal("socket should exist before stop")
	}

	// Stop socket
	server.StopSocket("container-1")

	// Verify socket is removed
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("socket should be removed after stop")
	}

	// GetSocketPath should return empty
	if path := server.GetSocketPath("container-1"); path != "" {
		t.Errorf("GetSocketPath returned %s, want empty", path)
	}
}

// TestStopSocket_NonExistent tests that stopping a non-existent socket is safe.
func TestStopSocket_NonExistent(t *testing.T) {
	server := NewServer(nil)
	// Should not panic
	server.StopSocket("nonexistent-container")
}

// TestGetSocketPath tests retrieving socket path.
func TestGetSocketPath(t *testing.T) {
	server := NewServer(nil)
	tmpDir := t.TempDir()
	server.SetBaseDir(tmpDir)

	// Non-existent container
	if path := server.GetSocketPath("container-1"); path != "" {
		t.Errorf("GetSocketPath for non-existent container = %s, want empty", path)
	}

	ws := WorkstreamInfo{
		ID:     "ws-123",
		Branch: "feature/test",
	}

	socketPath, _ := server.StartSocket(context.Background(), "container-1", ws)
	defer server.StopSocket("container-1")

	if path := server.GetSocketPath("container-1"); path != socketPath {
		t.Errorf("GetSocketPath = %s, want %s", path, socketPath)
	}
}

// TestShutdown tests that Shutdown stops all sockets.
func TestShutdown(t *testing.T) {
	server := NewServer(nil)
	tmpDir := t.TempDir()
	server.SetBaseDir(tmpDir)

	ws := WorkstreamInfo{ID: "ws-1", Branch: "branch-1"}

	server.StartSocket(context.Background(), "container-1", ws)
	server.StartSocket(context.Background(), "container-2", ws)

	server.Shutdown()

	// Verify all sockets are gone
	if path := server.GetSocketPath("container-1"); path != "" {
		t.Error("container-1 socket should be removed")
	}
	if path := server.GetSocketPath("container-2"); path != "" {
		t.Error("container-2 socket should be removed")
	}
}

// TestUpdateWorkstream tests updating workstream info.
func TestUpdateWorkstream(t *testing.T) {
	server := NewServer(nil)
	tmpDir := t.TempDir()
	server.SetBaseDir(tmpDir)

	ws := WorkstreamInfo{
		ID:       "ws-123",
		Branch:   "feature/test",
		PRNumber: 0,
	}

	server.StartSocket(context.Background(), "container-1", ws)
	defer server.StopSocket("container-1")

	// Update with PR number
	wsUpdated := WorkstreamInfo{
		ID:       "ws-123",
		Branch:   "feature/test",
		PRNumber: 42,
	}
	server.UpdateWorkstream("container-1", wsUpdated)

	// Non-existent container should not panic
	server.UpdateWorkstream("nonexistent", wsUpdated)
}

// sendRequest is a helper to send a request to a socket and read the response.
func sendRequest(t *testing.T, socketPath string, req Request) *Response {
	t.Helper()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect to socket: %v", err)
	}
	defer conn.Close()

	// Send request
	data, _ := json.Marshal(req)
	data = append(data, '\n')
	conn.Write(data)

	// Read response
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	return &resp
}

// TestHandleConnection_InvalidOperation tests that invalid operations are rejected.
func TestHandleConnection_InvalidOperation(t *testing.T) {
	server := NewServer(nil)
	tmpDir := t.TempDir()
	server.SetBaseDir(tmpDir)

	ws := WorkstreamInfo{
		ID:           "ws-123",
		Branch:       "feature/test",
		WorktreePath: t.TempDir(),
	}

	socketPath, err := server.StartSocket(context.Background(), "container-1", ws)
	if err != nil {
		t.Fatalf("StartSocket failed: %v", err)
	}
	defer server.StopSocket("container-1")

	// Give the accept loop time to start
	time.Sleep(10 * time.Millisecond)

	resp := sendRequest(t, socketPath, Request{
		Operation: "invalid-operation",
	})

	if resp.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", resp.ExitCode)
	}
	if resp.Error == "" {
		t.Error("expected error message for invalid operation")
	}
}

// TestHandleConnection_ValidationFailure tests that validation errors are returned.
func TestHandleConnection_ValidationFailure(t *testing.T) {
	server := NewServer(nil)
	tmpDir := t.TempDir()
	server.SetBaseDir(tmpDir)

	ws := WorkstreamInfo{
		ID:           "ws-123",
		Branch:       "feature/test",
		WorktreePath: t.TempDir(),
	}

	socketPath, err := server.StartSocket(context.Background(), "container-1", ws)
	if err != nil {
		t.Fatalf("StartSocket failed: %v", err)
	}
	defer server.StopSocket("container-1")

	time.Sleep(10 * time.Millisecond)

	// Try to push to wrong branch
	resp := sendRequest(t, socketPath, Request{
		Operation: OpGitPush,
		Args:      []string{"origin", "wrong-branch"},
	})

	if resp.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", resp.ExitCode)
	}
	if resp.Error == "" {
		t.Error("expected error message for validation failure")
	}
}

// TestHandleConnection_ExecutorError tests that executor errors are returned.
func TestHandleConnection_ExecutorError(t *testing.T) {
	server := NewServer(nil)
	tmpDir := t.TempDir()
	server.SetBaseDir(tmpDir)

	mock := &mockExecutor{
		response: &Response{
			ExitCode: 128,
			Stderr:   "fatal: some error",
			Error:    "command failed",
		},
	}
	server.SetExecutor(mock)

	ws := WorkstreamInfo{
		ID:           "ws-123",
		Branch:       "feature/test",
		WorktreePath: t.TempDir(),
	}

	socketPath, err := server.StartSocket(context.Background(), "container-1", ws)
	if err != nil {
		t.Fatalf("StartSocket failed: %v", err)
	}
	defer server.StopSocket("container-1")

	time.Sleep(10 * time.Millisecond)

	resp := sendRequest(t, socketPath, Request{
		Operation: OpGitFetch,
	})

	if resp.ExitCode != 128 {
		t.Errorf("ExitCode = %d, want 128", resp.ExitCode)
	}
	if resp.Stderr != "fatal: some error" {
		t.Errorf("Stderr = %s, want 'fatal: some error'", resp.Stderr)
	}
}

// TestHandleConnection_PRCreatedCallback tests that PR creation triggers callback.
func TestHandleConnection_PRCreatedCallback(t *testing.T) {
	var callbackWS string
	var callbackPR int
	var callbackURL string
	callbackCalled := make(chan struct{})

	server := NewServer(func(wsID string, prNumber int, prURL string) {
		callbackWS = wsID
		callbackPR = prNumber
		callbackURL = prURL
		close(callbackCalled)
	})
	tmpDir := t.TempDir()
	server.SetBaseDir(tmpDir)

	mock := &mockExecutor{
		response: &Response{
			ExitCode: 0,
			Stdout:   "https://github.com/owner/repo/pull/42",
		},
		prResult: &PRCreateResult{
			Number: 42,
			URL:    "https://github.com/owner/repo/pull/42",
		},
	}
	server.SetExecutor(mock)

	ws := WorkstreamInfo{
		ID:           "ws-123",
		Branch:       "feature/test",
		WorktreePath: t.TempDir(),
	}

	socketPath, err := server.StartSocket(context.Background(), "container-1", ws)
	if err != nil {
		t.Fatalf("StartSocket failed: %v", err)
	}
	defer server.StopSocket("container-1")

	time.Sleep(10 * time.Millisecond)

	resp := sendRequest(t, socketPath, Request{
		Operation: OpGHPRCreate,
		Args:      []string{"--title", "Test PR"},
	})

	if resp.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", resp.ExitCode)
	}

	// Wait for callback
	select {
	case <-callbackCalled:
		// good
	case <-time.After(time.Second):
		t.Fatal("PR created callback not called")
	}

	if callbackWS != "ws-123" {
		t.Errorf("callback workstream ID = %s, want ws-123", callbackWS)
	}
	if callbackPR != 42 {
		t.Errorf("callback PR number = %d, want 42", callbackPR)
	}
	if callbackURL != "https://github.com/owner/repo/pull/42" {
		t.Errorf("callback URL = %s, want https://github.com/owner/repo/pull/42", callbackURL)
	}
}

// TestHandleConnection_PushCompleteCallback tests that successful push triggers callback.
func TestHandleConnection_PushCompleteCallback(t *testing.T) {
	var callbackWS string
	callbackCalled := make(chan struct{})

	server := NewServer(nil)
	server.SetPushCompleteCallback(func(wsID string) {
		callbackWS = wsID
		close(callbackCalled)
	})
	tmpDir := t.TempDir()
	server.SetBaseDir(tmpDir)

	mock := &mockExecutor{
		response: &Response{
			ExitCode: 0,
			Stdout:   "Everything up-to-date",
		},
	}
	server.SetExecutor(mock)

	ws := WorkstreamInfo{
		ID:           "ws-456",
		Branch:       "feature/test",
		WorktreePath: t.TempDir(),
	}

	socketPath, err := server.StartSocket(context.Background(), "container-1", ws)
	if err != nil {
		t.Fatalf("StartSocket failed: %v", err)
	}
	defer server.StopSocket("container-1")

	time.Sleep(10 * time.Millisecond)

	resp := sendRequest(t, socketPath, Request{
		Operation: OpGitPush,
		Args:      []string{"origin", "feature/test"},
	})

	if resp.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", resp.ExitCode)
	}

	// Wait for callback
	select {
	case <-callbackCalled:
		// good
	case <-time.After(time.Second):
		t.Fatal("Push complete callback not called")
	}

	if callbackWS != "ws-456" {
		t.Errorf("callback workstream ID = %s, want ws-456", callbackWS)
	}
}

// TestHandleConnection_PushCompleteCallback_NotCalledOnFailure tests that failed push doesn't trigger callback.
func TestHandleConnection_PushCompleteCallback_NotCalledOnFailure(t *testing.T) {
	callbackCalled := false

	server := NewServer(nil)
	server.SetPushCompleteCallback(func(wsID string) {
		callbackCalled = true
	})
	tmpDir := t.TempDir()
	server.SetBaseDir(tmpDir)

	mock := &mockExecutor{
		response: &Response{
			ExitCode: 1,
			Stderr:   "push failed",
		},
	}
	server.SetExecutor(mock)

	ws := WorkstreamInfo{
		ID:           "ws-456",
		Branch:       "feature/test",
		WorktreePath: t.TempDir(),
	}

	socketPath, err := server.StartSocket(context.Background(), "container-1", ws)
	if err != nil {
		t.Fatalf("StartSocket failed: %v", err)
	}
	defer server.StopSocket("container-1")

	time.Sleep(10 * time.Millisecond)

	sendRequest(t, socketPath, Request{
		Operation: OpGitPush,
		Args:      []string{"origin", "feature/test"},
	})

	// Give time for callback to potentially fire
	time.Sleep(50 * time.Millisecond)

	if callbackCalled {
		t.Error("push complete callback should not be called on failure")
	}
}

// TestHandleConnection_InvalidJSON tests that invalid JSON is rejected.
func TestHandleConnection_InvalidJSON(t *testing.T) {
	server := NewServer(nil)
	tmpDir := t.TempDir()
	server.SetBaseDir(tmpDir)

	ws := WorkstreamInfo{
		ID:           "ws-123",
		Branch:       "feature/test",
		WorktreePath: t.TempDir(),
	}

	socketPath, err := server.StartSocket(context.Background(), "container-1", ws)
	if err != nil {
		t.Fatalf("StartSocket failed: %v", err)
	}
	defer server.StopSocket("container-1")

	time.Sleep(10 * time.Millisecond)

	// Connect and send invalid JSON
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	conn.Write([]byte("not valid json\n"))

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	var resp Response
	json.Unmarshal(line, &resp)

	if resp.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", resp.ExitCode)
	}
	if resp.Error == "" {
		t.Error("expected error for invalid JSON")
	}
}

// TestShortContainerID tests the container ID truncation helper.
func TestShortContainerID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"abc123def456", "abc123def456"},
		{"abc123def456789", "abc123def456"},
		{"short", "short"},
		{"", ""},
	}

	for _, tt := range tests {
		if got := shortContainerID(tt.input); got != tt.expected {
			t.Errorf("shortContainerID(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
