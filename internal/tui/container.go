package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/STRML/claude-cells/internal/claude"
	"github.com/STRML/claude-cells/internal/docker"
	"github.com/STRML/claude-cells/internal/git"
	"github.com/STRML/claude-cells/internal/gitproxy"
	"github.com/STRML/claude-cells/internal/orchestrator"
	"github.com/STRML/claude-cells/internal/sync"
	"github.com/STRML/claude-cells/internal/workstream"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// containerServices holds the container tracking services.
// This is set by the app layer and accessed by container commands.
// Thread-safe: these are set once at startup and read-only thereafter.
type containerServices struct {
	tracker   *docker.ContainerTracker
	refresher *docker.CredentialRefresher
	gitProxy  *gitproxy.Server
}

// services holds the global container services instance
var services containerServices

// SetContainerTracker sets the container tracker for tracking container lifecycle
func SetContainerTracker(tracker *docker.ContainerTracker) {
	services.tracker = tracker
}

// SetCredentialRefresher sets the credential refresher for OAuth token updates
func SetCredentialRefresher(refresher *docker.CredentialRefresher) {
	services.refresher = refresher
}

// GetContainerTracker returns the container tracker (for use in main.go cleanup)
func GetContainerTracker() *docker.ContainerTracker {
	return services.tracker
}

// GetCredentialRefresher returns the credential refresher
func GetCredentialRefresher() *docker.CredentialRefresher {
	return services.refresher
}

// SetGitProxyServer sets the git proxy server for proxying git/gh commands
func SetGitProxyServer(server *gitproxy.Server) {
	services.gitProxy = server
}

// GetGitProxyServer returns the git proxy server
func GetGitProxyServer() *gitproxy.Server {
	return services.gitProxy
}

// PRStatusRefreshRequestMsg requests a PR status refresh for a workstream.
// This is used by the git proxy to trigger a refresh after a successful push.
type PRStatusRefreshRequestMsg struct {
	WorkstreamID string
}

// RequestPRStatusRefresh sends a message to request PR status refresh for a workstream.
// This can be called from any goroutine (e.g., git proxy callback).
// Returns true if the message was sent, false if the program is not available.
func RequestPRStatusRefresh(workstreamID string) bool {
	return sendMsg(PRStatusRefreshRequestMsg{WorkstreamID: workstreamID})
}

// trackContainer adds a container to the tracker if available
func trackContainer(containerID, workstreamID, branchName, repoPath string) {
	if services.tracker != nil {
		services.tracker.Track(containerID, workstreamID, branchName, repoPath)
	}
}

// untrackContainer removes a container from the tracker if available
func untrackContainer(containerID string) {
	if services.tracker != nil {
		services.tracker.Untrack(containerID)
	}
}

// registerContainerCredentials registers a container with the credential refresher
func registerContainerCredentials(containerID, containerName, configDir string) {
	if services.refresher != nil {
		services.refresher.RegisterContainer(containerID, containerName, configDir)
	}
}

// unregisterContainerCredentials removes a container from the credential refresher
func unregisterContainerCredentials(containerID string) {
	if services.refresher != nil {
		services.refresher.UnregisterContainer(containerID)
	}
}

// startGitProxySocket starts a git proxy socket for a container
func startGitProxySocket(ctx context.Context, containerID string, ws *workstream.Workstream) string {
	if services.gitProxy == nil {
		return ""
	}
	wsInfo := gitproxy.WorkstreamInfo{
		ID:           ws.ID,
		Branch:       ws.BranchName,
		PRNumber:     ws.PRNumber,
		WorktreePath: ws.WorktreePath,
	}
	socketPath, err := services.gitProxy.StartSocket(ctx, containerID, wsInfo)
	if err != nil {
		LogWarn("Failed to start git proxy socket: %v", err)
		return ""
	}
	return socketPath
}

// stopGitProxySocket stops the git proxy socket for a container
func stopGitProxySocket(containerID string) {
	if services.gitProxy != nil {
		services.gitProxy.StopSocket(containerID)
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

// UntrackedFilesPromptMsg is sent when untracked files are detected in the repo.
type UntrackedFilesPromptMsg struct {
	WorkstreamID   string
	UntrackedFiles []string
}

// TitleGeneratedMsg is sent when a workstream title is generated via Claude CLI.
type TitleGeneratedMsg struct {
	WorkstreamID string
	Title        string
	Error        error
}

// SynopsisGeneratedMsg is sent when a workstream synopsis is generated via Claude CLI.
type SynopsisGeneratedMsg struct {
	WorkstreamID string
	Synopsis     string
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
		// Build the prompt for Claude CLI
		prompt := fmt.Sprintf(`Generate a 3-5 word title summarizing this task. Output ONLY the title text, no quotes or explanation.

Task: %s`, ws.Prompt)

		// Use ephemeral query to avoid polluting the resume log
		ctx := context.Background()
		title, err := claude.QueryWithTimeout(ctx, prompt, claude.DefaultTimeout)
		if err != nil {
			return TitleGeneratedMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}

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

// GenerateSynopsisCmd returns a command that generates a synopsis for a workstream
// based on git changes. This runs when a session ends to summarize what was done.
func GenerateSynopsisCmd(ws *workstream.Workstream) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Get git changes to summarize
		worktreePath := resolveWorktreePath(ws)
		if worktreePath == "" {
			return SynopsisGeneratedMsg{
				WorkstreamID: ws.ID,
				Error:        fmt.Errorf("no worktree path"),
			}
		}

		gitRepo := GitClientFactory(worktreePath)

		// Get branch info (commits, files changed)
		branchInfo, err := gitRepo.GetBranchInfo(ctx, ws.BranchName)
		if err != nil || branchInfo == "" {
			// No commits yet, nothing to summarize
			return SynopsisGeneratedMsg{
				WorkstreamID: ws.ID,
				Synopsis:     "",
			}
		}

		// Get a summary of recent commits
		commitLog, _ := gitRepo.GetBranchCommitLogs(ctx, ws.BranchName)

		// Build prompt for Claude
		prompt := fmt.Sprintf(`Generate a brief 1-line synopsis (max 100 chars) summarizing what was accomplished on this branch. Output ONLY the synopsis text, no quotes or explanation.

Original task: %s

Branch info:
%s

Recent commits:
%s`, ws.Prompt, branchInfo, commitLog)

		// Use ephemeral query to generate synopsis
		synopsis, err := claude.QueryWithTimeout(ctx, prompt, claude.DefaultTimeout)
		if err != nil {
			return SynopsisGeneratedMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}

		// Limit synopsis length
		if len(synopsis) > 120 {
			synopsis = synopsis[:117] + "..."
		}

		return SynopsisGeneratedMsg{
			WorkstreamID: ws.ID,
			Synopsis:     synopsis,
		}
	}
}

// StartContainerCmd returns a command that creates and starts a container.
// It first creates and checks out a feature branch for the workstream.
func StartContainerCmd(ws *workstream.Workstream) tea.Cmd {
	return startContainerWithFullOptions(ws, false, false)
}

// CheckUntrackedFilesCmd checks for untracked files in the repo before starting a container.
// If untracked files exist, returns UntrackedFilesPromptMsg. Otherwise returns StartContainerCmd result.
func CheckUntrackedFilesCmd(ws *workstream.Workstream) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Get current working directory as repo path
		repoPath, err := os.Getwd()
		if err != nil {
			// On error, proceed without copying (fail gracefully)
			return startContainerWithFullOptions(ws, false, false)()
		}

		gitRepo := GitClientFactory(repoPath)
		untrackedFiles, err := gitRepo.GetUntrackedFiles(ctx)
		if err != nil || len(untrackedFiles) == 0 {
			// No untracked files or error checking - proceed normally
			return startContainerWithFullOptions(ws, false, false)()
		}

		// Untracked files found - prompt user
		return UntrackedFilesPromptMsg{
			WorkstreamID:   ws.ID,
			UntrackedFiles: untrackedFiles,
		}
	}
}

// StartContainerWithCopyUntrackedFilesCmd starts container and copies untracked files to the worktree.
func StartContainerWithCopyUntrackedFilesCmd(ws *workstream.Workstream, copyFiles bool) tea.Cmd {
	return startContainerWithFullOptions(ws, false, copyFiles)
}

// StartContainerWithExistingBranchCmd starts a container using an existing branch.
func StartContainerWithExistingBranchCmd(ws *workstream.Workstream) tea.Cmd {
	return startContainerWithFullOptions(ws, true, false)
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
		return startContainerWithFullOptions(ws, false, false)()
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
		return startContainerWithFullOptions(ws, false, false)()
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

		// Create Docker client for orchestrator
		dockerClient, err := docker.NewClient()
		if err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}
		defer dockerClient.Close()

		// Create orchestrator
		gitFactory := func(path string) git.GitClient {
			return GitClientFactory(path)
		}
		orch := orchestrator.New(dockerClient, gitFactory, repoPath)

		// Use orchestrator to rebuild workstream
		opts := orchestrator.CreateOptions{
			RepoPath:          repoPath,
			UseExistingBranch: true, // Rebuild uses existing branch
		}

		result, err := orch.RebuildWorkstream(ctx, ws, opts)
		if err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}

		// Track the container for crash recovery
		trackContainer(result.ContainerID, ws.ID, ws.BranchName, result.WorktreePath)

		// Register for credential refresh
		registerContainerCredentials(result.ContainerID, result.ContainerName, result.ConfigDir)

		// Start git proxy socket for this container
		if result.GitProxySocketDir != "" {
			startGitProxySocket(ctx, result.ContainerID, ws)
		}

		// Return with IsResume=true so PTY uses --continue
		return ContainerStartedMsg{
			WorkstreamID: ws.ID,
			ContainerID:  result.ContainerID,
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

// copyUntrackedFilesToWorktree copies untracked files from the source repo to the worktree.
// It preserves directory structure and creates parent directories as needed.
func copyUntrackedFilesToWorktree(srcRepo, dstWorktree string, files []string) error {
	var lastErr error
	for _, relPath := range files {
		srcPath := filepath.Join(srcRepo, relPath)
		dstPath := filepath.Join(dstWorktree, relPath)

		// Create parent directory if it doesn't exist
		dstDir := filepath.Dir(dstPath)
		if err := os.MkdirAll(dstDir, 0755); err != nil {
			lastErr = fmt.Errorf("failed to create directory %s: %w", dstDir, err)
			continue
		}

		// Read source file
		data, err := os.ReadFile(srcPath)
		if err != nil {
			lastErr = fmt.Errorf("failed to read %s: %w", relPath, err)
			continue
		}

		// Get source file info for permissions
		srcInfo, err := os.Stat(srcPath)
		if err != nil {
			lastErr = fmt.Errorf("failed to stat %s: %w", relPath, err)
			continue
		}

		// Write to destination with same permissions
		if err := os.WriteFile(dstPath, data, srcInfo.Mode()); err != nil {
			lastErr = fmt.Errorf("failed to write %s: %w", relPath, err)
			continue
		}
	}
	return lastErr
}

// cleanupWorktree removes a worktree and its directory.
// Errors are logged but not returned since cleanup is best-effort.
func cleanupWorktree(ctx context.Context, gitRepo git.GitClient, branchName string) {
	worktreePath := getWorktreePath(branchName)
	if err := gitRepo.RemoveWorktree(ctx, worktreePath); err != nil {
		LogWarn("RemoveWorktree failed for %s: %v", worktreePath, err)
	}
	if err := os.RemoveAll(worktreePath); err != nil {
		LogWarn("RemoveAll failed for %s: %v", worktreePath, err)
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

// startContainerWithFullOptions is the internal implementation for starting containers.
// It uses git worktrees to avoid modifying the host repo's checked out branch.
// If copyUntrackedFiles is true, untracked files from the host repo are copied to the worktree.
func startContainerWithFullOptions(ws *workstream.Workstream, useExistingBranch bool, copyUntrackedFiles bool) tea.Cmd {
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

		// Create Docker client for orchestrator
		dockerClient, err := docker.NewClient()
		if err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}
		defer dockerClient.Close()

		// Create orchestrator
		gitFactory := func(path string) git.GitClient {
			return GitClientFactory(path)
		}
		orch := orchestrator.New(dockerClient, gitFactory, repoPath)

		// Check for branch conflict before creating (if not using existing branch)
		if !useExistingBranch {
			conflict, err := orch.CheckBranchConflict(ctx, ws.BranchName)
			if err != nil {
				return ContainerErrorMsg{
					WorkstreamID: ws.ID,
					Error:        err,
				}
			}
			if conflict != nil {
				return BranchConflictMsg{
					WorkstreamID: ws.ID,
					BranchName:   conflict.BranchName,
					RepoPath:     repoPath,
					BranchInfo:   conflict.BranchInfo,
				}
			}
		}

		// Get untracked files if needed
		var untrackedFiles []string
		if copyUntrackedFiles && !useExistingBranch {
			gitRepo := GitClientFactory(repoPath)
			files, err := gitRepo.GetUntrackedFiles(ctx)
			if err != nil {
				LogWarn("Failed to get untracked files: %v", err)
				// Continue without copying untracked files rather than failing
			} else {
				untrackedFiles = files
			}
		}

		// Create workstream using orchestrator
		opts := orchestrator.CreateOptions{
			RepoPath:          repoPath,
			UseExistingBranch: useExistingBranch,
			UpdateMain:        !useExistingBranch, // Auto-pull main for new branches
			CopyUntracked:     copyUntrackedFiles,
			UntrackedFiles:    untrackedFiles,
		}

		result, err := orch.CreateWorkstream(ctx, ws, opts)
		if err != nil {
			return ContainerErrorMsg{
				WorkstreamID: ws.ID,
				Error:        err,
			}
		}

		// Track the container for crash recovery
		trackContainer(result.ContainerID, ws.ID, ws.BranchName, result.WorktreePath)

		// Register for credential refresh
		registerContainerCredentials(result.ContainerID, result.ContainerName, result.ConfigDir)

		// Start git proxy socket for this container
		// The socket directory was created by the orchestrator, now we start the listener
		if result.GitProxySocketDir != "" {
			startGitProxySocket(ctx, result.ContainerID, ws)
		}

		return ContainerStartedMsg{
			WorkstreamID: ws.ID,
			ContainerID:  result.ContainerID,
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

		// Note: OAuth credentials are in ~/.claude/.credentials.json which is mounted from
		// the host. This allows Claude Code to manage credentials including token refresh.

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

		repoPath, err := os.Getwd()
		if err != nil {
			LogWarn("Failed to get cwd: %v", err)
			return ContainerStoppedMsg{WorkstreamID: ws.ID}
		}

		// Create Docker client for orchestrator
		dockerClient, err := docker.NewClient()
		if err != nil {
			LogWarn("Failed to create docker client: %v", err)
			return ContainerStoppedMsg{WorkstreamID: ws.ID}
		}
		defer dockerClient.Close()

		// Create orchestrator
		gitFactory := func(path string) git.GitClient {
			return GitClientFactory(path)
		}
		orch := orchestrator.New(dockerClient, gitFactory, repoPath)

		// Check if we should delete the branch (only if it has no commits)
		deleteBranch := false
		if ws.BranchName != "" {
			gitRepo := GitClientFactory(repoPath)
			hasCommits, err := gitRepo.BranchHasCommits(ctx, ws.BranchName)
			if err != nil {
				LogWarn("BranchHasCommits error: %v", err)
			} else {
				deleteBranch = !hasCommits
			}
		}

		// Use orchestrator to destroy workstream
		destroyOpts := orchestrator.DestroyOptions{
			DeleteBranch: deleteBranch,
			KeepWorktree: false,
		}
		if err := orch.DestroyWorkstream(ctx, ws, destroyOpts); err != nil {
			LogWarn("DestroyWorkstream error: %v", err)
		}

		// Untrack the container (TUI-layer concern)
		if ws.ContainerID != "" {
			untrackContainer(ws.ContainerID)
			unregisterContainerCredentials(ws.ContainerID)
			stopGitProxySocket(ws.ContainerID)
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

		// Start git proxy socket for the resumed container
		// (The server may have restarted, so we need to re-establish the socket)
		startGitProxySocket(ctx, ws.ContainerID, ws)

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
	WorkstreamID  string
	Error         error
	ForcePush     bool // True if this was a force push
	CommitsPushed int  // Number of commits that were pushed (0 if nothing to push)
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

		// Get the unpushed commit count BEFORE pushing so we know how many we pushed
		unpushedCount, _ := gitRepo.GetUnpushedCommitCount(ctx, ws.BranchName)

		if err := gitRepo.Push(ctx, ws.BranchName); err != nil {
			return PushBranchResultMsg{WorkstreamID: ws.ID, Error: err}
		}

		return PushBranchResultMsg{WorkstreamID: ws.ID, Error: nil, CommitsPushed: unpushedCount}
	}
}

// ForcePushBranchCmd returns a command that force pushes a branch to origin using --force-with-lease.
func ForcePushBranchCmd(ws *workstream.Workstream) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Use worktree path for git operations
		worktreePath := resolveWorktreePath(ws)
		if worktreePath == "" {
			return PushBranchResultMsg{WorkstreamID: ws.ID, Error: fmt.Errorf("no worktree path for branch"), ForcePush: true}
		}

		gitRepo := GitClientFactory(worktreePath)

		// Get the unpushed commit count BEFORE pushing so we know how many we pushed
		unpushedCount, _ := gitRepo.GetUnpushedCommitCount(ctx, ws.BranchName)

		if err := gitRepo.ForcePush(ctx, ws.BranchName); err != nil {
			return PushBranchResultMsg{WorkstreamID: ws.ID, Error: err, ForcePush: true}
		}

		return PushBranchResultMsg{WorkstreamID: ws.ID, Error: nil, ForcePush: true, CommitsPushed: unpushedCount}
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

		// Generate PR title and body using Claude
		prTitle, prBody := git.GeneratePRContent(ctx, gitRepo, ws.BranchName, ws.Prompt)

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
	WorkstreamID         string
	Error                error
	ConflictFiles        []string // Files with conflicts (if any)
	NeedsContainerRebase bool     // True if rebase must happen from within container (worktree conflict)
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
			// Check if it's a worktree conflict (branch checked out in container)
			var wtErr *git.WorktreeConflictError
			if errors.As(err, &wtErr) {
				return MergeBranchMsg{
					WorkstreamID:         ws.ID,
					Error:                err,
					NeedsContainerRebase: true,
				}
			}
			return MergeBranchMsg{WorkstreamID: ws.ID, Error: fmt.Errorf("failed to merge branch: %w", err)}
		}

		return MergeBranchMsg{WorkstreamID: ws.ID}
	}
}

// GHMergePRResultMsg is sent when a GitHub PR merge completes.
type GHMergePRResultMsg struct {
	WorkstreamID string
	MergeMethod  string // "squash", "merge", or "rebase"
	Error        error
}

// GHMergePRCmd returns a command that merges a PR via GitHub's gh CLI.
// mergeMethod should be "squash", "merge", or "rebase".
func GHMergePRCmd(ws *workstream.Workstream, mergeMethod string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		// Use worktree path for git operations
		worktreePath := resolveWorktreePath(ws)
		if worktreePath == "" {
			return GHMergePRResultMsg{WorkstreamID: ws.ID, MergeMethod: mergeMethod, Error: fmt.Errorf("no worktree path")}
		}

		gh := git.NewGH()

		// Build merge options
		opts := &git.PRMergeOptions{
			Method:       mergeMethod,
			DeleteBranch: false, // Don't delete - let the user decide via destroy dialog
		}

		if err := gh.MergePR(ctx, worktreePath, opts); err != nil {
			return GHMergePRResultMsg{WorkstreamID: ws.ID, MergeMethod: mergeMethod, Error: err}
		}

		return GHMergePRResultMsg{WorkstreamID: ws.ID, MergeMethod: mergeMethod}
	}
}

// RebaseBranchMsg is sent when a rebase completes.
type RebaseBranchMsg struct {
	WorkstreamID  string
	Error         error
	ConflictFiles []string // Files with conflicts (if rebase stopped)
}

// PRStatusMsg is sent when PR status is fetched.
type PRStatusMsg struct {
	WorkstreamID string
	Status       *git.PRStatusInfo
	Error        error
}

// FetchRebaseResultMsg is sent when a fetch-and-rebase operation completes.
type FetchRebaseResultMsg struct {
	WorkstreamID  string
	Error         error
	ConflictFiles []string // Files with conflicts if rebase failed
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

// FetchPRStatusCmd returns a command that fetches comprehensive PR status.
func FetchPRStatusCmd(ws *workstream.Workstream) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Use worktree path for git operations
		worktreePath := resolveWorktreePath(ws)
		if worktreePath == "" {
			return PRStatusMsg{WorkstreamID: ws.ID, Error: fmt.Errorf("no worktree path")}
		}

		gitRepo := GitClientFactory(worktreePath)
		gh := git.NewGH()

		status, err := gh.GetPRStatus(ctx, worktreePath, gitRepo)
		if err != nil {
			return PRStatusMsg{WorkstreamID: ws.ID, Error: err}
		}

		return PRStatusMsg{
			WorkstreamID: ws.ID,
			Status:       status,
		}
	}
}

// FetchRebaseCmd returns a command that fetches main and rebases the current branch onto it.
func FetchRebaseCmd(ws *workstream.Workstream) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		// Use worktree path for git operations
		worktreePath := resolveWorktreePath(ws)
		if worktreePath == "" {
			return FetchRebaseResultMsg{WorkstreamID: ws.ID, Error: fmt.Errorf("no worktree path")}
		}

		gitRepo := GitClientFactory(worktreePath)

		// Perform fetch and rebase
		if err := gitRepo.FetchAndRebase(ctx); err != nil {
			// Check if it's a conflict error
			if conflictErr, ok := err.(*git.MergeConflictError); ok {
				return FetchRebaseResultMsg{
					WorkstreamID:  ws.ID,
					Error:         err,
					ConflictFiles: conflictErr.ConflictFiles,
				}
			}
			return FetchRebaseResultMsg{WorkstreamID: ws.ID, Error: err}
		}

		return FetchRebaseResultMsg{WorkstreamID: ws.ID}
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
	Healthy    bool
	SyncStatus sync.SyncStatus
	StatusText string
	Conflicts  []string
	Problems   []string
	Error      error
}

// CheckPairingSyncHealthCmd returns a command that checks pairing sync health.
func CheckPairingSyncHealthCmd(orchestrator *sync.Pairing) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := orchestrator.CheckSyncHealth(ctx)
		state := orchestrator.GetState()

		return PairingSyncHealthMsg{
			Healthy:    state.SyncHealthy,
			SyncStatus: state.SyncStatus,
			StatusText: state.SyncStatusText,
			Conflicts:  state.Conflicts,
			Problems:   state.Problems,
			Error:      err,
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

// ClaudeUsageMsg is sent when Claude usage information is fetched.
type ClaudeUsageMsg struct {
	ContainerID string
	Usage       string
	Error       error
}

// usageParseScript is a shell script that parses Claude session JSONL files
// and sums up token usage. It finds the most recent session file and extracts
// usage data from .message.usage fields. Uses portable grep/sed/awk (no gawk).
const usageParseScript = `#!/bin/sh
SESSION_DIR="$HOME/.claude/projects"
if [ ! -d "$SESSION_DIR" ]; then
  echo "No session data"
  exit 0
fi

# Find most recently modified .jsonl file across all project dirs
LATEST=""
LATEST_TIME=0
for dir in "$SESSION_DIR"/*/; do
  for f in "$dir"*.jsonl; do
    [ -f "$f" ] || continue
    if stat --version 2>/dev/null | grep -q GNU; then
      MTIME=$(stat -c %Y "$f" 2>/dev/null)
    else
      MTIME=$(stat -f %m "$f" 2>/dev/null)
    fi
    if [ -n "$MTIME" ] && [ "$MTIME" -gt "$LATEST_TIME" ] 2>/dev/null; then
      LATEST_TIME=$MTIME
      LATEST=$f
    fi
  done
done

if [ -z "$LATEST" ]; then
  echo "No session files"
  exit 0
fi

# Parse usage with portable grep/sed/awk (works with busybox)
input=$(grep -o '"input_tokens":[0-9]*' "$LATEST" 2>/dev/null | sed 's/[^0-9]//g' | awk '{s+=$1} END {print s+0}')
output=$(grep -o '"output_tokens":[0-9]*' "$LATEST" 2>/dev/null | sed 's/[^0-9]//g' | awk '{s+=$1} END {print s+0}')
cache_create=$(grep -o '"cache_creation_input_tokens":[0-9]*' "$LATEST" 2>/dev/null | sed 's/[^0-9]//g' | awk '{s+=$1} END {print s+0}')
cache_read=$(grep -o '"cache_read_input_tokens":[0-9]*' "$LATEST" 2>/dev/null | sed 's/[^0-9]//g' | awk '{s+=$1} END {print s+0}')

total=$((input + output + cache_create))
printf "Session Tokens\n"
printf "----------------------\n"
printf "Input:        %8d\n" "$input"
printf "Output:       %8d\n" "$output"
printf "Cache Create: %8d\n" "$cache_create"
printf "Cache Read:   %8d\n" "$cache_read"
printf "----------------------\n"
printf "Total:        %8d\n" "$total"
`

// FetchClaudeUsageCmd returns a command that fetches Claude usage from a container.
// It parses session JSONL files to extract and sum token usage.
func FetchClaudeUsageCmd(containerID string) tea.Cmd {
	return func() tea.Msg {
		if containerID == "" {
			return ClaudeUsageMsg{Error: fmt.Errorf("no container ID")}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := docker.NewClient()
		if err != nil {
			return ClaudeUsageMsg{ContainerID: containerID, Error: err}
		}
		defer client.Close()

		// Execute the usage parsing script in the container
		usage, err := client.ExecInContainer(ctx, containerID, []string{"sh", "-c", usageParseScript})
		if err != nil {
			return ClaudeUsageMsg{ContainerID: containerID, Error: err}
		}

		return ClaudeUsageMsg{
			ContainerID: containerID,
			Usage:       usage,
		}
	}
}
