package workstream

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistentManager_AddRemove(t *testing.T) {
	t.Parallel()

	// Create temp directory for state file
	tmpDir := t.TempDir()

	pm := NewPersistentManager(tmpDir)
	defer pm.Close()

	// Add a workstream
	ws := NewWithID("test-id", "test-branch", "test prompt")
	if err := pm.Add(ws); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Verify it's in the manager
	if pm.Count() != 1 {
		t.Errorf("Count() = %d, want 1", pm.Count())
	}

	// Wait for auto-save
	time.Sleep(300 * time.Millisecond)

	// Check state file exists
	statePath := filepath.Join(tmpDir, stateFileName)
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Error("State file should exist after Add()")
	}

	// Remove the workstream
	pm.Remove(ws.ID)

	if pm.Count() != 0 {
		t.Errorf("Count() = %d, want 0", pm.Count())
	}

	// Wait for auto-save
	time.Sleep(300 * time.Millisecond)

	// Verify state was saved with empty workstreams
	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if len(state.Workstreams) != 0 {
		t.Errorf("State has %d workstreams, want 0", len(state.Workstreams))
	}
}

func TestPersistentManager_SetFocused(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	pm := NewPersistentManager(tmpDir)
	defer pm.Close()

	// Set focused index
	pm.SetFocused(3)
	if pm.GetFocused() != 3 {
		t.Errorf("GetFocused() = %d, want 3", pm.GetFocused())
	}

	// Wait for auto-save
	time.Sleep(300 * time.Millisecond)

	// Verify state was saved
	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if state.FocusedIndex != 3 {
		t.Errorf("State.FocusedIndex = %d, want 3", state.FocusedIndex)
	}
}

func TestPersistentManager_SetLayout(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	pm := NewPersistentManager(tmpDir)
	defer pm.Close()

	// Set layout
	pm.SetLayout(2)
	if pm.GetLayout() != 2 {
		t.Errorf("GetLayout() = %d, want 2", pm.GetLayout())
	}

	// Wait for auto-save
	time.Sleep(300 * time.Millisecond)

	// Verify state was saved
	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if state.Layout != 2 {
		t.Errorf("State.Layout = %d, want 2", state.Layout)
	}
}

func TestPersistentManager_UpdateWorkstream(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	pm := NewPersistentManager(tmpDir)
	defer pm.Close()

	// Add a workstream
	ws := NewWithID("test-id", "test-branch", "test prompt")
	ws.BranchName = "real-branch" // Set branch so it's saved
	_ = pm.Add(ws)

	// Update container ID
	ws.ContainerID = "container-123"
	pm.UpdateWorkstream(ws.ID)

	// Wait for auto-save
	time.Sleep(300 * time.Millisecond)

	// Verify state was saved with container ID
	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if len(state.Workstreams) != 1 {
		t.Fatalf("Expected 1 workstream, got %d", len(state.Workstreams))
	}
	if state.Workstreams[0].ContainerID != "container-123" {
		t.Errorf("ContainerID = %q, want 'container-123'", state.Workstreams[0].ContainerID)
	}
}

func TestPersistentManager_Flush(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	pm := NewPersistentManager(tmpDir)
	defer pm.Close()

	ws := NewWithID("test-id", "test-branch", "test prompt")
	ws.BranchName = "flush-test"
	_ = pm.Add(ws)

	// Immediate flush
	pm.Flush()

	// Check state file exists immediately
	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if len(state.Workstreams) != 1 {
		t.Errorf("Expected 1 workstream after Flush, got %d", len(state.Workstreams))
	}
}

func TestPersistentManager_LoadAndRestore(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create initial state
	ws := NewWithID("test-id", "test-branch", "test prompt")
	ws.BranchName = "restore-test"
	workstreams := []*Workstream{ws}
	if err := SaveState(tmpDir, workstreams, 5, 3); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	// Create manager and restore
	pm := NewPersistentManager(tmpDir)
	defer pm.Close()

	state, err := pm.LoadAndRestore()
	if err != nil {
		t.Fatalf("LoadAndRestore() error = %v", err)
	}

	// Verify state was restored
	if pm.GetFocused() != 5 {
		t.Errorf("GetFocused() = %d, want 5", pm.GetFocused())
	}
	if pm.GetLayout() != 3 {
		t.Errorf("GetLayout() = %d, want 3", pm.GetLayout())
	}
	if len(state.Workstreams) != 1 {
		t.Errorf("Expected 1 workstream, got %d", len(state.Workstreams))
	}
}

func TestPersistentManager_Close(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	pm := NewPersistentManager(tmpDir)

	ws := NewWithID("test-id", "test-branch", "test prompt")
	ws.BranchName = "close-test"
	_ = pm.Add(ws)

	// Close should flush
	pm.Close()

	// Multiple closes should be safe
	pm.Close()

	// Check state file
	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if len(state.Workstreams) != 1 {
		t.Errorf("Expected 1 workstream after Close, got %d", len(state.Workstreams))
	}
}

func TestPersistentManager_Debounce(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	pm := NewPersistentManager(tmpDir)
	defer pm.Close()

	// Rapid changes should be coalesced
	for i := 0; i < 10; i++ {
		pm.SetFocused(i)
	}

	// Wait for debounce
	time.Sleep(300 * time.Millisecond)

	// Should have last value
	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if state.FocusedIndex != 9 {
		t.Errorf("FocusedIndex = %d, want 9 (last value)", state.FocusedIndex)
	}
}
