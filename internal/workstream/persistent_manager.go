package workstream

import (
	"sync"
	"time"
)

// PersistentManager wraps Manager with auto-persistence.
// Any mutation to workstreams triggers a debounced save to disk.
type PersistentManager struct {
	*Manager
	stateDir   string
	layout     int
	focusedIdx int
	repoInfo   *RepoInfo // Optional repo metadata for state file header

	mu       sync.Mutex
	dirty    bool
	doneCh   chan struct{}
	finishCh chan struct{} // Signals saveLoop has completed
	closedMu sync.Mutex
	closed   bool
}

// NewPersistentManager creates a manager that auto-persists state changes.
func NewPersistentManager(stateDir string) *PersistentManager {
	pm := &PersistentManager{
		Manager:  NewManager(),
		stateDir: stateDir,
		doneCh:   make(chan struct{}),
		finishCh: make(chan struct{}),
	}
	go pm.saveLoop()
	return pm
}

// Add registers a workstream and triggers a save.
func (pm *PersistentManager) Add(ws *Workstream) error {
	err := pm.Manager.Add(ws)
	if err == nil {
		pm.markDirty()
	}
	return err
}

// Remove unregisters a workstream and triggers a save.
func (pm *PersistentManager) Remove(id string) {
	pm.Manager.Remove(id)
	pm.markDirty()
}

// SetLayout updates the layout and triggers a save.
func (pm *PersistentManager) SetLayout(layout int) {
	pm.mu.Lock()
	changed := pm.layout != layout
	pm.layout = layout
	pm.mu.Unlock()
	if changed {
		pm.markDirty()
	}
}

// GetLayout returns the current layout.
func (pm *PersistentManager) GetLayout() int {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.layout
}

// SetFocused updates the focused index and triggers a save.
func (pm *PersistentManager) SetFocused(idx int) {
	pm.mu.Lock()
	changed := pm.focusedIdx != idx
	pm.focusedIdx = idx
	pm.mu.Unlock()
	if changed {
		pm.markDirty()
	}
}

// GetFocused returns the current focused index.
func (pm *PersistentManager) GetFocused() int {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.focusedIdx
}

// SetRepoInfo sets the repo metadata to include in state files.
// This is only written on first save; subsequent saves preserve existing RepoInfo.
func (pm *PersistentManager) SetRepoInfo(info *RepoInfo) {
	pm.mu.Lock()
	pm.repoInfo = info
	pm.mu.Unlock()
}

// UpdateWorkstream marks state as dirty when a workstream is modified.
// Call this after modifying workstream fields (ContainerID, ClaudeSessionID, etc.)
func (pm *PersistentManager) UpdateWorkstream(id string) {
	// Verify the workstream exists
	if pm.Get(id) != nil {
		pm.markDirty()
	}
}

// markDirty flags that state needs to be saved.
func (pm *PersistentManager) markDirty() {
	pm.mu.Lock()
	pm.dirty = true
	pm.mu.Unlock()
}

// saveLoop runs in background, periodically flushing dirty state.
func (pm *PersistentManager) saveLoop() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	defer close(pm.finishCh) // Signal completion when loop exits

	for {
		select {
		case <-pm.doneCh:
			// Final flush on shutdown
			pm.flush()
			return
		case <-ticker.C:
			pm.flushIfDirty()
		}
	}
}

// flushIfDirty saves state only if marked dirty.
func (pm *PersistentManager) flushIfDirty() {
	pm.mu.Lock()
	if !pm.dirty {
		pm.mu.Unlock()
		return
	}
	pm.dirty = false
	layout := pm.layout
	focused := pm.focusedIdx
	repoInfo := pm.repoInfo
	pm.mu.Unlock()

	// Get workstreams snapshot (Manager.List() is thread-safe)
	workstreams := pm.List()

	// Save to disk (with RepoInfo if this is first save)
	_ = SaveStateWithRepoInfo(pm.stateDir, workstreams, focused, layout, repoInfo)
}

// flush forces an immediate save regardless of dirty flag.
func (pm *PersistentManager) flush() {
	pm.mu.Lock()
	pm.dirty = false
	layout := pm.layout
	focused := pm.focusedIdx
	repoInfo := pm.repoInfo
	pm.mu.Unlock()

	workstreams := pm.List()
	_ = SaveStateWithRepoInfo(pm.stateDir, workstreams, focused, layout, repoInfo)
}

// Flush forces an immediate save (public API for shutdown).
func (pm *PersistentManager) Flush() {
	pm.flush()
}

// Close stops the save loop and performs final flush.
// Blocks until the final flush is complete.
func (pm *PersistentManager) Close() {
	pm.closedMu.Lock()
	if pm.closed {
		pm.closedMu.Unlock()
		return
	}
	pm.closed = true
	pm.closedMu.Unlock()

	close(pm.doneCh)
	<-pm.finishCh // Wait for saveLoop to complete final flush
}

// LoadAndRestore loads state from disk and populates the manager.
// Returns the loaded state for UI restoration (focused index, layout).
func (pm *PersistentManager) LoadAndRestore() (*AppState, error) {
	state, err := LoadState(pm.stateDir)
	if err != nil {
		return nil, err
	}

	pm.mu.Lock()
	pm.layout = state.Layout
	pm.focusedIdx = state.FocusedIndex
	pm.mu.Unlock()

	// Workstreams are restored by the caller since they need container validation
	return state, nil
}
