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
	StateStarting State = "starting" // Container being created/started
	StateRunning  State = "running"  // Claude active, processing or waiting
	StateIdle     State = "idle"     // Claude finished, container alive
	StatePairing  State = "pairing"  // Mutagen sync active with local
	StateStopped  State = "stopped"  // Container halted, can resume
	StateError    State = "error"    // Container failed to start or crashed
)

// IsActive returns true if the workstream container should be running.
func (s State) IsActive() bool {
	return s == StateStarting || s == StateRunning || s == StateIdle || s == StatePairing
}

// Workstream represents a Docker container + git branch + Claude Code instance.
type Workstream struct {
	mu sync.RWMutex

	// Identity
	ID         string // Unique identifier
	Prompt     string // Original user prompt
	BranchName string // Generated git branch name
	Title      string // Short summary title (generated async via Claude CLI)

	// Docker
	ContainerID string // Docker container ID

	// Git worktree (container has isolated working directory)
	WorktreePath string // Path to git worktree on host

	// State
	State        State     // Current lifecycle state
	ErrorMessage string    // Error details if State == StateError
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
		State:        StateStarting,
		CreatedAt:    now,
		LastActivity: now,
	}
}

// NewWithUniqueBranch creates a new workstream with a branch name that's unique
// among the provided existing branch names.
func NewWithUniqueBranch(prompt string, existingBranches []string) *Workstream {
	now := time.Now()
	id := idCounter.Add(1)
	return &Workstream{
		ID:           fmt.Sprintf("%d-%d", now.UnixNano(), id),
		Prompt:       prompt,
		BranchName:   GenerateUniqueBranchName(prompt, existingBranches),
		State:        StateStarting,
		CreatedAt:    now,
		LastActivity: now,
	}
}

// NewWithID creates a workstream with a specific ID (for restoring from saved state).
func NewWithID(id, branchName, prompt string) *Workstream {
	now := time.Now()
	return &Workstream{
		ID:           id,
		Prompt:       prompt,
		BranchName:   branchName,
		State:        StateStarting,
		CreatedAt:    now,
		LastActivity: now,
	}
}

// SetState updates the workstream state.
func (w *Workstream) SetState(state State) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.State = state
	w.ErrorMessage = "" // Clear error when state changes
	w.LastActivity = time.Now()
}

// SetError sets the workstream to error state with a message.
func (w *Workstream) SetError(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.State = StateError
	if err != nil {
		w.ErrorMessage = err.Error()
	}
	w.LastActivity = time.Now()
}

// SetContainerID sets the Docker container ID.
func (w *Workstream) SetContainerID(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ContainerID = id
}

// SetTitle sets the workstream title (generated async via Claude CLI).
func (w *Workstream) SetTitle(title string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Title = title
}

// GetTitle returns the title, or BranchName as fallback if title not yet set.
func (w *Workstream) GetTitle() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.Title != "" {
		return w.Title
	}
	return w.BranchName
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
