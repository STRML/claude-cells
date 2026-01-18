# Claude Cells

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

## Commands

```bash
# Run tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific package tests
go test ./internal/workstream/...

# Build
go build ./cmd/ccells

# Run
./ccells
```

## Architecture Overview

```
cmd/ccells/main.go         # Entry point
internal/
  tui/                     # Bubble Tea UI components
  workstream/              # Workstream state & lifecycle
  docker/                  # Docker SDK wrapper
  sync/                    # Mutagen/pairing mode
  git/                     # Branch & PR operations
configs/
  base.Dockerfile          # Base image with Claude Code
```
