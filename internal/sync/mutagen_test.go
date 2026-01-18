package sync

import (
	"context"
	"testing"
)

func TestMutagenSessionName(t *testing.T) {
	tests := []struct {
		branch   string
		expected string
	}{
		{"add-auth", "ccells-add-auth"},
		{"feature/login", "ccells-feature-login"},
	}

	for _, tt := range tests {
		got := SessionName(tt.branch)
		if got != tt.expected {
			t.Errorf("SessionName(%q) = %q, want %q", tt.branch, got, tt.expected)
		}
	}
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
