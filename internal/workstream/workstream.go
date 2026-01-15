package workstream

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// idCounter ensures unique IDs even when created in quick succession
var idCounter atomic.Uint64

// State represents the lifecycle state of a workstream.
type State string

const (
	StateRunning State = "running" // Claude active, processing or waiting
	StateIdle    State = "idle"    // Claude finished, container alive
	StatePairing State = "pairing" // Mutagen sync active with local
	StateStopped State = "stopped" // Container halted, can resume
)

// IsActive returns true if the workstream container should be running.
func (s State) IsActive() bool {
	return s == StateRunning || s == StateIdle || s == StatePairing
}

// Workstream represents a Docker container + git branch + Claude Code instance.
type Workstream struct {
	mu sync.RWMutex

	// Identity
	ID         string // Unique identifier
	Prompt     string // Original user prompt
	BranchName string // Generated git branch name

	// Docker
	ContainerID string // Docker container ID

	// State
	State        State     // Current lifecycle state
	CreatedAt    time.Time // When workstream was created
	LastActivity time.Time // Last interaction time

	// PR info (optional)
	PRNumber int    // GitHub PR number if created
	PRURL    string // GitHub PR URL if created
}

// New creates a new workstream from a prompt.
func New(prompt string) *Workstream {
	now := time.Now()
	id := idCounter.Add(1)
	return &Workstream{
		ID:           fmt.Sprintf("%d-%d", now.UnixNano(), id),
		Prompt:       prompt,
		BranchName:   GenerateBranchName(prompt),
		State:        StateStopped,
		CreatedAt:    now,
		LastActivity: now,
	}
}

// SetState updates the workstream state.
func (w *Workstream) SetState(state State) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.State = state
	w.LastActivity = time.Now()
}

// SetContainerID sets the Docker container ID.
func (w *Workstream) SetContainerID(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ContainerID = id
}

// UpdateActivity updates the last activity timestamp.
func (w *Workstream) UpdateActivity() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.LastActivity = time.Now()
}

// GetState returns the current state (thread-safe).
func (w *Workstream) GetState() State {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.State
}

// String returns a human-readable representation.
func (w *Workstream) String() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return fmt.Sprintf("%s (%s)", w.BranchName, w.State)
}
