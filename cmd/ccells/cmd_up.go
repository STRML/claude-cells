package main

import (
	"context"
	"fmt"
	"os"

	"github.com/STRML/claude-cells/internal/tmux"
)

// runUp starts the tmux session + daemon if not running, then attaches.
func runUp(ctx context.Context, repoID, repoPath, stateDir, runtime string) error {
	socketName := fmt.Sprintf("ccells-%s", repoID)
	client := tmux.NewClient(socketName)
	sessionName := "ccells"

	running, err := client.HasSession(ctx, sessionName)
	if err != nil {
		return err
	}

	if running {
		// Already running — just attach
		return doAttach(client, sessionName)
	}

	// Create new session
	if err := client.NewSession(ctx, sessionName); err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}

	// TODO(task-10): Configure tmux chrome (status line, pane borders, keybindings)
	// TODO(task-8): Start daemon for credential refresh + state reconciliation
	// TODO(task-12): Restore workstreams from state file

	return doAttach(client, sessionName)
}

// doAttach execs into the tmux session, replacing the current process's stdio.
func doAttach(client *tmux.Client, sessionName string) error {
	cmd := client.AttachCommand(sessionName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
