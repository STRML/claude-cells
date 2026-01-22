package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/STRML/claude-cells/internal/docker"
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

	// Step 2: Copy untracked files (if requested)
	if opts.CopyUntracked && len(opts.UntrackedFiles) > 0 {
		if err := o.copyUntrackedFiles(opts.RepoPath, worktreePath, opts.UntrackedFiles); err != nil {
			o.cleanupWorktree(ctx, ws.BranchName)
			return "", fmt.Errorf("copy untracked files: %w", err)
		}
	}

	// Step 3: Create container config
	cfg := o.buildContainerConfig(ws, worktreePath, opts)

	// Step 4: Create and start container
	containerID, err := o.createAndStartContainer(ctx, cfg)
	if err != nil {
		o.cleanupWorktree(ctx, ws.BranchName)
		return "", fmt.Errorf("create container: %w", err)
	}

	ws.ContainerID = containerID
	return containerID, nil
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

func (o *Orchestrator) buildContainerConfig(ws *workstream.Workstream, worktreePath string, opts CreateOptions) *docker.ContainerConfig {
	cfg := docker.NewContainerConfig(ws.BranchName, worktreePath)
	cfg.HostGitDir = filepath.Join(o.repoPath, ".git")

	if opts.ImageName != "" {
		cfg.Image = opts.ImageName
	}

	return cfg
}

func (o *Orchestrator) createAndStartContainer(ctx context.Context, cfg *docker.ContainerConfig) (string, error) {
	containerID, err := o.dockerClient.CreateContainer(ctx, cfg)
	if err != nil {
		return "", fmt.Errorf("docker create: %w", err)
	}

	if err := o.dockerClient.StartContainer(ctx, containerID); err != nil {
		// Cleanup: remove container on start failure
		_ = o.dockerClient.RemoveContainer(ctx, containerID)
		return "", fmt.Errorf("docker start: %w", err)
	}

	return containerID, nil
}

func (o *Orchestrator) copyUntrackedFiles(srcRepo, dstWorktree string, files []string) error {
	for _, file := range files {
		src := filepath.Join(srcRepo, file)
		dst := filepath.Join(dstWorktree, file)

		// Ensure destination directory exists
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}

		// Copy file
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			return err
		}
	}
	return nil
}
