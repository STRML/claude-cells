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
	syncStatus        SyncStatus
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

func (m *MockMutagen) GetSessionStatus(ctx context.Context, branch string) (*SessionStatus, error) {
	status := m.syncStatus
	if status == SyncStatusUnknown && m.sessionExists {
		status = SyncStatusWatching // Default to watching if session exists
	}
	if !m.sessionExists {
		status = SyncStatusDisconnected
	}
	if len(m.conflicts) > 0 {
		status = SyncStatusConflicted
	}
	return &SessionStatus{
		Status:    status,
		Conflicts: m.conflicts,
	}, nil
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

func TestPairing_CheckSyncHealth_UpdatesEnhancedFields(t *testing.T) {
	tests := []struct {
		name           string
		syncStatus     SyncStatus
		sessionExists  bool
		conflicts      []string
		expectedStatus SyncStatus
		expectHealthy  bool
	}{
		{
			name:           "watching status is healthy",
			syncStatus:     SyncStatusWatching,
			sessionExists:  true,
			expectedStatus: SyncStatusWatching,
			expectHealthy:  true,
		},
		{
			name:           "scanning status is healthy",
			syncStatus:     SyncStatusScanning,
			sessionExists:  true,
			expectedStatus: SyncStatusScanning,
			expectHealthy:  true,
		},
		{
			name:           "syncing status is healthy",
			syncStatus:     SyncStatusSyncing,
			sessionExists:  true,
			expectedStatus: SyncStatusSyncing,
			expectHealthy:  true,
		},
		{
			name:           "connecting status is healthy",
			syncStatus:     SyncStatusConnecting,
			sessionExists:  true,
			expectedStatus: SyncStatusConnecting,
			expectHealthy:  true,
		},
		{
			name:           "conflicted status is unhealthy",
			syncStatus:     SyncStatusConflicted,
			sessionExists:  true,
			conflicts:      []string{"file.go"},
			expectedStatus: SyncStatusConflicted,
			expectHealthy:  false,
		},
		{
			name:           "error status is unhealthy",
			syncStatus:     SyncStatusError,
			sessionExists:  true,
			expectedStatus: SyncStatusError,
			expectHealthy:  false,
		},
		{
			name:           "disconnected status is unhealthy",
			syncStatus:     SyncStatusDisconnected,
			sessionExists:  false,
			expectedStatus: SyncStatusDisconnected,
			expectHealthy:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutagen := &MockMutagen{
				sessionExists: tt.sessionExists,
				syncStatus:    tt.syncStatus,
				conflicts:     tt.conflicts,
			}
			p := NewPairingWithMocks(&MockGit{}, mutagen)
			ctx := context.Background()

			// Enable pairing first (need session to exist during enable)
			mutagen.sessionExists = true
			_ = p.Enable(ctx, "feature-branch", "container-123", "/path/to/repo", "main")

			// Now set the test conditions
			mutagen.sessionExists = tt.sessionExists
			mutagen.syncStatus = tt.syncStatus
			mutagen.conflicts = tt.conflicts

			_ = p.CheckSyncHealth(ctx)

			state := p.GetState()
			if state.SyncStatus != tt.expectedStatus {
				t.Errorf("SyncStatus = %v, want %v", state.SyncStatus, tt.expectedStatus)
			}
			if state.SyncHealthy != tt.expectHealthy {
				t.Errorf("SyncHealthy = %v, want %v", state.SyncHealthy, tt.expectHealthy)
			}
		})
	}
}

func TestPairing_Enable_SetsInitialSyncStatus(t *testing.T) {
	mutagen := &MockMutagen{sessionExists: true}
	p := NewPairingWithMocks(&MockGit{}, mutagen)
	ctx := context.Background()

	_ = p.Enable(ctx, "feature-branch", "container-123", "/path/to/repo", "main")

	state := p.GetState()
	if state.SyncStatus != SyncStatusWatching {
		t.Errorf("Initial SyncStatus = %v, want %v", state.SyncStatus, SyncStatusWatching)
	}
	if state.SyncStatusText != "Watching for changes" {
		t.Errorf("Initial SyncStatusText = %q, want %q", state.SyncStatusText, "Watching for changes")
	}
	if state.LastUpdated.IsZero() {
		t.Error("LastUpdated should be set after Enable")
	}
}

func TestPairing_Disable_ClearsSyncStatus(t *testing.T) {
	mutagen := &MockMutagen{sessionExists: true}
	p := NewPairingWithMocks(&MockGit{}, mutagen)
	ctx := context.Background()

	_ = p.Enable(ctx, "feature-branch", "container-123", "/path/to/repo", "main")
	_ = p.Disable(ctx)

	state := p.GetState()
	if state.SyncStatus != SyncStatusUnknown {
		t.Errorf("SyncStatus after Disable = %v, want %v", state.SyncStatus, SyncStatusUnknown)
	}
	if state.SyncStatusText != "" {
		t.Errorf("SyncStatusText after Disable = %q, want empty", state.SyncStatusText)
	}
	if !state.LastUpdated.IsZero() {
		t.Error("LastUpdated should be zero after Disable")
	}
}
