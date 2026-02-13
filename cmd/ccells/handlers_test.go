package main

import (
	"strings"
	"testing"
)

func TestDockerExecWithAutoAccept(t *testing.T) {
	tests := []struct {
		name       string
		container  string
		runtime    string
		extraFlags []string
		wantExec   string // substring that must appear in the exec command
		notWant    string // substring that must NOT appear
	}{
		{
			name:      "no extra flags",
			container: "test-container",
			runtime:   "claude",
			wantExec:  "exec docker exec -it test-container claude --dangerously-skip-permissions",
		},
		{
			name:       "with prompt as positional arg",
			container:  "test-container",
			runtime:    "claude",
			extraFlags: []string{"'fix the bug'"},
			wantExec:   "exec docker exec -it test-container claude --dangerously-skip-permissions 'fix the bug'",
		},
		{
			name:       "with resume flag",
			container:  "test-container",
			runtime:    "claude",
			extraFlags: []string{"--resume"},
			wantExec:   "exec docker exec -it test-container claude --dangerously-skip-permissions --resume",
		},
		{
			name:       "prompt must not use -p flag for claude",
			container:  "my-container",
			runtime:    "claudesp",
			extraFlags: []string{"'my task'"},
			wantExec:   "exec docker exec -it my-container claudesp --dangerously-skip-permissions 'my task'",
			notWant:    "--dangerously-skip-permissions -p",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dockerExecWithAutoAccept(tt.container, tt.runtime, tt.extraFlags...)
			if !strings.Contains(got, tt.wantExec) {
				t.Errorf("dockerExecWithAutoAccept() = %q, want substring %q", got, tt.wantExec)
			}
			if tt.notWant != "" && strings.Contains(got, tt.notWant) {
				t.Errorf("dockerExecWithAutoAccept() = %q, must NOT contain %q", got, tt.notWant)
			}
		})
	}
}

func TestDockerExecWithAutoAccept_AutoAccepter(t *testing.T) {
	cmd := dockerExecWithAutoAccept("test-container", "claude")

	// Must include auto-accepter background process
	if !strings.Contains(cmd, "Bypass Permissions mode") {
		t.Error("expected auto-accepter to watch for 'Bypass Permissions mode'")
	}
	if !strings.Contains(cmd, "tmux send-keys") {
		t.Error("expected auto-accepter to use tmux send-keys")
	}
	if !strings.Contains(cmd, "$TMUX_PANE") {
		t.Error("expected auto-accepter to target pane via $TMUX_PANE")
	}
}
