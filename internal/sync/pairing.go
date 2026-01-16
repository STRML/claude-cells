package sync

import (
	"context"
	"sync"
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
	CreateSession(ctx context.Context, branch, container, path string) error
	TerminateSession(ctx context.Context, branch string) error
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

// Enable starts pairing mode for a workstream.
func (p *Pairing) Enable(ctx context.Context, branchName, containerID, localPath string) error {
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

	// Get current branch before switching
	prevBranch, err := p.git.CurrentBranch(ctx)
	if err != nil {
		return err
	}
	p.previousBranch = prevBranch

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

	return nil
}
