package main

import (
	"fmt"
	"strings"
)

// DetachInfo holds info for the detach summary.
type DetachInfo struct {
	Workstreams   []DetachWorkstream
	DaemonRunning bool
	RepoID        string
}

// DetachWorkstream holds info about a workstream for the detach summary.
type DetachWorkstream struct {
	Name   string
	Status string
	CPU    string
	Memory string
	PR     string
}

// formatDetachSummary produces the text printed after tmux detach.
func formatDetachSummary(info DetachInfo) string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString("  ccells session detached\n")
	b.WriteString("  ──────────────────────\n\n")

	if len(info.Workstreams) > 0 {
		// Header
		b.WriteString(fmt.Sprintf("  %-20s %-10s %-8s %-10s %s\n",
			"WORKSTREAM", "STATUS", "CPU", "MEMORY", "PR"))

		for _, ws := range info.Workstreams {
			pr := ws.PR
			if pr == "" {
				pr = "-"
			}
			b.WriteString(fmt.Sprintf("  %-20s %-10s %-8s %-10s %s\n",
				ws.Name, ws.Status, ws.CPU, ws.Memory, pr))
		}
		b.WriteString("\n")
	} else {
		b.WriteString("  No workstreams running.\n\n")
	}

	if info.DaemonRunning {
		b.WriteString("  Daemon: running\n")
	}

	b.WriteString("  Reattach: ccells attach\n")
	b.WriteString("  Stop all: ccells down\n")
	b.WriteString("  Destroy:  ccells down --rm\n")
	b.WriteString("\n")

	return b.String()
}
