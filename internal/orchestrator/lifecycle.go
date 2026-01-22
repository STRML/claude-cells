package orchestrator

import (
	"context"
	"fmt"

	"github.com/STRML/claude-cells/internal/workstream"
)

// PauseWorkstream pauses a running workstream's container.
func (o *Orchestrator) PauseWorkstream(ctx context.Context, ws *workstream.Workstream) error {
	if ws.ContainerID == "" {
		return fmt.Errorf("workstream has no container")
	}

	if err := o.dockerClient.PauseContainer(ctx, ws.ContainerID); err != nil {
		return fmt.Errorf("pause container: %w", err)
	}

	return nil
}

// ResumeWorkstream resumes a paused workstream's container.
func (o *Orchestrator) ResumeWorkstream(ctx context.Context, ws *workstream.Workstream) error {
	if ws.ContainerID == "" {
		return fmt.Errorf("workstream has no container")
	}

	if err := o.dockerClient.UnpauseContainer(ctx, ws.ContainerID); err != nil {
		return fmt.Errorf("resume container: %w", err)
	}

	return nil
}

// DestroyWorkstream removes container, worktree, and optionally the branch.
func (o *Orchestrator) DestroyWorkstream(ctx context.Context, ws *workstream.Workstream, opts DestroyOptions) error {
	var errs []error

	// Step 1: Stop and remove container
	if ws.ContainerID != "" {
		if err := o.dockerClient.StopContainer(ctx, ws.ContainerID); err != nil {
			// Container might already be stopped, continue
		}
		if err := o.dockerClient.RemoveContainer(ctx, ws.ContainerID); err != nil {
			errs = append(errs, fmt.Errorf("remove container: %w", err))
		}
	}

	// Step 2: Remove worktree (unless KeepWorktree is set)
	if !opts.KeepWorktree && ws.WorktreePath != "" {
		gitClient := o.gitFactory(o.repoPath)
		if err := gitClient.RemoveWorktree(ctx, ws.WorktreePath); err != nil {
			errs = append(errs, fmt.Errorf("remove worktree: %w", err))
		}
	}

	// Step 3: Delete branch (if requested)
	if opts.DeleteBranch && ws.BranchName != "" {
		gitClient := o.gitFactory(o.repoPath)
		if err := gitClient.DeleteBranch(ctx, ws.BranchName); err != nil {
			errs = append(errs, fmt.Errorf("delete branch: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("destroy had errors: %v", errs)
	}
	return nil
}

// RebuildWorkstream destroys and recreates the container with fresh state.
func (o *Orchestrator) RebuildWorkstream(ctx context.Context, ws *workstream.Workstream, opts CreateOptions) (string, error) {
	// Save branch and worktree info
	branchName := ws.BranchName
	worktreePath := ws.WorktreePath

	// Destroy container only (keep worktree and branch)
	destroyOpts := DestroyOptions{
		KeepWorktree: true,
		DeleteBranch: false,
	}
	if err := o.DestroyWorkstream(ctx, ws, destroyOpts); err != nil {
		return "", fmt.Errorf("destroy for rebuild: %w", err)
	}

	// Recreate with existing worktree
	ws.WorktreePath = worktreePath
	ws.BranchName = branchName

	cfg := o.buildContainerConfig(ws, worktreePath, opts)
	containerID, err := o.createAndStartContainer(ctx, cfg)
	if err != nil {
		return "", fmt.Errorf("recreate container: %w", err)
	}

	ws.ContainerID = containerID
	return containerID, nil
}
