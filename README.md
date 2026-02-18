# Claude Cells

**Claude Code, behind bars.** Each instance runs in its own isolated cell—no conflicts, no permission prompts, no babysitting.

<p align="center">
  <img src="Master-half.gif" alt="Claude Cells running parallel workstreams" width="100%">
</p>

> Other multi-agent tools wrap AI behind heavy custom UIs. We don't wrap Claude Code—**we put it in a cell**.

Claude Cells is a tmux-based orchestrator for AI-assisted development. Spin up Claude Code instances, each working on a different feature in complete isolation, and watch them all progress simultaneously.

## Why Claude Cells?

- **Total Isolation**: Each Claude runs in its own Docker container with its own git worktree. No `.git/index.lock` conflicts. No stepping on each other's toes.
- **Zero Permission Prompts**: Isolation means safety. Let Claude edit files, run commands, and iterate freely—it can't escape its cell.
- **Pure Claude Code**: No custom UI layer. You get Claude Code's native terminal, running directly in tmux panes.
- **Resumable**: Detach and reattach anytime. tmux keeps everything alive—containers, sessions, and state survive disconnects.
- **Set It and Forget It**: Spin up a workstream, move on. Come back when it's done—containers pause and resume exactly where you left off.
- **Devcontainer Support**: Already have a `.devcontainer/devcontainer.json`? It just works.
- **Your Hardware is the Limit**: Run as many parallel workstreams as your machine can handle.
- **Real-time Pairing**: Sync your local filesystem with any container for live editing alongside Claude.

<p align="center">
  <img src="cc-3-pane.png" alt="Claude Cells grid layout with 3 panes" width="100%">
</p>

## Features

### Core Capabilities

| Feature | Description |
|---------|-------------|
| **Parallel Workstreams** | Run multiple Claude Code sessions simultaneously, each working on different tasks |
| **Isolated Containers** | Each workstream runs in its own Docker container, preventing interference |
| **Automatic Branch Management** | Each workstream gets its own git branch, automatically named from your prompt |
| **Git Worktree Isolation** | Host repo stays untouched - each container uses its own worktree |
| **Session Persistence** | Quit and resume later - containers are paused and state is saved |
| **Push & PR** | Push branches and create pull requests from within workstreams |
| **Pairing Mode** | Sync your local filesystem with a container using Mutagen for real-time collaboration |

### tmux-Native Interface

Claude Cells runs as a tmux session. Each workstream is a pane running `docker exec` directly—no terminal emulation layer, no custom rendering. You interact with Claude Code natively.

Pane borders show workstream status (branch, state, PR info). The status line shows global info and keybinding hints.

## Prerequisites

- **[tmux](https://github.com/tmux/tmux)** - Terminal multiplexer (2.3+ required, 3.4+ recommended for multi-line status)
- **Docker runtime** - We recommend [OrbStack](https://orbstack.dev/) on macOS, or Docker Engine on Linux
- **Go 1.21+** - For building from source
- **[Mutagen](https://mutagen.io/)** - For pairing mode (optional)
- **[gh CLI](https://cli.github.com/)** - For PR creation (optional)
- **Claude Code** - Installed and authenticated on your system

## Installation

### From Source

```bash
git clone https://github.com/STRML/claude-cells.git
cd claude-cells
go build ./cmd/ccells
```

### Using Go Install

```bash
go install github.com/STRML/claude-cells/cmd/ccells@latest
```

## Quick Start

1. **Run ccells** from your project directory (the Docker image is built automatically on first run):
   ```bash
   ccells
   ```
   This starts a tmux session with a background daemon.

2. **Create a workstream**:
   ```bash
   # Interactive (via keybinding or popup):
   # Press prefix + n inside the tmux session

   # Or from the command line:
   ccells create --branch add-auth --prompt "add user authentication with JWT tokens"
   ```

3. **Watch Claude work** - the workstream will automatically:
   - Create a branch named `add-auth`
   - Start a Docker container with your project mounted
   - Launch Claude Code with your prompt in a tmux pane

4. **Create more workstreams** - press `prefix + n` again or run `ccells create`

5. **Detach and reattach** - press `prefix + d` to detach. Run `ccells` again to reattach.

## CLI Commands

Claude Cells uses compose-style CLI verbs:

| Command | Description |
|---------|-------------|
| `ccells` | Start session (`up`) or reattach (`attach`) |
| `ccells up` | Create tmux session + start daemon |
| `ccells attach` | Reattach to existing session |
| `ccells down` | Stop daemon + tmux server (containers survive) |
| `ccells create` | Create a new workstream |
| `ccells rm <name>` | Destroy a workstream |
| `ccells pause <name>` | Pause a workstream's container |
| `ccells unpause <name>` | Resume a paused workstream |
| `ccells ps` | List all workstreams and their status |
| `ccells pair <name>` | Start pairing mode for a workstream |
| `ccells unpair` | Stop pairing mode |
| `ccells logs` | View logs |
| `ccells --version` | Show version info |

## Keybindings

All keybindings use your tmux prefix (default: `Ctrl+B`).

| Key | Action |
|-----|--------|
| `prefix + n` | New workstream (interactive popup) |
| `prefix + d` | Destroy workstream (interactive popup) |
| `prefix + p` | Pause current workstream |
| `prefix + r` | Resume/unpause current workstream |
| `prefix + m` | Merge/PR menu |
| `prefix + f` | Toggle pairing mode |
| `prefix + s` | Refresh status |
| `prefix + ?` | Show help popup |

Standard tmux navigation also works:

| Key | Action |
|-----|--------|
| `prefix + ←`/`→`/`↑`/`↓` | Switch between panes |
| `prefix + z` | Zoom/unzoom current pane |
| `prefix + d` | Detach from session |

## How It Works

Each workstream gets its own git worktree and Docker container. Your host repo stays untouched—no branch switching, no lock conflicts. When you create a workstream, Claude Cells generates a branch name, creates the worktree, and launches Claude Code inside a tmux pane via `docker exec`.

A background daemon handles credential refresh, state reconciliation, and pairing health monitoring. The daemon communicates with CLI commands via a Unix domain socket.

### Pairing Mode

Enable bidirectional file sync between your local filesystem and a container via [Mutagen](https://mutagen.io/). Edit locally while Claude works in the container.

```bash
# Install Mutagen
brew install mutagen-io/mutagen/mutagen  # macOS

# Enable pairing
ccells pair <workstream-name>
```

### Session Persistence

Detach with `prefix + d`—tmux keeps everything running. Run `ccells` to reattach. Use `ccells down` to stop the daemon (containers survive and can be resumed later).

### Container Security

Containers run with hardened security defaults (capability drops, no-new-privileges, process limits). If a container fails to start, settings auto-relax to find a working configuration.

See **[docs/CONTAINER-SECURITY.md](docs/CONTAINER-SECURITY.md)** for configuration options and security tiers.

### Runtime Selection

Choose between standard Claude Code or experimental builds with feature flags enabled.

**Standard Claude Code** (default):
```bash
ccells  # Uses standard Claude Code runtime
```

**Claude Sneakpeek** (experimental with swarm mode):
```bash
ccells --runtime claudesp  # Enables swarm mode, delegate mode, team coordination
```

**Configuration file** (set default runtime):
```yaml
# ~/.claude-cells/config.yaml (global default)
runtime: claudesp

# .claude-cells/config.yaml (project-specific)
runtime: claudesp
```

**Priority**: CLI flag > project config > global config > default ("claude")

Each runtime:
- Has its own isolated config directory (`.claude` vs `.claude-sneakpeek`)
- Persists across session restarts (runtime saved in state)
- Applies to all workstreams in the session

See [claude-sneakpeek](https://github.com/mikekelly/claude-sneakpeek) for experimental features.

## Architecture

```
claude-cells/
├── cmd/ccells/                # CLI entry point + subcommands
│   ├── main.go                # Startup, arg parsing, command dispatch
│   ├── commands.go            # Command registry + flag parsing
│   ├── cmd_up.go              # up: create tmux session + daemon
│   ├── cmd_attach.go          # attach: reattach to session
│   ├── cmd_down.go            # down: stop daemon + tmux
│   ├── cmd_create.go          # create: new workstream via daemon
│   ├── cmd_rm.go              # rm: destroy workstream
│   ├── cmd_pause.go           # pause/unpause workstreams
│   ├── cmd_ps.go              # ps: list workstreams
│   ├── cmd_pair.go            # pair/unpair: pairing mode
│   ├── dialog_create.go       # Interactive create popup (Bubble Tea)
│   ├── dialog_merge.go        # Interactive merge popup
│   └── dialog_rm.go           # Interactive destroy popup
├── configs/base.Dockerfile    # Base Docker image
└── internal/
    ├── tmux/                  # tmux server + pane management
    │   ├── tmux.go            # Server lifecycle, session mgmt
    │   ├── pane.go            # Pane CRUD + metadata
    │   └── chrome.go          # Status line, pane borders, keybindings
    ├── daemon/                # Background daemon
    │   ├── daemon.go          # Socket server, reconciliation, pairing
    │   ├── api.go             # Request/Response protocol
    │   └── reconcile.go       # State reconciliation (tmux + Docker)
    ├── orchestrator/          # Workstream lifecycle orchestration
    ├── docker/                # Docker SDK wrapper
    ├── git/                   # Git operations (branches, worktrees, PRs)
    ├── gitproxy/              # Git proxy for container operations
    ├── sync/                  # Mutagen pairing mode
    ├── workstream/            # Workstream state & lifecycle
    └── claude/                # Claude CLI wrapper
```

### Key Technologies

- **[tmux](https://github.com/tmux/tmux)** - Terminal multiplexer (pane management, session persistence)
- **[Docker SDK](https://pkg.go.dev/github.com/docker/docker/client)** - Container management
- **[Bubble Tea](https://github.com/charmbracelet/bubbletea)** - Interactive dialog popups
- **[Mutagen](https://mutagen.io/)** - File synchronization

## Configuration

Claude Cells stores data in:

| Location | Purpose |
|----------|---------|
| `~/.claude-cells/config.yaml` | Global configuration (security, Dockerfile customization, runtime) |
| `~/.claude-cells/state/<repo-id>/state.json` | Workstream metadata (prompts, session IDs, PR info) |
| `~/.claude-cells/state/<repo-id>/daemon.sock` | Daemon Unix socket for CLI communication |
| `/tmp/ccells/worktrees/` | Git worktrees for container isolation |

### Dockerfile Customization

Customize what gets installed in container images via `~/.claude-cells/config.yaml`:

```yaml
dockerfile:
  inject:
    - "apt-get update && apt-get install -y vim"
    - "pip install ipython"
```

- Project-specific `.claude-cells/config.yaml` with `dockerfile.inject` replaces (not merges with) the global inject list
- Changing injections triggers an automatic image rebuild

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Container fails to start | Rebuild image: `docker build -t ccells-base -f configs/base.Dockerfile .` |
| Claude Code not responding | Run `ccells logs`; check Docker resources if startup times out |
| Can't reattach | Run `ccells down` then `ccells up` to reset the session |
| Daemon not running | `ccells up` auto-starts daemon; check `ccells ps` for status |
| Pairing mode not working | Ensure Mutagen is installed: `mutagen version` |
| Sync conflicts | Check `mutagen sync list`, resolve in local editor |

## Limitations

- Requires tmux 2.3+ (multi-line status needs 3.4+)
- Requires a Docker runtime (we recommend [OrbStack](https://orbstack.dev/) on macOS)
- Pairing mode requires Mutagen
- PR creation requires `gh` CLI authenticated

## License

MIT

## Commit Workflow (ccells containers)

**ALWAYS use `/ccells-commit` when committing inside ccells containers.** This skill:
- Handles CLAUDE.md updates automatically
- Runs pre-commit verification
- Provides proper commit message formatting

Never use raw `git commit` commands in ccells containers—a hook will block them.

See **[CLAUDE.md](CLAUDE.md#committing-ccells-containers)** for full development guidelines.

## Contributing

Contributions welcome! Please:

1. Run tests: `go test -race ./...`
2. Run linter: `go vet ./...`
3. Format code: `gofmt -s -w .`
4. Ensure tests cover your changes

See **[docs/TESTING.md](docs/TESTING.md)** for the full testing guide, including integration tests and manual verification procedures.
