package sync

import (
	"context"
	"testing"
)

func TestPairingState(t *testing.T) {
	p := NewPairing(nil, nil)

	if p.IsActive() {
		t.Error("IsActive() should be false initially")
	}
	if p.CurrentBranch() != "" {
		t.Error("CurrentBranch() should be empty initially")
	}
}

func TestPairing_StashedChanges(t *testing.T) {
	p := NewPairing(nil, nil)

	if p.HasStashedChanges() {
		t.Error("HasStashedChanges() should be false initially")
	}

	p.SetStashedChanges(true)

	if !p.HasStashedChanges() {
		t.Error("HasStashedChanges() should be true after SetStashedChanges(true)")
	}
}

// MockGit for testing without real git operations
type MockGit struct {
	stashCalled    bool
	checkoutCalled string
	hasChanges     bool
}

func (m *MockGit) HasUncommittedChanges(ctx context.Context) (bool, error) {
	return m.hasChanges, nil
}

func (m *MockGit) Stash(ctx context.Context) error {
	m.stashCalled = true
	return nil
}

func (m *MockGit) Checkout(ctx context.Context, branch string) error {
	m.checkoutCalled = branch
	return nil
}

func (m *MockGit) CurrentBranch(ctx context.Context) (string, error) {
	return "main", nil
}

// MockMutagen for testing
type MockMutagen struct {
	createCalled    bool
	terminateCalled bool
}

func (m *MockMutagen) CreateSession(ctx context.Context, branch, container, path string) error {
	m.createCalled = true
	return nil
}

func (m *MockMutagen) TerminateSession(ctx context.Context, branch string) error {
	m.terminateCalled = true
	return nil
}

func TestPairing_Enable_NoChanges(t *testing.T) {
	git := &MockGit{hasChanges: false}
	mutagen := &MockMutagen{}
	p := NewPairingWithMocks(git, mutagen)
	ctx := context.Background()

	err := p.Enable(ctx, "feature-branch", "container-123", "/path/to/repo")
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	if !mutagen.createCalled {
		t.Error("Mutagen CreateSession should be called")
	}
	if git.stashCalled {
		t.Error("Git Stash should not be called when no changes")
	}
	if git.checkoutCalled != "feature-branch" {
		t.Errorf("Git Checkout called with %q, want %q", git.checkoutCalled, "feature-branch")
	}
	if !p.IsActive() {
		t.Error("IsActive() should be true after Enable()")
	}
}

func TestPairing_Enable_WithChanges(t *testing.T) {
	git := &MockGit{hasChanges: true}
	mutagen := &MockMutagen{}
	p := NewPairingWithMocks(git, mutagen)
	ctx := context.Background()

	err := p.Enable(ctx, "feature-branch", "container-123", "/path/to/repo")
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	if !git.stashCalled {
		t.Error("Git Stash should be called when changes exist")
	}
	if !p.HasStashedChanges() {
		t.Error("HasStashedChanges() should be true")
	}
}

func TestPairing_Disable(t *testing.T) {
	git := &MockGit{}
	mutagen := &MockMutagen{}
	p := NewPairingWithMocks(git, mutagen)
	ctx := context.Background()

	// First enable
	_ = p.Enable(ctx, "feature-branch", "container-123", "/path/to/repo")

	// Then disable
	err := p.Disable(ctx)
	if err != nil {
		t.Fatalf("Disable() error = %v", err)
	}

	if !mutagen.terminateCalled {
		t.Error("Mutagen TerminateSession should be called")
	}
	if p.IsActive() {
		t.Error("IsActive() should be false after Disable()")
	}
}
