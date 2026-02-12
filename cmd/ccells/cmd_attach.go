package main

import (
	"context"
	"fmt"

	"github.com/STRML/claude-cells/internal/tmux"
)

// runAttach attaches to an existing session or errors if none is running.
func runAttach(ctx context.Context, repoID string) error {
	socketName := fmt.Sprintf("ccells-%s", repoID)
	client := tmux.NewClient(socketName)

	running, _ := client.HasSession(ctx, "ccells")
	if !running {
		return fmt.Errorf("no ccells session running for this repo. Run 'ccells up' first")
	}

	return doAttach(client, "ccells")
}
