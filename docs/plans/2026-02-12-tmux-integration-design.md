# tmux Integration Design

Replace the custom Bubble Tea TUI with tmux as the window manager. ccells becomes a sidecar orchestrator that manages a tmux server, sends commands to control panes, and handles dialogs via popups. Docker exec runs directly in tmux panes — no vt10x proxy, no PTY management code.

## Motivation

Three equal drivers:

1. **Session resumability** — detach/reattach via SSH, survive crashes, reconnect from anywhere
2. **Reduced complexity** — delete ~5000 lines of virtual terminal emulation, layout, and rendering code
3. **Better layout** — tmux handles splits, resizing, and scrollback natively

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│ tmux server (socket: ccells-<repo-id>)                   │
│                                                          │
│  ┌─────────────────┐  ┌─────────────────┐               │
│  │ Pane: workstream │  │ Pane: workstream │  ...         │
│  │ docker exec -it  │  │ docker exec -it  │              │
│  │ claude --resume  │  │ claude --resume  │              │
│  └─────────────────┘  └─────────────────┘               │
│                                                          │
│  [status line: workstream tabs | keyhints]               │
└──────────────────────────────────────────────────────────┘
         ↑ keybindings call ↓
┌──────────────────────────────────────────────────────────┐
│ ccells CLI (compose-style verbs)                         │
│  up / attach / down / create / rm / ps / pause / unpause │
└──────────────────────────────────────────────────────────┘
         ↕
┌──────────────────────────────────────────────────────────┐
│ ccells daemon (background)                               │
│  - Credential refresh (every 15 min)                     │
│  - State reconciliation (tmux + Docker → state file)     │
│  - Git worktree management                               │
│  - Docker container lifecycle                            │
└──────────────────────────────────────────────────────────┘
```

### Key Decisions

- **tmux-native UI**: popups for dialogs, pane borders for workstream identity, status line for keyhints. No custom rendering.
- **Docker exec in panes**: tmux owns the terminal. No vt10x, no PTY code.
- **Sidecar model**: ccells is a separate process that controls tmux via commands. Not a custom client or plugin.

## State Management

### Hybrid: tmux + Docker as operational truth, state file for metadata

Core operational state derives from tmux and Docker on demand:

- `tmux list-panes` → which panes exist (via `@ccells-workstream` metadata)
- `docker ps --filter label=ccells` → which containers are running
- Cross-reference → live state

Each tmux pane stores metadata via pane options:
```bash
tmux set-option -p -t %42 @ccells-workstream "feature-auth"
tmux set-option -p -t %42 @ccells-container "ccells-myrepo-feature-auth"
```

### What the state file holds

One state file per session at `~/.claude-cells/state/<repo-id>/state.json`.

Fields that **can** be derived from tmux + Docker:
- `ContainerID`, `BranchName`, `Runtime` → Docker container labels
- Focused pane → tmux active pane
- Container running/paused → `docker ps`

Fields that **need persistence** (no other source):
- `Prompt` — original user input
- `Title` — generated via Claude CLI
- `Synopsis` — work summary
- `ClaudeSessionID` — critical for `--resume`
- `WasInterrupted` — was Claude mid-work when stopped
- `HasBeenPushed` — push tracking
- `PRNumber` / `PRURL` — PR tracking
- `RepoInfo` — repo identity

The state file is **lossy-safe**: if stale or missing, workstreams still appear and function. Users lose titles, prompts, and session resume until the file rebuilds.

**Optimization**: Store `ClaudeSessionID` as a Docker container label (set when captured) to make the most critical field derivable.

### Reconciliation triggers

- On `ccells attach` (startup)
- Before mutations (`create`, `rm`)
- On tmux hooks (`pane-died`, `session-closed`)

## CLI Verbs

Compose-style verbs with tmux's `attach`:

```
ccells up                    # start session + attach (idempotent)
ccells attach                # reattach to existing session
ccells down                  # stop daemon, pause containers
ccells down --rm             # stop + destroy containers + worktrees
ccells create                # new workstream (popup inside tmux)
ccells rm [name]             # destroy workstream
ccells pause [name]          # pause workstream
ccells unpause [name]        # resume workstream
ccells ps                    # list workstreams + status
ccells logs [name]           # show container logs
ccells pair [name]           # start pairing (mutagen sync)
ccells unpair                # stop pairing, restore branch
```

Bare `ccells` with no verb: `up` if no session exists, `attach` if one does.

## Keybindings

tmux keybindings trigger ccells CLI commands:

```
prefix + n    →  ccells create      (popup dialog)
prefix + d    →  ccells rm          (popup: select + confirm)
prefix + p    →  ccells pause
prefix + r    →  ccells unpause
prefix + m    →  ccells merge       (popup: merge/PR dialog)
prefix + f    →  ccells pair         (popup: select workstream to pair)
prefix + ?    →  ccells help        (popup: keybinding reference)
prefix + s    →  ccells ps          (refresh status line)
```

Dialogs run inside `tmux display-popup`:
```bash
tmux display-popup -E -w 60 -h 20 "ccells create --interactive"
```

Each dialog is a small standalone program (can be Bubble Tea). The `--interactive` flag collects user input; without it, the command works headlessly (e.g., `ccells create --branch my-feature`).

## Chrome

Three layers, each using native tmux features:

### Pane borders (`pane-border-format`)

Each pane's top border shows identity:
```
─── auth-system [running] ✓ PR#42 ──────────────────────
```

Focused pane highlighted via `pane-active-border-style`.

### Status line

Global info + keyhints:
```
[ccells] 3 workstreams | ^b+n new  ^b+d destroy  ^b+m merge  ^b+? help
```

On tmux ≥ 3.4, use `status 2` for two lines — workstream tabs on top, keyhints below. On older tmux, pack into a single line with abbreviated names.

### Help popup (`prefix + ?`)

Full keybinding reference. Detailed help lives here, not crammed into the status line.

## Daemon

The `ccells up` process starts the tmux server, configures it, then stays alive as the daemon.

### Responsibilities

- **Credential refresh**: poll keychain every 15 min, push tokens to container configs
- **State reconciliation**: periodically query tmux + Docker, update state file
- **tmux hooks**: listen for `pane-died`, `session-closed` to trigger cleanup
- **PR status polling**: refresh every 5 min
- **Socket API**: CLI commands talk to daemon via `~/.claude-cells/state/<repo-id>/daemon.sock`

### Crash recovery

If daemon dies, tmux and containers survive. Any ccells CLI command detects no daemon and restarts it. Reconciliation rebuilds state from tmux + Docker.

## Detach Summary

On detach, print a summary to the terminal:

```
╭─ ccells detached ──────────────────────────────────────╮
│                                                        │
│  Session: myrepo (3 workstreams)                       │
│                                                        │
│  auth-system     running   ▲ 2.1% CPU   148 MB        │
│  fix-login-bug   running   ▲ 0.3% CPU    95 MB        │
│  refactor-db     paused    ○                           │
│                                                        │
│  Daemon: running (PID 48291)                           │
│  Credentials: refreshing every 15m                     │
│                                                        │
│  Reattach:   ccells attach                             │
│  Stop all:   ccells down                               │
│  Destroy:    ccells down --rm                           │
│  Manual:     tmux -L ccells-abc123 attach              │
│                                                        │
╰────────────────────────────────────────────────────────╯
```

Resource usage from `docker stats --no-stream`.

On `ccells down`:
```
Stopped 3 containers (resumable with ccells up)
Cleaned up daemon (PID 48291)
tmux server terminated

To fully remove: ccells down --rm
```

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| User kills a tmux pane | Container orphaned. Next ccells command offers cleanup. |
| User manually splits a pane | No `@ccells-workstream` metadata → ccells ignores it. |
| Docker container dies | tmux pane shows exit. Pane border: `[exited]`. User can `rm` or `unpause`. |
| Daemon crashes | tmux + containers survive. Next CLI command restarts daemon. |
| tmux server crashes | Containers survive. `ccells up` recreates panes from Docker state. |
| Machine reboot | Containers stopped. `ccells up` offers to resume. Worktrees recreated from branches. |

Git worktrees stay in `/tmp/ccells/worktrees/<branch>`. Branches are the durable artifact — worktrees are cheap to recreate.

## Code Impact

### Deleted (~5000+ lines)

The entire `internal/tui/` package:
- `app.go` — Bubble Tea event loop, layout, message routing
- `pane.go` — vt10x terminal emulation, rendering
- `pane_colors.go` — ANSI muting/stripping
- `pane_scroll.go` — custom scroll logic
- `pane_animation.go` — fade/spinner animations
- `pty.go` — Docker exec PTY session management
- `layout.go` — multi-pane layout calculations
- `styles.go` — Lipgloss styling
- `dialog.go` — modal dialog system
- `container.go` — TUI container command wrappers
- `statusbar.go`, `log_panel.go`, `viewport.go`

### Unchanged

- `internal/orchestrator/` — create, pause, resume, destroy
- `internal/docker/` — Docker SDK wrapper, security, credentials
- `internal/git/` — worktree, branch, PR operations
- `internal/gitproxy/` — container git/gh proxy
- `internal/sync/` — Mutagen pairing mode
- `internal/claude/` — ephemeral queries

### New (~2000 lines estimated)

- `internal/tmux/` — server management, pane CRUD, config generation, status line
- `internal/daemon/` — background process, socket API, reconciliation
- `cmd/ccells/` — rewritten as CLI subcommands
- Dialog programs for `ccells create --interactive`, `ccells merge --interactive`, etc.

### Simplified

- `internal/workstream/` — `PersistentManager` debounce loop likely unnecessary (daemon handles persistence). State structs stay.

## Prefix Key

Use the user's existing tmux prefix (default `Ctrl-b`). Do not set a custom prefix.

Query the active prefix at runtime via `tmux show-option -gv prefix` to display correct keybindings in the help popup and status line. If the user remaps to `Ctrl-a`, the help shows `^a+n new` automatically.

## Multiple Repos

First-class support. Each repo gets its own tmux server (socket: `ccells-<repo-id>`) and daemon process. They run independently.

When `ccells attach` finds multiple active sessions, it lists them and lets the user pick. `ccells ps --all` shows all active sessions across repos.

## Pairing Mode

The `Pairing` struct (`internal/sync/pairing.go`) moves from the TUI layer to the daemon. The struct itself is unchanged — `Enable()`, `Disable()`, `CheckSyncHealth()`, and the `GitOperations`/`MutagenOperations` interfaces stay as-is.

**Trigger**: `prefix + f` (follow) or `ccells pair [name]` / `ccells unpair`

**Integration**:
- Daemon calls `Pairing.Enable()` on pair, `Pairing.Disable()` on unpair
- Daemon polls `CheckSyncHealth()` every 5s alongside PR status polling
- Pane border reflects sync state: `─── auth-system [pairing] ⟳ 2 conflicts ───`
- Status line shows pairing indicator when active

**Constraint**: One workstream paired at a time (unchanged from today — pairing checks out the branch locally).

**CLI additions**:
```
ccells pair [name]           # start pairing for workstream
ccells unpair                # stop pairing, restore previous branch
```

**Keybinding**: `prefix + f` → `ccells pair --interactive` (popup to select workstream if not focused)

## Runtime Extensibility (opencode support)

The tmux architecture makes multi-runtime support straightforward: each pane runs a command, and that command is determined by the session's runtime selection.

**Current runtimes**: `claude` (default), `claudesp` (experimental)

**Future runtime**: `opencode` (or any CLI coding tool)

**Per-session selection** (same model as today):
```
ccells up --runtime opencode
```

Priority chain: CLI flag > project config > global config > default (`claude`)

**What varies by runtime:**
- Container setup script (install command, config directory)
- Pane command (`claude --resume <id>` vs `opencode` vs custom)
- Session ID capture (regex pattern for extracting resume token from output)
- Credential format (OAuth tokens vs API keys vs none)

**Design constraint:** The daemon, CLI verbs, tmux integration, state management, and git operations are all runtime-agnostic. Runtime-specific behavior is isolated to:
1. Container image build (Dockerfile install step)
2. Pane command template (what runs inside `docker exec`)
3. Session ID capture pattern (optional — some runtimes may not support resume)
4. Credential provisioning (how auth tokens reach the container)

**Implementation approach:** Define a `Runtime` interface:
```go
type Runtime interface {
    Name() string
    InstallScript() string           // Dockerfile RUN command
    PaneCommand(sessionID string) string  // docker exec command
    SessionIDPattern() *regexp.Regexp // nil if no resume support
    CredentialSetup(configDir string) error
}
```

This keeps the core architecture clean while allowing new runtimes to be added as implementations of this interface. Not needed for initial tmux migration — add when opencode support is prioritized
