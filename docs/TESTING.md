# Testing Guide

This document covers how to run and verify tests for Claude Cells.

## Quick Reference

```bash
# Unit tests (fast, no external deps)
go test ./...

# Unit tests with race detector
go test -race ./...

# Integration tests (requires Docker + tmux)
go test -tags=integration -v ./...

# Specific test suites
go test -v ./internal/tui/ -run TestViewportFillHeight  # Unit: viewport height
go test -tags=integration -v ./internal/tui/ -run TestTmux  # Integration: tmux harness
```

## Test Categories

### Unit Tests

Run with `go test ./...`. These don't require Docker or external services.

**What they cover:**
- Workstream state management
- Git operations (mocked)
- TUI component behavior (PaneModel, viewport, dialogs)
- Layout calculations
- Configuration parsing

### Integration Tests

Run with `go test -tags=integration ./...`. These require:
- Docker daemon running
- tmux installed
- The `ccells-base` Docker image built

**What they cover:**
- Full container lifecycle
- Real Docker operations
- TUI rendering in actual terminal (via tmux)

## Manual Verification

### Viewport Height Consistency

The viewport should maintain consistent height regardless of content amount. This prevents visual jumping when Claude Code's multi-line prompt changes.

**Automated test:**
```bash
go test -tags=integration -v ./internal/tui/ -run TestTmux
```

**Manual verification with tmux:**

1. Install tmux if needed:
   ```bash
   # macOS
   brew install tmux

   # Ubuntu/Debian
   sudo apt-get install tmux
   ```

2. Build and run in a tmux session:
   ```bash
   go build ./cmd/ccells

   # Start in a fixed-size tmux session
   tmux new-session -d -s test -x 120 -y 40 './ccells'
   tmux attach -t test
   ```

3. Create a workstream (press `n`) and observe:
   - Pane should fill its allocated space completely
   - No flickering or height changes as content updates
   - Consistent behavior after resize (`tmux resize-window -x 80 -y 30`)

4. Capture frames for inspection:
   ```bash
   # In another terminal
   tmux capture-pane -t test -p > frame.txt
   wc -l frame.txt  # Should match terminal height (40)
   ```

5. Cleanup:
   ```bash
   tmux kill-session -t test
   ```

### Container Lifecycle

**Manual verification:**

1. Start ccells and create a workstream
2. Verify container is running: `docker ps | grep ccells`
3. Quit with `q` (containers should pause, not remove)
4. Verify paused: `docker ps -a | grep ccells` shows "Paused" or "Exited"
5. Restart ccells - workstreams should resume

### Resize Behavior

1. Run ccells in a resizable terminal
2. Create 2-3 workstreams
3. Resize the terminal window
4. Verify:
   - All panes resize proportionally
   - No visual artifacts or truncation
   - Content remains visible and scrollable

## Writing Tests

### Unit Tests

Use table-driven tests for multiple scenarios:

```go
func TestSomething(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {"valid", "foo", "bar"},
        {"empty", "", ""},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test logic
        })
    }
}
```

### Docker Mocking

Use `docker.NewMockClient()` for unit tests that need Docker interactions:

```go
client := docker.NewMockClient()
cfg := &docker.ContainerConfig{Name: "test", Image: "alpine"}
id, _ := client.CreateContainer(ctx, cfg)
```

### TUI Testing

Test Bubble Tea models by sending messages and checking state:

```go
app := NewAppModel(context.Background())
app.width = 100
app.height = 40

model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
app = model.(AppModel)

if app.dialog == nil {
    t.Error("Dialog should be open")
}
```

### Integration Tests

Tag with `//go:build integration` and skip gracefully when deps are missing:

```go
//go:build integration

func TestSomething(t *testing.T) {
    if !dockerAvailable() {
        t.Skip("Docker not available")
    }
    // ...
}
```

## CI/CD

Tests run automatically on push/PR:

- **CI workflow** (`ci.yml`): Runs unit tests, linting, build verification
- **Integration workflow** (`integration-tests.yml`): Builds Docker image, runs full test suite with `-tags=integration`

Both workflows must pass before merging.
