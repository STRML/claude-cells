package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/STRML/claude-cells/internal/tmux"
)

// PSEntry represents a workstream for ps output.
type PSEntry struct {
	Name   string
	Status string
	Branch string
	PR     string
}

// formatPS formats workstream entries as a table.
func formatPS(entries []PSEntry) string {
	if len(entries) == 0 {
		return "No workstreams running."
	}

	var b strings.Builder

	// Calculate column widths
	nameW, statusW, branchW := 4, 6, 6 // minimum = header widths
	for _, e := range entries {
		if len(e.Name) > nameW {
			nameW = len(e.Name)
		}
		if len(e.Status) > statusW {
			statusW = len(e.Status)
		}
		if len(e.Branch) > branchW {
			branchW = len(e.Branch)
		}
	}

	// Header
	fmt.Fprintf(&b, "%-*s  %-*s  %-*s  %s\n",
		nameW, "NAME", statusW, "STATUS", branchW, "BRANCH", "PR")

	// Rows
	for _, e := range entries {
		pr := e.PR
		if pr == "" {
			pr = "-"
		}
		fmt.Fprintf(&b, "%-*s  %-*s  %-*s  %s\n",
			nameW, e.Name, statusW, e.Status, branchW, e.Branch, pr)
	}

	return b.String()
}

// runPS lists all workstreams with status.
func runPS(ctx context.Context, repoID string) error {
	socketName := fmt.Sprintf("ccells-%s", repoID)
	client := tmux.NewClient(socketName)

	running, _ := client.HasSession(ctx, "ccells")
	if !running {
		fmt.Println("No ccells session running for this repo.")
		return nil
	}

	panes, err := client.ListPanes(ctx, "ccells")
	if err != nil {
		return fmt.Errorf("failed to list panes: %w", err)
	}

	var entries []PSEntry
	for _, p := range panes {
		ws, _ := client.GetPaneOption(ctx, p.ID, "@ccells-workstream")
		if ws == "" {
			continue // non-ccells pane
		}
		status := "running"
		if p.Dead {
			status = "exited"
		} else {
			// Check for paused state via pane option
			paneStatus, _ := client.GetPaneOption(ctx, p.ID, "@ccells-status")
			if paneStatus == "paused" {
				status = "paused"
			}
		}

		branch, _ := client.GetPaneOption(ctx, p.ID, "@ccells-branch")
		pr, _ := client.GetPaneOption(ctx, p.ID, "@ccells-pr")

		entries = append(entries, PSEntry{
			Name:   ws,
			Status: status,
			Branch: branch,
			PR:     pr,
		})
	}

	fmt.Print(formatPS(entries))
	return nil
}
