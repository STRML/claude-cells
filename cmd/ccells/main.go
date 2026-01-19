package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/STRML/claude-cells/internal/docker"
	"github.com/STRML/claude-cells/internal/tui"
	"github.com/STRML/claude-cells/internal/workstream"
	tea "github.com/charmbracelet/bubbletea"
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

func (s *spinner) Start() {
	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
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

func main() {
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

	// Start heartbeat goroutine to detect crashes
	if tracker != nil {
		go runHeartbeat(appCtx, tracker)
	}

	app := tui.NewAppModel(appCtx)

	// Set the tracker on the app so it can track container lifecycle
	if tracker != nil {
		tui.SetContainerTracker(tracker)
	}

	p := tea.NewProgram(app, tea.WithAltScreen())

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
				spin.Start()
				_, err := docker.BuildWithDevcontainerCLI(buildCtx, projectPath, &buildOutput)
				spin.Stop()
				if err != nil {
					fmt.Println(buildOutput.String()) // Show output on error
					return fmt.Errorf("failed to build with devcontainer CLI: %w", err)
				}
			} else {
				// Fall back to simple docker build
				devCfg, err := docker.LoadDevcontainerConfig(projectPath)
				if err != nil {
					return fmt.Errorf("failed to load devcontainer config: %w", err)
				}

				spin := newSpinner(fmt.Sprintf("Building image '%s'...", result.ImageName))
				spin.Start()
				err = docker.BuildProjectImage(buildCtx, projectPath, devCfg, &buildOutput)
				spin.Stop()
				if err != nil {
					fmt.Println(buildOutput.String()) // Show output on error
					return fmt.Errorf("failed to build project image: %w", err)
				}
			}

			fmt.Printf("✓ Image '%s' built successfully\n", result.ImageName)
		} else if result.ImageName == docker.DefaultImage {
			// Build the default ccells image
			var buildOutput bytes.Buffer
			buildCtx, buildCancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer buildCancel()

			spin := newSpinner(fmt.Sprintf("Building image '%s'...", docker.DefaultImage))
			spin.Start()
			err := docker.BuildImage(buildCtx, &buildOutput)
			spin.Stop()
			if err != nil {
				fmt.Println(buildOutput.String()) // Show output on error
				return fmt.Errorf("failed to build image: %w", err)
			}

			fmt.Printf("✓ Image '%s' built successfully\n", docker.DefaultImage)
		} else {
			// External image from devcontainer.json - prompt to pull
			return fmt.Errorf("image '%s' from devcontainer.json not found. Run: docker pull %s", result.ImageName, result.ImageName)
		}

		// Re-validate to confirm
		result, err = docker.ValidatePrerequisites(ctx, projectPath)
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
// It uses the container tracker (if available) and state file to find which containers should be kept.
func cleanupOrphanedContainers(tracker *docker.ContainerTracker) {
	// Get current working directory for state file
	cwd, err := os.Getwd()
	if err != nil {
		return // Silently skip if we can't get cwd
	}

	// Collect known container IDs from multiple sources

	// 1. From state file (for graceful shutdown resume)
	var knownIDs []string
	if workstream.StateExists(cwd) {
		state, err := workstream.LoadState(cwd)
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

	// Create docker client and clean up orphaned containers
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := docker.NewClient()
	if err != nil {
		return // Silently skip if we can't connect to Docker
	}
	defer client.Close()

	// Clean up containers that aren't in knownIDs (from state file)
	removed, err := client.CleanupOrphanedContainers(ctx, knownIDs)
	if err == nil && removed > 0 {
		fmt.Printf("Cleaned up %d orphaned container(s) from previous session\n", removed)
	}

	// Clear the tracker since we've handled orphaned containers
	if tracker != nil && len(orphanedFromCrash) > 0 {
		tracker.Clear()
	}
}
