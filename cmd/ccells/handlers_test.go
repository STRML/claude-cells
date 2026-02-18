package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/STRML/claude-cells/internal/orchestrator"
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

func TestDockerExecCmd_HasAutoAccepter(t *testing.T) {
	cmd := dockerExecCmd("test-container", "claude")

	// The auto-accepter is a fallback for the "Bypass Permissions mode" prompt.
	// settings.json has skipDangerousModePermissionPrompt: true but it's unreliable,
	// so the auto-accepter watches the pane and sends keystrokes if needed.
	if !strings.Contains(cmd, "tmux send-keys") {
		t.Error("should contain tmux send-keys auto-accepter")
	}
	if !strings.Contains(cmd, "Bypass Permissions mode") {
		t.Error("should contain auto-accepter grep for Bypass Permissions mode")
	}
	if !strings.Contains(cmd, "sh -c") {
		t.Error("should wrap in sh -c for auto-accepter")
	}
	if !strings.Contains(cmd, "$TMUX_PANE") {
		t.Error("auto-accepter should target $TMUX_PANE")
	}
}

func TestWorktreePathDerivation(t *testing.T) {
	// Verify that the worktree path derivation in startGitProxiesForExistingPanes
	// matches what the orchestrator does in createWorktree.
	tests := []struct {
		branch   string
		expected string
	}{
		{"my-branch", filepath.Join(orchestrator.DefaultWorktreeBaseDir, "my-branch")},
		{"feature/foo", filepath.Join(orchestrator.DefaultWorktreeBaseDir, "feature-foo")},
		{"feature/sub/foo", filepath.Join(orchestrator.DefaultWorktreeBaseDir, "feature-sub-foo")},
	}
	for _, tt := range tests {
		safeName := orchestrator.SanitizeBranchName(tt.branch)
		got := filepath.Join(orchestrator.DefaultWorktreeBaseDir, safeName)
		if got != tt.expected {
			t.Errorf("worktree path for %q = %q, want %q", tt.branch, got, tt.expected)
		}
	}
}

func TestActionHandlers_GitProxyNilSafe(t *testing.T) {
	// Verify that handlers work correctly when gitProxy is nil
	// (e.g., if git proxy initialization fails).
	h := &actionHandlers{
		gitProxy: nil,
	}

	// startGitProxiesForExistingPanes should be a no-op when gitProxy is nil
	h.startGitProxiesForExistingPanes(nil)
	// No panic = success
}
