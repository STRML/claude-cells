package main

import (
	"strings"
	"testing"
)

func TestFormatDetachSummary(t *testing.T) {
	info := DetachInfo{
		Workstreams: []DetachWorkstream{
			{Name: "auth-system", Status: "running", CPU: "2.3%", Memory: "128MB", PR: "#42"},
			{Name: "fix-bug", Status: "paused", CPU: "0.0%", Memory: "64MB"},
		},
		DaemonRunning: true,
		RepoID:        "test-repo",
	}

	out := formatDetachSummary(info)

	if !strings.Contains(out, "auth-system") {
		t.Error("expected auth-system in output")
	}
	if !strings.Contains(out, "Daemon: running") {
		t.Error("expected daemon status in output")
	}
	if !strings.Contains(out, "ccells attach") {
		t.Error("expected reattach command")
	}
	if !strings.Contains(out, "ccells down --rm") {
		t.Error("expected destroy command")
	}
	if !strings.Contains(out, "#42") {
		t.Error("expected PR number")
	}
}

func TestFormatDetachSummaryEmpty(t *testing.T) {
	info := DetachInfo{}
	out := formatDetachSummary(info)

	if !strings.Contains(out, "No workstreams") {
		t.Error("expected 'No workstreams' for empty list")
	}
}

func TestFormatDetachSummary_ANSIColors(t *testing.T) {
	info := DetachInfo{
		Workstreams: []DetachWorkstream{
			{Name: "auth", Status: "running", CPU: "1%", Memory: "64MB"},
			{Name: "fix", Status: "paused", CPU: "0%", Memory: "32MB"},
		},
		DaemonRunning: true,
	}

	out := formatDetachSummary(info)

	// Verify ANSI escape codes are present
	if !strings.Contains(out, ansiGreen) {
		t.Error("expected green ANSI code for running status and header")
	}
	if !strings.Contains(out, ansiYellow) {
		t.Error("expected yellow ANSI code for paused status")
	}
	if !strings.Contains(out, ansiMagenta) {
		t.Error("expected magenta ANSI code for workstream names")
	}
	if !strings.Contains(out, ansiCyan) {
		t.Error("expected cyan ANSI code for workstream count")
	}
	if !strings.Contains(out, ansiBold) {
		t.Error("expected bold ANSI code in header")
	}
	if !strings.Contains(out, ansiReset) {
		t.Error("expected reset ANSI code")
	}
	if !strings.Contains(out, ansiDim) {
		t.Error("expected dim ANSI code for labels")
	}
}

func TestFormatDetachSummary_PRDash(t *testing.T) {
	info := DetachInfo{
		Workstreams: []DetachWorkstream{
			{Name: "ws1", Status: "running", CPU: "1%", Memory: "64MB", PR: ""},
		},
	}

	out := formatDetachSummary(info)

	// Empty PR should show "-"
	if !strings.Contains(out, " -\n") {
		t.Error("expected dash for empty PR")
	}
}

func TestFormatDetachSummary_DaemonNotRunning(t *testing.T) {
	info := DetachInfo{
		Workstreams: []DetachWorkstream{
			{Name: "ws1", Status: "running", CPU: "1%", Memory: "64MB"},
		},
		DaemonRunning: false,
	}

	out := formatDetachSummary(info)

	if strings.Contains(out, "Daemon: running") {
		t.Error("should not show daemon status when not running")
	}
}

func TestFormatDetachSummary_EmptyWithDaemon(t *testing.T) {
	info := DetachInfo{
		DaemonRunning: true,
	}

	out := formatDetachSummary(info)

	if !strings.Contains(out, "No workstreams") {
		t.Error("expected 'No workstreams' for empty list")
	}
	if !strings.Contains(out, "Daemon: running") {
		t.Error("expected daemon status even with no workstreams")
	}
}

func TestFormatDetachSummary_Commands(t *testing.T) {
	info := DetachInfo{}
	out := formatDetachSummary(info)

	// Verify all three commands are present
	commands := []string{"ccells attach", "ccells down", "ccells down --rm"}
	for _, cmd := range commands {
		if !strings.Contains(out, cmd) {
			t.Errorf("expected command %q in output", cmd)
		}
	}

	// Verify labels
	labels := []string{"Reattach:", "Stop all:", "Destroy:"}
	for _, label := range labels {
		if !strings.Contains(out, label) {
			t.Errorf("expected label %q in output", label)
		}
	}
}
