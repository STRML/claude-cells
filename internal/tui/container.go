package tui

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/STRML/claude-cells/internal/docker"
	"github.com/STRML/claude-cells/internal/git"
	"github.com/STRML/claude-cells/internal/sync"
	"github.com/STRML/claude-cells/internal/workstream"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// containerTracker holds the global container tracker for crash recovery
var containerTracker *docker.ContainerTracker

// SetContainerTracker sets the container tracker for tracking container lifecycle
func SetContainerTracker(tracker *docker.ContainerTracker) {
	containerTracker = tracker
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

// ContainerStartedMsg is sent when a container successfully starts.
type ContainerStartedMsg struct {
	WorkstreamID string
	ContainerID  string
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

		gitRepo := git.New(repoPath)
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

		gitRepo := git.New(repoPath)

		// With worktrees, the branch may be checked out in a worktree
		// First remove any worktree using this branch
		worktreePath := getWorktreePath(ws.BranchName)
		_ = gitRepo.RemoveWorktree(ctx, worktreePath)
		_ = os.RemoveAll(worktreePath)

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

// getWorktreePath returns the path for a workstream's worktree.
func getWorktreePath(branchName string) string {
	// Sanitize branch name for filesystem
	safeName := strings.ReplaceAll(branchName, "/", "-")
	safeName = strings.ReplaceAll(safeName, " ", "-")
	return fmt.Sprintf("/tmp/ccells/worktrees/%s", safeName)
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

		gitRepo := git.New(repoPath)

		// Determine worktree path for this container
		worktreePath := getWorktreePath(ws.BranchName)

		// Ensure the worktrees directory exists
		if err := os.MkdirAll("/tmp/ccells/worktrees", 0755); err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        fmt.Errorf("failed to create worktrees directory: %w", err),
			}
		}

		// Clean up any existing worktree at this path (in case of previous crash)
		_ = gitRepo.RemoveWorktree(ctx, worktreePath)
		_ = os.RemoveAll(worktreePath)

		if useExistingBranch {
			// Create worktree from existing branch (don't create new branch)
			if err := gitRepo.CreateWorktreeFromExisting(ctx, worktreePath, ws.BranchName); err != nil {
				return ContainerErrorMsg{
					WorkstreamID: ws.ID,
					Error:        fmt.Errorf("failed to create worktree for branch %s: %w", ws.BranchName, err),
				}
			}
		} else {
			// Check if branch already exists
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
			_ = gitRepo.RemoveWorktree(ctx, worktreePath)
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}
		defer dockerClient.Close()

		// Create container config - mount the WORKTREE, not the main repo
		cfg := docker.NewContainerConfig(ws.BranchName, worktreePath)
		cfg.Image = docker.RequiredImage
		// Mount host repo's .git directory so worktree references resolve correctly
		cfg.HostGitDir = repoPath + "/.git"

		// Create per-container isolated config directory
		// This prevents race conditions when multiple containers modify credentials
		configPaths, err := docker.CreateContainerConfig(cfg.Name)
		if err != nil {
			// Clean up worktree on failure
			_ = gitRepo.RemoveWorktree(ctx, worktreePath)
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        fmt.Errorf("failed to create container config: %w", err),
			}
		}
		cfg.ClaudeCfg = configPaths.ClaudeDir
		cfg.ClaudeJSON = configPaths.ClaudeJSON
		cfg.GitConfig = configPaths.GitConfig
		cfg.Credentials = configPaths.Credentials

		// Create container
		containerID, err := dockerClient.CreateContainer(ctx, cfg)
		if err != nil {
			// Clean up worktree on failure
			_ = gitRepo.RemoveWorktree(ctx, worktreePath)
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
			_ = gitRepo.RemoveWorktree(ctx, worktreePath)
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}

		// Track the container for crash recovery
		trackContainer(containerID, ws.ID, ws.BranchName, worktreePath)

		return ContainerStartedMsg{
			WorkstreamID: ws.ID,
			ContainerID:  containerID,
		}
	}
}

// StartPTYCmd returns a command that starts a PTY session in a container.
func StartPTYCmd(ws *workstream.Workstream, initialPrompt string, width, height int) tea.Cmd {
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

		// Build PTY options with terminal size
		opts := &PTYOptions{
			Width:  width,
			Height: height,
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

func init() {
	// Configure log to include timestamps
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
}

// StopContainerCmd returns a command that stops and removes a container.
func StopContainerCmd(ws *workstream.Workstream) tea.Cmd {
	return func() tea.Msg {
		log.Printf("[DEBUG] StopContainerCmd started for %s", ws.BranchName)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Stop and remove container if it exists
		if ws.ContainerID != "" {
			containerShort := ws.ContainerID
			if len(containerShort) > 12 {
				containerShort = containerShort[:12]
			}
			log.Printf("[DEBUG] Stopping container %s", containerShort)
			client, err := docker.NewClient()
			if err == nil {
				_ = client.StopContainer(ctx, ws.ContainerID)
				log.Printf("[DEBUG] Container stopped, removing")
				_ = client.RemoveContainer(ctx, ws.ContainerID)
				log.Printf("[DEBUG] Container removed")
				client.Close()
			}
			// Untrack the container since it's been removed
			untrackContainer(ws.ContainerID)
		}
		log.Printf("[DEBUG] Container cleanup done")

		// Clean up the worktree
		// Compute path from branch name if not set (e.g., restored from state)
		worktreePath := ws.WorktreePath
		if worktreePath == "" && ws.BranchName != "" {
			worktreePath = getWorktreePath(ws.BranchName)
		}
		log.Printf("[DEBUG] Worktree path: %s", worktreePath)

		// Get repo path once for all git operations
		repoPath, err := os.Getwd()
		if err != nil {
			log.Printf("[DEBUG] Failed to get cwd: %v", err)
			return ContainerStoppedMsg{WorkstreamID: ws.ID}
		}
		log.Printf("[DEBUG] Repo path: %s", repoPath)

		if worktreePath != "" {
			gitRepo := git.New(repoPath)

			log.Printf("[DEBUG] Removing worktree")
			// Remove worktree from git
			if err := gitRepo.RemoveWorktree(ctx, worktreePath); err != nil {
				log.Printf("[DEBUG] RemoveWorktree error (continuing): %v", err)
			}
			log.Printf("[DEBUG] Worktree removed from git")

			// Remove the directory (git worktree remove doesn't always delete the dir)
			log.Printf("[DEBUG] Removing directory %s", worktreePath)
			if err := os.RemoveAll(worktreePath); err != nil {
				log.Printf("[DEBUG] RemoveAll error for %s: %v", worktreePath, err)
			}
			log.Printf("[DEBUG] Directory removed")
		}

		// Delete branch if it has no commits or is merged
		if ws.BranchName != "" {
			gitRepo := git.New(repoPath)
			log.Printf("[DEBUG] Checking if branch has commits")
			// Check if branch has commits
			hasCommits, err := gitRepo.BranchHasCommits(ctx, ws.BranchName)
			if err != nil {
				log.Printf("[DEBUG] BranchHasCommits error: %v", err)
				return ContainerStoppedMsg{WorkstreamID: ws.ID}
			}
			log.Printf("[DEBUG] Branch hasCommits=%v", hasCommits)
			if !hasCommits {
				log.Printf("[DEBUG] Deleting empty branch")
				if err := gitRepo.DeleteBranch(ctx, ws.BranchName); err != nil {
					log.Printf("[DEBUG] DeleteBranch error for %s: %v", ws.BranchName, err)
				}
				log.Printf("[DEBUG] Branch deleted")
			}
		}

		log.Printf("[DEBUG] StopContainerCmd completed for %s", ws.BranchName)
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
		g := git.New(cwd)
		branches, err := g.ListCCellsBranches(ctx)
		if err != nil {
			// Not fatal - still report container cleanup
			return PruneAllResultMsg{
				ContainersPruned: containersPruned,
				BranchesPruned:   0,
				Error:            nil, // Don't fail on branch listing error
			}
		}

		for _, branch := range branches {
			hasCommits, err := g.BranchHasCommits(ctx, branch)
			if err != nil {
				continue // Skip on error
			}
			if !hasCommits {
				if err := g.DeleteBranch(ctx, branch); err == nil {
					branchesPruned++
				}
			}
		}

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
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        fmt.Errorf("container not found: %w", err),
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

		// Container is running, notify success
		return ContainerStartedMsg{
			WorkstreamID: ws.ID,
			ContainerID:  ws.ContainerID,
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

		repoPath, err := os.Getwd()
		if err != nil {
			return PushBranchResultMsg{WorkstreamID: ws.ID, Error: err}
		}

		gitRepo := git.New(repoPath)
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

		repoPath, err := os.Getwd()
		if err != nil {
			return PRCreatedMsg{WorkstreamID: ws.ID, Error: err}
		}

		gitRepo := git.New(repoPath)

		// First push the branch
		if err := gitRepo.Push(ctx, ws.BranchName); err != nil {
			return PRCreatedMsg{WorkstreamID: ws.ID, Error: fmt.Errorf("failed to push branch: %w", err)}
		}

		// Create PR using gh CLI
		gh := git.NewGH()

		// Generate PR title and body from the workstream prompt
		prTitle := ws.BranchName
		prBody := fmt.Sprintf("## Summary\n\n%s\n\n## Changes\n\nCreated by [claude-cells](https://github.com/STRML/claude-cells).", ws.Prompt)

		pr, err := gh.CreatePR(ctx, repoPath, &git.PRRequest{
			Title: prTitle,
			Body:  prBody,
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

// PairingEnabledMsg is sent when pairing mode is enabled.
type PairingEnabledMsg struct {
	WorkstreamID   string
	StashedChanges bool
	Error          error
}

// PairingDisabledMsg is sent when pairing mode is disabled.
type PairingDisabledMsg struct {
	WorkstreamID   string
	StashedChanges bool // True if user should run git stash pop
	Error          error
}

// EnablePairingCmd returns a command that enables pairing mode for a workstream.
func EnablePairingCmd(ws *workstream.Workstream) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		repoPath, err := os.Getwd()
		if err != nil {
			return PairingEnabledMsg{WorkstreamID: ws.ID, Error: err}
		}

		gitRepo := git.New(repoPath)
		var stashedChanges bool

		// Check for uncommitted changes
		hasChanges, err := gitRepo.HasUncommittedChanges(ctx)
		if err != nil {
			return PairingEnabledMsg{WorkstreamID: ws.ID, Error: fmt.Errorf("failed to check git status: %w", err)}
		}

		// Stash if needed
		if hasChanges {
			if err := gitRepo.Stash(ctx); err != nil {
				return PairingEnabledMsg{WorkstreamID: ws.ID, Error: fmt.Errorf("failed to stash changes: %w", err)}
			}
			stashedChanges = true
		}

		// Start mutagen sync
		mutagen := sync.NewMutagen()
		if err := mutagen.CreateSession(ctx, ws.BranchName, ws.ContainerID, repoPath); err != nil {
			// Try to unstash on failure
			if stashedChanges {
				_ = gitRepo.StashPop(ctx)
			}
			return PairingEnabledMsg{WorkstreamID: ws.ID, Error: fmt.Errorf("failed to start mutagen: %w", err)}
		}

		// Checkout workstream branch locally
		if err := gitRepo.Checkout(ctx, ws.BranchName); err != nil {
			// Try to clean up on failure
			_ = mutagen.TerminateSession(ctx, ws.BranchName)
			if stashedChanges {
				_ = gitRepo.StashPop(ctx)
			}
			return PairingEnabledMsg{WorkstreamID: ws.ID, Error: fmt.Errorf("failed to checkout branch: %w", err)}
		}

		return PairingEnabledMsg{
			WorkstreamID:   ws.ID,
			StashedChanges: stashedChanges,
		}
	}
}

// DisablePairingCmd returns a command that disables pairing mode.
func DisablePairingCmd(ws *workstream.Workstream, previousBranch string, stashedChanges bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		repoPath, err := os.Getwd()
		if err != nil {
			return PairingDisabledMsg{WorkstreamID: ws.ID, Error: err}
		}

		gitRepo := git.New(repoPath)
		mutagen := sync.NewMutagen()

		// Terminate mutagen sync
		_ = mutagen.TerminateSession(ctx, ws.BranchName)

		// Checkout previous branch
		if previousBranch != "" && previousBranch != ws.BranchName {
			if err := gitRepo.Checkout(ctx, previousBranch); err != nil {
				return PairingDisabledMsg{WorkstreamID: ws.ID, StashedChanges: stashedChanges, Error: fmt.Errorf("failed to checkout %s: %w", previousBranch, err)}
			}
		}

		return PairingDisabledMsg{
			WorkstreamID:   ws.ID,
			StashedChanges: stashedChanges,
		}
	}
}
