# Data Models and Schemas

Last updated: 2026-02-12 (Updated: removed TUI models, added daemon types)

## Overview

This document describes the key data structures used in Claude Cells, including persisted state, runtime models, and configuration types.

---

## Persisted State

### Application State (`state.json`)

Location: `~/.claude-cells/state/<repo-id>/state.json`

```json
{
  "version": 1,
  "repo": {
    "name": "my-project",
    "path": "/Users/me/projects/my-project",
    "remote": "git@github.com:me/my-project.git",
    "repo_id": "abc123def456",
    "created_at": "2024-01-15T10:30:00Z"
  },
  "workstreams": [
    {
      "id": "1705312200000000000-1",
      "branch_name": "ccells/add-user-auth",
      "prompt": "Add user authentication with OAuth",
      "title": "Add OAuth authentication",
      "container_id": "a1b2c3d4e5f6...",
      "claude_session_id": "01HQXYZ...",
      "was_interrupted": false,
      "has_been_pushed": true,
      "pr_number": 42,
      "pr_url": "https://github.com/me/my-project/pull/42",
      "created_at": "2024-01-15T10:30:00Z"
    }
  ],
  "focused_index": 0,
  "layout": 0,
  "saved_at": "2024-01-15T12:45:00Z"
}
```

### Go Types

```go
// state.go
type AppState struct {
    Version      int               `json:"version"`
    Repo         *RepoInfo         `json:"repo,omitempty"`
    Workstreams  []SavedWorkstream `json:"workstreams"`
    FocusedIndex int               `json:"focused_index"`
    Layout       int               `json:"layout"`
    SavedAt      time.Time         `json:"saved_at"`
}

type RepoInfo struct {
    Name      string    `json:"name"`
    Path      string    `json:"path"`
    Remote    string    `json:"remote,omitempty"`
    RepoID    string    `json:"repo_id"`
    CreatedAt time.Time `json:"created_at,omitempty"`
}

type SavedWorkstream struct {
    ID              string    `json:"id"`
    BranchName      string    `json:"branch_name"`
    Prompt          string    `json:"prompt"`
    Title           string    `json:"title,omitempty"`
    ContainerID     string    `json:"container_id"`
    ClaudeSessionID string    `json:"claude_session_id,omitempty"`
    WasInterrupted  bool      `json:"was_interrupted,omitempty"`
    HasBeenPushed   bool      `json:"has_been_pushed,omitempty"`
    PRNumber        int       `json:"pr_number,omitempty"`
    PRURL           string    `json:"pr_url,omitempty"`
    CreatedAt       time.Time `json:"created_at"`
}
```

---

## Configuration Files

### Security Config (`config.yaml`)

Locations:
- Global: `~/.claude-cells/config.yaml`
- Project: `<project>/.claude-cells/config.yaml`

```yaml
security:
  tier: moderate           # hardened | moderate | compat
  no_new_privileges: true
  init: true
  pids_limit: 1024
  cap_drop:
    - SYS_ADMIN
    - SYS_MODULE
  cap_add: []
  privileged: false
  host_network: false
  host_pid: false
  host_ipc: false
  docker_socket: false
  auto_relax: true
```

### Go Types

```go
// security.go
type SecurityTier string

const (
    TierHardened SecurityTier = "hardened"
    TierModerate SecurityTier = "moderate"
    TierCompat   SecurityTier = "compat"
)

type CellsConfig struct {
    Security SecurityConfig `yaml:"security,omitempty"`
}

type SecurityConfig struct {
    Tier            SecurityTier `yaml:"tier,omitempty"`
    NoNewPrivileges *bool        `yaml:"no_new_privileges,omitempty"`
    Init            *bool        `yaml:"init,omitempty"`
    PidsLimit       *int64       `yaml:"pids_limit,omitempty"`
    CapDrop         []string     `yaml:"cap_drop,omitempty"`
    CapAdd          []string     `yaml:"cap_add,omitempty"`
    Privileged      *bool        `yaml:"privileged,omitempty"`
    HostNetwork     *bool        `yaml:"host_network,omitempty"`
    HostPID         *bool        `yaml:"host_pid,omitempty"`
    HostIPC         *bool        `yaml:"host_ipc,omitempty"`
    DockerSocket    *bool        `yaml:"docker_socket,omitempty"`
    AutoRelax       *bool        `yaml:"auto_relax,omitempty"`
}
```

### Application State (`app-state.json`)

Location: `~/.claude-cells/app-state.json`

Internal state file (not user-editable). Automatically migrates from legacy `config.json` if present.

```json
{
  "version": 1,
  "introduction_shown": true
}
```

### Go Type

```go
// config/config.go
type GlobalConfig struct {
    Version           int  `json:"version"`
    IntroductionShown bool `json:"introduction_shown"`
}
```

---

## Orchestrator Types [NEW - PR #8]

### Orchestration Options and Results

```go
// orchestrator/orchestrator.go

// CreateOptions configures workstream creation.
type CreateOptions struct {
    RepoPath          string   // Repository path
    CopyUntracked     bool     // Copy untracked files to worktree
    UntrackedFiles    []string // List of untracked files to copy
    ImageName         string   // Empty = auto-detect from devcontainer or default
    IsResume          bool     // Resuming existing session (use --continue)
    UseExistingBranch bool     // Use existing branch without creating new one
    UpdateMain        bool     // Auto-pull main before creating branch
}

// CreateResult contains the result of workstream creation.
type CreateResult struct {
    ContainerID   string // Docker container ID
    ContainerName string // Container name (ccells-<branch>)
    ConfigDir     string // Container config directory for credential registration
    WorktreePath  string // Git worktree path
}

// DestroyOptions configures workstream destruction.
type DestroyOptions struct {
    DeleteBranch bool // Delete the git branch
    KeepWorktree bool // Keep worktree (for rebuild)
    Force        bool // Force even if container is running
}

// BranchConflict contains information about an existing branch conflict.
type BranchConflict struct {
    BranchName   string // The conflicting branch name
    HasWorktree  bool   // Whether an active worktree exists
    WorktreePath string // Path to existing worktree (if any)
    BranchInfo   string // Commit info for the branch
}
```

---

## Runtime Models

### Workstream

```go
// workstream/workstream.go
type State string

const (
    StateStarting State = "starting"
    StateRunning  State = "running"
    StateIdle     State = "idle"
    StatePairing  State = "pairing"
    StateStopped  State = "stopped"
    StateError    State = "error"
)

type Workstream struct {
    mu sync.RWMutex

    // Identity
    ID         string
    Prompt     string
    BranchName string
    Title      string

    // Docker
    ContainerID string

    // Git worktree
    WorktreePath string

    // Claude Code session
    ClaudeSessionID string

    // State
    State        State
    ErrorMessage string
    CreatedAt    time.Time
    LastActivity time.Time

    // PR info
    PRNumber int
    PRURL    string

    // Push tracking
    HasBeenPushed bool

    // Auto-continue
    WasInterrupted bool
}
```

### Container Configuration

```go
// docker/container.go
type ContainerConfig struct {
    Name        string
    Image       string
    RepoPath    string            // Worktree path
    HostGitDir  string            // Main repo .git
    ClaudeCfg   string            // ~/.claude mount source
    ClaudeJSON  string            // ~/.claude.json mount source
    GitConfig   string            // ~/.gitconfig mount source
    GitIdentity *GitIdentity
    Credentials string            // OAuth credentials path
    Timezone    string
    ExtraEnv    map[string]string // From devcontainer.json
    ExtraMounts []mount.Mount     // From devcontainer.json
    CPULimit    float64           // Default: 2.0
    MemoryLimit int64             // Default: 4GB
    Security    *SecurityConfig
}

type GitIdentity struct {
    Name  string
    Email string
}

type ContainerInfo struct {
    ID      string
    Name    string
    State   string
    Created time.Time
}
```

### Pairing State

```go
// sync/pairing.go
type PairingState struct {
    Active         bool
    CurrentBranch  string
    PreviousBranch string
    ContainerID    string
    StashedChanges bool
    SyncHealthy    bool
    Conflicts      []string
}
```

---

## tmux Integration Types

### Pane Information

```go
// tmux/pane.go
type PaneInfo struct {
    PaneID     string // tmux pane ID (e.g., "%0", "%1")
    Workstream string // from @ccells-workstream pane option
    Container  string // from @ccells-container pane option
}
```

### Status Line Data

```go
// tmux/chrome.go
type StatusWorkstream struct {
    Name      string
    Branch    string
    PRNumber  int
    HasPR     bool
    Running   bool
}
```

---

## Daemon Types

### Request/Response Protocol

```go
// daemon/api.go
type Request struct {
    Action string          `json:"action"`
    Params json.RawMessage `json:"params,omitempty"`
}

type Response struct {
    OK    bool            `json:"ok"`
    Error string          `json:"error,omitempty"`
    Data  json.RawMessage `json:"data,omitempty"`
}
```

### Reconciliation Types

```go
// daemon/reconcile.go
type PaneState struct {
    PaneID     string
    Workstream string // from @ccells-workstream option
    Container  string // from @ccells-container option
}

type ContainerState struct {
    ID      string
    Name    string
    Running bool
    Labels  map[string]string
}

type ReconcileResult struct {
    Healthy            []HealthyWorkstream
    OrphanedContainers []ContainerState // container running, no matching pane
    OrphanedPanes      []PaneState      // pane exists, no matching container
}

type HealthyWorkstream struct {
    PaneID      string
    ContainerID string
    Workstream  string
    Running     bool
}
```

### Pairing State (Daemon View)

```go
// daemon/daemon.go
type PairingState struct {
    Active         bool     `json:"active"`
    CurrentBranch  string   `json:"current_branch"`
    PreviousBranch string   `json:"previous_branch"`
    ContainerID    string   `json:"container_id"`
    SyncHealthy    bool     `json:"sync_healthy"`
    Conflicts      []string `json:"conflicts,omitempty"`
    SyncStatusText string   `json:"sync_status_text"`
}

type PairingProvider interface {
    IsActive() bool
    Enable(ctx context.Context, branchName, containerID, localPath, previousBranch string) error
    Disable(ctx context.Context) error
    CheckSyncHealth(ctx context.Context) error
    GetState() PairingState
}
```

---

## CLI Dialog Types (Bubble Tea)

### Create Dialog

```go
// cmd/ccells/dialog_create.go
type createDialog struct {
    step     int    // 0=prompt, 1=branch, 2=confirm
    prompt   string
    branch   string
    cursor   int
    input    string
    err      error
    done     bool
    stateDir string
    runtime  string
}
```

### Merge Dialog

```go
// cmd/ccells/dialog_merge.go
type mergeDialog struct {
    step       int    // 0=generate, 1=edit title, 2=edit body, 3=confirm
    title      string
    body       string
    cursor     int
    input      string
    generating bool
    err        error
    done       bool
}
```

### Remove Dialog

```go
// cmd/ccells/dialog_rm.go
type rmDialog struct {
    step         int  // 0=confirm, 1=delete branch option
    deleteBranch bool
    confirmed    bool
    done         bool
}
```

---

## Docker Integration Types

### Credential Types

```go
// docker/credentials.go
type ClaudeCredentials struct {
    Raw string  // Raw JSON from keychain
}

// Internal structure (not directly exposed)
type credentialsJSON struct {
    ClaudeAiOauth *oauthData `json:"claudeAiOauth,omitempty"`
}

type oauthData struct {
    AccessToken  string `json:"accessToken"`
    RefreshToken string `json:"refreshToken"`
    ExpiresAt    int64  `json:"expiresAt"`
}
```

### Container Tracking

```go
// docker/tracking.go
type TrackedContainer struct {
    ID        string
    Name      string
    CreatedAt time.Time
}

type ContainerTracker struct {
    path      string
    mu        sync.Mutex
    tracked   map[string]TrackedContainer
    heartbeat time.Time
}
```

### Validation Result

```go
// docker/validate.go
type ValidationResult struct {
    DockerAvailable bool
    ImageExists     bool
    ImageName       string
    NeedsBuild      bool
    Errors          []ValidationError
}

type ValidationError struct {
    Check   string
    Message string
}
```

---

## Devcontainer Configuration

```go
// docker/devcontainer.go
type DevcontainerConfig struct {
    Name              string            `json:"name,omitempty"`
    Image             string            `json:"image,omitempty"`
    Build             *BuildConfig      `json:"build,omitempty"`
    ContainerEnv      map[string]string `json:"containerEnv,omitempty"`
    RemoteEnv         map[string]string `json:"remoteEnv,omitempty"`
    Mounts            []string          `json:"mounts,omitempty"`
    RunArgs           []string          `json:"runArgs,omitempty"`
    OverrideCommand   *bool             `json:"overrideCommand,omitempty"`
    PostCreateCommand interface{}       `json:"postCreateCommand,omitempty"`
    PostStartCommand  interface{}       `json:"postStartCommand,omitempty"`
}

type BuildConfig struct {
    Dockerfile string            `json:"dockerfile,omitempty"`
    Context    string            `json:"context,omitempty"`
    Args       map[string]string `json:"args,omitempty"`
}
```


---

## File Locations Summary

| Data | Location |
|------|----------|
| Workstream state | `~/.claude-cells/state/<repo-id>/state.json` |
| Daemon socket | `~/.claude-cells/state/<repo-id>/daemon.sock` |
| Instance lock | `~/.claude-cells/state/<repo-id>/.ccells.lock` |
| App state | `~/.claude-cells/app-state.json` |
| Security config (global) | `~/.claude-cells/config.yaml` |
| Security config (project) | `<project>/.claude-cells/config.yaml` |
| Container configs | `~/.claude-cells/containers/<name>/` |
| Container credentials | `~/.claude-cells/containers/<name>/.claude/.credentials.json` |
| Ephemeral query state | `~/.claude-cells/ephemeral-query-state.json` |
| Git worktrees | `/tmp/ccells/worktrees/<branch>/` |
| Container tracking | `~/.claude-cells/containers.json` |
| Heartbeat | `~/.claude-cells/heartbeat` |
| Exported logs | `~/.claude-cells/logs/` |
| tmux server socket | `/tmp/tmux-<uid>/ccells-<repo-id>` |
