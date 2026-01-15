# Docker TUI Design

A terminal UI for running parallel Claude Code instances in isolated Docker containers.

## Overview

**Goal:** Run multiple Claude Code instances on a single repo without interference, each on its own branch in its own container.

**Inspiration:** [Sculptor](https://github.com/imbue-ai/sculptor) by Imbue

**Scope:** Personal productivity tool - lean, fast, minimal polish needed.

## Core Concepts

### Workstream

A workstream = Docker container + git branch + Claude Code instance

**Creation flow:**
1. User provides a prompt (e.g., "add user authentication with JWT")
2. TUI auto-generates branch name from prompt (e.g., `add-user-auth-jwt`)
3. Container spins up from base image
4. Repo copied into container at current HEAD
5. Branch created and checked out
6. Claude Code starts with `--dangerously-skip-permissions`
7. Initial prompt sent to Claude

**Lifecycle states:**
- `running` - Claude active, processing or waiting
- `idle` - Claude finished, container alive for follow-up
- `pairing` - Mutagen sync active with local
- `stopped` - Container halted, can resume

## Technology Stack

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Language | Go | Single binary, good Docker SDK |
| TUI Framework | Bubble Tea | Battle-tested, used by Sculptor |
| Styling | Lip Gloss | Pairs with Bubble Tea |
| File Sync | Mutagen | Bidirectional sync, proven solution |
| PR Creation | gh CLI | GitHub's official CLI |
| Containers | Docker SDK | Native Go integration |

## Architecture

```
docker-tui/
├── cmd/
│   └── docker-tui/
│       └── main.go           # Entry point
├── internal/
│   ├── tui/
│   │   ├── app.go            # Bubble Tea app, main model
│   │   ├── pane.go           # Individual workstream pane
│   │   ├── statusbar.go      # Bottom bar with keybindings
│   │   ├── dialog.go         # Confirmation dialogs
│   │   └── styles.go         # Lip Gloss styling
│   ├── workstream/
│   │   ├── workstream.go     # Workstream state & lifecycle
│   │   ├── manager.go        # Tracks all workstreams
│   │   └── branch.go         # Branch name generation from prompt
│   ├── docker/
│   │   ├── client.go         # Docker SDK wrapper
│   │   ├── container.go      # Container lifecycle
│   │   └── image.go          # Base image building
│   ├── sync/
│   │   ├── mutagen.go        # Mutagen session management
│   │   └── pairing.go        # Pairing mode orchestration
│   └── git/
│       ├── branch.go         # Branch operations
│       └── pr.go             # PR creation via gh CLI
├── configs/
│   └── base.Dockerfile       # Base image with Claude Code
├── go.mod
└── go.sum
```

## TUI Layout

Tmux-style splits showing multiple workstreams simultaneously:

```
┌─────────────────────────────────────────────────────────────────┐
│ docker-tui: 3 workstreams | [P]airing: auth-jwt | [?] help      │
├───────────────────────────────┬─────────────────────────────────┤
│ ● auth-jwt (pairing)          │ ○ fix-api-tests (running)       │
│                               │                                 │
│ > Added JWT middleware to     │ > Running test suite...         │
│   src/auth/jwt.ts             │   ✓ 42 passed                   │
│                               │   ✗ 3 failed                    │
│ Claude: I've implemented the  │                                 │
│ JWT auth. Want me to add      │ Claude: Found 3 failing tests   │
│ refresh tokens?               │ in api/users.test.ts...         │
│                               │                                 │
│ [prompt: _]                   │                                 │
├───────────────────────────────┴─────────────────────────────────┤
│ ◌ add-caching (idle) - "Added Redis caching layer" - 2h ago     │
├─────────────────────────────────────────────────────────────────┤
│ [n]ew | [p]air | [m]erge/PR | [d]estroy | [1-3] focus | [?]help │
└─────────────────────────────────────────────────────────────────┘
```

**Layout behavior:**
- Active/running workstreams get split panes (2-4 visible)
- Idle workstreams collapse to single status line
- Number keys switch focus between panes
- `Tab` cycles through panes
- `/` enters prompt mode in focused pane

**Status indicators:**
- `●` running
- `○` idle
- `◌` stopped
- `⟳` pairing mode active

## Container Setup

### Base Image

```dockerfile
ARG BASE_IMAGE=mcr.microsoft.com/devcontainers/base:ubuntu
FROM ${BASE_IMAGE}

# Install Node.js and Claude Code
RUN curl -fsSL https://nodejs.org/dist/v20.11.0/node-v20.11.0-linux-x64.tar.xz \
    | tar -xJ -C /usr/local --strip-components=1 \
    && npm install -g @anthropic-ai/claude-code

WORKDIR /workspace
CMD ["sleep", "infinity"]
```

### Container Creation

1. Check for `.devcontainer/Dockerfile` in project
   - If exists: extend it with Claude Code layer
   - If not: use default base image
2. Create container with mounts:
   - `/workspace` - copy of repo at HEAD
   - `~/.claude` (read-only) - global skills, settings
   - `~/.claude/auth` (read-write) - Claude Max auth tokens
3. Start container
4. Create and checkout branch
5. Launch `claude --dangerously-skip-permissions`
6. Send initial prompt

### Authentication

Uses Claude Max (OAuth), not API keys:
- Auth tokens stored in `~/.claude`
- Mount auth directory read-write for token refresh
- Rest of `~/.claude` mounted read-only (skills, settings)

## Pairing Mode

Bidirectional sync between container and local filesystem using Mutagen.

### Enabling Pairing

1. User presses `p` on a workstream
2. If local repo has uncommitted changes:
   - Run `git stash`
   - Notify user: "Stashed local changes"
3. Start Mutagen sync:
   ```bash
   mutagen sync create \
     --name "docker-tui-<branch>" \
     --sync-mode "two-way-resolved" \
     --ignore ".git" \
     docker://container-id/workspace \
     /path/to/local/repo
   ```
4. Check out workstream's branch locally
5. Status bar shows pairing indicator

### Disabling Pairing

1. User presses `p` again (or switches to another workstream)
2. Terminate Mutagen sync session
3. Check out previous branch locally (usually `main`)
4. If stash exists, notify user: "Run `git stash pop` to restore changes"

### Constraints

- Only one workstream in Pairing Mode at a time
- Switching pairing auto-disables previous
- Conflicts: `two-way-resolved` mode (most recent wins)
- True conflicts create `.sync-conflict` files (surfaced as warnings)

## PR Workflow (Primary)

Create PR is the main completion workflow.

### Flow

1. User presses `m`, selects "Create PR"
2. Push branch: `git push -u origin <branch>`
3. Auto-generate PR details by asking Claude in container:
   - "Generate a PR title and description for the changes you made"
4. Show preview with option to edit:
   ```
   ┌─────────────────────────────────────────────────┐
   │  Create PR for "add-user-auth-jwt"              │
   │                                                 │
   │  Title: [Add JWT-based user authentication   ]  │
   │                                                 │
   │  Body:                                          │
   │  ┌─────────────────────────────────────────┐   │
   │  │ ## Summary                              │   │
   │  │ Added JWT authentication middleware...  │   │
   │  └─────────────────────────────────────────┘   │
   │  [e] Edit  [Enter] Create  [Esc] Cancel        │
   └─────────────────────────────────────────────────┘
   ```
5. Create PR: `gh pr create --title "..." --body "..."`
6. Show PR URL
7. Offer to destroy workstream (not forced)

### After PR

- Workstream stays alive for iterations
- Status shows "PR #123" badge
- Can continue prompting Claude for changes

## Destruction Workflow

Deliberate confirmation required to prevent accidents.

### Flow

1. User presses `d` on a workstream
2. Show confirmation dialog:
   ```
   ┌─────────────────────────────────────────────┐
   │  Destroy workstream "add-user-auth-jwt"?    │
   │                                             │
   │  This will:                                 │
   │    • Stop and remove the container          │
   │    • End any active Mutagen sync            │
   │    • Delete local branch (remote untouched) │
   │                                             │
   │  Type "destroy" to confirm: _               │
   │                                             │
   │  [Esc] Cancel                               │
   └─────────────────────────────────────────────┘
   ```
3. User must type "destroy" (not just `y`)
4. On confirm:
   - Disable pairing if active (restore stash if any)
   - Stop and remove container
   - Delete local branch
   - Remove from TUI
5. If remote branch exists, ask once:
   ```
   Remote branch still exists. Delete it? [y/N]
   ```

## Keybindings

| Key | Action |
|-----|--------|
| `n` | New workstream (prompt for task) |
| `p` | Toggle Pairing Mode |
| `m` | Merge/PR menu |
| `d` | Destroy workstream (with confirm) |
| `1-9` | Focus pane by number |
| `Tab` | Cycle focus |
| `/` | Enter prompt in focused pane |
| `Enter` | Send prompt |
| `Esc` | Cancel/close dialog |
| `?` | Show help |
| `q` | Quit (confirms if workstreams active) |

## Dependencies

**Go modules:**
- `github.com/charmbracelet/bubbletea` - TUI framework
- `github.com/charmbracelet/lipgloss` - Styling
- `github.com/charmbracelet/bubbles` - TUI components
- `github.com/docker/docker/client` - Docker SDK

**External binaries (must be installed):**
- `docker` - Container runtime
- `mutagen` - File sync
- `gh` - GitHub CLI
- `git` - Version control

## Implementation Notes

- Write tests first (TDD approach)
- Tests must pass before moving to next component
- Use table-driven tests for Go
- Mock Docker client and exec calls for unit tests
- Integration tests can use real Docker with cleanup

## Future Considerations

Not in scope for v1, but possible later:
- Multiple repo support
- Persistent workstream state across TUI restarts
- Cost tracking for Claude Max usage
- Team sharing of workstream configs
