package main

import (
	"strings"
	"testing"
)

func TestDockerExecCmd(t *testing.T) {
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
			wantExec:  "docker exec -it test-container claude --dangerously-skip-permissions",
		},
		{
			name:       "with prompt as positional arg",
			container:  "test-container",
			runtime:    "claude",
			extraFlags: []string{"'fix the bug'"},
			wantExec:   "docker exec -it test-container claude --dangerously-skip-permissions 'fix the bug'",
		},
		{
			name:       "with resume flag",
			container:  "test-container",
			runtime:    "claude",
			extraFlags: []string{"--resume"},
			wantExec:   "docker exec -it test-container claude --dangerously-skip-permissions --resume",
		},
		{
			name:       "prompt must not use -p flag for claude",
			container:  "my-container",
			runtime:    "claudesp",
			extraFlags: []string{"'my task'"},
			wantExec:   "docker exec -it my-container claudesp --dangerously-skip-permissions 'my task'",
			notWant:    "--dangerously-skip-permissions -p",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dockerExecCmd(tt.container, tt.runtime, tt.extraFlags...)
			if !strings.Contains(got, tt.wantExec) {
				t.Errorf("dockerExecCmd() = %q, want substring %q", got, tt.wantExec)
			}
			if tt.notWant != "" && strings.Contains(got, tt.notWant) {
				t.Errorf("dockerExecCmd() = %q, must NOT contain %q", got, tt.notWant)
			}
		})
	}
}

func TestDockerExecCmd_NoAutoAccepter(t *testing.T) {
	cmd := dockerExecCmd("test-container", "claude")

	// The skipDangerousModePermissionPrompt setting in settings.json handles
	// the bypass prompt, so no background auto-accepter should be present.
	if strings.Contains(cmd, "tmux send-keys") {
		t.Error("should not contain tmux send-keys auto-accepter")
	}
	if strings.Contains(cmd, "Bypass Permissions mode") {
		t.Error("should not contain auto-accepter grep for Bypass Permissions mode")
	}
	if strings.Contains(cmd, "sh -c") {
		t.Error("should not wrap in sh -c (no auto-accepter needed)")
	}
}
