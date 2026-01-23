package sync

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// GitOperations defines git operations needed for pairing.
type GitOperations interface {
	HasUncommittedChanges(ctx context.Context) (bool, error)
	Stash(ctx context.Context) error
	Checkout(ctx context.Context, branch string) error
	CurrentBranch(ctx context.Context) (string, error)
}

// MutagenOperations defines mutagen operations needed for pairing.
type MutagenOperations interface {
	CheckInstalled(ctx context.Context) error
	CreateSession(ctx context.Context, branch, container, path string) error
	TerminateSession(ctx context.Context, branch string) error
	SessionExists(ctx context.Context, branch string) (bool, error)
	GetConflicts(ctx context.Context, branch string) ([]string, error)
	GetSessionStatus(ctx context.Context, branch string) (*SessionStatus, error)
}

// Pairing orchestrates file sync between container and local.
type Pairing struct {
	mu sync.RWMutex

	git     GitOperations
	mutagen MutagenOperations

	active         bool
	currentBranch  string
	previousBranch string
	containerID    string
	localPath      string
	stashedChanges bool
	syncHealthy    bool
	lastConflicts  []string

	// Enhanced sync status (Phase 1)
	syncStatus     SyncStatus
	syncStatusText string
	problems       []string
	lastUpdated    time.Time
}

// PairingState represents the current state of pairing mode.
type PairingState struct {
	Active         bool
	CurrentBranch  string
	PreviousBranch string
	ContainerID    string
	StashedChanges bool
	SyncHealthy    bool
	Conflicts      []string

	// Enhanced sync status (Phase 1)
	SyncStatus     SyncStatus
	SyncStatusText string
	Problems       []string
	LastUpdated    time.Time
}

// NewPairing creates a new pairing orchestrator.
func NewPairing(git GitOperations, mutagen MutagenOperations) *Pairing {
	return &Pairing{
		git:     git,
		mutagen: mutagen,
	}
}

// NewPairingWithMocks creates a pairing orchestrator with mock dependencies (for testing).
func NewPairingWithMocks(git GitOperations, mutagen MutagenOperations) *Pairing {
	return NewPairing(git, mutagen)
}

// IsActive returns true if pairing mode is active.
func (p *Pairing) IsActive() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.active
}

// CurrentBranch returns the branch being paired.
func (p *Pairing) CurrentBranch() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentBranch
}

// HasStashedChanges returns true if local changes were stashed.
func (p *Pairing) HasStashedChanges() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stashedChanges
}

// SetStashedChanges sets the stashed changes flag.
func (p *Pairing) SetStashedChanges(stashed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stashedChanges = stashed
}

// GetState returns the current pairing state (thread-safe snapshot).
func (p *Pairing) GetState() PairingState {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return PairingState{
		Active:         p.active,
		CurrentBranch:  p.currentBranch,
		PreviousBranch: p.previousBranch,
		ContainerID:    p.containerID,
		StashedChanges: p.stashedChanges,
		SyncHealthy:    p.syncHealthy,
		Conflicts:      append([]string(nil), p.lastConflicts...),
		SyncStatus:     p.syncStatus,
		SyncStatusText: p.syncStatusText,
		Problems:       append([]string(nil), p.problems...),
		LastUpdated:    p.lastUpdated,
	}
}

// PreviousBranch returns the branch that was active before pairing started.
func (p *Pairing) PreviousBranch() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.previousBranch
}

// ContainerID returns the container ID being paired.
func (p *Pairing) ContainerID() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.containerID
}

// CheckPrerequisites verifies that all requirements for pairing are met.
func (p *Pairing) CheckPrerequisites(ctx context.Context) error {
	if err := p.mutagen.CheckInstalled(ctx); err != nil {
		return fmt.Errorf("mutagen not installed: %w", err)
	}
	return nil
}

// GetSyncHealth returns the current sync health status and any conflicts.
func (p *Pairing) GetSyncHealth() (healthy bool, conflicts []string) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.syncHealthy, append([]string(nil), p.lastConflicts...)
}

// CheckSyncHealth checks the health of the mutagen sync session.
// Returns an error if the session is unhealthy or lost.
func (p *Pairing) CheckSyncHealth(ctx context.Context) error {
	// Take read lock to check active state and get branch name
	p.mu.RLock()
	if !p.active {
		p.mu.RUnlock()
		return nil
	}
	branch := p.currentBranch
	p.mu.RUnlock()

	// Get detailed session status (includes conflicts, problems, and sync state)
	// This is an external subprocess call - don't hold lock during I/O
	sessionStatus, err := p.mutagen.GetSessionStatus(ctx, branch)

	// Re-acquire write lock to update state
	p.mu.Lock()
	defer p.mu.Unlock()

	// Verify state hasn't changed while we were waiting
	if !p.active || p.currentBranch != branch {
		return nil
	}

	if err != nil {
		p.syncHealthy = false
		p.syncStatus = SyncStatusError
		p.syncStatusText = err.Error()
		p.lastUpdated = time.Now()
		return fmt.Errorf("failed to check session: %w", err)
	}

	// Update sync status fields
	p.syncStatus = sessionStatus.Status
	p.syncStatusText = sessionStatus.StatusText
	p.lastConflicts = sessionStatus.Conflicts
	p.problems = sessionStatus.Problems
	p.lastUpdated = sessionStatus.LastUpdate

	// Determine health based on status
	switch sessionStatus.Status {
	case SyncStatusDisconnected:
		p.syncHealthy = false
		return fmt.Errorf("sync session lost for branch %s", branch)
	case SyncStatusConflicted:
		p.syncHealthy = false
		return fmt.Errorf("sync has %d conflict(s)", len(sessionStatus.Conflicts))
	case SyncStatusError:
		p.syncHealthy = false
		return fmt.Errorf("sync error: %s", sessionStatus.StatusText)
	default:
		p.syncHealthy = true
		return nil
	}
}

// Enable starts pairing mode for a workstream.
// previousBranch is the branch to restore when pairing ends (captured by caller before async dispatch).
func (p *Pairing) Enable(ctx context.Context, branchName, containerID, localPath, previousBranch string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check for uncommitted changes
	hasChanges, err := p.git.HasUncommittedChanges(ctx)
	if err != nil {
		return err
	}

	// Stash if needed
	if hasChanges {
		if err := p.git.Stash(ctx); err != nil {
			return err
		}
		p.stashedChanges = true
	}

	// Store previous branch (passed in to avoid race condition with async dispatch)
	p.previousBranch = previousBranch

	// Start mutagen sync
	if err := p.mutagen.CreateSession(ctx, branchName, containerID, localPath); err != nil {
		return err
	}

	// Checkout workstream branch locally
	if err := p.git.Checkout(ctx, branchName); err != nil {
		// Try to clean up mutagen if checkout fails
		_ = p.mutagen.TerminateSession(ctx, branchName)
		return err
	}

	p.active = true
	p.currentBranch = branchName
	p.containerID = containerID
	p.localPath = localPath
	p.syncHealthy = true
	p.lastConflicts = nil
	p.syncStatus = SyncStatusWatching
	p.syncStatusText = "Watching for changes"
	p.problems = nil
	p.lastUpdated = time.Now()

	return nil
}

// Disable stops pairing mode.
func (p *Pairing) Disable(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.active {
		return nil
	}

	// Terminate mutagen sync
	if err := p.mutagen.TerminateSession(ctx, p.currentBranch); err != nil {
		return err
	}

	// Checkout previous branch
	if p.previousBranch != "" {
		if err := p.git.Checkout(ctx, p.previousBranch); err != nil {
			return err
		}
	}

	p.active = false
	p.currentBranch = ""
	p.containerID = ""
	p.syncHealthy = false
	p.lastConflicts = nil
	p.syncStatus = SyncStatusUnknown
	p.syncStatusText = ""
	p.problems = nil
	p.lastUpdated = time.Time{}

	return nil
}
