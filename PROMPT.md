# Implementation Prompt

Read the design at `docs/plans/2025-01-15-docker-tui-design.md` first.

## Your Task

Implement the Docker TUI according to the design spec. Use the `superpowers:writing-plans` skill to create a detailed implementation plan, then use `superpowers:test-driven-development` for each component.

## Critical Requirements

1. **TDD is mandatory** - Write tests first, make them pass, then move on
2. **Do not prompt until tests pass** - Keep working until green
3. **Follow the implementation order in CLAUDE.md**

## Starting Point

1. Initialize the Go module: `go mod init github.com/samuelreed/docker-tui`
2. Start with `internal/workstream/branch.go` - the branch name generator
3. Write tests first in `internal/workstream/branch_test.go`

## Branch Name Generation Spec

Given a prompt like "add user authentication with JWT tokens", generate a branch name like `add-user-auth-jwt`. Rules:
- Lowercase
- Hyphens instead of spaces
- Max 50 chars
- Strip common words (the, a, an, to, for, with)
- Keep it readable and meaningful

Begin by invoking `superpowers:writing-plans` to create a detailed implementation plan.
