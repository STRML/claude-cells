# Architecture Overview

Last updated: 2026-01-22 (Updated: PaneModel refactoring)

## Architecture Assessment: 7.5/10

### Strengths
- Clean message-passing with Bubble Tea (Elm Architecture)
- Strong interface abstractions (DockerClient, GitClient) with mocks
- Comprehensive testing infrastructure
- Excellent crash recovery & state persistence
- Security hardening by default with tiered relaxation

### Weaknesses
- Business logic leaking into TUI layer (`container.go` has 1600+ lines of orchestration)
- God objects (AppModel: 94 fields, PaneModel: 40+ fields)
- Global mutable state (`program`, `containerTracker`, `credentialRefresher`)
- Missing observability (no metrics, tracing)

## Summary

Claude Cells (ccells) is a terminal UI application that orchestrates parallel Claude Code instances in isolated Docker containers. It enables running multiple AI-assisted development workstreams simultaneously, each with its own git worktree, container, and Claude Code session.

## Entry Points

```
cmd/ccells/main.go          # Primary CLI entry point
cmd/keytest/main.go         # Keyboard testing utility (dev tool)
```

### Main Entry Flow

```
main() -> validatePrerequisites() -> NewAppModel() -> tea.NewProgram() -> Run()
                                          |
                                          v
                              tui.AppModel (Bubble Tea model)
```

Key startup operations:
1. Initialize logging (to avoid TUI pollution)
2. Acquire per-repo lock (prevents multiple instances)
3. Validate Docker prerequisites (image build if needed)
4. Initialize container tracker (crash recovery)
5. Clean up orphaned containers/worktrees
6. Start credential refresher (OAuth token sync)
7. Create Bubble Tea program and run TUI

## Main Components

### TUI Layer (`internal/tui/`)

The UI is built on [Bubble Tea](https://github.com/charmbracelet/bubbletea) (bubbletea v2), a Go framework for terminal applications.

```
AppModel                    # Main application state, message routing
  |-- PaneModel[]           # Individual workstream panes with vt10x terminals
  |-- StatusBarModel        # Bottom status bar
  |-- DialogModel           # Modal dialogs (new workstream, help, etc.)
  |-- LogPanelModel         # Debug log panel (toggleable)
  |-- PairingOrchestrator   # File sync coordination (Mutagen)
```

**Key files:**
- `app.go` - Central hub: state management, message routing, layout calculation
- `pane.go` - Terminal emulation via vt10x, ANSI color handling, scrollback
- `pty.go` - Docker exec PTY sessions, keyboard input, session ID capture
- `container.go` - Container lifecycle Bubble Tea commands (delegates to orchestrator)
- `dialog.go` - Modal dialog system
- `layout.go` - Multi-pane layout calculations
- `styles.go` - Lipgloss-based styling and color utilities

### Orchestration Layer (`internal/orchestrator/`) [NEW]

Workstream lifecycle business logic extracted from TUI. Enables testable code without Bubble Tea dependencies, reusable for CLI tools or API servers.

```
WorkstreamOrchestrator interface
  |-- CreateWorkstream()     - Full flow: worktree + image + config + container
  |-- CheckBranchConflict()  - Detect existing branch/worktree conflicts
  |-- PauseWorkstream()      - Pause container
  |-- ResumeWorkstream()     - Resume container
  |-- DestroyWorkstream()    - Remove container + worktree + optionally branch
  |-- RebuildWorkstream()    - Destroy + recreate (keeps worktree)
```

**Key files:**
- `orchestrator.go` - Interface, `Orchestrator` struct, options types (`CreateOptions`, `DestroyOptions`, `CreateResult`, `BranchConflict`)
- `create.go` - Complete creation flow: worktree, image resolution, config building, container start
- `lifecycle.go` - Pause, resume, destroy, rebuild operations

**CreateWorkstream flow:**
1. Update main branch (optional)
2. Create git worktree (new or from existing branch)
3. Copy untracked files (if requested)
4. Resolve image (devcontainer auto-detect or default)
5. Build container config (credentials, git identity, timezone, extra env)
6. Create and start container

### Workstream Management (`internal/workstream/`)

Tracks the state and metadata of each parallel workstream.

```
PersistentManager           # Auto-persisting wrapper (debounced saves)
  |-- Manager               # In-memory workstream registry
        |-- Workstream[]    # Individual workstream state
```

**Key concepts:**
- `Workstream`: Container ID + git branch + Claude session + state machine
- States: `starting` -> `running` -> `idle` / `pairing` / `stopped` / `error`
- State persisted to `~/.claude-cells/state/<repo-id>/state.json`
- Atomic writes with temp file + rename for crash safety

### Docker Integration (`internal/docker/`)

Manages container lifecycle and configuration.

```
DockerClient interface      # Abstraction for Docker operations
  |-- Client                # Real Docker SDK client
  |-- MockClient            # Test mock with in-memory state
```

**Key responsibilities:**
- Container CRUD (create, start, pause, unpause, remove)
- Image building (base image, devcontainer support)
- Security hardening (tiered capability drops)
- Credential management (OAuth token sync to containers)
- Orphan cleanup (crash recovery)

**Security tiers:** `hardened` > `moderate` (default) > `compat`
- Auto-relaxation: if container fails to start, tries less restrictive settings

### Git Operations (`internal/git/`)

Handles git worktrees, branches, and repository management.

```
GitClient interface         # Abstraction for git operations
  |-- Git                   # Real git CLI wrapper
  |-- MockClient            # Test mock
```

**Key features:**
- Git worktree management (isolation without repo conflicts)
- Branch operations (create, checkout, delete, push)
- PR creation via `gh` CLI
- Repo ID generation (first commit hash for stable identity)

### File Sync (`internal/sync/`)

Bidirectional file sync between containers and local filesystem.

```
Pairing                     # Orchestrates pairing mode
  |-- GitOperations         # Git interface for branch switching
  |-- MutagenOperations     # Mutagen session management
```

**Pairing mode:**
1. Stash local changes
2. Checkout workstream branch locally
3. Start Mutagen sync session
4. Real-time bidirectional sync with conflict detection

### Claude CLI Integration (`internal/claude/`)

Ephemeral queries to Claude CLI for metadata generation.

```
Query()                     # Execute stateless Claude query
  |-- Uses fixed session ID to avoid polluting resume log
  |-- Thread-safe (mutex-protected)
  |-- Periodic context clearing (every 100 queries)
```

Used for: title generation, commit message suggestions

## Data Flow

### Container Creation Flow

```
User presses 'n'
    |
    v
NewWorkstream dialog
    |
    v
Title generation (Claude CLI query)
    |
    v
Create git worktree (/tmp/ccells/worktrees/<branch>)
    |
    v
Create container config (credentials, mounts)
    |
    v
Docker: CreateContainer -> StartContainer
    |
    v
Create PTY session (docker exec with TTY)
    |
    v
Run container setup script (install Claude Code if needed)
    |
    v
Launch Claude Code with --dangerously-skip-permissions
    |
    v
Capture session ID from output
    |
    v
Workstream state -> "running"
```

### Message Flow (Bubble Tea)

```
tea.Msg -> AppModel.Update() -> dispatch to handlers
    |
    v
Key events -> route to focused pane or dialog
    |
    v
PTY output -> route to appropriate PaneModel
    |
    v
Timer ticks -> spinner animation, fade effects
    |
    v
State changes -> trigger UI re-render via View()
```

## Key Design Decisions

### Git Worktree Isolation
Each container gets its own worktree instead of sharing the host repo. This:
- Avoids `.git/index.lock` conflicts
- Keeps host working directory unchanged
- Shares git objects for efficiency
- Enables independent branch work

### OAuth Credential Architecture
```
Host keychain (source of truth)
    |
    v
CredentialRefresher (polls every 15 min)
    |
    v
Container config dirs (~/.claude-cells/containers/<name>/)
    |
    v
Mounted into containers at /root/.claude/.credentials.json
```

Why keychain is authoritative: containers may be paused while ccells is off. On restart, fresh credentials must be pushed to revive stale tokens.

### Virtual Terminal (vt10x)
Panes embed a virtual terminal emulator to properly handle:
- ANSI escape sequences (colors, cursor positioning)
- Scrollback buffer with color preservation
- Cursor rendering for input feedback
- Muted colors for unfocused panes

### State Persistence
- `PersistentManager` wraps `Manager` with auto-save
- 200ms debounced saves prevent disk thrashing
- Atomic writes ensure crash safety
- State includes: workstreams, focus, layout, repo metadata

## External Dependencies

| Dependency | Purpose |
|------------|---------|
| Docker | Container runtime |
| git | Version control, worktrees |
| gh | GitHub CLI for PRs |
| mutagen | File sync (optional, for pairing) |
| claude | Claude Code CLI (installed in containers) |

## Directory Structure (Runtime)

```
~/.claude-cells/
  state/<repo-id>/
    state.json              # Workstream state
    .ccells.lock            # Per-repo instance lock
  containers/<name>/
    .claude/                # Mounted as /root/.claude
      .credentials.json     # OAuth tokens
  config.yaml               # Global security config
  ephemeral-query-state.json

/tmp/ccells/
  worktrees/<branch>/       # Git worktrees per workstream
```

## Architectural Smells

### 1. Business Logic in TUI Layer (Significantly Improved)

**File:** `internal/tui/container.go` (1356 lines, reduced from 1653)

Container orchestration logic has been extracted to `internal/orchestrator` package. The TUI layer now delegates to the orchestrator for core operations.

**Completed extraction (PR #8):**
- `orchestrator.go` - `WorkstreamOrchestrator` interface and `Orchestrator` implementation
- `create.go` - Complete workstream creation flow:
  - `CreateWorkstream()` - Full orchestration of worktree + container + credentials
  - `CheckBranchConflict()` - Branch/worktree conflict detection
  - `resolveImage()` - Auto-detect image from devcontainer or use default
  - `buildFullContainerConfig()` - Container config with credentials, git identity
  - `copyUntrackedFiles()` - Copy untracked files to worktree
- `lifecycle.go` - `PauseWorkstream()`, `ResumeWorkstream()`, `DestroyWorkstream()`, `RebuildWorkstream()`
- AppModel has `orchestrator` field - accessed via `Orchestrator()` getter
- `container.go` uses orchestrator for create/destroy operations

**Remaining in `container.go`:**
- Bubble Tea message types and command wrappers
- PTY session management
- Pairing mode integration
- Title generation via Claude CLI
- Container tracking/credential registration (global state)

**Impact:** Core business logic is now testable without TUI. ~300 lines extracted.

### 2. God Objects (Improved)

| Object | File | Lines | Fields | Issue |
|--------|------|-------|--------|-------|
| AppModel | app.go | 2000+ | 94 | Main event loop - acceptable for Bubble Tea |
| PaneModel | pane.go | 1321 | 40+ | Core pane logic (reduced from 1701 lines) |

**PaneModel refactoring completed:** Split into focused files:
- `pane.go` (1321 lines) - Core struct, View, Update, vterm rendering
- `pane_colors.go` (164 lines) - ANSI color utilities (`muteANSI`, `stripANSI`, `color256ToRGB`)
- `pane_scroll.go` (77 lines) - Scroll methods (`ScrollPageUp`, `ScrollToBottom`, etc.)
- `pane_animation.go` (172 lines) - Animation state (`SummarizePhase`, fade/init methods)

### 3. Global Mutable State (Improved)

Package-level state now uses encapsulated structs with thread-safety:

```go
// container.go:26-32 - Services struct (set once at startup)
type containerServices struct {
    tracker   *docker.ContainerTracker
    refresher *docker.CredentialRefresher
}
var services containerServices

// pty.go:60-65 - Mutex-protected program sender
type programSender struct {
    mu      sync.RWMutex
    program *tea.Program
}
var sender programSender
```

**Current state:** Thread-safe (mutex protection, set-once patterns). Tests pass with `-race`.
**Remaining improvement:** Could move to AppModel fields for true dependency injection, but current pattern is acceptable.

## Design Patterns

### Well-Implemented
- **Command Pattern** (Bubble Tea tea.Cmd) - Clean async operations
- **Strategy Pattern** (GitClientFactory) - Test injection without DI framework
- **Repository Pattern** (Manager) - Thread-safe in-memory registry
- **Decorator Pattern** (PersistentManager wraps Manager) - Auto-persistence
- **State Machine** (Workstream.State) - Explicit state transitions

### Anti-Patterns Present
- **Anemic Domain Model** - Workstream is mostly getters/setters

## Layer Boundaries

```
┌─────────────────────────────────────────────────────────────┐
│ main.go (cmd/ccells)                                        │
│ - Startup, signal handling, crash recovery                  │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Presentation Layer (internal/tui)                          │
│ - AppModel, PaneModel, DialogModel, PTYSession             │
│ ⚠️ Also contains business logic (container.go)              │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Orchestration Layer (internal/orchestrator)         [NEW]  │
│ - WorkstreamOrchestrator: create, pause, resume, destroy   │
│ - Pure business logic, no TUI dependencies                 │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Domain Layer (internal/workstream)                         │
│ - Manager, PersistentManager, Workstream                   │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Infrastructure (internal/docker, git, sync, claude)        │
│ - DockerClient, GitClient, Mutagen, Claude CLI             │
└─────────────────────────────────────────────────────────────┘
```

## Improvement Roadmap

### Completed
1. **Extract Orchestration Layer** ✅ (PR #8)
   - `internal/orchestrator` package with full `WorkstreamOrchestrator` interface
   - Complete operations: `CreateWorkstream`, `PauseWorkstream`, `ResumeWorkstream`, `DestroyWorkstream`, `RebuildWorkstream`, `CheckBranchConflict`
   - Image resolution: auto-detect from devcontainer, build with devcontainer CLI, fallback to default
   - Container config: credentials, git identity, timezone, extra env vars
   - Worktree management: create, cleanup on error, branch sanitization
   - AppModel uses orchestrator via `Orchestrator()` getter

2. **Global State Thread-Safety** ✅
   - `containerServices` struct encapsulates tracker/refresher (set-once pattern)
   - `programSender` uses `sync.RWMutex` for concurrent PTY goroutine access
   - All tests pass with `-race` detector

3. **Refactor PaneModel** ✅
   - Split 1700-line file into focused components:
   - `pane_colors.go` (164 lines) - ANSI color utilities
   - `pane_scroll.go` (77 lines) - Scroll methods
   - `pane_animation.go` (172 lines) - Animation state/methods
   - Core `pane.go` reduced to 1321 lines

### Low Priority (Optional)
4. **Move Global State to AppModel** - For true dependency injection:
   - Pass `containerServices` via AppModel fields
   - Pass `programSender` through PTYSession constructor
   - Current encapsulated pattern is acceptable if this is deferred

### Future Vision
```
┌────────────────────────────────────────┐
│ TUI (Bubble Tea)                       │  ← Presentation only
├────────────────────────────────────────┤
│ Orchestrator API                       │  ← Business logic ✅ Created
├────────────────────────────────────────┤
│ Docker / Git / Sync                    │  ← Infrastructure
└────────────────────────────────────────┘
```
