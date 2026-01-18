package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/STRML/claude-cells/internal/docker"
	"github.com/STRML/claude-cells/internal/git"
	"github.com/STRML/claude-cells/internal/sync"
	"github.com/STRML/claude-cells/internal/workstream"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

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

		// First checkout main/master so we can delete the branch
		// (can't delete a branch that's currently checked out)
		currentBranch, _ := gitRepo.CurrentBranch(ctx)
		if currentBranch == ws.BranchName {
			// Try main first, then master
			if err := gitRepo.Checkout(ctx, "main"); err != nil {
				if err := gitRepo.Checkout(ctx, "master"); err != nil {
					return ContainerErrorMsg{
						WorkstreamID: ws.ID,
						Error:        fmt.Errorf("failed to checkout main/master before deleting branch: %w", err),
					}
				}
			}
		}

		// Delete the existing branch
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

// startContainerWithOptions is the internal implementation for starting containers.
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

		if useExistingBranch {
			// Just checkout the existing branch
			if err := gitRepo.Checkout(ctx, ws.BranchName); err != nil {
				return ContainerErrorMsg{
					WorkstreamID: ws.ID,
					Error:        fmt.Errorf("failed to checkout branch %s: %w", ws.BranchName, err),
				}
			}
		} else {
			// Check if branch already exists
			exists, _ := gitRepo.BranchExists(ctx, ws.BranchName)
			if exists {
				// Branch already exists - ask user what to do
				return BranchConflictMsg{
					WorkstreamID: ws.ID,
					BranchName:   ws.BranchName,
					RepoPath:     repoPath,
				}
			}

			// Create and checkout feature branch before starting container
			if err := gitRepo.CreateAndCheckout(ctx, ws.BranchName); err != nil {
				return ContainerErrorMsg{
					WorkstreamID: ws.ID,
					Error:        fmt.Errorf("failed to create branch %s: %w", ws.BranchName, err),
				}
			}
		}

		// Create Docker client
		dockerClient, err := docker.NewClient()
		if err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}
		defer dockerClient.Close()

		// Create container config with unique name
		cfg := docker.NewContainerConfig(ws.BranchName, repoPath)
		cfg.Image = docker.RequiredImage

		// Create per-container isolated config directory
		// This prevents race conditions when multiple containers modify credentials
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
		cfg.Credentials = configPaths.Credentials

		// Create container
		containerID, err := dockerClient.CreateContainer(ctx, cfg)
		if err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}

		// Start container
		err = dockerClient.StartContainer(ctx, containerID)
		if err != nil {
			// Clean up created container on start failure
			_ = dockerClient.RemoveContainer(ctx, containerID)
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}

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

// StopContainerCmd returns a command that stops and removes a container.
func StopContainerCmd(ws *workstream.Workstream) tea.Cmd {
	return func() tea.Msg {
		if ws.ContainerID == "" {
			return ContainerStoppedMsg{WorkstreamID: ws.ID}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := docker.NewClient()
		if err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}
		defer client.Close()

		// Stop and remove container
		_ = client.StopContainer(ctx, ws.ContainerID)
		_ = client.RemoveContainer(ctx, ws.ContainerID)

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
