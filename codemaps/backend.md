# Backend Structure

Last updated: 2026-01-22

## Quick Reference: Large Files

| File | Lines | Notes |
|------|-------|-------|
| `tui/app.go` | 2000+ | Main event loop - acceptable |
| `tui/pane.go` | 1701 | God object - candidate for split |
| `tui/container.go` | 1653 | Business logic in TUI - extract to orchestrator |
| `tui/dialog.go` | 1329 | Multiple dialog types - could split |
| `git/branch.go` | 700+ | Acceptable |
| `docker/container.go` | 600+ | Acceptable |

## Package Overview

```
internal/
  tui/          # Terminal UI (Bubble Tea) - main application logic
  workstream/   # Workstream state and lifecycle management
  docker/       # Docker SDK wrapper and container management
  git/          # Git CLI wrapper and worktree operations
  sync/         # Mutagen-based file sync for pairing mode
  claude/       # Claude CLI wrapper for ephemeral queries
  config/       # Global application configuration

configs/        # Embedded static files (Dockerfile)
```

---

## `internal/tui/`

The main application package implementing the Bubble Tea TUI.

### Key Types

| Type | File | Description |
|------|------|-------------|
| `AppModel` | app.go | Root Bubble Tea model, orchestrates all state |
| `PaneModel` | pane.go | Individual workstream pane with vt10x terminal |
| `PTYSession` | pty.go | Docker exec PTY session management |
| `DialogModel` | dialog.go | Modal dialog system |
| `StatusBarModel` | statusbar.go | Bottom status bar |
| `LogPanelModel` | logpanel.go | Debug log panel |
| `LayoutType` | layout.go | Layout enum (Vertical, Horizontal, Grid) |

### Message Types (Bubble Tea)

| Message | File | Purpose |
|---------|------|---------|
| `PTYOutputMsg` | pty.go | PTY output data |
| `PTYClosedMsg` | pty.go | PTY session closed |
| `SessionIDCapturedMsg` | pty.go | Claude session ID captured |
| `StateLoadedMsg` | app.go | State restored from disk |
| `StateSavedMsg` | app.go | State persisted |
| `spinnerTickMsg` | app.go | Animation tick |
| `fadeTickMsg` | app.go | Fade animation tick |
| `escapeTimeoutMsg` | app.go | Double-escape detection |
| `ContainerCreatedMsg` | container.go | Container creation complete |
| `ContainerPausedMsg` | container.go | Container pause complete |
| `ContainerResumedMsg` | container.go | Container resume complete |
| `ContainerDestroyedMsg` | container.go | Container destruction complete |
| `WorkstreamStartedMsg` | container.go | Workstream fully initialized |

### Key Functions

**app.go:**
- `NewAppModel(ctx)` - Create app with context for cancellation
- `Update(msg)` - Main message handler (implements `tea.Model`)
- `View()` - Render UI (implements `tea.Model`)

**container.go:**
- `startContainerCmd()` - Async container creation
- `pauseContainerCmd()` - Async container pause
- `resumeContainerCmd()` - Async container resume
- `destroyContainerCmd()` - Async container destruction

**pty.go:**
- `NewPTYSession()` - Create PTY session with Docker exec
- `StartReadLoop()` - Background output reader
- `escapeShellArg()` - Safe shell argument escaping

**pane.go:**
- `WritePTYOutput()` - Write to vterm with scrollback tracking
- `renderVTerm()` - Render vterm content with ANSI colors
- `muteANSI()` - Desaturate colors for unfocused panes

### Package-Level State

| Variable | Purpose |
|----------|---------|
| `program *tea.Program` | Global program ref for PTY message sending |
| `containerTracker` | Crash recovery tracking |
| `credentialRefresher` | OAuth token sync service |
| `versionInfo, commitHash` | Version display in help dialog |

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
| `RepoInfo` | state.go | Repository metadata |

### Workstream States

```go
const (
    StateStarting State = "starting"  // Container being created
    StateRunning  State = "running"   // Claude active
    StateIdle     State = "idle"      // Claude finished, container alive
    StatePairing  State = "pairing"   // Mutagen sync active
    StateStopped  State = "stopped"   // Container paused
    StateError    State = "error"     // Container failed
)
```

### Key Functions

**workstream.go:**
- `New(prompt)` - Create workstream with auto-generated branch
- `NewWithUniqueBranch()` - Create with collision-free branch name
- `NewWithID()` - Restore from saved state
- `SetState()`, `SetError()`, `SetContainerID()` - Thread-safe setters

**manager.go:**
- `Add()` - Register workstream (enforces `MaxWorkstreams = 12`)
- `Remove()` - Unregister workstream
- `Get()`, `GetByBranch()` - Lookup workstreams

**persistent_manager.go:**
- `markDirty()` - Flag state for save
- `saveLoop()` - Background 200ms debounced save
- `LoadAndRestore()` - Load state from disk

**state.go:**
- `SaveState()` - Atomic write to state file
- `LoadState()` - Read state file
- `GetStateDir()` - Compute state directory path
- `MigrateStateFile()` - Move state from old to new location

**branch.go:**
- `GenerateBranchName()` - Derive branch from prompt
- `GenerateUniqueBranchName()` - Add suffix for collision avoidance

**state_repair.go:**
- `ValidateAndRepairState()` - Extract session IDs from containers

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
| `ContainerInfo` | container.go | Container list entry |
| `SecurityConfig` | security.go | Security hardening settings |
| `SecurityTier` | security.go | Security level enum |
| `CellsConfig` | security.go | Top-level config file structure |
| `CredentialRefresher` | credentials.go | OAuth token sync service |
| `ContainerTracker` | tracking.go | Crash recovery tracking |
| `DevcontainerConfig` | devcontainer.go | devcontainer.json parser |
| `ValidationResult` | validate.go | Prerequisite check result |

### DockerClient Interface

```go
type DockerClient interface {
    // Lifecycle
    Ping(ctx) error
    Close() error

    // Container operations
    CreateContainer(ctx, cfg) (string, error)
    StartContainer(ctx, containerID) error
    StopContainer(ctx, containerID) error
    RemoveContainer(ctx, containerID) error
    PauseContainer(ctx, containerID) error
    UnpauseContainer(ctx, containerID) error
    GetContainerState(ctx, containerID) (string, error)
    IsContainerRunning(ctx, containerID) (bool, error)
    ExecInContainer(ctx, containerID, cmd) (string, error)
    SignalProcess(ctx, containerID, processName, signal) error

    // Container management
    ListDockerTUIContainers(ctx) ([]ContainerInfo, error)
    CleanupOrphanedContainers(ctx, projectName, knownIDs, worktrees) (int, error)

    // Image operations
    ImageExists(ctx, imageName) (bool, error)
}
```

### Security Tiers

```go
const (
    TierHardened SecurityTier = "hardened"  // Most restrictive
    TierModerate SecurityTier = "moderate"  // Default, balanced
    TierCompat   SecurityTier = "compat"    // Most compatible
)
```

### Key Functions

**container.go:**
- `NewContainerConfig()` - Create config from branch name
- `CreateContainer()` - Create container with mounts and security
- `CreateAndStartContainerWithFallback()` - Auto-relax security on failure
- `CleanupOrphanedContainers()` - Remove orphans after crash

**security.go:**
- `LoadSecurityConfig()` - Merge global + project configs
- `SaveProjectSecurityConfig()` - Persist auto-relaxation result
- `ConfigForTier()` - Get defaults for security tier

**credentials.go:**
- `GetClaudeCredentials()` - Read from macOS keychain
- `CredentialRefresher.Start()` - Begin background refresh loop
- `RegisterContainer()` - Add container to refresh list

**config.go:**
- `SetupContainerConfig()` - Create container config directory
- `GetContainerConfigDir()` - Get/create config path

**tracking.go:**
- `TrackContainer()` - Register container for crash recovery
- `GetOrphanedContainers()` - Find containers from crashed sessions

**validate.go:**
- `ValidatePrerequisites()` - Check Docker, image availability
- `BuildImage()` - Build default ccells image
- `BuildProjectImage()` - Build from devcontainer.json

**devcontainer.go:**
- `LoadDevcontainerConfig()` - Parse devcontainer.json
- `HasDevcontainerConfig()` - Check if project has devcontainer

---

## `internal/git/`

Git CLI wrapper for worktree and branch operations.

### Key Types

| Type | File | Description |
|------|------|-------------|
| `GitClient` | interface.go | Interface for git operations |
| `Git` | branch.go | Real git CLI implementation |
| `MockClient` | mock_client.go | Test mock |

### GitClient Interface

```go
type GitClient interface {
    // Branch operations
    CurrentBranch(ctx) (string, error)
    CreateBranch(ctx, name) error
    Checkout(ctx, branch) error
    DeleteBranch(ctx, name) error
    BranchExists(ctx, name) (bool, error)
    BranchHasCommits(ctx, branchName) (bool, error)
    ListCCellsBranches(ctx) ([]string, error)
    GetBaseBranch(ctx) (string, error)

    // Working directory
    HasUncommittedChanges(ctx) (bool, error)
    Stash(ctx) error
    StashPop(ctx) error

    // Remote operations
    Push(ctx, branch) error
    ForcePush(ctx, branch) error
    RemoteURL(ctx, remoteName) (string, error)

    // Merge/rebase
    MergeBranch(ctx, branch) error
    MergeBranchWithOptions(ctx, branch, squash) error
    RebaseBranch(ctx, branch) error

    // Worktrees
    CreateWorktree(ctx, path, branch) error
    RemoveWorktree(ctx, path) error
    WorktreeList(ctx) ([]string, error)

    // Repository
    RepoID(ctx) (string, error)
}
```

### Key Functions

**branch.go:**
- `New(repoPath)` - Create Git client for repo
- `runGit()` - Execute git command with context
- `CreateWorktree()` - Create isolated worktree
- `RepoID()` - Get first commit hash as stable ID

**pr.go:**
- `CreatePR()` - Create GitHub PR via `gh` CLI
- `GetPRForBranch()` - Check if PR exists

---

## `internal/sync/`

Mutagen-based file sync for pairing mode.

### Key Types

| Type | File | Description |
|------|------|-------------|
| `Pairing` | pairing.go | Pairing mode orchestrator |
| `PairingState` | pairing.go | Snapshot of pairing state |
| `GitOperations` | pairing.go | Git interface subset |
| `MutagenOperations` | pairing.go | Mutagen interface |
| `Mutagen` | mutagen.go | Real mutagen CLI wrapper |

### Key Functions

**pairing.go:**
- `Enable()` - Start pairing (stash, checkout, sync)
- `Disable()` - Stop pairing (terminate sync, restore branch)
- `CheckSyncHealth()` - Verify sync session, detect conflicts

**mutagen.go:**
- `CreateSession()` - Start mutagen sync
- `TerminateSession()` - Stop mutagen sync
- `GetConflicts()` - List sync conflicts

---

## `internal/claude/`

Claude CLI wrapper for ephemeral queries.

### Key Types

| Type | File | Description |
|------|------|-------------|
| `QueryOptions` | query.go | Query configuration |
| `CommandExecutor` | query.go | Abstraction for command execution |

### Key Functions

**query.go:**
- `Query()` - Execute ephemeral Claude query
- `QueryWithTimeout()` - Convenience wrapper with timeout
- Uses `--no-session-persistence` to avoid polluting resume log
- Thread-safe via mutex

---

## `internal/config/`

Global application configuration.

### Key Types

| Type | File | Description |
|------|------|-------------|
| `GlobalConfig` | config.go | Global ccells config |

### Key Functions

**config.go:**
- `Load()` - Load config from disk
- `Save()` - Persist config
- `IsFirstRun()` - Check if intro should be shown
- `MarkIntroductionShown()` - Update first-run flag

---

## `configs/`

Embedded static files.

### Key Variables

| Variable | File | Description |
|----------|------|-------------|
| `BaseDockerfile` | configs.go | Embedded base.Dockerfile content |

### Key Functions

**configs.go:**
- `BaseDockerfileHash()` - 12-char content hash for image tagging

---

## Dependencies Between Packages

```
cmd/ccells/main.go
    |
    +-> internal/tui (main app)
    |     |
    |     +-> internal/workstream (state management)
    |     +-> internal/docker (container ops)
    |     +-> internal/git (git ops)
    |     +-> internal/sync (pairing)
    |     +-> internal/config (global config)
    |
    +-> internal/docker (validation, tracking)
    +-> internal/git (repo ID)
    +-> internal/workstream (state loading)

internal/docker
    +-> configs (embedded Dockerfile)
    +-> (Docker SDK)

internal/workstream
    +-> internal/docker (session ID extraction)

internal/sync
    +-> internal/git (GitOperations interface)
```

## Coupling Analysis

### Strong Boundaries ✅

1. **Docker Abstraction** - `DockerClient` interface hides SDK types from TUI
2. **Git Abstraction** - `GitClient` interface with domain-specific errors
3. **Workstream Domain** - Clean separation between `Workstream` (runtime) and `SavedWorkstream` (persistence)

### Leaky Boundaries ⚠️

1. **TUI → Docker Coupling** (`container.go`)
   - 1653 lines of orchestration logic inside TUI package
   - Returns `tea.Msg` types, tightly coupling to Bubble Tea

2. **Global State in TUI**
   ```go
   var program *tea.Program       // pty.go
   var containerTracker *...      // container.go
   var credentialRefresher *...   // container.go
   ```

3. **PaneModel God Object** - Mixes rendering, PTY, scrolling, animations

### No Circular Dependencies ✅

Go's package system prevents import cycles. Well done!

## Essential Reading Order

1. `cmd/ccells/main.go` - Startup flow
2. `internal/tui/app.go` - Main event loop
3. `internal/workstream/workstream.go` - Core domain model
4. `internal/workstream/persistent_manager.go` - Auto-saving state
5. `internal/docker/interface.go` - Docker abstraction
6. `internal/git/interface.go` - Git abstraction
7. `internal/tui/container.go` - Container orchestration (⚠️ business logic in TUI)
