# Docker TUI

A terminal UI for running parallel Claude Code instances in isolated Docker containers.

## Development Approach

**MANDATORY: Test-Driven Development**
- Write tests FIRST, before any implementation
- Tests must pass before moving to the next component
- Do not prompt the user until tests pass
- Use table-driven tests (Go idiom)

## Tech Stack

- **Language:** Go
- **TUI:** Bubble Tea + Lip Gloss
- **Containers:** Docker SDK for Go
- **File Sync:** Mutagen
- **Git/PR:** gh CLI

## Key Files

- `docs/plans/2025-01-15-docker-tui-design.md` - Full design spec

## Commands

```bash
# Run tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific package tests
go test ./internal/workstream/...

# Build
go build ./cmd/docker-tui

# Run
./docker-tui
```

## Architecture Overview

```
cmd/docker-tui/main.go     # Entry point
internal/
  tui/                     # Bubble Tea UI components
  workstream/              # Workstream state & lifecycle
  docker/                  # Docker SDK wrapper
  sync/                    # Mutagen/pairing mode
  git/                     # Branch & PR operations
configs/
  base.Dockerfile          # Base image with Claude Code
```

## Implementation Order

1. `internal/workstream/branch.go` - Branch name generation from prompts
2. `internal/docker/client.go` - Docker client wrapper
3. `internal/docker/container.go` - Container lifecycle
4. `internal/workstream/workstream.go` - Workstream state
5. `internal/workstream/manager.go` - Multi-workstream management
6. `internal/sync/mutagen.go` - Mutagen wrapper
7. `internal/sync/pairing.go` - Pairing mode logic
8. `internal/git/branch.go` - Git branch operations
9. `internal/git/pr.go` - PR creation
10. `internal/tui/*` - TUI components (last)
