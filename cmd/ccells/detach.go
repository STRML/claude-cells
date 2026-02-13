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

// ANSI color helpers for terminal output (not tmux format sequences).
const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiDim     = "\033[2m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiCyan    = "\033[36m"
	ansiMagenta = "\033[35m"
)

// formatDetachSummary produces the text printed after tmux detach.
func formatDetachSummary(info DetachInfo) string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s%s● ccells session detached%s\n", ansiBold, ansiGreen, ansiReset))
	b.WriteString(fmt.Sprintf("  %s──────────────────────────%s\n\n", ansiDim, ansiReset))

	if len(info.Workstreams) > 0 {
		b.WriteString(fmt.Sprintf("  %s%d workstream(s) running in background:%s\n\n", ansiCyan, len(info.Workstreams), ansiReset))
		// Header
		b.WriteString(fmt.Sprintf("  %s%-20s %-10s %-8s %-10s %s%s\n",
			ansiDim, "WORKSTREAM", "STATUS", "CPU", "MEMORY", "PR", ansiReset))

		for _, ws := range info.Workstreams {
			pr := ws.PR
			if pr == "" {
				pr = "-"
			}
			statusColor := ansiGreen
			if ws.Status == "paused" {
				statusColor = ansiYellow
			}
			b.WriteString(fmt.Sprintf("  %s%-20s%s %s%-10s%s %-8s %-10s %s\n",
				ansiMagenta, ws.Name, ansiReset,
				statusColor, ws.Status, ansiReset,
				ws.CPU, ws.Memory, pr))
		}
		if info.DaemonRunning {
			b.WriteString(fmt.Sprintf("  %sDaemon: running%s\n", ansiDim, ansiReset))
		}
		b.WriteString("\n")
	} else {
		b.WriteString(fmt.Sprintf("  %sNo workstreams running.%s\n", ansiDim, ansiReset))
		if info.DaemonRunning {
			b.WriteString(fmt.Sprintf("  %sDaemon: running%s\n", ansiDim, ansiReset))
		}
		b.WriteString("\n")
	}

	b.WriteString(fmt.Sprintf("  %sReattach:%s  ccells attach\n", ansiDim, ansiReset))
	b.WriteString(fmt.Sprintf("  %sStop all:%s  ccells down\n", ansiDim, ansiReset))
	b.WriteString(fmt.Sprintf("  %sDestroy:%s   ccells down --rm\n", ansiDim, ansiReset))
	b.WriteString("\n")

	return b.String()
}
