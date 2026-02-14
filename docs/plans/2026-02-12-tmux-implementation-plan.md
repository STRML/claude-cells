# tmux Integration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the Bubble Tea TUI with tmux as the window manager, making ccells a sidecar orchestrator with compose-style CLI verbs.

**Architecture:** ccells owns a tmux server (one per repo), runs Docker exec directly in tmux panes, handles dialogs via `tmux display-popup`, and persists metadata in a supplementary state file. A background daemon manages credential refresh, state reconciliation, and tmux hooks.

**Tech Stack:** Go stdlib, tmux CLI, existing orchestrator/docker/git packages. No new external Go dependencies.

**Design doc:** `docs/plans/2026-02-12-tmux-integration-design.md`

---

## Build Order

The plan builds bottom-up: tmux wrapper first, then daemon, then CLI, then wire it all together, then delete TUI.

```
Task 1-3:   internal/tmux/       (tmux command wrapper + server + pane management)
Task 4-5:   internal/daemon/     (background daemon + socket API)
Task 6-8:   cmd/ccells/          (CLI subcommands: up, attach, down, create, rm, ps)
Task 9:     Dialog programs      (ccells create --interactive, etc.)
Task 10:    Chrome               (status line, pane borders, help popup)
Task 11:    Detach summary       (post-detach resource report)
Task 12:    State reconciliation (tmux + Docker → state file sync)
Task 13:    Pairing integration  (move from TUI to daemon)
Task 14:    Wire main.go         (replace TUI startup with tmux startup)
Task 15:    Delete TUI           (remove internal/tui/, update go.mod)
Task 16:    Docs & cleanup       (README, CLAUDE.md, codemaps)
```

---

### Task 1: tmux Command Wrapper

Create a thin Go wrapper around tmux CLI commands with testable interface.

**Files:**
- Create: `internal/tmux/tmux.go`
- Create: `internal/tmux/tmux_test.go`

**Step 1: Write the failing test**

```go
// internal/tmux/tmux_test.go
package tmux

import (
    "context"
    "testing"
)

func TestNewClient(t *testing.T) {
    c := NewClient("ccells-test")
    if c.Socket() != "ccells-test" {
        t.Errorf("expected socket ccells-test, got %s", c.Socket())
    }
}

func TestServerRunning(t *testing.T) {
    c := NewClient("ccells-test-nonexistent")
    ctx := context.Background()
    running, err := c.ServerRunning(ctx)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if running {
        t.Error("expected server not running")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tmux/ -v -run TestNewClient`
Expected: FAIL — package doesn't exist

**Step 3: Write minimal implementation**

```go
// internal/tmux/tmux.go
package tmux

import (
    "context"
    "fmt"
    "os/exec"
    "strings"
)

// Client wraps tmux CLI commands for a specific server socket.
type Client struct {
    socket string
}

// NewClient creates a tmux client targeting the given socket name.
func NewClient(socket string) *Client {
    return &Client{socket: socket}
}

// Socket returns the socket name.
func (c *Client) Socket() string {
    return c.socket
}

// run executes a tmux command with the socket flag.
func (c *Client) run(ctx context.Context, args ...string) (string, error) {
    fullArgs := append([]string{"-L", c.socket}, args...)
    cmd := exec.CommandContext(ctx, "tmux", fullArgs...)
    out, err := cmd.CombinedOutput()
    return strings.TrimSpace(string(out)), err
}

// ServerRunning checks if a tmux server is running on this socket.
func (c *Client) ServerRunning(ctx context.Context) (bool, error) {
    _, err := c.run(ctx, "list-sessions")
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            // Exit code 1 = no server running
            if exitErr.ExitCode() == 1 {
                return false, nil
            }
        }
        // tmux not installed or other error
        if strings.Contains(err.Error(), "executable file not found") {
            return false, fmt.Errorf("tmux not installed: %w", err)
        }
        return false, nil
    }
    return true, nil
}

// Version returns the tmux version string (e.g., "3.4").
func (c *Client) Version(ctx context.Context) (string, error) {
    out, err := exec.CommandContext(ctx, "tmux", "-V").CombinedOutput()
    if err != nil {
        return "", fmt.Errorf("tmux not installed: %w", err)
    }
    // "tmux 3.4" -> "3.4"
    parts := strings.Fields(strings.TrimSpace(string(out)))
    if len(parts) >= 2 {
        return parts[1], nil
    }
    return strings.TrimSpace(string(out)), nil
}

// Prefix returns the user's configured tmux prefix key (e.g., "C-b").
func (c *Client) Prefix(ctx context.Context) (string, error) {
    out, err := c.run(ctx, "show-option", "-gv", "prefix")
    if err != nil {
        return "C-b", nil // default fallback
    }
    return out, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tmux/ -v`
Expected: PASS

**Step 5: Commit**

```
git add internal/tmux/
git commit -m "feat(tmux): add tmux command wrapper client

Task 1: Thin wrapper around tmux CLI with socket targeting,
server detection, version query, and prefix key reading."
```

---

### Task 2: tmux Session Management

Add session create/attach/kill to the tmux client.

**Files:**
- Modify: `internal/tmux/tmux.go`
- Modify: `internal/tmux/tmux_test.go`

**Step 1: Write the failing test**

```go
// Add to tmux_test.go — integration test (needs real tmux)
//go:build integration

func TestSessionLifecycle(t *testing.T) {
    ctx := context.Background()
    c := NewClient("ccells-test-lifecycle")
    defer c.KillServer(ctx)

    // Create session
    err := c.NewSession(ctx, "test-session")
    if err != nil {
        t.Fatalf("NewSession failed: %v", err)
    }

    // Verify running
    running, _ := c.ServerRunning(ctx)
    if !running {
        t.Fatal("server should be running after NewSession")
    }

    // Verify session exists
    exists, _ := c.HasSession(ctx, "test-session")
    if !exists {
        t.Fatal("session should exist")
    }

    // Kill server
    err = c.KillServer(ctx)
    if err != nil {
        t.Fatalf("KillServer failed: %v", err)
    }

    running, _ = c.ServerRunning(ctx)
    if running {
        t.Fatal("server should not be running after KillServer")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test -tags=integration ./internal/tmux/ -v -run TestSessionLifecycle`
Expected: FAIL — methods don't exist

**Step 3: Write minimal implementation**

```go
// Add to tmux.go

// NewSession creates a new detached tmux session.
func (c *Client) NewSession(ctx context.Context, name string) error {
    _, err := c.run(ctx, "new-session", "-d", "-s", name, "-x", "200", "-y", "50")
    return err
}

// HasSession checks if a named session exists.
func (c *Client) HasSession(ctx context.Context, name string) (bool, error) {
    _, err := c.run(ctx, "has-session", "-t", name)
    if err != nil {
        return false, nil
    }
    return true, nil
}

// KillServer kills the tmux server on this socket.
func (c *Client) KillServer(ctx context.Context) error {
    _, err := c.run(ctx, "kill-server")
    return err
}

// KillSession kills a specific session.
func (c *Client) KillSession(ctx context.Context, name string) error {
    _, err := c.run(ctx, "kill-session", "-t", name)
    return err
}

// Attach attaches to an existing session (replaces current process).
// This should be the last call — it exec's tmux attach.
func (c *Client) AttachCommand(name string) *exec.Cmd {
    return exec.Command("tmux", "-L", c.socket, "attach-session", "-t", name)
}
```

**Step 4: Run test to verify it passes**

Run: `go test -tags=integration ./internal/tmux/ -v -run TestSessionLifecycle`
Expected: PASS

**Step 5: Commit**

```
git commit -m "feat(tmux): add session lifecycle management

Task 2: NewSession, HasSession, KillServer, KillSession,
AttachCommand for session create/attach/kill."
```

---

### Task 3: tmux Pane Management

Add pane CRUD, metadata, and layout to the tmux client.

**Files:**
- Modify: `internal/tmux/tmux.go`
- Create: `internal/tmux/pane.go`
- Modify: `internal/tmux/tmux_test.go`

**Step 1: Write the failing test**

```go
//go:build integration

func TestPaneLifecycle(t *testing.T) {
    ctx := context.Background()
    c := NewClient("ccells-test-panes")
    defer c.KillServer(ctx)

    err := c.NewSession(ctx, "test-panes")
    if err != nil {
        t.Fatalf("NewSession: %v", err)
    }

    // Initial pane exists (session starts with one)
    panes, err := c.ListPanes(ctx, "test-panes")
    if err != nil {
        t.Fatalf("ListPanes: %v", err)
    }
    if len(panes) != 1 {
        t.Fatalf("expected 1 pane, got %d", len(panes))
    }

    // Create a second pane running a command
    paneID, err := c.SplitWindow(ctx, "test-panes", "sleep 300")
    if err != nil {
        t.Fatalf("SplitWindow: %v", err)
    }

    // Set metadata
    err = c.SetPaneOption(ctx, paneID, "@ccells-workstream", "test-branch")
    if err != nil {
        t.Fatalf("SetPaneOption: %v", err)
    }

    // Read metadata
    val, err := c.GetPaneOption(ctx, paneID, "@ccells-workstream")
    if err != nil {
        t.Fatalf("GetPaneOption: %v", err)
    }
    if val != "test-branch" {
        t.Errorf("expected test-branch, got %s", val)
    }

    // List panes — should be 2
    panes, err = c.ListPanes(ctx, "test-panes")
    if err != nil {
        t.Fatalf("ListPanes: %v", err)
    }
    if len(panes) != 2 {
        t.Fatalf("expected 2 panes, got %d", len(panes))
    }

    // Kill pane
    err = c.KillPane(ctx, paneID)
    if err != nil {
        t.Fatalf("KillPane: %v", err)
    }

    panes, _ = c.ListPanes(ctx, "test-panes")
    if len(panes) != 1 {
        t.Fatalf("expected 1 pane after kill, got %d", len(panes))
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test -tags=integration ./internal/tmux/ -v -run TestPaneLifecycle`
Expected: FAIL — methods don't exist

**Step 3: Write minimal implementation**

```go
// internal/tmux/pane.go
package tmux

import (
    "context"
    "fmt"
    "strings"
)

// PaneInfo describes a tmux pane.
type PaneInfo struct {
    ID        string // e.g., "%0"
    Index     int
    Active    bool
    Width     int
    Height    int
    Command   string
}

// SplitWindow creates a new pane in the session running the given command.
// Returns the pane ID (e.g., "%3").
func (c *Client) SplitWindow(ctx context.Context, session, command string) (string, error) {
    out, err := c.run(ctx, "split-window", "-t", session, "-P", "-F", "#{pane_id}", command)
    if err != nil {
        return "", fmt.Errorf("split-window: %w", err)
    }
    return strings.TrimSpace(out), nil
}

// ListPanes returns all panes in a session.
func (c *Client) ListPanes(ctx context.Context, session string) ([]PaneInfo, error) {
    format := "#{pane_id}\t#{pane_index}\t#{pane_active}\t#{pane_width}\t#{pane_height}\t#{pane_current_command}"
    out, err := c.run(ctx, "list-panes", "-t", session, "-F", format)
    if err != nil {
        return nil, fmt.Errorf("list-panes: %w", err)
    }
    var panes []PaneInfo
    for _, line := range strings.Split(out, "\n") {
        if line == "" {
            continue
        }
        fields := strings.SplitN(line, "\t", 6)
        if len(fields) < 6 {
            continue
        }
        p := PaneInfo{
            ID:      fields[0],
            Active:  fields[2] == "1",
            Command: fields[5],
        }
        fmt.Sscanf(fields[1], "%d", &p.Index)
        fmt.Sscanf(fields[3], "%d", &p.Width)
        fmt.Sscanf(fields[4], "%d", &p.Height)
        panes = append(panes, p)
    }
    return panes, nil
}

// KillPane destroys a pane by ID.
func (c *Client) KillPane(ctx context.Context, paneID string) error {
    _, err := c.run(ctx, "kill-pane", "-t", paneID)
    return err
}

// SelectPane focuses a pane by ID.
func (c *Client) SelectPane(ctx context.Context, paneID string) error {
    _, err := c.run(ctx, "select-pane", "-t", paneID)
    return err
}

// SetPaneOption sets a user-defined pane option (e.g., @ccells-workstream).
func (c *Client) SetPaneOption(ctx context.Context, paneID, key, value string) error {
    _, err := c.run(ctx, "set-option", "-p", "-t", paneID, key, value)
    return err
}

// GetPaneOption reads a user-defined pane option.
func (c *Client) GetPaneOption(ctx context.Context, paneID, key string) (string, error) {
    out, err := c.run(ctx, "show-option", "-p", "-t", paneID, "-v", key)
    if err != nil {
        return "", err
    }
    return out, nil
}

// SelectLayout sets the layout for panes in a session.
// Layouts: "tiled", "even-horizontal", "even-vertical", "main-horizontal", "main-vertical"
func (c *Client) SelectLayout(ctx context.Context, session, layout string) error {
    _, err := c.run(ctx, "select-layout", "-t", session, layout)
    return err
}

// SendKeys sends keystrokes to a pane.
func (c *Client) SendKeys(ctx context.Context, paneID string, keys ...string) error {
    args := append([]string{"send-keys", "-t", paneID}, keys...)
    _, err := c.run(ctx, args...)
    return err
}

// DisplayPopup shows a popup window running a command.
func (c *Client) DisplayPopup(ctx context.Context, session string, width, height int, command string) error {
    _, err := c.run(ctx, "display-popup", "-t", session,
        "-E", "-w", fmt.Sprintf("%d", width), "-h", fmt.Sprintf("%d", height), command)
    return err
}

// RespawnPane replaces a dead pane's command.
func (c *Client) RespawnPane(ctx context.Context, paneID, command string) error {
    _, err := c.run(ctx, "respawn-pane", "-t", paneID, "-k", command)
    return err
}
```

**Step 4: Run test to verify it passes**

Run: `go test -tags=integration ./internal/tmux/ -v -run TestPaneLifecycle`
Expected: PASS

**Step 5: Commit**

```
git commit -m "feat(tmux): add pane management and metadata

Task 3: SplitWindow, ListPanes, KillPane, SelectPane, pane
options for @ccells-* metadata, layouts, popups, send-keys."
```

---

### Task 4: Daemon Core

Background daemon process with Unix socket API. Reuse the socket pattern from `internal/gitproxy/server.go`.

**Files:**
- Create: `internal/daemon/daemon.go`
- Create: `internal/daemon/api.go`
- Create: `internal/daemon/daemon_test.go`

**Step 1: Write the failing test**

```go
// internal/daemon/daemon_test.go
package daemon

import (
    "context"
    "encoding/json"
    "net"
    "path/filepath"
    "testing"
    "time"
)

func TestDaemonStartStop(t *testing.T) {
    dir := t.TempDir()
    sockPath := filepath.Join(dir, "daemon.sock")

    d := New(Config{
        SocketPath: sockPath,
    })

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    errCh := make(chan error, 1)
    go func() { errCh <- d.Run(ctx) }()

    // Wait for socket to appear
    time.Sleep(100 * time.Millisecond)

    // Connect and send ping
    conn, err := net.Dial("unix", sockPath)
    if err != nil {
        t.Fatalf("dial: %v", err)
    }
    defer conn.Close()

    req := Request{Action: "ping"}
    json.NewEncoder(conn).Encode(req)

    var resp Response
    json.NewDecoder(conn).Decode(&resp)
    if !resp.OK {
        t.Errorf("expected OK response, got error: %s", resp.Error)
    }

    // Stop
    cancel()
    select {
    case err := <-errCh:
        if err != nil && err != context.Canceled {
            t.Fatalf("Run returned error: %v", err)
        }
    case <-time.After(2 * time.Second):
        t.Fatal("daemon did not stop within 2s")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/daemon/ -v -run TestDaemonStartStop`
Expected: FAIL — package doesn't exist

**Step 3: Write minimal implementation**

```go
// internal/daemon/api.go
package daemon

// Request is the JSON message sent from CLI to daemon.
type Request struct {
    Action string          `json:"action"`
    Params json.RawMessage `json:"params,omitempty"`
}

// Response is the JSON message sent from daemon to CLI.
type Response struct {
    OK    bool            `json:"ok"`
    Error string          `json:"error,omitempty"`
    Data  json.RawMessage `json:"data,omitempty"`
}
```

```go
// internal/daemon/daemon.go
package daemon

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "log"
    "net"
    "os"
    "sync"
)

// Config holds daemon configuration.
type Config struct {
    SocketPath string
}

// Daemon is the background process managing credentials, state, and tmux hooks.
type Daemon struct {
    config   Config
    listener net.Listener
    wg       sync.WaitGroup
}

// New creates a new daemon.
func New(config Config) *Daemon {
    return &Daemon{config: config}
}

// Run starts the daemon and blocks until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
    // Clean up stale socket
    os.Remove(d.config.SocketPath)

    listener, err := net.Listen("unix", d.config.SocketPath)
    if err != nil {
        return fmt.Errorf("listen: %w", err)
    }
    d.listener = listener

    // Accept loop
    d.wg.Add(1)
    go func() {
        defer d.wg.Done()
        for {
            conn, err := listener.Accept()
            if err != nil {
                select {
                case <-ctx.Done():
                    return
                default:
                    log.Printf("accept: %v", err)
                    continue
                }
            }
            d.wg.Add(1)
            go func() {
                defer d.wg.Done()
                d.handleConnection(ctx, conn)
            }()
        }
    }()

    // Wait for shutdown
    <-ctx.Done()
    listener.Close()
    d.wg.Wait()

    // Clean up socket
    os.Remove(d.config.SocketPath)
    return ctx.Err()
}

func (d *Daemon) handleConnection(ctx context.Context, conn net.Conn) {
    defer conn.Close()
    scanner := bufio.NewScanner(conn)
    if !scanner.Scan() {
        return
    }

    var req Request
    if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
        writeResponse(conn, Response{Error: "invalid request"})
        return
    }

    resp := d.dispatch(ctx, req)
    writeResponse(conn, resp)
}

func (d *Daemon) dispatch(ctx context.Context, req Request) Response {
    switch req.Action {
    case "ping":
        return Response{OK: true}
    default:
        return Response{Error: fmt.Sprintf("unknown action: %s", req.Action)}
    }
}

func writeResponse(conn net.Conn, resp Response) {
    json.NewEncoder(conn).Encode(resp)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/daemon/ -v -run TestDaemonStartStop`
Expected: PASS

**Step 5: Commit**

```
git commit -m "feat(daemon): add core daemon with Unix socket API

Task 4: Background daemon with socket listener, JSON request/
response protocol, ping action, clean shutdown on context cancel."
```

---

### Task 5: Daemon Services (Credential Refresh + State Reconciliation)

Wire existing services (CredentialRefresher, ContainerTracker, PersistentManager) into the daemon's run loop.

**Files:**
- Modify: `internal/daemon/daemon.go`
- Create: `internal/daemon/reconcile.go`
- Modify: `internal/daemon/daemon_test.go`

**Step 1: Write the failing test**

```go
func TestDaemonReconciliation(t *testing.T) {
    // Test that Reconcile produces a valid state snapshot
    // from mock tmux panes and Docker containers
    r := &Reconciler{}

    tmuxPanes := []PaneState{
        {PaneID: "%0", Workstream: "auth-system", Container: "ccells-repo-auth"},
    }
    dockerContainers := []ContainerState{
        {ID: "abc123", Name: "ccells-repo-auth", Running: true},
    }

    result := r.Reconcile(tmuxPanes, dockerContainers)

    if len(result.Healthy) != 1 {
        t.Errorf("expected 1 healthy workstream, got %d", len(result.Healthy))
    }
    if len(result.OrphanedContainers) != 0 {
        t.Errorf("expected 0 orphaned containers, got %d", len(result.OrphanedContainers))
    }
    if len(result.OrphanedPanes) != 0 {
        t.Errorf("expected 0 orphaned panes, got %d", len(result.OrphanedPanes))
    }
}

func TestReconcileOrphanedContainer(t *testing.T) {
    r := &Reconciler{}

    // Container exists but no matching pane
    tmuxPanes := []PaneState{}
    dockerContainers := []ContainerState{
        {ID: "abc123", Name: "ccells-repo-auth", Running: true},
    }

    result := r.Reconcile(tmuxPanes, dockerContainers)

    if len(result.OrphanedContainers) != 1 {
        t.Errorf("expected 1 orphaned container, got %d", len(result.OrphanedContainers))
    }
}

func TestReconcileOrphanedPane(t *testing.T) {
    r := &Reconciler{}

    // Pane exists but no matching container
    tmuxPanes := []PaneState{
        {PaneID: "%0", Workstream: "auth-system", Container: "ccells-repo-auth"},
    }
    dockerContainers := []ContainerState{}

    result := r.Reconcile(tmuxPanes, dockerContainers)

    if len(result.OrphanedPanes) != 1 {
        t.Errorf("expected 1 orphaned pane, got %d", len(result.OrphanedPanes))
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/daemon/ -v -run TestReconcil`
Expected: FAIL — types don't exist

**Step 3: Write minimal implementation**

```go
// internal/daemon/reconcile.go
package daemon

// PaneState represents a tmux pane from list-panes.
type PaneState struct {
    PaneID     string
    Workstream string // from @ccells-workstream option
    Container  string // from @ccells-container option
}

// ContainerState represents a Docker container.
type ContainerState struct {
    ID      string
    Name    string
    Running bool
    Labels  map[string]string
}

// ReconcileResult describes the current state after cross-referencing
// tmux panes with Docker containers.
type ReconcileResult struct {
    Healthy             []HealthyWorkstream
    OrphanedContainers  []ContainerState // container running, no matching pane
    OrphanedPanes       []PaneState      // pane exists, no matching container
}

// HealthyWorkstream is a pane+container pair that match.
type HealthyWorkstream struct {
    PaneID      string
    ContainerID string
    Workstream  string
    Running     bool
}

// Reconciler cross-references tmux and Docker state.
type Reconciler struct{}

// Reconcile compares tmux panes with Docker containers and categorizes them.
func (r *Reconciler) Reconcile(panes []PaneState, containers []ContainerState) ReconcileResult {
    result := ReconcileResult{}

    // Index containers by name
    containerByName := make(map[string]ContainerState, len(containers))
    matched := make(map[string]bool)
    for _, c := range containers {
        containerByName[c.Name] = c
    }

    // Match panes to containers
    for _, p := range panes {
        if p.Container == "" {
            continue // non-ccells pane, ignore
        }
        if c, ok := containerByName[p.Container]; ok {
            result.Healthy = append(result.Healthy, HealthyWorkstream{
                PaneID:      p.PaneID,
                ContainerID: c.ID,
                Workstream:  p.Workstream,
                Running:     c.Running,
            })
            matched[p.Container] = true
        } else {
            result.OrphanedPanes = append(result.OrphanedPanes, p)
        }
    }

    // Find unmatched containers
    for _, c := range containers {
        if !matched[c.Name] {
            result.OrphanedContainers = append(result.OrphanedContainers, c)
        }
    }

    return result
}
```

Then expand `daemon.go` Config to accept services:

```go
// Add to Config struct
type Config struct {
    SocketPath        string
    Refresher         *docker.CredentialRefresher  // nil = skip
    Tracker           *docker.ContainerTracker     // nil = skip
    Manager           *workstream.PersistentManager // nil = skip
    TmuxClient        *tmux.Client
    SessionName       string
    ReconcileInterval time.Duration // default 30s
}
```

Add a `runBackgroundTasks` goroutine in `Run()` that ticks the reconciler and credential refresh.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/daemon/ -v`
Expected: PASS

**Step 5: Commit**

```
git commit -m "feat(daemon): add state reconciliation and service wiring

Task 5: Reconciler cross-references tmux panes with Docker containers.
Daemon Config accepts existing services (refresher, tracker, manager)."
```

---

### Task 6: CLI Framework — `ccells up` and `ccells attach`

Replace manual arg parsing with subcommands. Keep it lightweight — no external framework, just a command registry.

**Files:**
- Create: `cmd/ccells/commands.go`
- Create: `cmd/ccells/cmd_up.go`
- Create: `cmd/ccells/cmd_attach.go`
- Create: `cmd/ccells/commands_test.go`

**Step 1: Write the failing test**

```go
// cmd/ccells/commands_test.go
package main

import "testing"

func TestParseCommand(t *testing.T) {
    tests := []struct {
        args    []string
        want    string
    }{
        {[]string{}, "up"},          // bare ccells = smart default
        {[]string{"up"}, "up"},
        {[]string{"attach"}, "attach"},
        {[]string{"down"}, "down"},
        {[]string{"ps"}, "ps"},
        {[]string{"--version"}, "version"},
        {[]string{"--help"}, "help"},
    }
    for _, tt := range tests {
        t.Run(tt.want, func(t *testing.T) {
            got := parseCommand(tt.args)
            if got != tt.want {
                t.Errorf("parseCommand(%v) = %q, want %q", tt.args, got, tt.want)
            }
        })
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/ccells/ -v -run TestParseCommand`
Expected: FAIL — function doesn't exist

**Step 3: Write minimal implementation**

```go
// cmd/ccells/commands.go
package main

// parseCommand determines which subcommand to run.
// Bare "ccells" defaults to "up" (which does smart attach-or-create).
func parseCommand(args []string) string {
    if len(args) == 0 {
        return "up"
    }
    switch args[0] {
    case "up", "attach", "down", "create", "rm", "pause", "unpause",
         "ps", "logs", "pair", "unpair", "help", "status", "merge":
        return args[0]
    case "--version", "-v":
        return "version"
    case "--help", "-h":
        return "help"
    case "--runtime":
        // Flag before command — skip flag+value, re-parse
        if len(args) >= 3 {
            return parseCommand(args[2:])
        }
        return "up"
    default:
        return "help"
    }
}
```

```go
// cmd/ccells/cmd_up.go
package main

import (
    "context"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"

    "github.com/anthropics/ccells/internal/tmux"
)

// runUp starts the tmux session + daemon if not running, then attaches.
func runUp(ctx context.Context, repoID, repoPath, stateDir, runtime string) error {
    socketName := fmt.Sprintf("ccells-%s", repoID)
    client := tmux.NewClient(socketName)
    sessionName := "ccells"

    running, err := client.HasSession(ctx, sessionName)
    if err != nil {
        return err
    }

    if running {
        // Already running — just attach
        return attachToSession(client, sessionName)
    }

    // Create new session
    if err := client.NewSession(ctx, sessionName); err != nil {
        return fmt.Errorf("failed to create tmux session: %w", err)
    }

    // TODO: Start daemon, configure tmux chrome, restore workstreams

    return attachToSession(client, sessionName)
}

func attachToSession(client *tmux.Client, sessionName string) error {
    cmd := client.AttachCommand(sessionName)
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}
```

```go
// cmd/ccells/cmd_attach.go
package main

import (
    "context"
    "fmt"

    "github.com/anthropics/ccells/internal/tmux"
)

// runAttach attaches to an existing session or lists available sessions.
func runAttach(ctx context.Context, repoID string) error {
    socketName := fmt.Sprintf("ccells-%s", repoID)
    client := tmux.NewClient(socketName)

    running, _ := client.HasSession(ctx, "ccells")
    if !running {
        return fmt.Errorf("no ccells session running for this repo. Run 'ccells up' first")
    }

    return attachToSession(client, "ccells")
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/ccells/ -v -run TestParseCommand`
Expected: PASS

**Step 5: Commit**

```
git commit -m "feat(cli): add command parser and up/attach subcommands

Task 6: Lightweight command registry with smart default (bare ccells = up).
up creates tmux session + attaches, attach reattaches to existing."
```

---

### Task 7: CLI — `ccells down`, `ccells ps`

**Files:**
- Create: `cmd/ccells/cmd_down.go`
- Create: `cmd/ccells/cmd_ps.go`

**Step 1: Write tests for ps output formatting**

```go
func TestFormatPS(t *testing.T) {
    workstreams := []PSEntry{
        {Name: "auth-system", Status: "running", Branch: "feat/auth", PR: "#42"},
        {Name: "fix-bug", Status: "paused", Branch: "fix/login", PR: ""},
    }
    out := formatPS(workstreams)
    if !strings.Contains(out, "auth-system") {
        t.Error("expected auth-system in output")
    }
    if !strings.Contains(out, "paused") {
        t.Error("expected paused in output")
    }
}
```

**Step 2-5: Implement, test, commit**

`ccells down` sends shutdown to daemon socket, then kills tmux server. With `--rm`, also destroys containers via orchestrator.

`ccells ps` queries daemon for workstream list, formats as table.

```
git commit -m "feat(cli): add down and ps subcommands

Task 7: down stops daemon + tmux (--rm destroys containers).
ps lists workstreams with status, branch, PR info."
```

---

### Task 8: CLI — `ccells create`, `ccells rm`, `ccells pause/unpause`

**Files:**
- Create: `cmd/ccells/cmd_create.go`
- Create: `cmd/ccells/cmd_rm.go`
- Create: `cmd/ccells/cmd_pause.go`

These commands send requests to the daemon via the socket API. The daemon calls the orchestrator.

**Daemon API actions to add:**
```go
case "create":
    // Parse params: branch, prompt, runtime
    // Call orchestrator.CreateWorkstream()
    // Create tmux pane with docker exec
    // Set pane metadata
    // Update state file

case "rm":
    // Parse params: workstream name
    // Call orchestrator.DestroyWorkstream()
    // Kill tmux pane
    // Update state file

case "pause":
    // Call orchestrator.PauseWorkstream()
    // Update pane border

case "unpause":
    // Call orchestrator.ResumeWorkstream()
    // Respawn pane command (docker exec)
```

**Step 1-5: Test each action through daemon socket, implement, commit**

```
git commit -m "feat(cli): add create, rm, pause, unpause subcommands

Task 8: Workstream lifecycle commands via daemon socket API.
create calls orchestrator + adds tmux pane, rm destroys both."
```

---

### Task 9: Interactive Dialogs

Small Bubble Tea programs that run inside `tmux display-popup`.

**Files:**
- Create: `cmd/ccells/dialog_create.go`
- Create: `cmd/ccells/dialog_rm.go`
- Create: `cmd/ccells/dialog_merge.go`

**`ccells create --interactive`**: Bubble Tea app that prompts for task description, shows title generation, confirms branch name. Outputs the created workstream name on success.

**`ccells rm --interactive`**: Lists workstreams, lets user select, confirms deletion.

**`ccells merge --interactive`**: Shows diff summary, prompts for PR title/description, creates PR.

Each dialog is a self-contained Bubble Tea program that:
1. Collects user input
2. Sends a request to the daemon socket
3. Prints result and exits

These programs are invoked by tmux keybindings:
```
bind-key -T prefix n run-shell 'tmux display-popup -E -w 60 -h 20 "ccells create --interactive"'
```

**Step 1-5: Build create dialog first (most complex), test manually, commit**

```
git commit -m "feat(dialogs): add interactive create/rm/merge popups

Task 9: Bubble Tea dialog programs for tmux display-popup.
Each collects input, sends to daemon socket, prints result."
```

---

### Task 10: Chrome (Status Line, Pane Borders, Help)

**Files:**
- Create: `internal/tmux/chrome.go`
- Create: `internal/tmux/chrome_test.go`

**Step 1: Write the failing test**

```go
func TestFormatStatusLine(t *testing.T) {
    workstreams := []StatusWorkstream{
        {Name: "auth", Status: "running", HasPR: true, PRMerged: false},
        {Name: "fix-bug", Status: "paused", HasPR: false},
    }
    prefix := "C-b"

    line := FormatStatusLine(workstreams, prefix, false)

    if !strings.Contains(line, "auth") {
        t.Error("expected auth in status line")
    }
    if !strings.Contains(line, "C-b") {
        t.Error("expected prefix in keyhints")
    }
}

func TestFormatPaneBorder(t *testing.T) {
    border := FormatPaneBorder("auth-system", "running", 42, "")
    if !strings.Contains(border, "auth-system") {
        t.Error("expected workstream name in border")
    }
    if !strings.Contains(border, "PR#42") {
        t.Error("expected PR number in border")
    }
}
```

**Step 2-5: Implement formatting, apply via tmux commands, commit**

Chrome setup runs once after session creation:
```go
func (c *Client) ConfigureChrome(ctx context.Context, session string) error {
    // Pane borders
    c.run(ctx, "set-option", "-g", "pane-border-format",
        "─── #{@ccells-border-text} ───")
    c.run(ctx, "set-option", "-g", "pane-border-status", "top")
    c.run(ctx, "set-option", "-g", "pane-active-border-style", "fg=cyan")

    // Status line
    c.run(ctx, "set-option", "-g", "status-right",
        "#(ccells status --format=tmux)")
    c.run(ctx, "set-option", "-g", "status-interval", "5")

    // Multi-line status if tmux >= 3.4
    // ...

    return nil
}
```

```
git commit -m "feat(chrome): add status line, pane borders, help popup

Task 10: tmux chrome configuration with dynamic prefix display,
per-pane border text, and help popup via prefix+?."
```

---

### Task 11: Detach Summary

**Files:**
- Create: `cmd/ccells/detach.go`

Print resource usage and instructions after tmux detach. Use tmux `client-detached` hook or wrap the attach command.

```go
func printDetachSummary(ctx context.Context, repoID string) {
    // Query docker stats --no-stream --format for each container
    // Print formatted box with workstream status + resource usage
    // Print reattach/cleanup instructions
}
```

```
git commit -m "feat(cli): add detach summary with resource usage

Task 11: On detach, prints workstream status, CPU/RAM per container,
daemon status, and reattach/cleanup commands."
```

---

### Task 12: State Reconciliation Integration

Wire the reconciler into the daemon's background loop and the `ccells up` startup path.

**Files:**
- Modify: `internal/daemon/daemon.go`
- Modify: `internal/daemon/reconcile.go`
- Create: `internal/daemon/reconcile_test.go`

On `ccells up`:
1. Query Docker for ccells containers (by label)
2. If tmux session exists, query panes
3. Reconcile → restore state file from Docker labels + pane metadata
4. Recreate missing panes for orphaned containers
5. Offer cleanup for orphaned containers without panes

```
git commit -m "feat(daemon): integrate state reconciliation into startup

Task 12: On ccells up, reconcile tmux panes with Docker containers.
Recreate panes for orphaned containers, offer cleanup."
```

---

### Task 13: Pairing Mode Integration

Move pairing orchestration from TUI to daemon.

**Files:**
- Modify: `internal/daemon/daemon.go`
- Modify: `internal/daemon/api.go`
- Create: `cmd/ccells/cmd_pair.go`

Add daemon actions: `pair`, `unpair`. Daemon holds the `Pairing` struct, polls health alongside PR status.

```
git commit -m "feat(daemon): add pairing mode via daemon

Task 13: ccells pair/unpair commands. Daemon manages Pairing struct,
polls sync health every 5s, updates pane borders with sync status."
```

---

### Task 14: Wire main.go

Rewrite `cmd/ccells/main.go` to use the new CLI + tmux + daemon architecture. Remove all Bubble Tea references.

**Files:**
- Rewrite: `cmd/ccells/main.go`

Keep: lock file, prerequisites validation, orphan cleanup, repo ID resolution.
Remove: `tea.NewProgram`, `tui.NewAppModel`, all `tui.Set*` calls.
Add: command dispatch to `parseCommand`, service creation passed to daemon.

```
git commit -m "refactor(main): replace TUI startup with tmux+daemon

Task 14: main.go dispatches to CLI subcommands. ccells up creates
tmux session + starts daemon. No Bubble Tea references remain."
```

---

### Task 15: Delete TUI

Remove the entire `internal/tui/` package and unused dependencies.

**Files:**
- Delete: `internal/tui/*.go` (all files)
- Delete: `internal/tui/*_test.go` (all test files)
- Modify: `go.mod` — remove bubbletea, bubbles, lipgloss, vt10x, etc.
- Run: `go mod tidy`

**Step 1: Delete TUI package**

```bash
rm -rf internal/tui/
```

**Step 2: Remove unused deps**

```bash
go mod tidy
```

**Step 3: Verify build**

Run: `go build ./cmd/ccells && go test ./...`
Expected: PASS — no references to `internal/tui` remain

**Step 4: Verify tests**

Run: `go test -race ./...`
Expected: All tests pass

**Step 5: Commit**

```
git commit -m "refactor: delete internal/tui/ package (~6000 lines)

Task 15: Remove Bubble Tea TUI, vt10x, PTY management, custom
layout engine. All functionality now via tmux integration."
```

---

### Task 16: Documentation

Update all docs to reflect the new architecture.

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `codemaps/architecture.md`
- Modify: `codemaps/backend.md`

**Changes:**
- README: Installation now requires tmux. New CLI usage section with compose-style verbs.
- CLAUDE.md: Replace TUI architecture section with tmux architecture. Update commands section.
- Codemaps: Replace TUI layer with tmux/daemon layer. Update component diagram.

```
git commit -m "docs: update README, CLAUDE.md, codemaps for tmux architecture

Task 16: Document compose-style CLI, tmux requirements,
daemon architecture, and new keybindings."
```

---

## Verification

After all tasks, verify end-to-end:

1. `ccells up` — creates tmux session, starts daemon
2. `prefix + n` — popup creates workstream with Docker container
3. Claude runs in tmux pane with direct PTY
4. `prefix + d` — popup destroys workstream
5. `Ctrl-b d` — detach shows resource summary
6. `ccells attach` — reattach from another terminal
7. `ccells ps` — lists all workstreams with status
8. `ccells down` — stops daemon and tmux
9. `ccells down --rm` — destroys everything
10. Kill daemon manually → `ccells attach` restarts it
11. Kill tmux pane manually → `ccells ps` shows orphaned container
