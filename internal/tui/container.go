package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/STRML/claude-cells/internal/docker"
	"github.com/STRML/claude-cells/internal/git"
	"github.com/STRML/claude-cells/internal/sync"
	"github.com/STRML/claude-cells/internal/workstream"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// containerTracker holds the global container tracker for crash recovery
var containerTracker *docker.ContainerTracker

// credentialRefresher holds the global credential refresher for OAuth token updates
var credentialRefresher *docker.CredentialRefresher

// SetContainerTracker sets the container tracker for tracking container lifecycle
func SetContainerTracker(tracker *docker.ContainerTracker) {
	containerTracker = tracker
}

// SetCredentialRefresher sets the credential refresher for OAuth token updates
func SetCredentialRefresher(refresher *docker.CredentialRefresher) {
	credentialRefresher = refresher
}

// trackContainer adds a container to the tracker if available
func trackContainer(containerID, workstreamID, branchName, repoPath string) {
	if containerTracker != nil {
		containerTracker.Track(containerID, workstreamID, branchName, repoPath)
	}
}

// untrackContainer removes a container from the tracker if available
func untrackContainer(containerID string) {
	if containerTracker != nil {
		containerTracker.Untrack(containerID)
	}
}

// registerContainerCredentials registers a container with the credential refresher
func registerContainerCredentials(containerID, containerName, configDir string) {
	if credentialRefresher != nil {
		credentialRefresher.RegisterContainer(containerID, containerName, configDir)
	}
}

// unregisterContainerCredentials removes a container from the credential refresher
func unregisterContainerCredentials(containerID string) {
	if credentialRefresher != nil {
		credentialRefresher.UnregisterContainer(containerID)
	}
}

// ContainerStartedMsg is sent when a container successfully starts.
type ContainerStartedMsg struct {
	WorkstreamID string
	ContainerID  string
	IsResume     bool // True when resuming a saved session (use --continue)
}

// ContainerErrorMsg is sent when container creation/start fails.
type ContainerErrorMsg struct {
	WorkstreamID string
	Error        error
}

// ContainerOutputMsg is sent when there's output from a container.
type ContainerOutputMsg struct {
	WorkstreamID string
	Output       []byte
}

// ContainerStoppedMsg is sent when a container stops.
type ContainerStoppedMsg struct {
	WorkstreamID string
}

// ContainerNotFoundMsg is sent when a container no longer exists but can be rebuilt.
// This triggers automatic rebuild with --continue to resume the Claude session.
type ContainerNotFoundMsg struct {
	WorkstreamID string
}

// PTYReadyMsg is sent when a PTY session is ready for use.
type PTYReadyMsg struct {
	WorkstreamID string
	Session      *PTYSession
}

// ContainerLogsMsg is sent when container logs are fetched.
type ContainerLogsMsg struct {
	WorkstreamID string
	Logs         string
	Error        error
}

// BranchConflictMsg is sent when a branch already exists.
type BranchConflictMsg struct {
	WorkstreamID string
	BranchName   string
	RepoPath     string
	BranchInfo   string // Summary of commits and diff on the existing branch
}

// TitleGeneratedMsg is sent when a workstream title is generated via Claude CLI.
type TitleGeneratedMsg struct {
	WorkstreamID string
	Title        string
	Error        error
}

// UncommittedChangesMsg is sent after checking for uncommitted changes in a worktree.
type UncommittedChangesMsg struct {
	WorkstreamID string
	HasChanges   bool
	BranchInfo   string // Branch statistics (commits, files changed, lines)
	Error        error
}

// CheckUncommittedChangesCmd checks if a worktree has uncommitted changes.
// Also fetches branch statistics for the merge dialog.
func CheckUncommittedChangesCmd(ws *workstream.Workstream) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Use the worktree path if set, otherwise compute it
		worktreePath := resolveWorktreePath(ws)
		if worktreePath == "" {
			return UncommittedChangesMsg{
				WorkstreamID: ws.ID,
				HasChanges:   false,
				Error:        fmt.Errorf("no worktree path"),
			}
		}

		gitRepo := GitClientFactory(worktreePath)
		hasChanges, err := gitRepo.HasUncommittedChanges(ctx)
		if err != nil {
			return UncommittedChangesMsg{
				WorkstreamID: ws.ID,
				HasChanges:   false,
				Error:        err,
			}
		}

		// Fetch branch statistics for the merge dialog
		branchInfo, _ := gitRepo.GetBranchInfo(ctx, ws.BranchName)

		return UncommittedChangesMsg{
			WorkstreamID: ws.ID,
			HasChanges:   hasChanges,
			BranchInfo:   branchInfo,
		}
	}
}

// GenerateTitleCmd returns a command that generates a short title for a workstream
// using the Claude CLI. This runs asynchronously while the container starts.
func GenerateTitleCmd(ws *workstream.Workstream) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Build the prompt for Claude CLI
		prompt := fmt.Sprintf(`Generate a 3-5 word title summarizing this task. Output ONLY the title text, no quotes or explanation.

Task: %s`, ws.Prompt)

		// Run claude CLI with -p flag for non-interactive mode
		cmd := exec.CommandContext(ctx, "claude", "-p", prompt)
		output, err := cmd.Output()
		if err != nil {
			return TitleGeneratedMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}

		// Clean up the output - trim whitespace
		title := strings.TrimSpace(string(output))

		// Limit title length to prevent UI issues
		if len(title) > 80 {
			title = title[:77] + "..."
		}

		return TitleGeneratedMsg{
			WorkstreamID: ws.ID,
			Title:        title,
		}
	}
}

// StartContainerCmd returns a command that creates and starts a container.
// It first creates and checks out a feature branch for the workstream.
func StartContainerCmd(ws *workstream.Workstream) tea.Cmd {
	return startContainerWithOptions(ws, false)
}

// StartContainerWithExistingBranchCmd starts a container using an existing branch.
func StartContainerWithExistingBranchCmd(ws *workstream.Workstream) tea.Cmd {
	return startContainerWithOptions(ws, true)
}

// StartContainerWithNewBranchCmd creates a new branch with a unique name and starts the container.
func StartContainerWithNewBranchCmd(ws *workstream.Workstream, existingBranches []string) tea.Cmd {
	return func() tea.Msg {
		// Generate a unique branch name by appending a suffix
		originalName := ws.BranchName
		newName := originalName
		suffix := 2

		// Check if the generated name conflicts with existing branches
		for {
			conflict := false
			for _, existing := range existingBranches {
				if existing == newName {
					conflict = true
					break
				}
			}
			if !conflict {
				break
			}
			newName = fmt.Sprintf("%s-%d", originalName, suffix)
			suffix++
		}

		// Also check git in case there are branches not in our list
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		repoPath, err := os.Getwd()
		if err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}

		gitRepo := GitClientFactory(repoPath)
		for {
			exists, _ := gitRepo.BranchExists(ctx, newName)
			if !exists {
				break
			}
			newName = fmt.Sprintf("%s-%d", originalName, suffix)
			suffix++
		}

		// Update workstream with new branch name
		ws.BranchName = newName

		// Now start with the new branch name
		return startContainerWithOptions(ws, false)()
	}
}

// DeleteAndRestartContainerCmd deletes the existing branch and creates a new one.
func DeleteAndRestartContainerCmd(ws *workstream.Workstream) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		repoPath, err := os.Getwd()
		if err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}

		gitRepo := GitClientFactory(repoPath)

		// With worktrees, the branch may be checked out in a worktree
		// First remove any worktree using this branch
		cleanupWorktree(ctx, gitRepo, ws.BranchName)

		// Now we can delete the branch (it's no longer checked out anywhere)
		if err := gitRepo.DeleteBranch(ctx, ws.BranchName); err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        fmt.Errorf("failed to delete branch %s: %w", ws.BranchName, err),
			}
		}

		// Now start with a fresh branch (false = create new branch)
		return startContainerWithOptions(ws, false)()
	}
}

// RebuildContainerCmd rebuilds a container for a workstream whose container was lost.
// This reuses the existing branch/worktree and sets IsResume=true so claude --continue is used.
func RebuildContainerCmd(ws *workstream.Workstream) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Get current working directory as repo path
		repoPath, err := os.Getwd()
		if err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}

		gitRepo := GitClientFactory(repoPath)

		// Determine worktree path for this container
		worktreePath := getWorktreePath(ws.BranchName)

		// Ensure the worktrees directory exists
		if err := os.MkdirAll("/tmp/ccells/worktrees", 0755); err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        fmt.Errorf("failed to create worktrees directory: %w", err),
			}
		}

		// Check if worktree exists, create if needed
		if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
			// Worktree doesn't exist - create from existing branch
			if err := gitRepo.CreateWorktreeFromExisting(ctx, worktreePath, ws.BranchName); err != nil {
				return ContainerErrorMsg{
					WorkstreamID: ws.ID,
					Error:        fmt.Errorf("failed to create worktree for branch %s: %w", ws.BranchName, err),
				}
			}
		}

		// Store worktree path in workstream
		ws.WorktreePath = worktreePath

		// Create Docker client
		dockerClient, err := docker.NewClient()
		if err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}
		defer dockerClient.Close()

		// Create container config - mount the WORKTREE
		cfg := docker.NewContainerConfig(ws.BranchName, worktreePath)
		cfg.HostGitDir = repoPath + "/.git"

		// Load devcontainer config and determine image
		devCfg, err := docker.LoadDevcontainerConfig(repoPath)
		if err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        fmt.Errorf("failed to load devcontainer config: %w", err),
			}
		}

		imageName, needsBuild, err := docker.GetProjectImage(repoPath)
		if err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        fmt.Errorf("failed to determine project image: %w", err),
			}
		}

		imageExists, err := dockerClient.ImageExists(ctx, imageName)
		if err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        fmt.Errorf("failed to check image existence: %w", err),
			}
		}

		// Build image if needed
		if !imageExists && needsBuild && devCfg != nil {
			buildCtx, buildCancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer buildCancel()

			cliStatus := docker.CheckDevcontainerCLI()
			if cliStatus.Available {
				baseImage, err := docker.BuildWithDevcontainerCLI(buildCtx, repoPath, io.Discard)
				if err != nil {
					return ContainerErrorMsg{
						WorkstreamID: ws.ID,
						Error:        fmt.Errorf("failed to build with devcontainer CLI: %w", err),
					}
				}
				// Build enhanced image with Claude Code on top
				if err := docker.BuildEnhancedImage(buildCtx, baseImage, imageName, io.Discard); err != nil {
					return ContainerErrorMsg{
						WorkstreamID: ws.ID,
						Error:        fmt.Errorf("failed to build enhanced image: %w", err),
					}
				}
			} else {
				if err := docker.BuildProjectImage(buildCtx, repoPath, devCfg, io.Discard); err != nil {
					return ContainerErrorMsg{
						WorkstreamID: ws.ID,
						Error:        fmt.Errorf("failed to build project image: %w", err),
					}
				}
			}
		} else if !imageExists && !needsBuild {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        fmt.Errorf("image '%s' not found. Run: docker pull %s", imageName, imageName),
			}
		}

		cfg.Image = imageName

		if devCfg != nil && devCfg.ContainerEnv != nil {
			cfg.ExtraEnv = devCfg.ContainerEnv
		}

		configPaths, err := docker.CreateContainerConfig(cfg.Name)
		if err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        fmt.Errorf("failed to create container config: %w", err),
			}
		}
		cfg.ClaudeCfg = configPaths.ClaudeDir
		cfg.ClaudeJSON = configPaths.ClaudeJSON
		cfg.GitConfig = configPaths.GitConfig
		cfg.GitIdentity = docker.GetGitIdentity()
		cfg.Credentials = configPaths.Credentials

		containerID, err := dockerClient.CreateContainer(ctx, cfg)
		if err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}

		err = dockerClient.StartContainer(ctx, containerID)
		if err != nil {
			_ = dockerClient.RemoveContainer(ctx, containerID)
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}

		trackContainer(containerID, ws.ID, ws.BranchName, worktreePath)

		// Register for credential refresh - use parent of ClaudeDir as config dir
		registerContainerCredentials(containerID, cfg.Name, filepath.Dir(configPaths.ClaudeDir))

		// Return with IsResume=true so PTY uses --continue
		return ContainerStartedMsg{
			WorkstreamID: ws.ID,
			ContainerID:  containerID,
			IsResume:     true,
		}
	}
}

// getWorktreePath returns the path for a workstream's worktree.
func getWorktreePath(branchName string) string {
	// Sanitize branch name for filesystem
	safeName := strings.ReplaceAll(branchName, "/", "-")
	safeName = strings.ReplaceAll(safeName, " ", "-")
	return fmt.Sprintf("/tmp/ccells/worktrees/%s", safeName)
}

// resolveWorktreePath returns the worktree path for a workstream,
// using the stored path if available, otherwise computing from branch name.
func resolveWorktreePath(ws *workstream.Workstream) string {
	if ws.WorktreePath != "" {
		return ws.WorktreePath
	}
	if ws.BranchName != "" {
		return getWorktreePath(ws.BranchName)
	}
	return ""
}

// cleanupWorktree removes a worktree and its directory.
// Errors are logged but not returned since cleanup is best-effort.
func cleanupWorktree(ctx context.Context, gitRepo git.GitClient, branchName string) {
	worktreePath := getWorktreePath(branchName)
	if err := gitRepo.RemoveWorktree(ctx, worktreePath); err != nil {
		LogDebug("RemoveWorktree warning: %v", err)
	}
	if err := os.RemoveAll(worktreePath); err != nil {
		LogDebug("RemoveAll warning: %v", err)
	}
}

// pruneEmptyBranches deletes branches that have no commits beyond the base branch.
// Returns the count of branches successfully deleted.
func pruneEmptyBranches(ctx context.Context, g git.GitClient, branches []string) int {
	pruned := 0
	for _, branch := range branches {
		hasCommits, err := g.BranchHasCommits(ctx, branch)
		if err != nil {
			continue // Skip on error
		}
		if !hasCommits {
			if err := g.DeleteBranch(ctx, branch); err == nil {
				pruned++
			}
		}
	}
	return pruned
}

// startContainerWithOptions is the internal implementation for starting containers.
// It uses git worktrees to avoid modifying the host repo's checked out branch.
func startContainerWithOptions(ws *workstream.Workstream, useExistingBranch bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Get current working directory as repo path
		repoPath, err := os.Getwd()
		if err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}

		gitRepo := GitClientFactory(repoPath)

		// Auto-pull main before creating branch to avoid stale base
		// This updates local main to match origin/main without checking it out
		// Errors are non-fatal (e.g., no network, no remote, local changes)
		if !useExistingBranch {
			_ = gitRepo.UpdateMainBranch(ctx)
		}

		// Determine worktree path for this container
		worktreePath := getWorktreePath(ws.BranchName)

		// Ensure the worktrees directory exists
		if err := os.MkdirAll("/tmp/ccells/worktrees", 0755); err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        fmt.Errorf("failed to create worktrees directory: %w", err),
			}
		}

		// IMPORTANT: Check for existing worktree/branch BEFORE any cleanup.
		// This prevents destroying worktree metadata for a running container.
		if !useExistingBranch {
			// Check if there's already an active worktree for this branch
			existingPath, hasWorktree := gitRepo.WorktreeExistsForBranch(ctx, ws.BranchName)
			if hasWorktree {
				// A worktree exists for this branch - don't clean it up!
				branchInfo, _ := gitRepo.GetBranchInfo(ctx, ws.BranchName)
				return BranchConflictMsg{
					WorkstreamID: ws.ID,
					BranchName:   ws.BranchName,
					RepoPath:     repoPath,
					BranchInfo:   fmt.Sprintf("Active worktree at: %s\n%s", existingPath, branchInfo),
				}
			}

			// Check if branch exists (even without a worktree)
			exists, _ := gitRepo.BranchExists(ctx, ws.BranchName)
			if exists {
				// Get branch info (commits and diff stats)
				branchInfo, _ := gitRepo.GetBranchInfo(ctx, ws.BranchName)

				// Branch already exists - ask user what to do
				return BranchConflictMsg{
					WorkstreamID: ws.ID,
					BranchName:   ws.BranchName,
					RepoPath:     repoPath,
					BranchInfo:   branchInfo,
				}
			}
		}

		// Now safe to clean up any stale worktree at this path (no active worktree for this branch)
		cleanupWorktree(ctx, gitRepo, ws.BranchName)

		if useExistingBranch {
			// Create worktree from existing branch (don't create new branch)
			if err := gitRepo.CreateWorktreeFromExisting(ctx, worktreePath, ws.BranchName); err != nil {
				return ContainerErrorMsg{
					WorkstreamID: ws.ID,
					Error:        fmt.Errorf("failed to create worktree for branch %s: %w", ws.BranchName, err),
				}
			}
		} else {
			// Create worktree with new branch (git worktree add -b <branch> <path>)
			if err := gitRepo.CreateWorktree(ctx, worktreePath, ws.BranchName); err != nil {
				return ContainerErrorMsg{
					WorkstreamID: ws.ID,
					Error:        fmt.Errorf("failed to create worktree for branch %s: %w", ws.BranchName, err),
				}
			}
		}

		// Store worktree path in workstream for later cleanup
		ws.WorktreePath = worktreePath

		// Create Docker client
		dockerClient, err := docker.NewClient()
		if err != nil {
			// Clean up worktree on failure
			cleanupWorktree(ctx, gitRepo, ws.BranchName)
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}
		defer dockerClient.Close()

		// Create container config - mount the WORKTREE, not the main repo
		cfg := docker.NewContainerConfig(ws.BranchName, worktreePath)
		// Mount host repo's .git directory so worktree references resolve correctly
		cfg.HostGitDir = repoPath + "/.git"

		// Load devcontainer config and determine image
		devCfg, err := docker.LoadDevcontainerConfig(repoPath)
		if err != nil {
			cleanupWorktree(ctx, gitRepo, ws.BranchName)
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        fmt.Errorf("failed to load devcontainer config: %w", err),
			}
		}

		// Get the image to use
		imageName, needsBuild, err := docker.GetProjectImage(repoPath)
		if err != nil {
			cleanupWorktree(ctx, gitRepo, ws.BranchName)
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        fmt.Errorf("failed to determine project image: %w", err),
			}
		}

		// Check if image exists
		imageExists, err := dockerClient.ImageExists(ctx, imageName)
		if err != nil {
			cleanupWorktree(ctx, gitRepo, ws.BranchName)
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        fmt.Errorf("failed to check image existence: %w", err),
			}
		}

		// Build image if needed
		if !imageExists && needsBuild && devCfg != nil {
			// Use a longer timeout for build
			buildCtx, buildCancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer buildCancel()

			// Use devcontainer CLI if available for proper feature support
			cliStatus := docker.CheckDevcontainerCLI()
			if cliStatus.Available {
				baseImage, err := docker.BuildWithDevcontainerCLI(buildCtx, repoPath, io.Discard)
				if err != nil {
					cleanupWorktree(ctx, gitRepo, ws.BranchName)
					return ContainerErrorMsg{
						WorkstreamID: ws.ID,
						Error:        fmt.Errorf("failed to build with devcontainer CLI: %w", err),
					}
				}
				// Build enhanced image with Claude Code on top
				if err := docker.BuildEnhancedImage(buildCtx, baseImage, imageName, io.Discard); err != nil {
					cleanupWorktree(ctx, gitRepo, ws.BranchName)
					return ContainerErrorMsg{
						WorkstreamID: ws.ID,
						Error:        fmt.Errorf("failed to build enhanced image: %w", err),
					}
				}
			} else {
				// Fall back to simple docker build (handles both Dockerfile and image-only configs)
				if err := docker.BuildProjectImage(buildCtx, repoPath, devCfg, io.Discard); err != nil {
					cleanupWorktree(ctx, gitRepo, ws.BranchName)
					return ContainerErrorMsg{
						WorkstreamID: ws.ID,
						Error:        fmt.Errorf("failed to build project image: %w", err),
					}
				}
			}
		} else if !imageExists && !needsBuild {
			// Image doesn't exist and doesn't need building - should pull
			cleanupWorktree(ctx, gitRepo, ws.BranchName)
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        fmt.Errorf("image '%s' not found. Run: docker pull %s", imageName, imageName),
			}
		}

		cfg.Image = imageName

		// Apply containerEnv from devcontainer.json
		if devCfg != nil && devCfg.ContainerEnv != nil {
			cfg.ExtraEnv = devCfg.ContainerEnv
		}

		// Create per-container isolated config directory
		// This prevents race conditions when multiple containers modify credentials
		configPaths, err := docker.CreateContainerConfig(cfg.Name)
		if err != nil {
			// Clean up worktree on failure
			cleanupWorktree(ctx, gitRepo, ws.BranchName)
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        fmt.Errorf("failed to create container config: %w", err),
			}
		}
		cfg.ClaudeCfg = configPaths.ClaudeDir
		cfg.ClaudeJSON = configPaths.ClaudeJSON
		cfg.GitConfig = configPaths.GitConfig
		cfg.GitIdentity = docker.GetGitIdentity()
		cfg.Credentials = configPaths.Credentials

		// Create container
		containerID, err := dockerClient.CreateContainer(ctx, cfg)
		if err != nil {
			// Clean up worktree on failure
			cleanupWorktree(ctx, gitRepo, ws.BranchName)
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}

		// Start container
		err = dockerClient.StartContainer(ctx, containerID)
		if err != nil {
			// Clean up created container and worktree on start failure
			_ = dockerClient.RemoveContainer(ctx, containerID)
			cleanupWorktree(ctx, gitRepo, ws.BranchName)
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}

		// Track the container for crash recovery
		trackContainer(containerID, ws.ID, ws.BranchName, worktreePath)

		// Register for credential refresh - use parent of ClaudeDir as config dir
		registerContainerCredentials(containerID, cfg.Name, filepath.Dir(configPaths.ClaudeDir))

		return ContainerStartedMsg{
			WorkstreamID: ws.ID,
			ContainerID:  containerID,
		}
	}
}

// StartPTYCmd returns a command that starts a PTY session in a container.
// If isResume is true, uses 'claude --resume <session_id>' (or --continue as fallback).
func StartPTYCmd(ws *workstream.Workstream, initialPrompt string, width, height int, isResume bool) tea.Cmd {
	return func() tea.Msg {
		// Use a timeout for PTY session creation
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}

		// Get host project path for session data copying
		hostProjectPath, _ := os.Getwd()

		// Build PTY options with terminal size
		opts := &PTYOptions{
			Width:           width,
			Height:          height,
			IsResume:        isResume,
			ClaudeSessionID: ws.GetClaudeSessionID(), // Pass session ID for --resume
			HostProjectPath: hostProjectPath,
		}

		// Pass through ANTHROPIC_API_KEY if set (fallback for non-OAuth auth)
		if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
			opts.EnvVars = append(opts.EnvVars, "ANTHROPIC_API_KEY="+apiKey)
		}

		// Get OAuth credentials from keychain and pass as env var
		creds, err := docker.GetClaudeCredentials()
		if err == nil && creds != nil && creds.Raw != "" {
			// Pass the raw credentials JSON - Claude Code will parse it
			opts.EnvVars = append(opts.EnvVars, "CLAUDE_CODE_CREDENTIALS="+creds.Raw)
		}

		// Disable Claude Code auto-updater, error reporting, and telemetry
		opts.EnvVars = append(opts.EnvVars,
			"DISABLE_AUTOUPDATER=1",
			"DISABLE_ERROR_REPORTING=1",
			"DISABLE_TELEMETRY=1",
		)

		session, err := NewPTYSession(ctx, dockerClient, ws.ContainerID, ws.ID, initialPrompt, opts)
		if err != nil {
			dockerClient.Close()
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}

		return PTYReadyMsg{
			WorkstreamID: ws.ID,
			Session:      session,
		}
	}
}

// StopContainerCmd returns a command that stops and removes a container.
func StopContainerCmd(ws *workstream.Workstream) tea.Cmd {
	return func() tea.Msg {
		LogDebug("StopContainerCmd started for %s", ws.BranchName)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Stop and remove container if it exists
		if ws.ContainerID != "" {
			containerShort := ws.ContainerID
			if len(containerShort) > 12 {
				containerShort = containerShort[:12]
			}
			LogDebug("Stopping container %s", containerShort)
			client, err := docker.NewClient()
			if err == nil {
				_ = client.StopContainer(ctx, ws.ContainerID)
				LogDebug("Container stopped, removing with config cleanup")
				_ = client.RemoveContainerAndConfig(ctx, ws.ContainerID)
				LogDebug("Container and config removed")
				client.Close()
			}
			// Untrack the container since it's been removed
			untrackContainer(ws.ContainerID)
			// Unregister from credential refresh
			unregisterContainerCredentials(ws.ContainerID)
		}
		LogDebug("Container cleanup done")

		// Clean up the worktree
		worktreePath := resolveWorktreePath(ws)
		LogDebug("Worktree path: %s", worktreePath)

		// Get repo path once for all git operations
		repoPath, err := os.Getwd()
		if err != nil {
			LogWarn("Failed to get cwd: %v", err)
			return ContainerStoppedMsg{WorkstreamID: ws.ID}
		}
		LogDebug("Repo path: %s", repoPath)

		if worktreePath != "" {
			gitRepo := GitClientFactory(repoPath)

			LogDebug("Removing worktree")
			// Remove worktree from git
			if err := gitRepo.RemoveWorktree(ctx, worktreePath); err != nil {
				LogWarn("RemoveWorktree error (continuing): %v", err)
			}
			LogDebug("Worktree removed from git")

			// Remove the directory (git worktree remove doesn't always delete the dir)
			LogDebug("Removing directory %s", worktreePath)
			if err := os.RemoveAll(worktreePath); err != nil {
				LogWarn("RemoveAll error for %s: %v", worktreePath, err)
			}
			LogDebug("Directory removed")
		}

		// Delete branch if it has no commits or is merged
		if ws.BranchName != "" {
			gitRepo := GitClientFactory(repoPath)
			LogDebug("Checking if branch has commits")
			// Check if branch has commits
			hasCommits, err := gitRepo.BranchHasCommits(ctx, ws.BranchName)
			if err != nil {
				LogWarn("BranchHasCommits error: %v", err)
				return ContainerStoppedMsg{WorkstreamID: ws.ID}
			}
			LogDebug("Branch hasCommits=%v", hasCommits)
			if !hasCommits {
				LogDebug("Deleting empty branch")
				if err := gitRepo.DeleteBranch(ctx, ws.BranchName); err != nil {
					LogWarn("DeleteBranch error for %s: %v", ws.BranchName, err)
				}
				LogDebug("Branch deleted")
			}
		}

		LogDebug("StopContainerCmd completed for %s", ws.BranchName)
		return ContainerStoppedMsg{WorkstreamID: ws.ID}
	}
}

// FetchContainerLogsCmd returns a command that fetches container logs.
func FetchContainerLogsCmd(ws *workstream.Workstream) tea.Cmd {
	return func() tea.Msg {
		if ws.ContainerID == "" {
			return ContainerLogsMsg{
				WorkstreamID: ws.ID,
				Logs:         "",
				Error:        nil,
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return ContainerLogsMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}
		defer dockerClient.Close()

		// Get container logs
		options := container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Tail:       "100", // Last 100 lines
		}

		reader, err := dockerClient.ContainerLogs(ctx, ws.ContainerID, options)
		if err != nil {
			return ContainerLogsMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}
		defer reader.Close()

		logs, err := io.ReadAll(reader)
		if err != nil {
			return ContainerLogsMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}

		return ContainerLogsMsg{
			WorkstreamID: ws.ID,
			Logs:         string(logs),
		}
	}
}

// PruneStoppedContainersCmd returns a command that prunes stopped ccells containers.
func PruneStoppedContainersCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := docker.NewClient()
		if err != nil {
			return PruneResultMsg{Error: err}
		}
		defer client.Close()

		count, err := client.PruneDockerTUIContainers(ctx)
		if err != nil {
			return PruneResultMsg{Error: err}
		}

		return PruneResultMsg{PrunedCount: count}
	}
}

// PruneAllContainersCmd returns a command that prunes ALL ccells containers.
func PruneAllContainersCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := docker.NewClient()
		if err != nil {
			return PruneResultMsg{Error: err}
		}
		defer client.Close()

		count, err := client.PruneAllDockerTUIContainers(ctx)
		if err != nil {
			return PruneResultMsg{Error: err}
		}

		return PruneResultMsg{PrunedCount: count}
	}
}

// PruneAllResultMsg is sent when the full prune all operation completes.
type PruneAllResultMsg struct {
	ContainersPruned int
	BranchesPruned   int
	Error            error
}

// PruneAllContainersAndBranchesCmd returns a command that prunes ALL ccells containers
// and deletes any ccells branches with no commits.
func PruneAllContainersAndBranchesCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		var containersPruned int
		var branchesPruned int

		// First, prune all containers
		client, err := docker.NewClient()
		if err != nil {
			return PruneAllResultMsg{Error: err}
		}
		defer client.Close()

		containersPruned, err = client.PruneAllDockerTUIContainers(ctx)
		if err != nil {
			return PruneAllResultMsg{
				ContainersPruned: containersPruned,
				Error:            err,
			}
		}

		// Get current working directory for git operations
		cwd, err := os.Getwd()
		if err != nil {
			return PruneAllResultMsg{
				ContainersPruned: containersPruned,
				Error:            err,
			}
		}

		// Now prune empty branches
		g := GitClientFactory(cwd)
		branches, err := g.ListCCellsBranches(ctx)
		if err != nil {
			// Not fatal - still report container cleanup
			return PruneAllResultMsg{
				ContainersPruned: containersPruned,
				BranchesPruned:   0,
				Error:            nil, // Don't fail on branch listing error
			}
		}

		branchesPruned = pruneEmptyBranches(ctx, g, branches)

		return PruneAllResultMsg{
			ContainersPruned: containersPruned,
			BranchesPruned:   branchesPruned,
		}
	}
}

// PruneProjectContainersAndBranchesCmd returns a command that prunes ccells containers
// for a specific project and deletes any ccells branches with no commits.
func PruneProjectContainersAndBranchesCmd(projectName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		var containersPruned int
		var branchesPruned int

		// First, prune containers for this project only
		client, err := docker.NewClient()
		if err != nil {
			return PruneAllResultMsg{Error: err}
		}
		defer client.Close()

		containersPruned, err = client.PruneAllDockerTUIContainersForProject(ctx, projectName)
		if err != nil {
			return PruneAllResultMsg{
				ContainersPruned: containersPruned,
				Error:            err,
			}
		}

		// Get current working directory for git operations
		cwd, err := os.Getwd()
		if err != nil {
			return PruneAllResultMsg{
				ContainersPruned: containersPruned,
				Error:            err,
			}
		}

		// Now prune empty branches (these are repo-local anyway)
		g := GitClientFactory(cwd)
		branches, err := g.ListCCellsBranches(ctx)
		if err != nil {
			// Not fatal - still report container cleanup
			return PruneAllResultMsg{
				ContainersPruned: containersPruned,
				BranchesPruned:   0,
				Error:            nil, // Don't fail on branch listing error
			}
		}

		branchesPruned = pruneEmptyBranches(ctx, g, branches)

		return PruneAllResultMsg{
			ContainersPruned: containersPruned,
			BranchesPruned:   branchesPruned,
		}
	}
}

// ListContainersCmd returns a command that counts ccells containers.
func ListContainersCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client, err := docker.NewClient()
		if err != nil {
			return ContainerCountMsg{Error: err}
		}
		defer client.Close()

		containers, err := client.ListDockerTUIContainers(ctx)
		if err != nil {
			return ContainerCountMsg{Error: err}
		}

		return ContainerCountMsg{Count: len(containers)}
	}
}

// ContainerCountMsg is sent with the count of ccells containers.
type ContainerCountMsg struct {
	Count int
	Error error
}

// PauseContainerCmd returns a command that pauses a container.
func PauseContainerCmd(containerID string) tea.Cmd {
	return func() tea.Msg {
		if containerID == "" {
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client, err := docker.NewClient()
		if err != nil {
			return nil
		}
		defer client.Close()

		_ = client.PauseContainer(ctx, containerID)
		return nil
	}
}

// ResumeContainerCmd returns a command that unpauses a container and starts a PTY session.
func ResumeContainerCmd(ws *workstream.Workstream, width, height int) tea.Cmd {
	return func() tea.Msg {
		if ws.ContainerID == "" {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        fmt.Errorf("no container ID"),
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		dockerClient, err := docker.NewClient()
		if err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}

		// Check container state
		state, err := dockerClient.GetContainerState(ctx, ws.ContainerID)
		if err != nil {
			dockerClient.Close()
			// Container no longer exists - trigger rebuild
			return ContainerNotFoundMsg{
				WorkstreamID: ws.ID,
			}
		}

		// Unpause if paused
		if state == "paused" {
			if err := dockerClient.UnpauseContainer(ctx, ws.ContainerID); err != nil {
				dockerClient.Close()
				return ContainerErrorMsg{
					WorkstreamID: ws.ID,
					Error:        fmt.Errorf("failed to unpause: %w", err),
				}
			}
		} else if state != "running" {
			// Container is not in a resumable state
			dockerClient.Close()
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        fmt.Errorf("container is %s, cannot resume", state),
			}
		}

		dockerClient.Close()

		// Track the resumed container for crash recovery
		repoPath, _ := os.Getwd()
		trackContainer(ws.ContainerID, ws.ID, ws.BranchName, repoPath)

		// Container is running, notify success (resuming existing session)
		return ContainerStartedMsg{
			WorkstreamID: ws.ID,
			ContainerID:  ws.ContainerID,
			IsResume:     true,
		}
	}
}

// PushBranchResultMsg is sent when branch push completes.
type PushBranchResultMsg struct {
	WorkstreamID string
	Error        error
}

// PushBranchCmd returns a command that pushes a branch to origin.
func PushBranchCmd(ws *workstream.Workstream) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Use worktree path for git operations
		worktreePath := resolveWorktreePath(ws)
		if worktreePath == "" {
			return PushBranchResultMsg{WorkstreamID: ws.ID, Error: fmt.Errorf("no worktree path for branch")}
		}

		gitRepo := GitClientFactory(worktreePath)
		if err := gitRepo.Push(ctx, ws.BranchName); err != nil {
			return PushBranchResultMsg{WorkstreamID: ws.ID, Error: err}
		}

		return PushBranchResultMsg{WorkstreamID: ws.ID, Error: nil}
	}
}

// PRCreatedMsg is sent when a PR is created.
type PRCreatedMsg struct {
	WorkstreamID string
	PRURL        string
	PRNumber     int
	Error        error
}

// CreatePRCmd returns a command that creates a pull request.
func CreatePRCmd(ws *workstream.Workstream) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		// Use worktree path for git operations - that's where the branch is checked out
		worktreePath := resolveWorktreePath(ws)
		if worktreePath == "" {
			return PRCreatedMsg{WorkstreamID: ws.ID, Error: fmt.Errorf("no worktree path for branch")}
		}

		gitRepo := GitClientFactory(worktreePath)

		// First push the branch from the worktree
		if err := gitRepo.Push(ctx, ws.BranchName); err != nil {
			return PRCreatedMsg{WorkstreamID: ws.ID, Error: fmt.Errorf("failed to push branch: %w", err)}
		}

		// Create PR using gh CLI - run from worktree so it picks up the right branch
		gh := git.NewGH()

		// Generate PR title and body from the workstream prompt
		prTitle := ws.BranchName
		prBody := fmt.Sprintf("## Summary\n\n%s\n\n## Changes\n\nCreated by [claude-cells](https://github.com/STRML/claude-cells).", ws.Prompt)

		pr, err := gh.CreatePR(ctx, worktreePath, &git.PRRequest{
			Title: prTitle,
			Body:  prBody,
			Head:  ws.BranchName, // Explicitly specify branch for worktrees
		})
		if err != nil {
			return PRCreatedMsg{WorkstreamID: ws.ID, Error: err}
		}

		return PRCreatedMsg{
			WorkstreamID: ws.ID,
			PRURL:        pr.URL,
			PRNumber:     pr.Number,
		}
	}
}

// MergeBranchMsg is sent when a branch merge completes.
type MergeBranchMsg struct {
	WorkstreamID  string
	Error         error
	ConflictFiles []string // Files with conflicts (if any)
}

// MergeBranchCmd returns a command that merges a branch into main.
func MergeBranchCmd(ws *workstream.Workstream) tea.Cmd {
	return MergeBranchWithOptionsCmd(ws, false)
}

// SquashMergeBranchCmd returns a command that squash merges a branch into main.
func SquashMergeBranchCmd(ws *workstream.Workstream) tea.Cmd {
	return MergeBranchWithOptionsCmd(ws, true)
}

// MergeBranchWithOptionsCmd returns a command that merges a branch into main with optional squash.
func MergeBranchWithOptionsCmd(ws *workstream.Workstream, squash bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		repoPath, err := os.Getwd()
		if err != nil {
			return MergeBranchMsg{WorkstreamID: ws.ID, Error: err}
		}

		gitRepo := GitClientFactory(repoPath)

		// Merge the branch into main
		if err := gitRepo.MergeBranchWithOptions(ctx, ws.BranchName, squash); err != nil {
			// Check if it's a conflict error
			if conflictErr, ok := err.(*git.MergeConflictError); ok {
				return MergeBranchMsg{
					WorkstreamID:  ws.ID,
					Error:         err,
					ConflictFiles: conflictErr.ConflictFiles,
				}
			}
			return MergeBranchMsg{WorkstreamID: ws.ID, Error: fmt.Errorf("failed to merge branch: %w", err)}
		}

		return MergeBranchMsg{WorkstreamID: ws.ID}
	}
}

// RebaseBranchMsg is sent when a rebase completes.
type RebaseBranchMsg struct {
	WorkstreamID  string
	Error         error
	ConflictFiles []string // Files with conflicts (if rebase stopped)
}

// RebaseBranchCmd returns a command that rebases a branch onto main.
func RebaseBranchCmd(ws *workstream.Workstream) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		// Use the worktree path if available
		worktreePath := resolveWorktreePath(ws)
		gitRepo := GitClientFactory(worktreePath)

		// Rebase the branch onto main
		if err := gitRepo.RebaseBranch(ctx, ws.BranchName); err != nil {
			// Check if it's a conflict error
			if conflictErr, ok := err.(*git.MergeConflictError); ok {
				return RebaseBranchMsg{
					WorkstreamID:  ws.ID,
					Error:         err,
					ConflictFiles: conflictErr.ConflictFiles,
				}
			}
			return RebaseBranchMsg{WorkstreamID: ws.ID, Error: err}
		}

		return RebaseBranchMsg{WorkstreamID: ws.ID}
	}
}

// PairingEnabledMsg is sent when pairing mode is enabled.
type PairingEnabledMsg struct {
	WorkstreamID   string
	StashedChanges bool
	PreviousBranch string
	Error          error
}

// PairingDisabledMsg is sent when pairing mode is disabled.
type PairingDisabledMsg struct {
	WorkstreamID   string
	StashedChanges bool // True if user should run git stash pop
	Error          error
}

// EnablePairingCmd returns a command that enables pairing mode for a workstream.
func EnablePairingCmd(orchestrator *sync.Pairing, ws *workstream.Workstream, previousBranch string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		repoPath, err := os.Getwd()
		if err != nil {
			return PairingEnabledMsg{WorkstreamID: ws.ID, Error: err}
		}

		// Check prerequisites first (is mutagen installed?)
		if err := orchestrator.CheckPrerequisites(ctx); err != nil {
			return PairingEnabledMsg{WorkstreamID: ws.ID, Error: err}
		}

		// Use orchestrator to enable pairing
		if err := orchestrator.Enable(ctx, ws.BranchName, ws.ContainerID, repoPath, previousBranch); err != nil {
			return PairingEnabledMsg{WorkstreamID: ws.ID, Error: err}
		}

		// Get stashed status from orchestrator state
		state := orchestrator.GetState()

		return PairingEnabledMsg{
			WorkstreamID:   ws.ID,
			StashedChanges: state.StashedChanges,
			PreviousBranch: state.PreviousBranch,
		}
	}
}

// DisablePairingCmd returns a command that disables pairing mode.
func DisablePairingCmd(orchestrator *sync.Pairing, ws *workstream.Workstream) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Get state before disabling (to know if we should remind about stash pop)
		state := orchestrator.GetState()

		// Use orchestrator to disable pairing
		if err := orchestrator.Disable(ctx); err != nil {
			return PairingDisabledMsg{
				WorkstreamID:   ws.ID,
				StashedChanges: state.StashedChanges,
				Error:          err,
			}
		}

		return PairingDisabledMsg{
			WorkstreamID:   ws.ID,
			StashedChanges: state.StashedChanges,
		}
	}
}

// PairingSyncHealthMsg is sent when pairing sync health is checked.
type PairingSyncHealthMsg struct {
	Healthy   bool
	Conflicts []string
	Error     error
}

// CheckPairingSyncHealthCmd returns a command that checks pairing sync health.
func CheckPairingSyncHealthCmd(orchestrator *sync.Pairing) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := orchestrator.CheckSyncHealth(ctx)
		healthy, conflicts := orchestrator.GetSyncHealth()

		return PairingSyncHealthMsg{
			Healthy:   healthy,
			Conflicts: conflicts,
			Error:     err,
		}
	}
}

// ResourceStatsMsg is sent when resource stats are fetched.
type ResourceStatsMsg struct {
	Stats       []docker.ContainerStats
	TotalCPU    float64
	TotalMemory uint64
	DiskUsage   *docker.DiskUsage
	IsGlobal    bool
	Error       error
}

// FetchResourceStatsCmd returns a command that fetches resource usage statistics.
func FetchResourceStatsCmd(global bool, projectContainerIDs []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := docker.NewClient()
		if err != nil {
			return ResourceStatsMsg{IsGlobal: global, Error: err}
		}
		defer client.Close()

		var stats []docker.ContainerStats
		if global {
			stats, err = client.GetAllCCellsStats(ctx)
		} else {
			stats, err = client.GetProjectCCellsStats(ctx, projectContainerIDs)
		}

		if err != nil {
			return ResourceStatsMsg{IsGlobal: global, Error: err}
		}

		// Calculate totals
		var totalCPU float64
		var totalMemory uint64
		for _, s := range stats {
			totalCPU += s.CPUPercent
			totalMemory += s.MemoryUsage
		}

		// Get disk usage (don't fail if this errors)
		diskUsage, _ := client.GetDiskUsage(ctx)

		return ResourceStatsMsg{
			Stats:       stats,
			TotalCPU:    totalCPU,
			TotalMemory: totalMemory,
			DiskUsage:   diskUsage,
			IsGlobal:    global,
		}
	}
}
