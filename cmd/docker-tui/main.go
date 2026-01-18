package main

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/samuelreed/docker-tui/internal/docker"
	"github.com/samuelreed/docker-tui/internal/tui"
)

func main() {
	// Validate Docker prerequisites before starting
	if err := validatePrerequisites(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	app := tui.NewAppModel()
	p := tea.NewProgram(app, tea.WithAltScreen())

	// Set the program reference so PTY sessions can send messages
	tui.SetProgram(p)

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
