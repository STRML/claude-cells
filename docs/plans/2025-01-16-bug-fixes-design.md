# Docker TUI Bug Fixes Design

**Date:** 2025-01-16
**Status:** Approved

## Problems

1. **Keybind conflicts** - App-level keybinds (n, d, p, m, 1-9) intercept keypresses before pane text input
2. **No container creation** - Only data model created, no Docker container started
3. **Placeholder Claude integration** - "Processing..." but nothing happens

## Solution 1: Double-Escape Mode Switching

### Behavior

**Navigation Mode (default):**
- `n, d, p, m, tab, 1-9, ?, l` → app shortcuts
- Focus a pane → auto-enter input mode

**Input Mode:**
- All keys → routed to pane PTY session
- Single Escape (after 300ms) → exit to navigation mode
- Double Escape (within 300ms) → send actual Escape to Claude Code

### Implementation

Add to `AppModel`:
```go
type AppModel struct {
    // ... existing fields
    inputMode      bool
    lastEscapeTime time.Time
}
```

Update key handling:
```go
case tea.KeyMsg:
    if m.inputMode {
        if msg.String() == "esc" {
            if time.Since(m.lastEscapeTime) < 300*time.Millisecond {
                // Double escape - send to pane
                return m.routeToPTY(msg)
            }
            m.lastEscapeTime = time.Now()
            m.inputMode = false
            return m, nil
        }
        return m.routeToPTY(msg)
    }
    // ... existing nav mode handling
```

### Visual Indicator

Status bar shows current mode: `[NAV]` or `[INPUT]`
Focused pane border color changes based on mode.

---

## Solution 2: Container Creation Flow

### Flow

```
DialogConfirmMsg (new workstream)
    ↓
1. workstream.New(prompt) - create data model
2. workstream.SetState(StateStarting)
3. Add pane to UI (shows "Starting container...")
4. tea.Cmd: StartContainerCmd(workstream)
    ↓ async
5. EnsureBaseImage() - build/pull if needed
6. CreateContainer(config)
7. StartContainer()
8. Return ContainerStartedMsg or ContainerErrorMsg
    ↓
9. Update workstream state (running or error)
10. Initialize PTY session to Claude Code
```

### New State

Add `StateStarting` to workstream states:
```go
const (
    StateStarting State = "starting" // Container being created
    StateRunning  State = "running"
    StateIdle     State = "idle"
    StatePairing  State = "pairing"
    StateStopped  State = "stopped"
    StateError    State = "error"
)
```

### Container Config

```go
type ContainerConfig struct {
    Image:      "claude-code-base:latest"
    WorkingDir: "/workspace"
    Mounts:     []Mount{{Source: projectDir, Target: "/workspace", ReadOnly: true}}
    Env:        []string{"ANTHROPIC_API_KEY=" + os.Getenv("ANTHROPIC_API_KEY")}
}
```

### Keybind for Logs

`l` (in nav mode) → Toggle container logs view for focused workstream

---

## Solution 3: Claude Code PTY Integration

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│  Pane                                                   │
│  ├── viewport (output)  ←── reads from PTY stdout       │
│  └── input mode         ──→ writes to PTY stdin         │
└─────────────────────────────────────────────────────────┘
         │                    ▲
         ▼                    │
┌─────────────────────────────────────────────────────────┐
│  PTYSession                                             │
│  - docker exec -it <container> claude <initial-prompt>  │
│  - Goroutine reads stdout, sends OutputMsg to tea       │
│  - Stdin writer for user input                          │
└─────────────────────────────────────────────────────────┘
```

### New Files

`internal/tui/pty.go`:
```go
type PTYSession struct {
    containerID string
    execID      string
    stdin       io.WriteCloser
    stdout      io.ReadCloser
    done        chan struct{}
}

func NewPTYSession(client *docker.Client, containerID, initialPrompt string) (*PTYSession, error)
func (p *PTYSession) Write(data []byte) error
func (p *PTYSession) ReadLoop(outputChan chan<- []byte)
func (p *PTYSession) Close() error
```

### Messages

```go
type PTYOutputMsg struct {
    WorkstreamID string
    Output       []byte
}

type ContainerStartedMsg struct {
    WorkstreamID string
    ContainerID  string
}

type ContainerErrorMsg struct {
    WorkstreamID string
    Error        error
}
```

### Output Handling

- PTY output preserves ANSI codes
- Viewport renders with `lipgloss` ANSI support
- Auto-scroll to bottom on new output

---

## Implementation Order

1. Add StateStarting, StateError to workstream states
2. Implement double-escape mode switching in app.go
3. Add mode indicator to status bar
4. Create StartContainerCmd and message types
5. Wire container creation into DialogConfirmMsg handler
6. Implement PTYSession in internal/tui/pty.go
7. Connect PTY to pane output/input
8. Add `l` keybind for container logs
9. Integration testing
