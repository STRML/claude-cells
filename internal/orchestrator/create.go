package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/STRML/claude-cells/internal/workstream"
)

const worktreeBaseDir = "/tmp/ccells/worktrees"

// CreateWorkstream creates a new workstream with container and worktree.
func (o *Orchestrator) CreateWorkstream(ctx context.Context, ws *workstream.Workstream, opts CreateOptions) (string, error) {
	// Step 1: Create git worktree
	worktreePath, err := o.createWorktree(ctx, ws.BranchName)
	if err != nil {
		return "", fmt.Errorf("create worktree: %w", err)
	}
	ws.WorktreePath = worktreePath

	// TODO: Steps 2-4 will be added in subsequent tasks
	// Step 2: Copy untracked files (if requested)
	// Step 3: Create container
	// Step 4: Start container

	return "", nil // Container ID will be returned once implemented
}

func (o *Orchestrator) createWorktree(ctx context.Context, branchName string) (string, error) {
	// Ensure base directory exists
	if err := os.MkdirAll(worktreeBaseDir, 0755); err != nil {
		return "", fmt.Errorf("create worktree base dir: %w", err)
	}

	worktreePath := filepath.Join(worktreeBaseDir, branchName)

	gitClient := o.gitFactory(o.repoPath)
	if err := gitClient.CreateWorktree(ctx, worktreePath, branchName); err != nil {
		return "", fmt.Errorf("git create worktree: %w", err)
	}

	return worktreePath, nil
}

// cleanupWorktree removes a worktree on error.
func (o *Orchestrator) cleanupWorktree(ctx context.Context, branchName string) {
	worktreePath := filepath.Join(worktreeBaseDir, branchName)
	gitClient := o.gitFactory(o.repoPath)
	_ = gitClient.RemoveWorktree(ctx, worktreePath)
}
