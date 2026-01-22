package orchestrator

import (
	"context"

	"github.com/STRML/claude-cells/internal/workstream"
)

// PauseWorkstream pauses a running workstream's container.
func (o *Orchestrator) PauseWorkstream(ctx context.Context, ws *workstream.Workstream) error {
	// TODO: Implementation will be added in Task 4
	return nil
}

// ResumeWorkstream resumes a paused workstream's container.
func (o *Orchestrator) ResumeWorkstream(ctx context.Context, ws *workstream.Workstream) error {
	// TODO: Implementation will be added in Task 4
	return nil
}

// DestroyWorkstream removes container, worktree, and optionally the branch.
func (o *Orchestrator) DestroyWorkstream(ctx context.Context, ws *workstream.Workstream, opts DestroyOptions) error {
	// TODO: Implementation will be added in Task 4
	return nil
}

// RebuildWorkstream destroys and recreates the container with fresh state.
func (o *Orchestrator) RebuildWorkstream(ctx context.Context, ws *workstream.Workstream, opts CreateOptions) (string, error) {
	// TODO: Implementation will be added in Task 4
	return "", nil
}
