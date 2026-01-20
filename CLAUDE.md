# Claude Cells

A terminal UI for running parallel Claude Code instances in isolated Docker containers.

## Commands

```bash
go test ./...                         # Run all tests
go test -v -race ./...                # Run with race detector
go test -cover ./...                  # Run with coverage
go build ./cmd/ccells                 # Build binary (dev version)
./ccells                              # Run

# Build with version info (for releases)
go build -ldflags "-X main.CommitHash=$(git rev-parse --short HEAD)" ./cmd/ccells
```

## Architecture

```
cmd/ccells/main.go         # Entry point
internal/
  tui/                     # Bubble Tea UI (app.go is the hub)
    app.go                 # Main model, Update loop, View rendering
    pane.go                # Individual workstream pane with vterm
    pty.go                 # Docker exec PTY session management
    dialog.go              # Modal dialogs
    container.go           # Container lifecycle & git worktree management
  workstream/              # Workstream state & lifecycle
    manager.go             # In-memory workstream tracking
    persistent_manager.go  # Auto-persisting state manager (wraps Manager)
    state.go               # State file read/write
  docker/                  # Docker SDK wrapper
  sync/                    # Mutagen/pairing mode
  git/                     # Branch, worktree & PR operations
```

### Git Worktree Isolation

Each container gets its own **git worktree** at `/tmp/ccells/worktrees/<branch-name>`. This keeps the host repository's working directory untouched while still sharing git objects. Benefits:

- Host repo stays on its current branch (never changes)
- No `.git/index.lock` conflicts
- Each container has isolated working directory
- Changes are still part of the main repo's git history (can push/PR)
- Worktrees are cleaned up when containers are destroyed

## Development Rules

### Test-Driven Development (Mandatory)
- Write tests FIRST before implementing features
- All new code must have corresponding unit tests
- Tests must pass before moving on
- Use table-driven tests for multiple scenarios
- Run `go test -race ./...` before committing

### Testing Guidelines

**Unit Tests**
- Every new function/method should have unit tests
- Test both success and error paths
- Use table-driven tests for testing multiple inputs:
```go
func TestSomething(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {"valid input", "foo", "bar", false},
        {"empty input", "", "", true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test logic
        })
    }
}
```

**Docker Mocking**

The `internal/docker` package provides `MockClient` for testing Docker interactions without a real Docker daemon. This enables fast, isolated unit tests.

```go
// Create a mock client
client := docker.NewMockClient()
ctx := context.Background()

// Use it like a real client
cfg := &docker.ContainerConfig{Name: "test", Image: "alpine"}
id, _ := client.CreateContainer(ctx, cfg)
_ = client.StartContainer(ctx, id)

// Mock client tracks container state in memory
state, _ := client.GetContainerState(ctx, id) // returns "running"
```

MockClient features:
- In-memory container state tracking (created/running/paused/exited)
- Configurable behaviors via function fields (`CreateContainerFn`, `ImageExistsFn`)
- Implements the full `DockerClient` interface
- Thread-safe with mutex protection

**Integration Tests**

For tests that need real Docker:
- Use build tags: `//go:build integration`
- Run with: `go test -tags=integration ./...`
- These tests are slower and require Docker daemon

**TUI Testing**

The TUI uses Bubble Tea's message-passing architecture which makes testing straightforward:
```go
app := NewAppModel(context.Background())
app.width = 100
app.height = 40

// Send a message and check state
model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
app = model.(AppModel)

if app.dialog == nil {
    t.Error("Dialog should be open")
}
```

### Completed Technical Improvements

- **Git Worktree Isolation**: Each container gets its own git worktree, avoiding host repo conflicts
- **Shell Escaping**: `escapeShellArg()` handles newlines (`\n`, `\r`), null bytes, and other special characters
- **Resource Limits**: Manager limits workstreams to 12 (MaxWorkstreams constant)
- **Auto-Persisting State**: `PersistentManager` auto-saves on any mutation (add/remove workstream, focus change, layout change). 200ms debounced saves prevent disk thrashing. Force-quit leaves coherent state.
- **Atomic State Writes**: `SaveState` writes to temp file then renames for crash safety
- **PTY Clean Shutdown**: Uses done channel for clean goroutine shutdown
- **Container Cleanup**: Orphaned containers from crashed sessions are cleaned up on startup
- **Context Timeouts**: All Docker operations use timeouts (no unbounded context.Background())
- **Session Persistence**: Claude sessions are persisted from container runtime location to mount point before pause, surviving container rebuilds

### Remaining Technical Debt

1. **Full Context Propagation**: Most operations use timeouts, but a root context for app-wide cancellation would be cleaner (pass context from main through AppModel to all commands)

### Code Standards

**Error Handling**
```go
// Bad - swallows error
_ = someFunc()

// Good - at minimum log it
if err := someFunc(); err != nil {
    log.Printf("someFunc failed: %v", err)
}
```

**Context Usage**
```go
// Bad
ctx := context.Background()

// Good - propagate from caller or use timeout
ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
defer cancel()
```

**Goroutine Lifecycle**
```go
// Bad - goroutine can leak
go func() {
    for { /* work */ }
}()

// Good - clean shutdown
go func() {
    for {
        select {
        case <-done:
            return
        case work := <-workCh:
            // process
        }
    }
}()
```

### Security Checklist

- [x] Never interpolate user input into shell commands without escaping (see `escapeShellArg()`)
- [x] Credentials should be read-only in containers where possible (mounted as ReadOnly)
- [ ] Validate all branch names before using in paths/commands
- [ ] Container mounts should be minimal (principle of least privilege)

### Before Committing

1. `go test -race ./...` - no races
2. `go vet ./...` - no issues
3. `gofmt -s -w .` - formatted
4. Tests cover the change
5. No `TODO` without an issue number
