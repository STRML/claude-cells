# Architecture Overview

Last updated: 2026-01-22

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
- `container.go` - Container lifecycle commands (create, pause, resume, destroy)
- `dialog.go` - Modal dialog system
- `layout.go` - Multi-pane layout calculations
- `styles.go` - Lipgloss-based styling and color utilities

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

### 1. Business Logic in TUI Layer (In Progress - Partially Resolved)

**File:** `internal/tui/container.go` (1653 lines)

Container orchestration logic (git + docker operations) is embedded in `tea.Cmd` functions. This mixes UI concerns with business logic.

**Progress:** The `internal/orchestrator` package has been created to address this:
- `orchestrator.go` - `WorkstreamOrchestrator` interface and `Orchestrator` implementation
- `create.go` - `CreateWorkstream()` with worktree and container creation
- `lifecycle.go` - `PauseWorkstream()`, `ResumeWorkstream()`, `DestroyWorkstream()`, `RebuildWorkstream()`
- AppModel now has an `orchestrator` field for clean delegation

**Remaining work:** `container.go` still contains additional complexity (image building, devcontainer support, credential management) that requires extending the orchestrator.

**Impact:** Can't reuse logic in CLI tools or API servers; hard to unit test.

### 2. God Objects

| Object | File | Lines | Fields | Issue |
|--------|------|-------|--------|-------|
| AppModel | app.go | 2000+ | 94 | Main event loop - acceptable for Bubble Tea |
| PaneModel | pane.go | 1701 | 40+ | Mixes rendering, PTY, scrolling, animations |

**Recommendation:** Split PaneModel into PaneRenderer, ScrollController, AnimationController.

### 3. Global Mutable State

```go
var program *tea.Program           // pty.go:58
var containerTracker *...          // container.go:23
var credentialRefresher *...       // container.go:26
```

**Impact:** Breaks testability, risks data races.
**Fix:** Pass as AppModel fields.

## Design Patterns

### Well-Implemented
- **Command Pattern** (Bubble Tea tea.Cmd) - Clean async operations
- **Strategy Pattern** (GitClientFactory) - Test injection without DI framework
- **Repository Pattern** (Manager) - Thread-safe in-memory registry
- **Decorator Pattern** (PersistentManager wraps Manager) - Auto-persistence
- **State Machine** (Workstream.State) - Explicit state transitions

### Anti-Patterns Present
- **Anemic Domain Model** - Workstream is mostly getters/setters
- **God Object** - PaneModel handles too many concerns

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

### High Priority
1. **Extract Orchestration Layer** ✅ In Progress
   - `internal/orchestrator` package created with `WorkstreamOrchestrator` interface
   - Basic CRUD operations: `CreateWorkstream`, `PauseWorkstream`, `ResumeWorkstream`, `DestroyWorkstream`, `RebuildWorkstream`
   - AppModel wired to use orchestrator
   - **Next steps:** Extend orchestrator to handle image building, devcontainer support, credential management

### Medium Priority
2. **Refactor PaneModel** - Split into focused components
3. **Eliminate Global State** - Pass dependencies via AppModel

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
