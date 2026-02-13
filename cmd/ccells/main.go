package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
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
	"github.com/STRML/claude-cells/internal/tmux"
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
				fmt.Print("\r\033[K")
				return
			case <-s.done:
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
		pidStr := strings.TrimSpace(string(data))
		if pid, err := strconv.Atoi(pidStr); err == nil {
			if process, err := os.FindProcess(pid); err == nil {
				if err := process.Signal(syscall.Signal(0)); err == nil {
					return nil, fmt.Errorf("another ccells instance is already running (PID %d)", pid)
				}
			}
		}
		os.Remove(lockPath)
	}

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

// getRepoInfo returns the repoID, project path, and state directory.
func getRepoInfo() (repoID, repoPath, stateDir string, err error) {
	repoPath, err = os.Getwd()
	if err != nil {
		return "", "", "", fmt.Errorf("failed to get working directory: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gitOps := git.New(repoPath)
	repoID, err = gitOps.RepoID(ctx)
	if err != nil || repoID == "" {
		return "", repoPath, repoPath, nil
	}

	stateDir, err = workstream.GetStateDir(repoID)
	if err != nil {
		return repoID, repoPath, repoPath, nil
	}

	return repoID, repoPath, stateDir, nil
}

// getStateDir returns the state directory for the current repo.
// Returns an error if the repo info cannot be determined.
func getStateDir() (string, error) {
	_, _, stateDir, err := getRepoInfo()
	if err != nil {
		return "", err
	}
	if stateDir == "" {
		return "", fmt.Errorf("could not determine state directory (not in a git repository?)")
	}
	return stateDir, nil
}

func printKeybindings() {
	fmt.Print(`
  Claude Cells - Keybindings
  ══════════════════════════

  prefix + n     Create new workstream
  prefix + x     Destroy workstream
  prefix + p     Pause current workstream
  prefix + r     Resume current workstream
  prefix + m     Create/view pull request
  prefix + s     Refresh status line
  prefix + ?     Show this help

  Standard tmux keybindings also work:
  prefix + arrow  Navigate between panes
  prefix + z      Zoom current pane (toggle)
  prefix + d      Detach from session

  Press q or Esc to close.
`)
}

func printHelp() {
	fmt.Printf(`ccells - Claude Cells: Run parallel Claude Code instances in Docker containers

Usage:
  ccells [command] [options]

Commands:
  up              Start session (create + attach) [default]
  attach          Reattach to running session
  down            Stop session (daemon + tmux)
  down --rm       Stop session and destroy containers
  create          Create a new workstream
  rm <name>       Destroy a workstream
  pause <name>    Pause a workstream
  unpause <name>  Resume a workstream
  ps              List workstreams with status
  pair <name>     Start pairing mode (live sync with local)
  unpair          Stop pairing mode
  status          Show pairing status

Options:
  -h, --help          Show this help message
  -v, --version       Show version information
  --runtime <name>    Runtime to use: "claude" (default) or "claudesp" (experimental)
  --repair-state      Validate and repair the state file

Keybindings (in tmux session, prefix + key):
  n     Create new workstream
  x     Destroy workstream (with confirmation)
  p     Pause workstream
  r     Resume workstream
  m     Create/view PR
  ?     Show help

For more information: https://github.com/STRML/claude-cells
`)
}

func main() {
	// Suppress log output to stderr unless debugging
	log.SetOutput(os.Stderr)

	// Parse global flags
	runtimeFlag, args := parseFlags(os.Args[1:])

	// Validate runtime flag early
	if runtimeFlag != "" {
		runtimeFlag = strings.ToLower(strings.TrimSpace(runtimeFlag))
		if !isValidRuntime(runtimeFlag) {
			fmt.Fprintf(os.Stderr, "Error: invalid runtime %q (must be one of: %v)\n", runtimeFlag, AllowedRuntimes)
			os.Exit(1)
		}
	}

	// Prevent nesting: ccells cannot run inside a ccells-managed container
	if os.Getenv("CCELLS_SESSION") == "1" {
		fmt.Fprintf(os.Stderr, "Error: ccells cannot run inside a ccells-managed container.\n")
		fmt.Fprintf(os.Stderr, "You are already in a Claude Code workstream.\n")
		os.Exit(1)
	}

	// Determine command
	cmd := parseCommand(args)

	// Handle commands that don't need repo context
	switch cmd {
	case "help":
		// Check for --keybindings flag (used by tmux ? popup)
		for _, a := range args {
			if a == "--keybindings" {
				printKeybindings()
				os.Exit(0)
			}
		}
		printHelp()
		os.Exit(0)
	case "version":
		fmt.Printf("ccells %s (%s)\n", Version, CommitHash)
		os.Exit(0)
	}

	// Handle --repair-state (special case, not a subcommand)
	if len(args) > 0 && args[0] == "--repair-state" {
		if err := runStateRepair(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Resolve repo info
	repoID, repoPath, stateDir, err := getRepoInfo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Resolve runtime from flag + config
	runtime, err := ResolveRuntime(runtimeFlag, repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Create cancellable context for the application
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	// Set up signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		appCancel()
	}()

	// Extract subcommand args (everything after the command name)
	cmdArgs := args
	if len(cmdArgs) > 0 {
		cmdArgs = cmdArgs[1:]
	}

	// Dispatch command
	switch cmd {
	case "up":
		// Validate prerequisites before starting
		if err := validatePrerequisites(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Acquire lock
		lock, err := acquireLock(stateDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			fmt.Fprintf(os.Stderr, "If the other instance crashed, delete: %s/%s\n", stateDir, lockFileName)
			os.Exit(1)
		}
		defer lock.Release()

		// Initialize container tracker for crash recovery
		tracker, err := docker.NewContainerTracker()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to initialize container tracker: %v\n", err)
		}

		// Clean up orphaned containers and worktrees
		cleanupOrphanedContainers(tracker)
		cleanupOrphanedWorktrees(stateDir)

		// Start heartbeat
		if tracker != nil {
			go runHeartbeat(appCtx, tracker)
			defer func() {
				tracker.RemoveHeartbeat()
				tracker.Clear()
			}()
		}

		if err := runUp(appCtx, repoID, repoPath, stateDir, runtime); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "attach":
		if err := runAttach(appCtx, repoID, stateDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "down":
		destroyContainers := false
		for _, arg := range cmdArgs {
			if arg == "--rm" {
				destroyContainers = true
			}
		}
		if err := runDown(appCtx, repoID, stateDir, destroyContainers); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "create":
		branch := ""
		prompt := ""
		interactive := false
		for i := 0; i < len(cmdArgs); i++ {
			switch cmdArgs[i] {
			case "--branch", "-b":
				if i+1 < len(cmdArgs) {
					branch = cmdArgs[i+1]
					i++
				}
			case "--prompt", "-p":
				if i+1 < len(cmdArgs) {
					prompt = cmdArgs[i+1]
					i++
				}
			case "--interactive", "-i":
				interactive = true
			default:
				// Positional: treat as branch if not set
				if branch == "" {
					branch = cmdArgs[i]
				}
			}
		}

		if interactive || (branch == "" && prompt == "") {
			// Interactive mode — show dialog via tmux popup
			if err := runCreateInteractive(stateDir, runtime); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		} else {
			if branch != "" {
				if err := validateBranchName(branch); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			}
			if err := runCreate(stateDir, branch, prompt, runtime); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}

	case "rm":
		name := ""
		interactive := false
		for _, arg := range cmdArgs {
			switch arg {
			case "--interactive", "-i":
				interactive = true
			default:
				if name == "" {
					name = arg
				}
			}
		}
		if interactive {
			if err := runRmInteractive(appCtx, repoID, stateDir); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		} else {
			if name == "" {
				fmt.Fprintf(os.Stderr, "Usage: ccells rm <workstream-name>\n")
				os.Exit(1)
			}
			if err := runRemove(stateDir, name); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}

	case "pause":
		name := ""
		if len(cmdArgs) > 0 {
			name = cmdArgs[0]
		}
		if name == "" {
			fmt.Fprintf(os.Stderr, "Usage: ccells pause <workstream-name>\n")
			os.Exit(1)
		}
		if err := runPause(stateDir, name); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "unpause":
		name := ""
		if len(cmdArgs) > 0 {
			name = cmdArgs[0]
		}
		if name == "" {
			fmt.Fprintf(os.Stderr, "Usage: ccells unpause <workstream-name>\n")
			os.Exit(1)
		}
		if err := runUnpause(stateDir, name); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "ps":
		if err := runPS(appCtx, repoID); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "pair":
		name := ""
		if len(cmdArgs) > 0 {
			name = cmdArgs[0]
		}
		if name == "" {
			fmt.Fprintf(os.Stderr, "Usage: ccells pair <workstream-name>\n")
			os.Exit(1)
		}
		if err := runPair(stateDir, name); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "unpair":
		if err := runUnpair(stateDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "status":
		formatTmux := false
		for _, arg := range cmdArgs {
			if arg == "--format=tmux" {
				formatTmux = true
			}
		}
		if formatTmux {
			if err := runStatusTmux(appCtx, repoID); err != nil {
				// Silently fail — tmux status line should not show errors
				fmt.Print("[ccells]")
			}
		} else {
			if err := runPairStatus(stateDir); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}

	case "merge":
		interactive := false
		for _, arg := range cmdArgs {
			if arg == "--interactive" || arg == "-i" {
				interactive = true
			}
		}
		if interactive {
			if err := runMergeInteractive(appCtx, repoID, stateDir); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		} else {
			fmt.Println("Usage: ccells merge --interactive (or use prefix+m keybinding)")
		}

	case "welcome":
		if err := runWelcome(stateDir, runtime); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "logs":
		// TODO: implement logs command
		fmt.Println("logs command not yet implemented")

	default:
		printHelp()
		os.Exit(1)
	}
}

// runCreateInteractive launches the interactive create dialog as a Bubble Tea program.
func runCreateInteractive(stateDir, runtime string) error {
	m := newCreateDialog(stateDir, runtime)
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}

// runRmInteractive launches the interactive rm dialog as a Bubble Tea program.
func runRmInteractive(ctx context.Context, repoID, stateDir string) error {
	names, err := listWorkstreamNames(ctx, repoID)
	if err != nil {
		return err
	}
	m := newRmDialog(stateDir, names)
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

// runMergeInteractive launches the interactive merge dialog as a Bubble Tea program.
func runMergeInteractive(ctx context.Context, repoID, stateDir string) error {
	names, err := listWorkstreamNames(ctx, repoID)
	if err != nil {
		return err
	}
	m := newMergeDialog(names)
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}

// listWorkstreamNames returns workstream names from tmux panes.
func listWorkstreamNames(ctx context.Context, repoID string) ([]string, error) {
	socketName := fmt.Sprintf("ccells-%s", repoID)
	client := tmux.NewClient(socketName)

	panes, err := client.ListPanes(ctx, "ccells")
	if err != nil {
		return nil, err
	}

	var names []string
	for _, p := range panes {
		ws, _ := client.GetPaneOption(ctx, p.ID, "@ccells-workstream")
		if ws != "" {
			names = append(names, ws)
		}
	}
	return names, nil
}

// runStatusTmux prints the colored workstream status to stdout.
// tmux's #() command substitution interprets the #[...] color sequences in the output.
func runStatusTmux(ctx context.Context, repoID string) error {
	socketName := fmt.Sprintf("ccells-%s", repoID)
	client := tmux.NewClient(socketName)

	prefix, _ := client.Prefix(ctx)

	panes, err := client.ListPanes(ctx, "ccells")
	if err != nil {
		return err
	}

	var workstreams []tmux.StatusWorkstream
	for _, p := range panes {
		ws, _ := client.GetPaneOption(ctx, p.ID, "@ccells-workstream")
		if ws == "" {
			continue
		}

		// Determine status from pane metadata
		status := "running"
		if paneStatus, _ := client.GetPaneOption(ctx, p.ID, "@ccells-status"); paneStatus == "paused" {
			status = "paused"
		} else if p.Dead {
			status = "exited"
		}

		sw := tmux.StatusWorkstream{
			Name:   ws,
			Status: status,
		}
		workstreams = append(workstreams, sw)
	}

	// Print colored status line to stdout — tmux #() interprets the #[...] sequences
	colored := tmux.FormatStatusLine(workstreams, prefix, false)
	fmt.Print(colored)
	return nil
}

// runHeartbeat writes heartbeat every 5 seconds until context is cancelled
func runHeartbeat(ctx context.Context, tracker *docker.ContainerTracker) {
	pid := os.Getpid()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

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
	projectPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Check for devcontainer CLI
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
			var buildOutput bytes.Buffer
			buildCtx, buildCancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer buildCancel()

			if cliStatus.Available {
				spin := newSpinner(fmt.Sprintf("Building image '%s' with devcontainer CLI...", result.ImageName))
				spin.Start(buildCtx)
				baseImage, err := docker.BuildWithDevcontainerCLI(buildCtx, projectPath, &buildOutput)
				spin.Stop()
				if err != nil {
					fmt.Println(buildOutput.String())
					return fmt.Errorf("failed to build with devcontainer CLI: %w", err)
				}
				spin = newSpinner("Adding Claude Code to image...")
				spin.Start(buildCtx)
				err = docker.BuildEnhancedImage(buildCtx, baseImage, result.ImageName, &buildOutput)
				spin.Stop()
				if err != nil {
					fmt.Println(buildOutput.String())
					return fmt.Errorf("failed to build enhanced image: %w", err)
				}
			} else {
				devCfg, err := docker.LoadDevcontainerConfig(projectPath)
				if err != nil {
					return fmt.Errorf("failed to load devcontainer config: %w", err)
				}

				spin := newSpinner(fmt.Sprintf("Building image '%s'...", result.ImageName))
				spin.Start(buildCtx)
				err = docker.BuildProjectImage(buildCtx, projectPath, devCfg, &buildOutput)
				spin.Stop()
				if err != nil {
					fmt.Println(buildOutput.String())
					return fmt.Errorf("failed to build project image: %w", err)
				}
			}

			fmt.Printf("Image '%s' built successfully\n", result.ImageName)
		} else if result.ImageName == docker.GetBaseImageName() {
			var buildOutput bytes.Buffer
			buildCtx, buildCancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer buildCancel()

			spin := newSpinner(fmt.Sprintf("Building image '%s'...", result.ImageName))
			spin.Start(buildCtx)
			err := docker.BuildImage(buildCtx, &buildOutput)
			spin.Stop()
			if err != nil {
				fmt.Println(buildOutput.String())
				return fmt.Errorf("failed to build image: %w", err)
			}

			fmt.Printf("Image '%s' built successfully\n", result.ImageName)
		} else {
			return fmt.Errorf("image '%s' from devcontainer.json not found. Run: docker pull %s", result.ImageName, result.ImageName)
		}

		// Re-validate
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
func cleanupOrphanedContainers(tracker *docker.ContainerTracker) {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}

	projectName := filepath.Base(cwd)
	stateDir, err := getStateDir()
	if err != nil {
		return
	}

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

	var orphanedFromCrash []docker.TrackedContainer
	if tracker != nil {
		orphanedFromCrash = tracker.GetOrphanedContainers()
		if len(orphanedFromCrash) > 0 {
			fmt.Printf("Detected %d container(s) from crashed session\n", len(orphanedFromCrash))
		}
	}

	existingWorktrees := listExistingWorktrees()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := docker.NewClient()
	if err != nil {
		return
	}
	defer client.Close()

	removed, err := client.CleanupOrphanedContainers(ctx, projectName, knownIDs, existingWorktrees)
	if err == nil && removed > 0 {
		fmt.Printf("Cleaned up %d orphaned container(s) from previous session\n", removed)
	}

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

	if tracker != nil && len(orphanedFromCrash) > 0 {
		tracker.Clear()
	}
}

// listExistingWorktrees returns the names of all existing ccells worktrees
func listExistingWorktrees() []string {
	worktreeDir := "/tmp/ccells/worktrees"
	entries, err := os.ReadDir(worktreeDir)
	if err != nil {
		return nil
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
func cleanupOrphanedWorktrees(stateDir string) {
	worktreeDir := "/tmp/ccells/worktrees"
	entries, err := os.ReadDir(worktreeDir)
	if err != nil {
		return
	}

	knownWorktrees := make(map[string]bool)
	if workstream.StateExists(stateDir) {
		state, err := workstream.LoadState(stateDir)
		if err == nil {
			for _, ws := range state.Workstreams {
				if ws.BranchName != "" {
					safeName := strings.ReplaceAll(ws.BranchName, "/", "-")
					safeName = strings.ReplaceAll(safeName, " ", "-")
					knownWorktrees[safeName] = true
				}
			}
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	projectName := filepath.Base(cwd)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runningContainerBranches := make(map[string]bool)
	client, err := docker.NewClient()
	if err == nil {
		containers, err := client.ListDockerTUIContainersForProject(ctx, projectName)
		if err == nil {
			for _, cont := range containers {
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

		if knownWorktrees[worktreeName] {
			continue
		}
		if runningContainerBranches[worktreeName] {
			continue
		}

		worktreeGit := git.New(worktreePath)
		hasChanges, err := worktreeGit.HasUncommittedChanges(ctx)
		if err != nil {
			continue
		}
		if hasChanges {
			continue
		}

		branchName, err := worktreeGit.CurrentBranch(ctx)
		if err != nil {
			continue
		}

		hasCommits, err := mainRepo.BranchHasCommits(ctx, branchName)
		if err != nil {
			continue
		}
		if hasCommits {
			continue
		}

		if err := mainRepo.RemoveWorktree(ctx, worktreePath); err != nil {
			// Log but continue
		}

		if err := os.RemoveAll(worktreePath); err == nil {
			cleaned++
		}

		if err := mainRepo.DeleteBranch(ctx, branchName); err != nil {
			// Ignore
		}
	}

	if cleaned > 0 {
		fmt.Printf("Cleaned up %d orphaned worktree(s)\n", cleaned)
	}
}

// extractBranchFromContainerName extracts the branch name from a container name.
func extractBranchFromContainerName(containerName, projectName string) string {
	prefix := "ccells-" + projectName + "-"
	if strings.HasPrefix(containerName, prefix) {
		return strings.TrimPrefix(containerName, prefix)
	}
	prefix = "/ccells-" + projectName + "-"
	if strings.HasPrefix(containerName, prefix) {
		return strings.TrimPrefix(containerName, prefix)
	}
	return ""
}

// runStateRepair validates and repairs the state file by extracting session IDs from running containers
func runStateRepair() error {
	stateDir, err := getStateDir()
	if err != nil {
		return fmt.Errorf("failed to determine state directory: %w", err)
	}

	if !workstream.StateExists(stateDir) {
		fmt.Printf("No state file found at %s. Nothing to repair.\n", stateDir)
		return nil
	}

	state, err := workstream.LoadState(stateDir)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	if len(state.Workstreams) == 0 {
		fmt.Println("State file has no workstreams. Nothing to repair.")
		return nil
	}

	fmt.Printf("Found %d workstream(s) in state file\n", len(state.Workstreams))

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

	fmt.Println("\nCurrent state:")
	for i, ws := range workstreams {
		sessionID := ws.GetClaudeSessionID()
		if sessionID == "" {
			sessionID = "(missing)"
		}
		containerShort := ws.ContainerID
		if len(containerShort) > 12 {
			containerShort = containerShort[:12]
		}
		fmt.Printf("  %d. %s\n     Container: %s\n     Session ID: %s\n",
			i+1, ws.Title, containerShort, sessionID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("\nRepairing state...")
	result, err := workstream.ValidateAndRepairState(ctx, workstreams)
	if err != nil {
		return fmt.Errorf("repair failed: %w", err)
	}

	fmt.Printf("\nResult: %s\n", result.Summary())

	if result.WasRepaired() {
		if err := workstream.SaveState(stateDir, workstreams, state.FocusedIndex, state.Layout); err != nil {
			return fmt.Errorf("failed to save repaired state: %w", err)
		}
		fmt.Println("State file updated successfully.")

		fmt.Println("\nUpdated state:")
		for i, ws := range workstreams {
			sessionID := ws.GetClaudeSessionID()
			if sessionID == "" {
				sessionID = "(missing)"
			}
			containerShort := ws.ContainerID
			if len(containerShort) > 12 {
				containerShort = containerShort[:12]
			}
			fmt.Printf("  %d. %s\n     Container: %s\n     Session ID: %s\n",
				i+1, ws.Title, containerShort, sessionID)
		}
	} else if result.IsCorrupted() {
		fmt.Println("\nWarning: Some session IDs could not be recovered.")
		fmt.Println("The affected containers may start with fresh sessions on next launch.")
	} else {
		fmt.Println("\nState is already valid. No repairs needed.")
	}

	return nil
}
