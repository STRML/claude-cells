# Critical Fixes Plan

## Overview

This plan addresses 5 critical architectural issues that can cause container leaks, data races, and poor shutdown behavior.

---

## Issue 1: Container Cleanup on Crash

**Problem:** If the app crashes or receives SIGKILL, containers keep running with no way to resume them.

**Root Cause:** Only `PauseAllAndSaveCmd` saves state, and it only runs during graceful shutdown.

### Fix

1. **Write container IDs to a tracking file immediately when containers start**
   - File: `~/.claude-cells/containers.json`
   - Updated atomically on each container start/stop
   - Contains: container ID, workstream ID, branch name, repo path

2. **On startup, reconcile running containers with tracking file**
   - Find containers with label `ccells=true` that aren't in current state
   - Either offer to resume or clean them up

3. **Add heartbeat file**
   - Write PID + timestamp every 5 seconds
   - On startup, check if stale (> 30s old) = previous crash

### Files to Modify

- `cmd/ccells/main.go` - Add tracking file writes to signal handler
- `internal/docker/container.go` - Write to tracking file on create/destroy
- `internal/docker/tracking.go` (new) - Tracking file management
- `internal/docker/client.go` - Add `ListCCellsContainers()` method

### Implementation Steps

```
1. [ ] Create internal/docker/tracking.go with:
       - TrackContainer(id, wsID, branch string)
       - UntrackContainer(id string)
       - GetTrackedContainers() []TrackedContainer
       - WriteHeartbeat(pid int)
       - IsHeartbeatStale() bool

2. [ ] Update StartContainer in container.go to call TrackContainer()

3. [ ] Update StopContainer in container.go to call UntrackContainer()

4. [ ] Add goroutine in main.go to write heartbeat every 5s

5. [ ] Update cleanupOrphanedContainers() to use tracking file + stale heartbeat

6. [ ] Add integration test for crash recovery
```

---

## Issue 2: PTY Close Race Condition

**Problem:** `StartReadLoop()` can race with `Close()`, causing panics or orphaned goroutines.

**Location:** `internal/tui/pty.go:261-312`

### Fix

Use a context for cancellation instead of checking `closed` under mutex.

```go
type PTYSession struct {
    // ... existing fields
    ctx        context.Context
    cancelFunc context.CancelFunc
}

func NewPTYSession(parentCtx context.Context, ...) (*PTYSession, error) {
    ctx, cancel := context.WithCancel(parentCtx)
    session := &PTYSession{
        // ...
        ctx:        ctx,
        cancelFunc: cancel,
    }
    // ...
}

func (p *PTYSession) StartReadLoop() {
    buf := make([]byte, 4096)
    for {
        select {
        case <-p.ctx.Done():
            return
        default:
        }

        // Use SetReadDeadline if available, or wrap in goroutine with context
        readResult := make(chan struct{ n int; err error }, 1)
        go func() {
            n, err := p.conn.Reader.Read(buf)
            readResult <- struct{ n int; err error }{n, err}
        }()

        select {
        case <-p.ctx.Done():
            return
        case result := <-readResult:
            if result.err != nil {
                // handle error
                return
            }
            // process result.n bytes
        }
    }
}

func (p *PTYSession) Close() error {
    p.cancelFunc() // Signal all goroutines to stop

    p.mu.Lock()
    defer p.mu.Unlock()

    if p.closed {
        return nil
    }
    p.closed = true

    if p.conn != nil {
        p.conn.Close()
    }
    return nil
}
```

### Files to Modify

- `internal/tui/pty.go` - Refactor PTYSession with context-based cancellation

### Implementation Steps

```
1. [ ] Add ctx and cancelFunc fields to PTYSession

2. [ ] Update NewPTYSession to accept parent context and create child context

3. [ ] Refactor StartReadLoop to use context for cancellation

4. [ ] Refactor autoAcceptBypassPermissions to respect context

5. [ ] Update Close() to call cancelFunc() before closing connection

6. [ ] Update callers in container.go to pass context

7. [ ] Add test for concurrent Close() and Read()
```

---

## Issue 3: Concurrent SaveState Race

**Problem:** Two concurrent SaveState calls use the same temp file, causing corruption.

**Location:** `internal/workstream/state.go:37-74`

### Fix

Use unique temp file names and/or file locking.

```go
func SaveState(dir string, workstreams []*Workstream, focusedIndex int) error {
    finalPath := stateFilePath(dir)

    // Use unique temp file name
    tempPath := fmt.Sprintf("%s.tmp.%d", finalPath, time.Now().UnixNano())

    // ... write to tempPath ...

    // Atomic rename
    if err := os.Rename(tempPath, finalPath); err != nil {
        os.Remove(tempPath)
        return err
    }

    return nil
}
```

Additionally, add a mutex at the package level to prevent concurrent writes:

```go
var stateMu sync.Mutex

func SaveState(...) error {
    stateMu.Lock()
    defer stateMu.Unlock()
    // ...
}
```

### Files to Modify

- `internal/workstream/state.go` - Add mutex and unique temp names

### Implementation Steps

```
1. [ ] Add package-level stateMu sync.Mutex

2. [ ] Wrap SaveState in mutex lock

3. [ ] Use time-based unique temp file name

4. [ ] Add test for concurrent SaveState calls
```

---

## Issue 4: Docker Client Leak in PTY Sessions

**Problem:** Each PTY session stores a Docker client that's never closed.

**Location:** `internal/tui/pty.go:60`, `container.go:225`

### Fix

Either:
A. Pass Docker client from a shared pool (preferred)
B. Close the client in PTYSession.Close()

Option A is better because it avoids creating/destroying connections:

```go
// In app.go or a shared location
type AppModel struct {
    // ...
    dockerClient *client.Client  // Shared client
}

func (m AppModel) Init() tea.Cmd {
    // Create shared client
    client, err := docker.NewClient()
    if err != nil {
        // handle
    }
    m.dockerClient = client
    // ...
}

// Pass shared client to StartPTYCmd
func StartPTYCmd(ws *Workstream, dockerClient *client.Client, ...) tea.Cmd {
    // Use passed client instead of creating new one
}
```

### Files to Modify

- `internal/tui/app.go` - Add shared Docker client
- `internal/tui/container.go` - Accept client parameter instead of creating
- `internal/tui/pty.go` - Don't store client, just use for setup

### Implementation Steps

```
1. [ ] Add dockerClient field to AppModel

2. [ ] Create client in Init() and close in shutdown path

3. [ ] Update StartContainerCmd, ResumeContainerCmd, StartPTYCmd to accept client

4. [ ] Update PTYSession to not store client (only use for initial Resize)

5. [ ] Update Resize() to accept client as parameter or use global
```

---

## Issue 5: Context Propagation

**Problem:** All operations use `context.Background()`, preventing cascade cancellation.

**Locations:** 47 uses of `context.Background()` across the codebase.

### Fix

Thread a cancellable context through from the top level:

1. Create a context in `main()` that's cancelled on SIGINT/SIGTERM
2. Pass to AppModel and store as field
3. All Cmd functions use this context (or a child of it)

```go
// main.go
func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Signal handler
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-sigCh
        cancel() // Cancel all operations
        // Give operations time to clean up
        time.Sleep(5 * time.Second)
        os.Exit(1)
    }()

    // Pass ctx to app
    app := tui.NewAppModel(ctx)
    // ...
}

// app.go
type AppModel struct {
    ctx context.Context
    // ...
}

func NewAppModel(ctx context.Context) AppModel {
    return AppModel{
        ctx: ctx,
        // ...
    }
}
```

### Files to Modify

- `cmd/ccells/main.go` - Create cancellable context
- `internal/tui/app.go` - Store and propagate context
- `internal/tui/container.go` - Use app context instead of Background()
- `internal/tui/pty.go` - Accept context in NewPTYSession

### Implementation Steps

```
1. [ ] Create context.WithCancel in main.go

2. [ ] Update signal handler to cancel context

3. [ ] Add ctx field to AppModel

4. [ ] Update NewAppModel to accept context

5. [ ] Update all Cmd functions to use m.ctx or child context

6. [ ] Update PTY to use passed context

7. [ ] Test that SIGINT cancels in-progress operations
```

---

## Priority Order

1. **Issue 2: PTY Close Race** - Can cause panics, fix first
2. **Issue 1: Container Cleanup** - Major UX issue, containers leak
3. **Issue 5: Context Propagation** - Enables proper shutdown, fixes Issue 2 properly
4. **Issue 4: Docker Client Leak** - Resource leak, but not urgent
5. **Issue 3: SaveState Race** - Rare in practice, low priority

---

## Testing Requirements

Each fix should include:
- Unit tests for the specific fix
- Integration test for the scenario it addresses
- Race detector validation (`go test -race`)

### Specific Tests to Add

```go
// pty_test.go
func TestPTYSession_ConcurrentClose(t *testing.T)
func TestPTYSession_CloseWhileReading(t *testing.T)
func TestPTYSession_ContextCancellation(t *testing.T)

// state_test.go
func TestSaveState_Concurrent(t *testing.T)

// tracking_test.go (new)
func TestTrackContainer(t *testing.T)
func TestHeartbeat_Stale(t *testing.T)
func TestReconcileOrphanedContainers(t *testing.T)
```

---

## Estimated Effort

| Issue | Files Changed | New Lines | Test Lines | Effort |
|-------|--------------|-----------|------------|--------|
| 1. Container Cleanup | 4 | ~200 | ~100 | Large |
| 2. PTY Race | 2 | ~50 | ~50 | Medium |
| 3. SaveState Race | 1 | ~10 | ~30 | Small |
| 4. Docker Client Leak | 3 | ~30 | ~20 | Small |
| 5. Context Propagation | 4 | ~100 | ~50 | Medium |

Total: ~12-16 hours of focused work.
