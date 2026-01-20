package sync

import (
	"context"
	"fmt"
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
	createCalled      bool
	terminateCalled   bool
	checkInstalledErr error
	sessionExists     bool
	conflicts         []string
}

func (m *MockMutagen) CheckInstalled(ctx context.Context) error {
	return m.checkInstalledErr
}

func (m *MockMutagen) CreateSession(ctx context.Context, branch, container, path string) error {
	m.createCalled = true
	return nil
}

func (m *MockMutagen) TerminateSession(ctx context.Context, branch string) error {
	m.terminateCalled = true
	return nil
}

func (m *MockMutagen) SessionExists(ctx context.Context, branch string) (bool, error) {
	return m.sessionExists, nil
}

func (m *MockMutagen) GetConflicts(ctx context.Context, branch string) ([]string, error) {
	return m.conflicts, nil
}

func TestPairing_Enable_NoChanges(t *testing.T) {
	git := &MockGit{hasChanges: false}
	mutagen := &MockMutagen{}
	p := NewPairingWithMocks(git, mutagen)
	ctx := context.Background()

	err := p.Enable(ctx, "feature-branch", "container-123", "/path/to/repo", "main")
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
	if p.PreviousBranch() != "main" {
		t.Errorf("PreviousBranch() = %q, want %q", p.PreviousBranch(), "main")
	}
}

func TestPairing_Enable_WithChanges(t *testing.T) {
	git := &MockGit{hasChanges: true}
	mutagen := &MockMutagen{}
	p := NewPairingWithMocks(git, mutagen)
	ctx := context.Background()

	err := p.Enable(ctx, "feature-branch", "container-123", "/path/to/repo", "develop")
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
	_ = p.Enable(ctx, "feature-branch", "container-123", "/path/to/repo", "main")

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

func TestPairing_CheckPrerequisites_MutagenNotInstalled(t *testing.T) {
	mutagen := &MockMutagen{checkInstalledErr: fmt.Errorf("mutagen not found")}
	p := NewPairingWithMocks(nil, mutagen)
	ctx := context.Background()

	err := p.CheckPrerequisites(ctx)
	if err == nil {
		t.Error("CheckPrerequisites() should fail when mutagen not installed")
	}
}

func TestPairing_GetState(t *testing.T) {
	git := &MockGit{}
	mutagen := &MockMutagen{}
	p := NewPairingWithMocks(git, mutagen)
	ctx := context.Background()

	// Check initial state
	state := p.GetState()
	if state.Active {
		t.Error("Initial state should not be active")
	}

	// Enable pairing
	_ = p.Enable(ctx, "feature-branch", "container-123", "/path/to/repo", "main")

	// Check state after enable
	state = p.GetState()
	if !state.Active {
		t.Error("State should be active after Enable")
	}
	if state.CurrentBranch != "feature-branch" {
		t.Errorf("CurrentBranch = %q, want %q", state.CurrentBranch, "feature-branch")
	}
	if state.PreviousBranch != "main" {
		t.Errorf("PreviousBranch = %q, want %q", state.PreviousBranch, "main")
	}
	if state.ContainerID != "container-123" {
		t.Errorf("ContainerID = %q, want %q", state.ContainerID, "container-123")
	}
	if !state.SyncHealthy {
		t.Error("SyncHealthy should be true after Enable")
	}
}

func TestPairing_SyncHealth_DetectsLostSession(t *testing.T) {
	git := &MockGit{}
	mutagen := &MockMutagen{sessionExists: false}
	p := NewPairingWithMocks(git, mutagen)
	ctx := context.Background()

	// Enable pairing (need session to exist during enable)
	mutagen.sessionExists = true
	_ = p.Enable(ctx, "feature-branch", "container-123", "/path/to/repo", "main")

	// Simulate session being lost
	mutagen.sessionExists = false

	err := p.CheckSyncHealth(ctx)
	if err == nil {
		t.Error("CheckSyncHealth() should return error when session lost")
	}

	healthy, _ := p.GetSyncHealth()
	if healthy {
		t.Error("GetSyncHealth() should report unhealthy when session lost")
	}
}

func TestPairing_SyncHealth_DetectsConflicts(t *testing.T) {
	git := &MockGit{}
	mutagen := &MockMutagen{sessionExists: true, conflicts: []string{"file1.go", "file2.go"}}
	p := NewPairingWithMocks(git, mutagen)
	ctx := context.Background()

	_ = p.Enable(ctx, "feature-branch", "container-123", "/path/to/repo", "main")

	err := p.CheckSyncHealth(ctx)
	if err == nil {
		t.Error("CheckSyncHealth() should return error when conflicts exist")
	}

	healthy, conflicts := p.GetSyncHealth()
	if healthy {
		t.Error("GetSyncHealth() should report unhealthy with conflicts")
	}
	if len(conflicts) != 2 {
		t.Errorf("GetSyncHealth() conflicts = %d, want 2", len(conflicts))
	}
}
