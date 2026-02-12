# Architecture Overview

Last updated: 2026-02-12 (Updated: tmux migration, TUI removed)

## Architecture Assessment: 8/10

### Strengths
- tmux-native UI: no terminal emulation, no rendering code, fully resumable sessions
- Clean sidecar model: ccells orchestrates, tmux owns the terminal
- Strong interface abstractions (DockerClient, GitClient) with mocks
- Comprehensive testing infrastructure
- Excellent crash recovery & state persistence
- Security hardening by default with tiered relaxation
- Background daemon with Unix socket API

### Weaknesses
- TODO stubs in daemon handlers need wiring to orchestrator
- Missing observability (no metrics, tracing)

## Summary

Claude Cells (ccells) is a tmux-based orchestrator that runs parallel Claude Code instances in isolated Docker containers. It uses compose-style CLI commands (`up`/`down`/`create`/`rm`/`ps`) to manage workstreams, with a background daemon handling credential refresh, state reconciliation, and pairing health.

## Entry Points

```
cmd/ccells/main.go          # Primary CLI entry point
cmd/ccells/commands.go       # Command registry + flag parsing
cmd/keytest/main.go         # Keyboard testing utility (dev tool)
```

### Main Entry Flow

```
main() -> parseCommand() -> dispatch:
  "up"     -> validatePrerequisites() -> runUp() -> tmux session + daemon + attach
  "attach" -> runAttach() -> tmux attach
  "down"   -> runDown() -> daemon shutdown + tmux kill
  "create" -> runCreate() -> daemon request
  "rm"     -> runRemove() -> daemon request
  "ps"     -> runPS() -> tmux list-panes query
  "pair"   -> runPair() -> daemon request
```

Key startup operations (in `up`):
1. Validate Docker prerequisites (image build if needed)
2. Acquire per-repo lock (prevents multiple instances)
3. Clean up orphaned containers/worktrees
4. Start heartbeat for crash recovery
5. Create tmux session (detached)
6. Configure chrome (status line, pane borders, keybindings)
7. Start background daemon (credential refresh, reconciliation)
8. Attach to tmux session (blocks until detach)
9. Print detach summary on exit

## Main Components

### tmux Layer (`internal/tmux/`)

Manages the tmux server, sessions, and panes. ccells owns a dedicated tmux server per repo (socket: `ccells-<repo-id>`).

```
Client                      # tmux CLI wrapper for a specific socket
  |-- NewSession()          # Create detached session
  |-- HasSession()          # Check if session exists
  |-- KillServer()          # Kill tmux server
  |-- AttachCommand()       # Returns exec.Cmd for attach
  |-- ConfigureChrome()     # Set up status line, borders, keybindings
  |-- CreatePane()          # Split window to add pane
  |-- KillPane()            # Kill a specific pane
  |-- SetPaneOption()       # Set per-pane metadata
  |-- ListPanes()           # Query panes with metadata
```

**Key files:**
- `tmux.go` - Server lifecycle, session management, version detection
- `pane.go` - Pane CRUD, metadata (workstream/container vars)
- `chrome.go` - Status line formatting, pane border formatting, keybindings, help

### Daemon (`internal/daemon/`)

Background process running alongside the tmux session. Communicates with CLI commands via Unix domain socket.

```
Daemon                      # Background process
  |-- Run()                 # Start socket server + background loops
  |-- dispatch()            # Route requests to handlers
  |-- handleCreate()        # Create workstream (TODO: wire to orchestrator)
  |-- handleRemove()        # Remove workstream
  |-- handlePause()         # Pause workstream
  |-- handleUnpause()       # Resume workstream
  |-- handlePair()          # Enable pairing mode
  |-- handleUnpair()        # Disable pairing mode
  |-- handlePairStatus()    # Query pairing state
```

**Background loops:**
- Reconciliation (30s default): Cross-references tmux panes with Docker containers
- Pairing health (5s default): Checks Mutagen sync health when pairing is active

**Key files:**
- `daemon.go` - Socket server, request dispatch, background loops
- `api.go` - Request/Response JSON protocol
- `reconcile.go` - State reconciliation types and logic

### CLI Layer (`cmd/ccells/`)

Compose-style CLI commands. Each command is a separate file:

```
cmd_up.go      # up: create tmux session + daemon + attach
cmd_attach.go  # attach: reattach to existing session
cmd_down.go    # down: stop daemon + tmux + shared daemon client helpers
cmd_create.go  # create: new workstream via daemon
cmd_rm.go      # rm: destroy workstream
cmd_pause.go   # pause/unpause workstreams
cmd_ps.go      # ps: list workstreams from tmux panes
cmd_pair.go    # pair/unpair/status: pairing mode
```

**Interactive dialogs (Bubble Tea programs for tmux display-popup):**
- `dialog_create.go` - New workstream form
- `dialog_merge.go` - PR/merge flow
- `dialog_rm.go` - Destroy confirmation

### Orchestration Layer (`internal/orchestrator/`)

Workstream lifecycle business logic. Pure Go, no TUI or tmux dependencies.

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
- `orchestrator.go` - Interface, struct, options/result types
- `create.go` - Complete creation flow
- `lifecycle.go` - Pause, resume, destroy, rebuild operations

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

### Git Operations (`internal/git/`)

Handles git worktrees, branches, and repository management.

### File Sync (`internal/sync/`)

Bidirectional file sync between containers and local filesystem via Mutagen.

### Claude CLI Integration (`internal/claude/`)

Ephemeral queries to Claude CLI for metadata generation (titles, commit messages).

## Data Flow

### Container Creation Flow

```
User: ccells create --branch my-feature --prompt "add auth"
    |
    v
CLI sends "create" request to daemon via Unix socket
    |
    v
Daemon handler (TODO: wire to orchestrator)
    |
    v
Orchestrator.CreateWorkstream():
  1. Create git worktree (/tmp/ccells/worktrees/<branch>)
  2. Resolve Docker image
  3. Build container config (credentials, mounts)
  4. Docker: CreateContainer -> StartContainer
    |
    v
Create tmux pane: docker exec -it <container> claude --resume
    |
    v
Set pane metadata (@ccells-workstream, @ccells-container)
    |
    v
Update status line
```

### CLI → Daemon Communication

```
CLI command (e.g., ccells create)
    |
    v
net.Dial("unix", daemon.sock)
    |
    v
JSON request: {"action": "create", "params": {...}}
    |
    v
Daemon.dispatch() -> handler
    |
    v
JSON response: {"ok": true, "data": {...}}
```

## Key Design Decisions

### tmux Sidecar Model
ccells runs as an orchestrator alongside tmux, not inside it. Benefits:
- **Resumability**: Detach and reattach without losing state
- **Native terminal**: No vterm emulation, no rendering bugs
- **Simplicity**: ~2000 lines replaces ~24,500 lines of TUI code
- **Standard interaction**: Users get full tmux functionality (copy mode, zoom, etc.)

### Hybrid State Model
- **Operational truth**: Derived from tmux (`list-panes`) + Docker (`docker ps`)
- **Metadata**: Supplementary state file for prompts, session IDs, PR info
- **Lossy-safe**: If state file is stale/missing, workstreams still function

### Background Daemon
- Runs in-process alongside tmux attach
- Unix socket for CLI command communication
- Survives tmux detach (same process)
- Crash recovery: `ccells attach` restarts daemon if needed

### Git Worktree Isolation
Each container gets its own worktree instead of sharing the host repo. This:
- Avoids `.git/index.lock` conflicts
- Keeps host working directory unchanged
- Shares git objects for efficiency

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
Mounted into containers at /home/claude/.credentials.json
```

## External Dependencies

| Dependency | Purpose |
|------------|---------|
| tmux | Terminal multiplexer (pane management, sessions) |
| Docker | Container runtime |
| git | Version control, worktrees |
| gh | GitHub CLI for PRs |
| mutagen | File sync (optional, for pairing) |
| claude | Claude Code CLI (installed in containers) |

## Directory Structure (Runtime)

```
~/.claude-cells/
  state/<repo-id>/
    state.json              # Workstream metadata
    daemon.sock             # Daemon Unix socket
    .ccells.lock            # Per-repo instance lock
  containers/<name>/
    .claude/                # Mounted as /home/claude
      .credentials.json     # OAuth tokens
  config.yaml               # Global config
  logs/                     # Exported logs

/tmp/ccells/
  worktrees/<branch>/       # Git worktrees per workstream

/tmp/tmux-<uid>/
  ccells-<repo-id>          # tmux server socket
```

## Layer Boundaries

```
┌─────────────────────────────────────────────────────────────┐
│ CLI Layer (cmd/ccells)                                      │
│ - Command dispatch, flag parsing, interactive dialogs       │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ tmux Layer (internal/tmux)                                  │
│ - Server management, pane CRUD, chrome, keybindings         │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Daemon (internal/daemon)                                    │
│ - Socket server, reconciliation, pairing health             │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Orchestration Layer (internal/orchestrator)                  │
│ - WorkstreamOrchestrator: create, pause, resume, destroy    │
│ - Pure business logic, no tmux/daemon dependencies          │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Domain Layer (internal/workstream)                          │
│ - Manager, PersistentManager, Workstream                    │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Infrastructure (internal/docker, git, sync, claude)         │
│ - DockerClient, GitClient, Mutagen, Claude CLI              │
└─────────────────────────────────────────────────────────────┘
```

## Design Patterns

### Well-Implemented
- **Sidecar Pattern** (ccells + tmux) - Orchestrator manages, tmux owns terminal
- **Command Pattern** (daemon request/response) - Clean async operations
- **Strategy Pattern** (GitClientFactory) - Test injection without DI framework
- **Repository Pattern** (Manager) - Thread-safe in-memory registry
- **Decorator Pattern** (PersistentManager wraps Manager) - Auto-persistence
- **State Machine** (Workstream.State) - Explicit state transitions
- **Reconciliation Pattern** (daemon) - Cross-reference tmux + Docker for truth
