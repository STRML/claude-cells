package sync

import (
	"context"
	"testing"
)

func TestMutagenSessionName(t *testing.T) {
	tests := []struct {
		branch string
	}{
		{"add-auth"},
		{"feature/login"},
		{"very-long-branch-name-that-exceeds-twenty-characters"},
	}

	for _, tt := range tests {
		got := SessionName(tt.branch)
		// Should start with "ccells-"
		if !hasPrefix(got, "ccells-") {
			t.Errorf("SessionName(%q) = %q, should start with 'ccells-'", tt.branch, got)
		}
		// Should have hash suffix (8 hex chars after last dash)
		parts := splitAfterLast(got, "-")
		if len(parts) < 2 || len(parts[1]) != 8 {
			t.Errorf("SessionName(%q) = %q, should have 8-char hash suffix", tt.branch, got)
		}
	}
}

func TestSessionName_NoCollisions(t *testing.T) {
	// Different branches should produce different session names
	branches := []string{
		"feature/auth",
		"feature/auth-v2",
		"feature-auth",
		"feat/auth",
		"very-long-branch-name-one",
		"very-long-branch-name-two",
	}

	seen := make(map[string]string)
	for _, branch := range branches {
		name := SessionName(branch)
		if existing, ok := seen[name]; ok {
			t.Errorf("Collision: %q and %q both produce %q", existing, branch, name)
		}
		seen[name] = branch
	}
}

func TestSessionName_TruncatesLongNames(t *testing.T) {
	longBranch := "this-is-a-very-long-branch-name-that-exceeds-twenty-characters"
	name := SessionName(longBranch)

	// Name should be "ccells-" + up to 20 chars + "-" + 8 hex chars
	// Max length: 7 + 20 + 1 + 8 = 36 chars
	if len(name) > 36 {
		t.Errorf("SessionName produced name too long: %d chars: %q", len(name), name)
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func splitAfterLast(s, sep string) []string {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == sep[0] {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

func TestMutagen_CheckInstalled(t *testing.T) {
	m := NewMutagen()
	ctx := context.Background()

	err := m.CheckInstalled(ctx)
	// This test passes if mutagen is installed, skips otherwise
	if err != nil {
		t.Skipf("mutagen not installed: %v", err)
	}
}

func TestMutagen_CreateSession_InvalidContainer(t *testing.T) {
	m := NewMutagen()
	ctx := context.Background()

	err := m.CreateSession(ctx, "nonexistent-branch", "invalid-container", "/tmp")
	if err == nil {
		t.Error("CreateSession() should fail with invalid container")
	}
}

func TestSyncStatus_String(t *testing.T) {
	tests := []struct {
		status   SyncStatus
		expected string
	}{
		{SyncStatusUnknown, "Unknown"},
		{SyncStatusDisconnected, "Disconnected"},
		{SyncStatusConnecting, "Connecting"},
		{SyncStatusWatching, "Watching"},
		{SyncStatusScanning, "Scanning"},
		{SyncStatusSyncing, "Syncing"},
		{SyncStatusConflicted, "Conflicted"},
		{SyncStatusError, "Error"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.status.String(); got != tt.expected {
				t.Errorf("SyncStatus.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSyncStatus_IsActive(t *testing.T) {
	tests := []struct {
		status   SyncStatus
		expected bool
	}{
		{SyncStatusUnknown, false},
		{SyncStatusDisconnected, false},
		{SyncStatusConnecting, true},
		{SyncStatusWatching, false},
		{SyncStatusScanning, true},
		{SyncStatusSyncing, true},
		{SyncStatusConflicted, false},
		{SyncStatusError, false},
	}

	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			if got := tt.status.IsActive(); got != tt.expected {
				t.Errorf("SyncStatus.IsActive() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseSessionStatus(t *testing.T) {
	tests := []struct {
		name           string
		output         string
		expectedStatus SyncStatus
		expectedText   string
		hasConflicts   bool
		hasProblems    bool
	}{
		{
			name: "watching for changes",
			output: `Name: test-session
Status: Watching for changes
`,
			expectedStatus: SyncStatusWatching,
			expectedText:   "Watching for changes",
		},
		{
			name: "scanning alpha",
			output: `Name: test-session
Status: Scanning (alpha)
`,
			expectedStatus: SyncStatusScanning,
			expectedText:   "Scanning (alpha)",
		},
		{
			name: "staging files",
			output: `Name: test-session
Status: Staging files on beta
`,
			expectedStatus: SyncStatusSyncing,
			expectedText:   "Staging files on beta",
		},
		{
			name: "reconciling changes",
			output: `Name: test-session
Status: Reconciling changes
`,
			expectedStatus: SyncStatusSyncing,
			expectedText:   "Reconciling changes",
		},
		{
			name: "connecting",
			output: `Name: test-session
Status: Connecting to beta
`,
			expectedStatus: SyncStatusConnecting,
			expectedText:   "Connecting to beta",
		},
		{
			name: "with conflicts",
			output: `Name: test-session
Status: Watching for changes
Conflicts: 2 files
`,
			expectedStatus: SyncStatusConflicted,
			expectedText:   "Watching for changes",
			hasConflicts:   true,
		},
		{
			name: "with problems",
			output: `Name: test-session
Status: Watching for changes
Problems: Unable to scan directory
`,
			expectedStatus: SyncStatusWatching,
			expectedText:   "Watching for changes",
			hasProblems:    true,
		},
		{
			name: "transferring",
			output: `Name: test-session
Status: Transferring files to beta
`,
			expectedStatus: SyncStatusSyncing,
			expectedText:   "Transferring files to beta",
		},
		{
			name: "applying changes",
			output: `Name: test-session
Status: Applying changes on alpha
`,
			expectedStatus: SyncStatusSyncing,
			expectedText:   "Applying changes on alpha",
		},
		{
			name: "disconnected",
			output: `Name: test-session
Status: Disconnected
`,
			expectedStatus: SyncStatusDisconnected,
			expectedText:   "Disconnected",
		},
		{
			name: "error status",
			output: `Name: test-session
Status: Error: permission denied
`,
			expectedStatus: SyncStatusError,
			expectedText:   "Error: permission denied",
		},
		{
			name:           "empty output",
			output:         ``,
			expectedStatus: SyncStatusUnknown,
			expectedText:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := parseSessionStatus(tt.output)

			if status.Status != tt.expectedStatus {
				t.Errorf("Status = %v, want %v", status.Status, tt.expectedStatus)
			}
			if status.StatusText != tt.expectedText {
				t.Errorf("StatusText = %q, want %q", status.StatusText, tt.expectedText)
			}
			if tt.hasConflicts && len(status.Conflicts) == 0 {
				t.Error("Expected conflicts but got none")
			}
			if tt.hasProblems && len(status.Problems) == 0 {
				t.Error("Expected problems but got none")
			}
		})
	}
}
