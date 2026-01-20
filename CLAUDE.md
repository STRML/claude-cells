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
- Write tests FIRST
- Tests must pass before moving on
- Use table-driven tests

### Completed Technical Improvements

- **Git Worktree Isolation**: Each container gets its own git worktree, avoiding host repo conflicts
- **Shell Escaping**: `escapeShellArg()` handles newlines (`\n`, `\r`), null bytes, and other special characters
- **Resource Limits**: Manager limits workstreams to 12 (MaxWorkstreams constant)
- **Atomic State Persistence**: `SaveState` writes to temp file then renames for crash safety
- **PTY Clean Shutdown**: Uses done channel for clean goroutine shutdown
- **Container Cleanup**: Orphaned containers from crashed sessions are cleaned up on startup
- **Context Timeouts**: All Docker operations use timeouts (no unbounded context.Background())

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
