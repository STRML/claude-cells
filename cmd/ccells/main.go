package main

import (
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

func main() {
	// Validate Docker prerequisites before starting
	if err := validatePrerequisites(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Clean up orphaned containers from crashed sessions
	cleanupOrphanedContainers()

	app := tui.NewAppModel()
	p := tea.NewProgram(app, tea.WithAltScreen())

	// Set the program reference so PTY sessions can send messages
	tui.SetProgram(p)

	// Set up signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		// Bubbletea will handle the signal and call our shutdown logic
		// Just quit the program cleanly
		p.Quit()
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}

func validatePrerequisites() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := docker.ValidatePrerequisites(ctx)
	if err != nil {
		return fmt.Errorf("failed to validate prerequisites: %w", err)
	}

	// If Docker is available but image is missing, build it automatically
	if result.DockerAvailable && !result.ImageExists {
		fmt.Printf("Required image '%s' not found. Building automatically...\n\n", docker.RequiredImage)

		// Use a longer timeout for the build (can take several minutes)
		buildCtx, buildCancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer buildCancel()

		if err := docker.BuildImage(buildCtx, os.Stdout); err != nil {
			return fmt.Errorf("failed to build image: %w", err)
		}

		fmt.Println("\nImage built successfully!")

		// Re-validate to confirm
		result, err = docker.ValidatePrerequisites(ctx)
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
// It loads the state file to find which containers should be kept.
func cleanupOrphanedContainers() {
	// Get current working directory for state file
	cwd, err := os.Getwd()
	if err != nil {
		return // Silently skip if we can't get cwd
	}

	// Load known container IDs from state file
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

	// Create docker client and clean up orphaned containers
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := docker.NewClient()
	if err != nil {
		return // Silently skip if we can't connect to Docker
	}
	defer client.Close()

	removed, err := client.CleanupOrphanedContainers(ctx, knownIDs)
	if err == nil && removed > 0 {
		fmt.Printf("Cleaned up %d orphaned container(s) from previous session\n", removed)
	}
}
