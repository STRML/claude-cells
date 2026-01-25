package orchestrator

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/STRML/claude-cells/internal/docker"
	"github.com/STRML/claude-cells/internal/gitproxy"
	"github.com/STRML/claude-cells/internal/workstream"
)

// DefaultGitProxyBaseDir is the base directory for git proxy sockets.
const DefaultGitProxyBaseDir = "/tmp/ccells/gitproxy"

// DefaultWorktreeBaseDir is the default directory for git worktrees.
// Can be overridden via Orchestrator.WorktreeBaseDir for testing.
const DefaultWorktreeBaseDir = "/tmp/ccells/worktrees"

// sanitizeBranchName converts a branch name to a safe filesystem path component.
// It replaces path separators and spaces with dashes to prevent nested directories.
// Example: "feature/foo" -> "feature-foo", "my branch" -> "my-branch"
func sanitizeBranchName(branchName string) string {
	safe := branchName
	safe = strings.ReplaceAll(safe, "/", "-")
	safe = strings.ReplaceAll(safe, "\\", "-")
	safe = strings.ReplaceAll(safe, " ", "-")
	// Trim leading/trailing dashes that might result from edge cases
	safe = strings.Trim(safe, "-")
	// Fallback if the result is empty
	if safe == "" {
		safe = "unnamed"
	}
	return safe
}

// CreateWorkstream creates a new workstream with container and worktree.
// This is the complete flow including:
// - Optional main branch update
// - Worktree creation (new or from existing branch)
// - Image detection and building
// - Container config setup (credentials, git identity)
// - Container creation and starting
func (o *Orchestrator) CreateWorkstream(ctx context.Context, ws *workstream.Workstream, opts CreateOptions) (*CreateResult, error) {
	gitClient := o.gitFactory(o.repoPath)

	// Step 1: Update main branch (optional, non-fatal)
	if opts.UpdateMain && !opts.UseExistingBranch {
		_ = gitClient.UpdateMainBranch(ctx)
	}

	// Step 2: Create git worktree
	worktreePath, err := o.createWorktree(ctx, ws.BranchName, opts.UseExistingBranch)
	if err != nil {
		return nil, fmt.Errorf("create worktree: %w", err)
	}
	ws.WorktreePath = worktreePath

	// Step 3: Copy untracked files (if requested and not using existing branch)
	if opts.CopyUntracked && len(opts.UntrackedFiles) > 0 && !opts.UseExistingBranch {
		if err := o.copyUntrackedFiles(o.repoPath, worktreePath, opts.UntrackedFiles); err != nil {
			o.cleanupWorktree(ctx, ws.BranchName)
			return nil, fmt.Errorf("copy untracked files: %w", err)
		}
	}

	// Step 4: Determine image (auto-detect or use provided)
	imageName, err := o.resolveImage(ctx, opts)
	if err != nil {
		o.cleanupWorktree(ctx, ws.BranchName)
		return nil, fmt.Errorf("resolve image: %w", err)
	}

	// Step 5: Build container config with credentials
	cfgResult, err := o.buildFullContainerConfig(ws, worktreePath, imageName, opts)
	if err != nil {
		o.cleanupWorktree(ctx, ws.BranchName)
		return nil, fmt.Errorf("build container config: %w", err)
	}

	// Step 6: Create and start container
	containerID, err := o.createAndStartContainer(ctx, cfgResult.config)
	if err != nil {
		o.cleanupWorktree(ctx, ws.BranchName)
		// Also clean up container config on failure
		if cfgResult.configDir != "" {
			_ = docker.CleanupContainerConfig(cfgResult.config.Name)
		}
		// Clean up git proxy socket directory
		if cfgResult.gitProxySocketDir != "" {
			_ = os.RemoveAll(cfgResult.gitProxySocketDir)
		}
		return nil, fmt.Errorf("create container: %w", err)
	}

	ws.ContainerID = containerID

	return &CreateResult{
		ContainerID:       containerID,
		ContainerName:     cfgResult.config.Name,
		ConfigDir:         cfgResult.configDir,
		WorktreePath:      worktreePath,
		GitProxySocketDir: cfgResult.gitProxySocketDir,
	}, nil
}

// CheckBranchConflict checks if a branch already exists.
// Returns nil if there's no conflict, or BranchConflict with details.
func (o *Orchestrator) CheckBranchConflict(ctx context.Context, branchName string) (*BranchConflict, error) {
	gitClient := o.gitFactory(o.repoPath)

	// Check for active worktree first
	existingPath, hasWorktree := gitClient.WorktreeExistsForBranch(ctx, branchName)
	if hasWorktree {
		branchInfo, _ := gitClient.GetBranchInfo(ctx, branchName)
		return &BranchConflict{
			BranchName:   branchName,
			HasWorktree:  true,
			WorktreePath: existingPath,
			BranchInfo:   fmt.Sprintf("Active worktree at: %s\n%s", existingPath, branchInfo),
		}, nil
	}

	// Check if branch exists (without worktree)
	exists, err := gitClient.BranchExists(ctx, branchName)
	if err != nil {
		return nil, fmt.Errorf("check branch exists: %w", err)
	}
	if exists {
		branchInfo, _ := gitClient.GetBranchInfo(ctx, branchName)
		return &BranchConflict{
			BranchName:   branchName,
			HasWorktree:  false,
			WorktreePath: "",
			BranchInfo:   branchInfo,
		}, nil
	}

	return nil, nil // No conflict
}

func (o *Orchestrator) createWorktree(ctx context.Context, branchName string, useExisting bool) (string, error) {
	baseDir := o.getWorktreeBaseDir()

	// Ensure base directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", fmt.Errorf("create worktree base dir: %w", err)
	}

	// Sanitize branch name for filesystem path (e.g., "feature/foo" -> "feature-foo")
	safeName := sanitizeBranchName(branchName)
	worktreePath := filepath.Join(baseDir, safeName)

	// Clean up orphaned worktree directory if it exists but git doesn't know about it
	gitClient := o.gitFactory(o.repoPath)
	if _, err := os.Stat(worktreePath); err == nil {
		// Directory exists - check if git knows about it
		_, hasWorktree := gitClient.WorktreeExistsForBranch(ctx, branchName)
		if !hasWorktree {
			// Orphaned directory - clean it up
			_ = gitClient.RemoveWorktree(ctx, worktreePath)
			_ = os.RemoveAll(worktreePath)
		}
	}

	if useExisting {
		// Create worktree from existing branch
		if err := gitClient.CreateWorktreeFromExisting(ctx, worktreePath, branchName); err != nil {
			return "", fmt.Errorf("git create worktree from existing: %w", err)
		}
	} else {
		// Create worktree with new branch
		if err := gitClient.CreateWorktree(ctx, worktreePath, branchName); err != nil {
			return "", fmt.Errorf("git create worktree: %w", err)
		}
	}

	return worktreePath, nil
}

// cleanupWorktree removes a worktree on error.
func (o *Orchestrator) cleanupWorktree(ctx context.Context, branchName string) {
	safeName := sanitizeBranchName(branchName)
	worktreePath := filepath.Join(o.getWorktreeBaseDir(), safeName)
	gitClient := o.gitFactory(o.repoPath)
	_ = gitClient.RemoveWorktree(ctx, worktreePath)
	_ = os.RemoveAll(worktreePath)
}

func (o *Orchestrator) resolveImage(ctx context.Context, opts CreateOptions) (string, error) {
	// If image is explicitly provided, use it
	if opts.ImageName != "" {
		return opts.ImageName, nil
	}

	// Auto-detect from devcontainer or use default
	imageName, needsBuild, err := docker.GetProjectImage(o.repoPath)
	if err != nil {
		return "", fmt.Errorf("get project image: %w", err)
	}

	// Check if image exists
	imageExists, err := o.dockerClient.ImageExists(ctx, imageName)
	if err != nil {
		return "", fmt.Errorf("check image exists: %w", err)
	}

	if !imageExists {
		if needsBuild {
			// Need to build the image
			devCfg, err := docker.LoadDevcontainerConfig(o.repoPath)
			if err != nil {
				return "", fmt.Errorf("load devcontainer config: %w", err)
			}

			// Check for devcontainer CLI
			cliStatus := docker.CheckDevcontainerCLI()
			if cliStatus.Available {
				baseImage, err := docker.BuildWithDevcontainerCLI(ctx, o.repoPath, io.Discard)
				if err != nil {
					return "", fmt.Errorf("build with devcontainer CLI: %w", err)
				}
				// Build enhanced image with Claude Code on top
				if err := docker.BuildEnhancedImage(ctx, baseImage, imageName, io.Discard); err != nil {
					return "", fmt.Errorf("build enhanced image: %w", err)
				}
			} else {
				// Fall back to simple docker build
				if err := docker.BuildProjectImage(ctx, o.repoPath, devCfg, io.Discard); err != nil {
					return "", fmt.Errorf("build project image: %w", err)
				}
			}
		} else {
			// Image doesn't exist and doesn't need building - user needs to pull
			return "", fmt.Errorf("image '%s' not found. Run: docker pull %s", imageName, imageName)
		}
	}

	return imageName, nil
}

// containerConfigResult holds the result of building container config
type containerConfigResult struct {
	config            *docker.ContainerConfig
	configDir         string
	gitProxySocketDir string
}

func (o *Orchestrator) buildFullContainerConfig(ws *workstream.Workstream, worktreePath, imageName string, opts CreateOptions) (*containerConfigResult, error) {
	cfg := docker.NewContainerConfig(ws.BranchName, worktreePath)
	cfg.HostGitDir = filepath.Join(o.repoPath, ".git")
	cfg.Image = imageName

	// Load devcontainer config for extra env vars
	devCfg, err := docker.LoadDevcontainerConfig(o.repoPath)
	if err != nil {
		return nil, fmt.Errorf("load devcontainer config: %w", err)
	}
	if devCfg != nil && devCfg.ContainerEnv != nil {
		cfg.ExtraEnv = devCfg.ContainerEnv
	}

	// Create per-container isolated config directory
	// TODO: Pass runtime from CreateOptions once wired up
	configPaths, err := docker.CreateContainerConfig(cfg.Name, "claude")
	if err != nil {
		return nil, fmt.Errorf("create container config: %w", err)
	}

	cfg.ClaudeCfg = configPaths.ClaudeDir
	cfg.ClaudeJSON = configPaths.ClaudeJSON
	cfg.GitConfig = configPaths.GitConfig
	cfg.GitIdentity = docker.GetGitIdentity()
	cfg.Credentials = configPaths.Credentials
	cfg.Timezone = docker.GetHostTimezone()

	// Create git proxy socket directory
	// The socket itself is created later by the TUI's gitproxy server
	gitProxySocketDir := filepath.Join(DefaultGitProxyBaseDir, cfg.Name)
	if err := os.MkdirAll(gitProxySocketDir, 0755); err != nil {
		return nil, fmt.Errorf("create git proxy socket dir: %w", err)
	}
	cfg.GitProxySocketDir = gitProxySocketDir

	// Inject git proxy script into container's Claude settings
	if err := gitproxy.InjectProxyConfig(configPaths.ClaudeDir); err != nil {
		// Non-fatal - log warning and continue without proxy
		// (The container will work but won't have git proxy)
		log.Printf("[orchestrator] Warning: failed to inject git proxy config for %s: %v", configPaths.ClaudeDir, err)
	}

	// Return config dir (parent of ClaudeDir) for credential registration
	configDir := filepath.Dir(configPaths.ClaudeDir)

	return &containerConfigResult{
		config:            cfg,
		configDir:         configDir,
		gitProxySocketDir: gitProxySocketDir,
	}, nil
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

		// Get source file info to preserve permissions
		srcInfo, err := os.Stat(src)
		if err != nil {
			return err
		}

		// Copy file content
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}

		// Write with same permissions as source file
		if err := os.WriteFile(dst, data, srcInfo.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}
