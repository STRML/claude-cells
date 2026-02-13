# Backend Structure

Last updated: 2026-02-13 (Updated: powerline chrome, welcome dialog, create dialog AI flow)

## Quick Reference: Key Files

| File | Lines | Notes |
|------|-------|-------|
| `cmd/ccells/main.go` | ~850 | Entry point, arg parsing, command dispatch |
| `cmd/ccells/commands.go` | ~100 | Command registry + flag parsing |
| `cmd/ccells/dialog_create.go` | ~210 | Interactive create popup (4-step with AI title gen) |
| `cmd/ccells/dialog_welcome.go` | ~85 | First-run welcome screen (Bubble Tea) |
| `internal/tmux/tmux.go` | ~115 | tmux server lifecycle |
| `internal/tmux/chrome.go` | ~320 | Powerline status, pane borders, keybindings, path utils |
| `internal/daemon/daemon.go` | ~320 | Socket server, background loops |
| `orchestrator/create.go` | ~310 | Creation flow (worktree + container) |
| `git/branch.go` | ~700 | Acceptable |
| `docker/container.go` | ~600 | Acceptable |

## Package Overview

```
cmd/ccells/           # CLI entry point + compose-style subcommands
  main.go             # Startup, arg parsing, command dispatch
  commands.go         # Command registry + flag parsing
  cmd_up.go           # up: create tmux session + daemon + attach
  cmd_attach.go       # attach: reattach to existing session
  cmd_down.go         # down: stop daemon + tmux + daemon client helpers
  cmd_create.go       # create: new workstream via daemon
  cmd_rm.go           # rm: destroy workstream
  cmd_pause.go        # pause/unpause workstreams
  cmd_ps.go           # ps: list workstreams from tmux panes
  cmd_pair.go         # pair/unpair/status: pairing mode
  dialog_create.go    # Interactive create popup (4-step with AI title gen)
  dialog_welcome.go   # First-run welcome screen (intro + keybindings)
  dialog_merge.go     # Interactive merge popup
  dialog_rm.go        # Interactive destroy popup
  detach.go           # Detach summary formatting
  runtime.go          # Runtime selection (claude/claudesp)

internal/
  tmux/               # tmux server + pane management
  daemon/             # Background daemon (sidecar process)
  orchestrator/       # Workstream lifecycle orchestration
  workstream/         # Workstream state and lifecycle management
  docker/             # Docker SDK wrapper and container management
  git/                # Git CLI wrapper and worktree operations
  gitproxy/           # Git proxy for container operations
  sync/               # Mutagen-based file sync for pairing mode
  claude/             # Claude CLI wrapper for ephemeral queries
  config/             # Global application configuration

configs/              # Embedded static files (Dockerfile)
```

---

## `internal/tmux/`

tmux server and pane management. Each ccells session uses a dedicated tmux server socket (`ccells-<repo-id>`).

### Key Types

| Type | File | Description |
|------|------|-------------|
| `Client` | tmux.go | tmux CLI wrapper targeting a specific socket |
| `PaneInfo` | pane.go | Pane metadata (ID, workstream, container) |
| `StatusWorkstream` | chrome.go | Data for status line rendering |

### Key Functions

**tmux.go:**
- `NewClient(socket)` - Create client for a socket
- `ServerRunning()` - Check if tmux server is running
- `Version()` - Get tmux version string
- `Prefix()` - Get user's tmux prefix key
- `NewSession()` - Create detached session
- `NewSessionWithCommand()` - Create session with initial pane command
- `HasSession()` - Check if session exists
- `KillServer()` - Kill tmux server
- `KillSession()` - Kill specific session
- `AttachCommand()` - Returns exec.Cmd for attach

**pane.go:**
- `CreatePane()` - Split window to add a pane
- `KillPane()` - Kill a specific pane
- `SetPaneOption()` - Set per-pane metadata (@ccells-workstream, etc.)
- `ListPanes()` - Query panes with custom format string

**chrome.go:**
- `ConfigureChrome(ctx, session, ccellsBin, repoPath, branch)` - Set up powerline status, pane borders, keybindings
- `FormatPowerlineLeft(repoPath, branch)` - Build powerline-style status-left with abbreviated path + branch icon
- `AbbreviatePath(path)` - Shorten path (e.g., `~/g/o/claude-cells`)
- `FormatStatusLine()` - Render status-right content (workstream indicators + keyhints)
- `FormatPaneBorder()` - Render pane border text
- `FormatPrefixHint()` - Convert "C-b" to "^b" for display
- `SetSessionOption()` - Set session-level tmux options
- `SetHook()` - Set tmux hooks on a session

---

## `internal/daemon/`

Background process managing credentials, state reconciliation, and pairing health. Communicates with CLI commands via Unix domain socket.

### Key Types

| Type | File | Description |
|------|------|-------------|
| `Daemon` | daemon.go | Background process with socket server |
| `Config` | daemon.go | Daemon configuration (socket path, intervals, providers) |
| `ReconcileFunc` | daemon.go | Callback for periodic reconciliation |
| `PairingProvider` | daemon.go | Interface for pairing operations |
| `PairingState` | daemon.go | Snapshot of pairing state |
| `Request` | api.go | JSON request from CLI |
| `Response` | api.go | JSON response from daemon |
| `Reconciler` | reconcile.go | Cross-references tmux + Docker state |
| `ReconcileResult` | reconcile.go | Categorized state (healthy/orphaned) |

### Daemon Actions

| Action | Handler | Description |
|--------|---------|-------------|
| `ping` | inline | Health check |
| `create` | `handleCreate` | Create workstream (calls orchestrator) |
| `rm` | `handleRemove` | Remove workstream (calls orchestrator) |
| `pause` | `handlePause` | Pause workstream (calls orchestrator) |
| `unpause` | `handleUnpause` | Resume workstream (calls orchestrator) |
| `pair` | `handlePair` | Enable pairing mode (calls sync.Pairing) |
| `unpair` | `handleUnpair` | Disable pairing mode |
| `pair-status` | `handlePairStatus` | Query pairing state |
| `shutdown` | inline | Graceful shutdown |

### Key Functions

**daemon.go:**
- `New(config)` - Create daemon instance
- `Run(ctx)` - Start socket server + background loops (blocks until ctx cancelled)
- `dispatch(ctx, req)` - Route request to handler
- `handlePair()` / `handleUnpair()` / `handlePairStatus()` - Pairing operations

**reconcile.go:**
- `Reconciler.Reconcile(panes, containers)` - Cross-reference and categorize state

---

## `cmd/ccells/`

CLI entry point with compose-style subcommands.

### Key Functions

**main.go:**
- `main()` - Parse args, resolve repo, dispatch command (includes `welcome` dispatch)
- `getRepoInfo()` - Returns `(repoID, repoPath, stateDir, error)` with proper error propagation
- `getStateDir()` - Returns `(stateDir, error)` convenience wrapper
- `runStatusTmux()` - Prints colored workstream status to stdout (for tmux `#()` command substitution)
- `validatePrerequisites()` - Check Docker, build image if needed
- `cleanupOrphanedContainers()` - Remove containers from crashed sessions
- `cleanupOrphanedWorktrees()` - Remove stale worktrees
- `runHeartbeat()` - Background heartbeat for crash recovery
- `runStateRepair()` - Validate and repair state file

**cmd_up.go:**
- `runUp()` - Smart session startup: determines initial pane command based on state, creates tmux session, gets git branch for powerline, configures chrome, starts daemon, attaches
- `doAttach()` - Attach to tmux session (blocks until detach)
- `printDetachSummary()` - Print summary after detach

**cmd_down.go:**
- `runDown()` - Send shutdown to daemon, kill tmux server
- `sendDaemonRequest()` - Send action to daemon (fire-and-forget)
- `sendDaemonRequestWithResponse()` - Send action and read response

**commands.go:**
- `parseCommand()` - Extract command name from args
- `parseFlags()` - Extract global flags (--runtime, --help, etc.)

**dialog_create.go:**
- `createDialog` - Bubble Tea model for interactive create popup
- 4-step flow: `0=prompt` -> `1=summarizing` (Claude CLI) -> `2=confirm` -> `3=creating`
- `generateTitle()` - Calls Claude CLI to generate short 3-5 word title from task prompt
- Uses `workstream.GenerateBranchName()` for branch derivation from AI-generated title
- `summarizeResultMsg` - Async message type for title generation result

**dialog_welcome.go:**
- `welcomeDialog` - Bubble Tea model for first-run welcome screen
- Shows intro text, keybindings overview, then chains to create dialog on Enter/n
- `runWelcome()` - Entry point; runs welcome dialog, optionally chains to create

**detach.go:**
- `formatDetachSummary()` - Format detach summary text

**runtime.go:**
- `ResolveRuntime()` - Resolve runtime from CLI flag + config files

---

## `internal/orchestrator/`

Workstream lifecycle orchestration. Pure business logic, no tmux/daemon dependencies.

### Key Types

| Type | File | Description |
|------|------|-------------|
| `WorkstreamOrchestrator` | orchestrator.go | Interface for lifecycle operations |
| `Orchestrator` | orchestrator.go | Implementation coordinating Docker + Git |
| `CreateOptions` | orchestrator.go | Creation config |
| `CreateResult` | orchestrator.go | Result with container ID, name, paths |
| `DestroyOptions` | orchestrator.go | Destruction config |
| `BranchConflict` | orchestrator.go | Branch conflict info |

### Key Functions

**orchestrator.go:**
- `New(dockerClient, gitFactory, repoPath)` - Create orchestrator instance

**create.go:**
- `CreateWorkstream()` - Complete creation flow
- `CheckBranchConflict()` - Check for existing branch/worktree
- `resolveImage()` - Auto-detect image from devcontainer
- `buildFullContainerConfig()` - Build config with credentials, git identity

**lifecycle.go:**
- `PauseWorkstream()` - Pause container
- `ResumeWorkstream()` - Resume container
- `DestroyWorkstream()` - Stop container, remove worktree, optionally delete branch
- `RebuildWorkstream()` - Destroy + recreate container

---

## `internal/workstream/`

Workstream state management and persistence.

### Key Types

| Type | File | Description |
|------|------|-------------|
| `Workstream` | workstream.go | Single workstream state (container, branch, session) |
| `State` | workstream.go | Lifecycle state enum |
| `Manager` | manager.go | In-memory workstream registry |
| `PersistentManager` | persistent_manager.go | Auto-persisting Manager wrapper |
| `AppState` | state.go | Serializable application state |
| `SavedWorkstream` | state.go | Serializable workstream data |

### Key Functions

**manager.go:**
- `Add()` - Register workstream (enforces `MaxWorkstreams = 12`)
- `Remove()` - Unregister workstream
- `Get()`, `GetByBranch()` - Lookup workstreams

**persistent_manager.go:**
- `markDirty()` - Flag state for save
- `saveLoop()` - Background 200ms debounced save

**state.go:**
- `SaveState()` - Atomic write to state file
- `LoadState()` - Read state file
- `GetStateDir()` - Compute state directory path

---

## `internal/docker/`

Docker SDK wrapper and container configuration.

### Key Types

| Type | File | Description |
|------|------|-------------|
| `DockerClient` | interface.go | Interface for Docker operations |
| `Client` | client.go | Real Docker SDK implementation |
| `MockClient` | mock_client.go | Test mock with in-memory state |
| `ContainerConfig` | container.go | Container creation config |
| `SecurityConfig` | security.go | Security hardening settings |
| `CredentialRefresher` | credentials.go | OAuth token sync service |
| `ContainerTracker` | tracking.go | Crash recovery tracking |

---

## `internal/git/`

Git CLI wrapper for worktree and branch operations.

### Key Types

| Type | File | Description |
|------|------|-------------|
| `GitClient` | interface.go | Interface for git operations |
| `Git` | branch.go | Real git CLI implementation |
| `MockClient` | mock_client.go | Test mock |

---

## `internal/gitproxy/`

Secure git proxy for container operations. Containers use git/gh commands that are intercepted by hooks and proxied through the host.

### Key Types

| Type | File | Description |
|------|------|-------------|
| `Server` | server.go | Unix socket server per container |
| `Manager` | server.go | Manages multiple proxy sockets |

---

## `internal/sync/`

Mutagen-based file sync for pairing mode.

### Key Types

| Type | File | Description |
|------|------|-------------|
| `Pairing` | pairing.go | Pairing mode orchestrator |
| `PairingState` | pairing.go | Snapshot of pairing state |
| `Mutagen` | mutagen.go | Real mutagen CLI wrapper |

---

## `internal/claude/`

Claude CLI wrapper for ephemeral queries.

### Key Functions

**query.go:**
- `Query()` - Execute ephemeral Claude query
- `QueryWithTimeout()` - Convenience wrapper with timeout

---

## Dependencies Between Packages

```
cmd/ccells/main.go
    |
    +-> internal/tmux (session + pane management)
    +-> internal/daemon (background process)
    +-> internal/docker (validation, tracking, cleanup)
    +-> internal/git (repo ID)
    +-> internal/workstream (state loading)

internal/daemon
    +-> (no internal deps — uses interfaces/callbacks)

internal/orchestrator
    +-> internal/docker (DockerClient interface)
    +-> internal/git (GitClient via factory)
    +-> internal/workstream (Workstream type)

internal/docker
    +-> configs (embedded Dockerfile)
    +-> (Docker SDK)

internal/workstream
    +-> internal/docker (session ID extraction)

internal/sync
    +-> internal/git (GitOperations interface)

internal/gitproxy
    +-> internal/git (GitClient)
```

## Essential Reading Order

1. `cmd/ccells/main.go` - Startup flow
2. `cmd/ccells/cmd_up.go` - Session creation + daemon start
3. `internal/tmux/tmux.go` - tmux server management
4. `internal/tmux/chrome.go` - Status line + keybindings
5. `internal/daemon/daemon.go` - Background process
6. `internal/daemon/api.go` - Request/Response protocol
7. `internal/orchestrator/orchestrator.go` - Workstream lifecycle interface
8. `internal/orchestrator/create.go` - Creation flow
9. `internal/workstream/workstream.go` - Core domain model
10. `internal/docker/interface.go` - Docker abstraction
