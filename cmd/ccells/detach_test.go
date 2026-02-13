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
