package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/STRML/claude-cells/internal/docker"
	"github.com/STRML/claude-cells/internal/git"
	"github.com/STRML/claude-cells/internal/gitproxy"
	"github.com/STRML/claude-cells/internal/tui"
	"github.com/STRML/claude-cells/internal/workstream"
)

// Version info - set via ldflags at build time
// go build -ldflags "-X main.Version=v1.0.0 -X main.CommitHash=$(git rev-parse --short HEAD)"
var (
	Version    = "dev"
	CommitHash = "unknown"
)

// spinner displays a progress spinner while a function runs
type spinner struct {
	frames  []string
	current int
	done    chan struct{}
	message string
}

func newSpinner(message string) *spinner {
	return &spinner{
		frames:  []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		message: message,
		done:    make(chan struct{}),
	}
}

func (s *spinner) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				// Context cancelled - clear spinner and exit
				fmt.Print("\r\033[K")
				return
			case <-s.done:
				// Clear the spinner line
				fmt.Print("\r\033[K")
				return
			case <-ticker.C:
				fmt.Printf("\r%s %s", s.frames[s.current], s.message)
				s.current = (s.current + 1) % len(s.frames)
			}
		}
	}()
}

func (s *spinner) Stop() {
	close(s.done)
	// Small delay to ensure spinner is cleared
	time.Sleep(100 * time.Millisecond)
}

const lockFileName = ".ccells.lock"

// lockFile represents an acquired lock
type lockFile struct {
	path string
}

// acquireLock attempts to acquire an exclusive lock for this repo.
// Returns a lockFile on success, or an error if another instance is running.
func acquireLock(stateDir string) (*lockFile, error) {
	lockPath := filepath.Join(stateDir, lockFileName)

	// Check if lock file exists
	if data, err := os.ReadFile(lockPath); err == nil {
		// Lock file exists - check if the process is still running
		pidStr := strings.TrimSpace(string(data))
		if pid, err := strconv.Atoi(pidStr); err == nil {
			// Check if process is still alive
			if process, err := os.FindProcess(pid); err == nil {
				// On Unix, FindProcess always succeeds, so we need to send signal 0
				if err := process.Signal(syscall.Signal(0)); err == nil {
					// Process is still running
					return nil, fmt.Errorf("another ccells instance is already running (PID %d)", pid)
				}
			}
		}
		// Stale lock file - remove it
		os.Remove(lockPath)
	}

	// Create lock file with our PID
	pid := os.Getpid()
	if err := os.WriteFile(lockPath, []byte(strconv.Itoa(pid)), 0644); err != nil {
		return nil, fmt.Errorf("failed to create lock file: %w", err)
	}

	return &lockFile{path: lockPath}, nil
}

// Release removes the lock file
func (l *lockFile) Release() {
	if l != nil && l.path != "" {
		os.Remove(l.path)
	}
}

// getStateDir returns the state directory for the current repo.
// Falls back to cwd if repo ID cannot be determined.
func getStateDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return cwd
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gitOps := git.New(cwd)
	repoID, err := gitOps.RepoID(ctx)
	if err != nil || repoID == "" {
		return cwd
	}

	stateDir, err := workstream.GetStateDir(repoID)
	if err != nil {
		return cwd
	}

	return stateDir
}

func printHelp() {
	fmt.Printf(`ccells - Claude Cells: Run parallel Claude Code instances in Docker containers

Usage:
  ccells [options]

Options:
  -h, --help          Show this help message
  -v, --version       Show version information
  --runtime <name>    Runtime to use: "claude" (default) or "claudesp" (experimental)
                      Overrides runtime setting from config files
  --repair-state      Validate and repair the state file by extracting
                      session IDs from running containers

Keyboard Shortcuts (in TUI):
  n             Create new workstream
  d             Destroy workstream (with confirmation)
  1-9           Jump to pane by number
  Tab/Shift+Tab Navigate between panes
  Space         Toggle between main pane and others
  l             Toggle layout (vertical/horizontal/grid)
  Enter         Enter input mode (type in focused pane)
  Esc Esc       Exit input mode (double-tap)
  ?             Show help dialog
  q             Quit (saves state for resume)

State Management:
  ccells automatically saves state on exit and resumes on restart.
  If session IDs are corrupted, use --repair-state to fix them.

For more information: https://github.com/STRML/claude-cells
`)
}

func main() {
	// Initialize logging early to prevent any log.Printf from polluting TUI
	tui.InitLogging()

	// Parse runtime flag
	var runtimeFlag string
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		if args[i] == "--runtime" && i+1 < len(args) {
			runtimeFlag = args[i+1]
			// Remove runtime flag from args for other processing
			args = append(args[:i], args[i+2:]...)
			break
		}
	}

	// Handle command-line flags
	if len(args) > 0 {
		switch args[0] {
		case "--help", "-h":
			printHelp()
			os.Exit(0)
		case "--version", "-v":
			fmt.Printf("ccells %s (%s)\n", Version, CommitHash)
			os.Exit(0)
		case "--repair-state":
			if err := runStateRepair(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}

	// Acquire lock to ensure only one instance runs per repo
	stateDir := getStateDir()
	lock, err := acquireLock(stateDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "If the other instance crashed, delete: %s/%s\n", stateDir, lockFileName)
		os.Exit(1)
	}
	defer lock.Release()

	// Create a cancellable context for the entire application.
	// This context is cancelled on SIGINT/SIGTERM and propagates
	// cancellation to all running operations.
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	// Validate Docker prerequisites before starting
	if err := validatePrerequisites(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Initialize container tracker for crash recovery
	tracker, err := docker.NewContainerTracker()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize container tracker: %v\n", err)
		// Continue without tracking - not fatal
	}

	// Clean up orphaned containers from crashed sessions
	cleanupOrphanedContainers(tracker)

	// Clean up orphaned worktrees (conservative - only removes empty/clean ones)
	cleanupOrphanedWorktrees(stateDir)

	// Start heartbeat goroutine to detect crashes
	if tracker != nil {
		go runHeartbeat(appCtx, tracker)
	}

	// Start credential refresher to keep OAuth tokens updated in containers
	credRefresher := docker.NewCredentialRefresher(15 * time.Minute)
	credRefresher.Start()
	defer credRefresher.Stop()
	tui.SetCredentialRefresher(credRefresher)

	// Start git proxy server for proxying git/gh commands from containers
	gitProxyServer := gitproxy.NewServer(func(workstreamID string, prNumber int, prURL string) {
		// Callback when a PR is created via the proxy.
		// Currently only logs the event. State updates happen via the gitproxy
		// server's internal workstream tracking (UpdateWorkstream method).
		tui.LogDebug("PR #%d created for workstream %s: %s", prNumber, workstreamID, prURL)
	})
	// Set callback to refresh PR status after successful push
	gitProxyServer.SetPushCompleteCallback(func(workstreamID string) {
		tui.RequestPRStatusRefresh(workstreamID)
	})
	defer gitProxyServer.Shutdown()
	tui.SetGitProxyServer(gitProxyServer)

	// Set version info for display in help dialog
	tui.SetVersionInfo(Version, CommitHash)

	// Load config and set runtime
	projectPath, _ := os.Getwd()
	cellsConfig := docker.LoadConfig(projectPath)
	// CLI flag overrides config file
	if runtimeFlag != "" {
		cellsConfig.Runtime = runtimeFlag
	}
	tui.SetRuntime(cellsConfig.Runtime)

	app := tui.NewAppModel(appCtx)

	// Set the tracker on the app so it can track container lifecycle
	if tracker != nil {
		tui.SetContainerTracker(tracker)
	}

	// Note: AltScreen and MouseMode are now controlled via View() in bubbletea v2
	p := tea.NewProgram(app)

	// Set the program reference so PTY sessions can send messages
	tui.SetProgram(p)

	// Set up signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		// Cancel the app context to signal all operations to stop
		appCancel()
		// Also tell bubbletea to quit
		p.Quit()
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}

	// Clean shutdown - remove heartbeat and clear tracking
	if tracker != nil {
		tracker.RemoveHeartbeat()
		tracker.Clear()
	}
}

// runHeartbeat writes heartbeat every 5 seconds until context is cancelled
func runHeartbeat(ctx context.Context, tracker *docker.ContainerTracker) {
	pid := os.Getpid()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Write initial heartbeat
	tracker.WriteHeartbeat(pid)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tracker.WriteHeartbeat(pid)
		}
	}
}

func validatePrerequisites() error {
	// Get project path (current working directory)
	projectPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Check for devcontainer CLI - warn if not present
	cliStatus := docker.CheckDevcontainerCLI()
	if !cliStatus.Available {
		if docker.HasDevcontainerConfig(projectPath) {
			fmt.Fprintln(os.Stderr, "\033[33mWarning: devcontainer CLI not found.\033[0m")
			fmt.Fprintln(os.Stderr, "Devcontainer features won't be installed. Containers may have issues.")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, docker.DevcontainerCLIInstallInstructions())
			fmt.Fprintln(os.Stderr, "")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := docker.ValidatePrerequisites(ctx, projectPath)
	if err != nil {
		return fmt.Errorf("failed to validate prerequisites: %w", err)
	}

	// If Docker is available but image is missing, try to build/pull it
	if result.DockerAvailable && !result.ImageExists {
		if result.NeedsBuild {
			// Build from devcontainer.json
			var buildOutput bytes.Buffer
			buildCtx, buildCancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer buildCancel()

			// Use devcontainer CLI if available for proper feature support
			if cliStatus.Available {
				spin := newSpinner(fmt.Sprintf("Building image '%s' with devcontainer CLI...", result.ImageName))
				spin.Start(buildCtx)
				baseImage, err := docker.BuildWithDevcontainerCLI(buildCtx, projectPath, &buildOutput)
				spin.Stop()
				if err != nil {
					fmt.Println(buildOutput.String()) // Show output on error
					return fmt.Errorf("failed to build with devcontainer CLI: %w", err)
				}
				// Build enhanced image with Claude Code on top
				spin = newSpinner("Adding Claude Code to image...")
				spin.Start(buildCtx)
				err = docker.BuildEnhancedImage(buildCtx, baseImage, result.ImageName, &buildOutput)
				spin.Stop()
				if err != nil {
					fmt.Println(buildOutput.String())
					return fmt.Errorf("failed to build enhanced image: %w", err)
				}
			} else {
				// Fall back to simple docker build
				devCfg, err := docker.LoadDevcontainerConfig(projectPath)
				if err != nil {
					return fmt.Errorf("failed to load devcontainer config: %w", err)
				}

				spin := newSpinner(fmt.Sprintf("Building image '%s'...", result.ImageName))
				spin.Start(buildCtx)
				err = docker.BuildProjectImage(buildCtx, projectPath, devCfg, &buildOutput)
				spin.Stop()
				if err != nil {
					fmt.Println(buildOutput.String()) // Show output on error
					return fmt.Errorf("failed to build project image: %w", err)
				}
			}

			fmt.Printf("✓ Image '%s' built successfully\n", result.ImageName)
		} else if result.ImageName == docker.GetBaseImageName() {
			// Build the default ccells image (hash-tagged for content-based rebuilds)
			var buildOutput bytes.Buffer
			buildCtx, buildCancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer buildCancel()

			spin := newSpinner(fmt.Sprintf("Building image '%s'...", result.ImageName))
			spin.Start(buildCtx)
			err := docker.BuildImage(buildCtx, &buildOutput)
			spin.Stop()
			if err != nil {
				fmt.Println(buildOutput.String()) // Show output on error
				return fmt.Errorf("failed to build image: %w", err)
			}

			fmt.Printf("✓ Image '%s' built successfully\n", result.ImageName)
		} else {
			// External image from devcontainer.json - prompt to pull
			return fmt.Errorf("image '%s' from devcontainer.json not found. Run: docker pull %s", result.ImageName, result.ImageName)
		}

		// Re-validate to confirm (use fresh context since build may have taken a while)
		revalidateCtx, revalidateCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer revalidateCancel()
		result, err = docker.ValidatePrerequisites(revalidateCtx, projectPath)
		if err != nil {
			return fmt.Errorf("failed to validate after build: %w", err)
		}
	}

	if !result.IsValid() {
		fmt.Fprintln(os.Stderr, "Docker prerequisites not met:")
		for _, e := range result.Errors {
			fmt.Fprintf(os.Stderr, "  - %s: %s\n", e.Check, e.Message)
		}
		return fmt.Errorf("please fix the issues above and try again")
	}

	return nil
}

// cleanupOrphanedContainers removes ccells containers from previous crashed sessions.
// It uses the container tracker (if available), state file, and worktree existence to find which containers should be kept.
// IMPORTANT: Never removes containers that have corresponding worktrees (work in progress).
func cleanupOrphanedContainers(tracker *docker.ContainerTracker) {
	// Get current working directory for project name
	cwd, err := os.Getwd()
	if err != nil {
		return // Silently skip if we can't get cwd
	}

	// Get project name from directory
	projectName := filepath.Base(cwd)

	// Use the new state directory
	stateDir := getStateDir()

	// Collect known container IDs from multiple sources

	// 1. From state file (for graceful shutdown resume)
	var knownIDs []string
	if workstream.StateExists(stateDir) {
		state, err := workstream.LoadState(stateDir)
		if err == nil {
			for _, ws := range state.Workstreams {
				if ws.ContainerID != "" {
					knownIDs = append(knownIDs, ws.ContainerID)
				}
			}
		}
	}

	// 2. From tracker - if heartbeat is stale, these are orphaned from crash
	var orphanedFromCrash []docker.TrackedContainer
	if tracker != nil {
		orphanedFromCrash = tracker.GetOrphanedContainers()
		if len(orphanedFromCrash) > 0 {
			fmt.Printf("Detected %d container(s) from crashed session\n", len(orphanedFromCrash))
		}
	}

	// 3. Get list of existing worktrees - these should NEVER be cleaned up
	existingWorktrees := listExistingWorktrees()

	// Create docker client and clean up orphaned containers
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := docker.NewClient()
	if err != nil {
		return // Silently skip if we can't connect to Docker
	}
	defer client.Close()

	// Clean up containers that aren't in knownIDs (from state file) and don't have worktrees
	removed, err := client.CleanupOrphanedContainers(ctx, projectName, knownIDs, existingWorktrees)
	if err == nil && removed > 0 {
		fmt.Printf("Cleaned up %d orphaned container(s) from previous session\n", removed)
	}

	// Clean up orphaned container configs (configs for containers that no longer exist)
	containers, err := client.ListDockerTUIContainers(ctx)
	if err == nil {
		existingContainerNames := make(map[string]bool)
		for _, c := range containers {
			existingContainerNames[c.Name] = true
		}
		configsRemoved, _ := docker.CleanupOrphanedContainerConfigs(existingContainerNames)
		if configsRemoved > 0 {
			fmt.Printf("Cleaned up %d orphaned container config(s)\n", configsRemoved)
		}
	}

	// Clear the tracker since we've handled orphaned containers
	if tracker != nil && len(orphanedFromCrash) > 0 {
		tracker.Clear()
	}
}

// listExistingWorktrees returns the names of all existing ccells worktrees
func listExistingWorktrees() []string {
	worktreeDir := "/tmp/ccells/worktrees"
	entries, err := os.ReadDir(worktreeDir)
	if err != nil {
		return nil // No worktrees or can't read
	}

	var worktrees []string
	for _, entry := range entries {
		if entry.IsDir() {
			worktrees = append(worktrees, entry.Name())
		}
	}
	return worktrees
}

// cleanupOrphanedWorktrees removes worktrees that are no longer associated with any workstream.
// This is very conservative - only removes worktrees that:
// 1. Are NOT in the state file (no workstream references them)
// 2. Have no corresponding running container
// 3. Have a clean working tree (no uncommitted changes)
// 4. Branch has no commits beyond the base branch
func cleanupOrphanedWorktrees(stateDir string) {
	worktreeDir := "/tmp/ccells/worktrees"
	entries, err := os.ReadDir(worktreeDir)
	if err != nil {
		return // No worktrees directory
	}

	// Load state to see which worktrees are known
	knownWorktrees := make(map[string]bool)
	if workstream.StateExists(stateDir) {
		state, err := workstream.LoadState(stateDir)
		if err == nil {
			for _, ws := range state.Workstreams {
				// Mark by sanitized branch name (worktree directory name)
				if ws.BranchName != "" {
					safeName := strings.ReplaceAll(ws.BranchName, "/", "-")
					safeName = strings.ReplaceAll(safeName, " ", "-")
					knownWorktrees[safeName] = true
				}
			}
		}
	}

	// Get current working directory for git operations
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	projectName := filepath.Base(cwd)

	// Get list of running ccells containers for this project
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runningContainerBranches := make(map[string]bool)
	client, err := docker.NewClient()
	if err == nil {
		containers, err := client.ListDockerTUIContainersForProject(ctx, projectName)
		if err == nil {
			for _, cont := range containers {
				// Container names are like "ccells-projectname-branchname"
				branchName := extractBranchFromContainerName(cont.Name, projectName)
				if branchName != "" {
					runningContainerBranches[branchName] = true
				}
			}
		}
		client.Close()
	}

	mainRepo := git.New(cwd)
	var cleaned int

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		worktreeName := entry.Name()
		worktreePath := filepath.Join(worktreeDir, worktreeName)

		// Safety check 1: Skip if in state file
		if knownWorktrees[worktreeName] {
			continue
		}

		// Safety check 2: Skip if there's a running container for this branch
		if runningContainerBranches[worktreeName] {
			continue
		}

		// Safety check 3: Check if worktree has uncommitted changes
		worktreeGit := git.New(worktreePath)
		hasChanges, err := worktreeGit.HasUncommittedChanges(ctx)
		if err != nil {
			continue // Error checking - skip to be safe
		}
		if hasChanges {
			continue // Has uncommitted changes - skip
		}

		// Safety check 4: Check if branch has commits beyond base
		// First, get the branch name from the worktree
		branchName, err := worktreeGit.CurrentBranch(ctx)
		if err != nil {
			continue // Can't determine branch - skip to be safe
		}

		hasCommits, err := mainRepo.BranchHasCommits(ctx, branchName)
		if err != nil {
			continue // Error checking - skip to be safe
		}
		if hasCommits {
			continue // Has commits - skip (user may want to keep)
		}

		// All safety checks passed - safe to remove
		// First remove from git worktree list
		if err := mainRepo.RemoveWorktree(ctx, worktreePath); err != nil {
			// Log but continue - the directory removal might still work
		}

		// Then remove the directory
		if err := os.RemoveAll(worktreePath); err == nil {
			cleaned++
		}

		// Also delete the empty branch
		if err := mainRepo.DeleteBranch(ctx, branchName); err != nil {
			// Ignore - branch might not exist or be the current branch
		}
	}

	if cleaned > 0 {
		fmt.Printf("Cleaned up %d orphaned worktree(s)\n", cleaned)
	}
}

// extractBranchFromContainerName extracts the branch name from a container name.
// Container names follow the pattern: ccells-<projectname>-<branchname>
func extractBranchFromContainerName(containerName, projectName string) string {
	prefix := "ccells-" + projectName + "-"
	if strings.HasPrefix(containerName, prefix) {
		return strings.TrimPrefix(containerName, prefix)
	}
	// Also try with leading slash (Docker sometimes includes it)
	prefix = "/ccells-" + projectName + "-"
	if strings.HasPrefix(containerName, prefix) {
		return strings.TrimPrefix(containerName, prefix)
	}
	return ""
}

// runStateRepair validates and repairs the state file by extracting session IDs from running containers
func runStateRepair() error {
	// Use the same state directory logic as the main app
	stateDir := getStateDir()

	// Check if state file exists
	if !workstream.StateExists(stateDir) {
		fmt.Printf("No state file found at %s. Nothing to repair.\n", stateDir)
		return nil
	}

	// Load current state
	state, err := workstream.LoadState(stateDir)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	if len(state.Workstreams) == 0 {
		fmt.Println("State file has no workstreams. Nothing to repair.")
		return nil
	}

	fmt.Printf("Found %d workstream(s) in state file\n", len(state.Workstreams))

	// Convert saved workstreams to full workstreams for repair
	var workstreams []*workstream.Workstream
	for _, saved := range state.Workstreams {
		ws := workstream.NewWithID(saved.ID, saved.BranchName, saved.Prompt)
		ws.ContainerID = saved.ContainerID
		ws.Title = saved.Title
		ws.Synopsis = saved.Synopsis
		ws.CreatedAt = saved.CreatedAt
		ws.ClaudeSessionID = saved.ClaudeSessionID
		workstreams = append(workstreams, ws)
	}

	// Show current state
	fmt.Println("\nCurrent state:")
	for i, ws := range workstreams {
		sessionID := ws.GetClaudeSessionID()
		if sessionID == "" {
			sessionID = "(missing)"
		}
		fmt.Printf("  %d. %s\n     Container: %s\n     Session ID: %s\n",
			i+1, ws.Title, ws.ContainerID[:12], sessionID)
	}

	// Run validation and repair
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("\nRepairing state...")
	result, err := workstream.ValidateAndRepairState(ctx, workstreams)
	if err != nil {
		return fmt.Errorf("repair failed: %w", err)
	}

	fmt.Printf("\nResult: %s\n", result.Summary())

	if result.WasRepaired() {
		// Save the repaired state
		if err := workstream.SaveState(stateDir, workstreams, state.FocusedIndex, state.Layout); err != nil {
			return fmt.Errorf("failed to save repaired state: %w", err)
		}
		fmt.Println("State file updated successfully.")

		// Show updated state
		fmt.Println("\nUpdated state:")
		for i, ws := range workstreams {
			sessionID := ws.GetClaudeSessionID()
			if sessionID == "" {
				sessionID = "(missing)"
			}
			fmt.Printf("  %d. %s\n     Container: %s\n     Session ID: %s\n",
				i+1, ws.Title, ws.ContainerID[:12], sessionID)
		}
	} else if result.IsCorrupted() {
		fmt.Println("\nWarning: Some session IDs could not be recovered.")
		fmt.Println("The affected containers may start with fresh sessions on next launch.")
	} else {
		fmt.Println("\nState is already valid. No repairs needed.")
	}

	return nil
}
