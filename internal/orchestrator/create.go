package orchestrator

import (
	"context"

	"github.com/STRML/claude-cells/internal/workstream"
)

const worktreeBaseDir = "/tmp/ccells/worktrees"

// CreateWorkstream creates a new workstream with container and worktree.
func (o *Orchestrator) CreateWorkstream(ctx context.Context, ws *workstream.Workstream, opts CreateOptions) (string, error) {
	// TODO: Implementation will be added in Task 2 and Task 3
	return "", nil
}
