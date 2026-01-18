# Claude Cells

Run multiple Claude Code instances in parallel, each in its own isolated Docker container with automatic git branch management.

## Features

- **Parallel Workstreams**: Run multiple Claude Code sessions simultaneously, each working on different tasks
- **Isolated Containers**: Each workstream runs in its own Docker container, preventing interference
- **Automatic Branch Management**: Each workstream gets its own git branch, created from your prompt
- **Pairing Mode**: Sync your local filesystem with a container for real-time collaboration
- **Session Persistence**: Quit and resume later - containers are paused and state is saved
- **Push & PR**: Push branches and create pull requests directly from the TUI

## Prerequisites

- Docker
- Go 1.21+
- [Mutagen](https://mutagen.io/) (for pairing mode)
- [gh CLI](https://cli.github.com/) (for PR creation)
- Claude Code installed and authenticated

## Installation

```bash
go install github.com/STRML/claude-cells/cmd/ccells@latest
```

Or build from source:

```bash
git clone https://github.com/STRML/claude-cells.git
cd claude-cells
go build ./cmd/ccells
```

## Quick Start

1. Build the base Docker image:
   ```bash
   docker build -t ccells-base -f configs/base.Dockerfile .
   ```

2. Run ccells from your project directory:
   ```bash
   ccells
   ```

3. Press `n` to create a new workstream and enter a prompt for Claude

## Keybindings

### Navigation Mode (default)
| Key | Action |
|-----|--------|
| `n` | New workstream |
| `d` | Destroy workstream |
| `p` | Toggle pairing mode |
| `m` | Merge/PR menu |
| `l` | View logs |
| `s` | Settings |
| `i` / `Enter` | Enter input mode |
| `Tab` | Cycle focus |
| `1-9` | Focus pane by number |
| `Ctrl+b` + arrow | Switch panes (tmux-style) |
| `q` / `Ctrl+c` | Quit (pauses containers) |

### Input Mode
| Key | Action |
|-----|--------|
| `Esc` | Exit to navigation mode |
| `Esc Esc` | Send escape to Claude |
| `Ctrl+c` | Send interrupt to Claude |
| `Ctrl+b` + arrow | Switch panes |

## How It Works

1. **Workstream Creation**: When you create a workstream, ccells:
   - Generates a branch name from your prompt (e.g., "add user auth" → `add-user-auth`)
   - Creates a Docker container with your project mounted
   - Starts Claude Code with your prompt

2. **Isolation**: Each container has:
   - Its own copy of your Claude credentials (read-only)
   - A mounted copy of your project directory
   - Its own git branch

3. **Pairing Mode**: Enables bidirectional file sync between your local filesystem and the container using Mutagen. This lets you:
   - Edit files locally while Claude works in the container
   - See Claude's changes in real-time
   - Use your local IDE alongside Claude

4. **Session Persistence**: When you quit:
   - All containers are paused (not stopped)
   - State is saved to `.claude-cells/state.json`
   - On restart, containers are resumed and PTY sessions reconnected

## Configuration

ccells stores its data in `~/.claude-cells/`:
- `claude-config/` - Isolated copy of your Claude credentials
- `state.json` - Session state for resume (in project directory)

## License

MIT
