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
