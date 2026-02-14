# Claude Cells

A tmux-based orchestrator for running parallel Claude Code instances in isolated Docker containers.

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
cmd/ccells/                # CLI entry point + subcommands
  main.go                  # Startup, arg parsing, command dispatch
  commands.go              # Command registry + flag parsing
  cmd_up.go                # up: create tmux session + daemon
  cmd_attach.go            # attach: reattach to session
  cmd_down.go              # down: stop daemon + tmux
  cmd_create.go            # create: new workstream via daemon
  cmd_rm.go                # rm: destroy workstream
  cmd_pause.go             # pause/unpause workstreams
  cmd_ps.go                # ps: list workstreams
  cmd_pair.go              # pair/unpair: pairing mode
  dialog_create.go         # Interactive create popup (Bubble Tea)
  dialog_merge.go          # Interactive merge/PR popup (create PR, merge, view)
  dialog_rm.go             # Interactive destroy popup
  dialog_welcome.go        # First-run welcome screen with shimmer animation
  handlers.go              # Action handlers wiring orchestrator + tmux
  detach.go                # Detach summary display
  runtime.go               # Runtime selection (claude/claudesp)
internal/
  tmux/                    # tmux server + pane management
    tmux.go                # Server lifecycle, session creation/destruction
    pane.go                # Pane CRUD + metadata (workstream/container vars)
    chrome.go              # Status line, pane borders, keybindings, help
  daemon/                  # Background daemon (sidecar process)
    daemon.go              # Socket server, reconciliation loop, pairing health
    api.go                 # Request/Response JSON protocol
    reconcile.go           # State reconciliation (tmux + Docker)
  orchestrator/            # Workstream lifecycle orchestration
    orchestrator.go        # WorkstreamOrchestrator interface & implementation
    create.go              # CreateWorkstream - worktree + container creation
    lifecycle.go           # Pause, Resume, Destroy, Rebuild operations
  workstream/              # Workstream state & lifecycle
    manager.go             # In-memory workstream tracking
    persistent_manager.go  # Auto-persisting state manager (wraps Manager)
    state.go               # State file read/write
  docker/                  # Docker SDK wrapper
  sync/                    # Mutagen/pairing mode
  git/                     # Branch, worktree & PR operations
  gitproxy/                # Git proxy for container operations
  claude/                  # Claude CLI wrapper for ephemeral queries
```

### Codemaps

**Always read codemaps when attempting to understand project structure.**

The `codemaps/` directory contains detailed architecture documentation:
- `codemaps/architecture.md` - Overall design, entry points, data flow, key patterns
- `codemaps/backend.md` - Package structure, dependencies, internal APIs
- `codemaps/data.md` - Data models, state formats, config schemas

Before modifying code or answering questions about how the codebase works, read the relevant codemap(s) first.

### Data Directories

- `~/.claude-cells/state/<repo-id>/state.json` - Per-repo workstream metadata (prompts, session IDs, PR info)
- `~/.claude-cells/state/<repo-id>/daemon.sock` - Daemon Unix socket for CLI communication
- `~/.claude-cells/containers/<container-name>/` - Container-specific config files (credentials, .claude.json copies)
- `~/.claude-cells/logs/` - Exported logs (pane logs and system logs)

### OAuth Credential Management

Each container gets isolated credentials copied from the host's macOS keychain. The flow:

1. **On container creation**: Fresh credentials copied from keychain to `~/.claude-cells/containers/<name>/.credentials.json`
2. **Container environment**: `CLAUDE_CONFIG_DIR=/home/claude` tells Claude Code where to find config
3. **Mounted files**: `.credentials.json` mounted at `/home/claude/.credentials.json` (the path Claude Code expects with `CLAUDE_CONFIG_DIR`)
4. **Periodic refresh**: `CredentialRefresher` polls keychain every 15 min, pushes fresh tokens to all container configs
5. **On ccells restart**: Existing container configs are re-registered with the refresher

**Why keychain is source of truth**: Containers may be paused for extended periods while ccells is off. Their tokens expire. When ccells restarts, it must push fresh credentials from keychain to revive paused containers. Sub-claudes can also refresh their own tokens (writing to the mounted file), but the keychain refresh ensures containers work after long pauses.

**Isolation**: Each container has its own config directory - they don't share or modify the host's `~/.claude` config.

See: [claude-code#1736](https://github.com/anthropics/claude-code/issues/1736) for Docker credential patterns.

### Git Worktree Isolation

Each container gets its own **git worktree** at `/tmp/ccells/worktrees/<branch-name>`. This keeps the host repository's working directory untouched while still sharing git objects. Benefits:

- Host repo stays on its current branch (never changes)
- No `.git/index.lock` conflicts
- Each container has isolated working directory
- Changes are still part of the main repo's git history (can push/PR)
- Worktrees are cleaned up when containers are destroyed

### Container Security

Containers run with hardened security defaults. See [docs/CONTAINER-SECURITY.md](docs/CONTAINER-SECURITY.md) for full details.

**Key features:**
- **Security tiers**: `hardened` > `moderate` (default) > `compat`
- **Auto-relaxation**: If containers fail to start, automatically tries less restrictive settings
- **Config files**: `~/.claude-cells/config.yaml` (global), `.claude-cells/config.yaml` (project)

**Default security settings (moderate tier):**
- `no-new-privileges: true` - Blocks setuid privilege escalation
- `init: true` - Proper signal handling via init process
- `pids_limit: 1024` - Prevents fork bombs
- `cap_drop: [SYS_ADMIN, SYS_MODULE]` - Blocks container escape vectors

To relax settings for a project, create `.claude-cells/config.yaml`:
```yaml
security:
  tier: compat  # Or add specific capabilities with cap_add
```

### Dockerfile Customization

Custom packages can be injected into container images via `~/.claude-cells/config.yaml`:

```yaml
dockerfile:
  inject:
    - "apt-get update && apt-get install -y vim"
    - "pip install ipython"
```

- **Global config**: `~/.claude-cells/config.yaml` applies to all projects
- **Project config**: `.claude-cells/config.yaml` overrides global (entire inject list replaced, not merged)
- **Default**: If no config exists, no packages are injected (base image only)
- **Rebuild trigger**: Changing injections triggers an automatic image rebuild

Injected commands run as `RUN` instructions after Claude Code installation but before the container's `WORKDIR` is set.

### Runtime Selection

Claude Cells supports two Claude Code runtimes:

**Standard Claude Code (default: `claude`)**
- Stable, production-ready runtime
- Recommended for most use cases

**Claude Sneakpeek (experimental: `claudesp`)**
- Experimental build with advanced features
- Swarm Mode: Multi-agent orchestration via TeammateTool
- Delegate Mode: Spawn background agents for parallel tasks
- Team Coordination: Teammate messaging and task ownership
- See [claude-sneakpeek](https://github.com/mikekelly/claude-sneakpeek)

**CLI Usage:**
```bash
ccells --runtime claudesp  # Use experimental runtime
ccells --runtime claude    # Use standard runtime (default)
ccells                     # Use default runtime
```

**Configuration Files:**

Global config (`~/.claude-cells/config.yaml`):
```yaml
runtime: claudesp  # Sets default for all projects
```

Project config (`.claude-cells/config.yaml`):
```yaml
runtime: claudesp  # Sets runtime for this project only
```

**Priority Chain:** CLI flag > project config > global config > default (`claude`)

**Runtime Isolation:**
- Each runtime has its own isolated config directory (`.claude` vs `.claude-sneakpeek`)
- Runtime selection persists across session restarts
- Runtime applies to all workstreams in the session
- Changing runtime requires restarting ccells

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

**tmux/Daemon Testing**

The tmux package wraps shell commands and can be tested by examining command construction. The daemon uses Unix sockets and can be tested with in-process clients:
```go
// Daemon test pattern
d := daemon.New(daemon.Config{SocketPath: socketPath})
go d.Run(ctx)
// Send requests via daemon client
conn, _ := net.Dial("unix", socketPath)
json.NewEncoder(conn).Encode(daemon.Request{Action: "ping"})
```

### Completed Technical Improvements

- **tmux-Based Architecture**: Replaced Bubble Tea TUI (~24,500 lines) with tmux sidecar model (~2000 lines). Docker exec runs directly in tmux panes—no terminal emulation layer.
- **Background Daemon**: Unix socket daemon handles credential refresh, state reconciliation, and pairing health. Survives tmux detach/reattach cycles.
- **Compose-Style CLI**: `up`/`down`/`create`/`rm`/`ps`/`pause`/`unpause`/`pair` commands. All support non-interactive mode for scripting.
- **Interactive Dialogs**: Bubble Tea programs run in `tmux display-popup` for create/merge/destroy flows.
- **State Reconciliation**: Cross-references tmux panes with Docker containers to detect orphans and stale state.
- **Git Worktree Isolation**: Each container gets its own git worktree, avoiding host repo conflicts.
- **Shell Escaping**: `EscapeShellArg()` handles newlines (`\n`, `\r`), null bytes, and other special characters.
- **Resource Limits**: Manager limits workstreams to 12 (MaxWorkstreams constant).
- **Auto-Persisting State**: `PersistentManager` auto-saves on any mutation. 200ms debounced saves prevent disk thrashing.
- **Atomic State Writes**: `SaveState` writes to temp file then renames for crash safety.
- **Container Cleanup**: Orphaned containers from crashed sessions are cleaned up on startup.
- **Context Timeouts**: All Docker operations use timeouts (no unbounded context.Background()).
- **Session Persistence**: Claude sessions are persisted from container runtime location to mount point before pause, surviving container rebuilds.
- **OAuth Credential Refresh**: `CredentialRefresher` re-registers existing containers on startup, ensuring credentials stay fresh even after ccells restarts.
- **Container Security Hardening**: Tiered security defaults (hardened/moderate/compat) with auto-relaxation on startup failure.
- **PR Generation via Claude CLI**: Uses Claude (haiku model) to generate PR titles and descriptions from branch diffs.
- **Native Claude Code Installer**: Uses `claude.ai/install.sh` instead of npm. Configurable injection via `~/.claude-cells/config.yaml`.
- **Full PR Workflow**: Merge dialog supports push + create PR (Claude-generated), merge with method selection (squash/merge/rebase), and view PR URL.
- **State Reconciliation Wired**: Daemon cross-references tmux panes with Docker containers every 30s, killing orphaned panes and logging orphaned containers.
- **Pane Death Handling**: `pane-died` hook respawns last pane with create dialog; kills extra dead panes.
- **Welcome Screen**: Centered display with animated pearlescent shimmer title, keybinding reference, chains to create dialog.

### Remaining Technical Debt

1. **Full Context Propagation**: Root context from main should flow through daemon to all operations for clean shutdown.

### Git Proxy (Container Git Operations)

Containers can now use `git` and `gh` commands for remote operations via a secure proxy:

- **Supported operations**: `git fetch`, `git pull`, `git push`, `gh pr create`, `gh pr view`, `gh pr merge`
- **Branch restriction**: Containers can only push to their assigned branch (prevents cross-contamination)
- **PR restriction**: `gh pr merge` only works on the container's own PR
- **Automatic status refresh**: PR status updates after pushes and polls every 5 minutes

The proxy is transparent - Claude uses normal git/gh commands, which are intercepted by hooks and proxied through the host. See `internal/gitproxy/` for implementation details.

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

- [x] Never interpolate user input into shell commands without escaping (see `EscapeShellArg()`)
- [x] Credentials should be read-only in containers where possible (mounted as ReadOnly)
- [x] Container security hardening with capability drops and no-new-privileges
- [x] Auto-relaxation with config persistence for compatibility
- [x] Validate all branch names before using in paths/commands (see `validateBranchName()`)

### Committing (ccells containers)

**ALWAYS use `/ccells-commit` when committing inside ccells containers.** This skill:
- Handles CLAUDE.md updates automatically
- Runs pre-commit verification
- Provides proper commit message formatting

Never use raw `git commit` commands in ccells containers - a hook will block them.

### Before Committing

1. `go test -race ./...` - no races
2. `go vet ./...` - no issues
3. `gofmt -s -w .` - formatted
4. Tests cover the change
5. No `TODO` without an issue number
6. **Update README.md and CLAUDE.md** for user-facing changes (new features, config options, CLI flags)
