package workstream

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestStateFilePath(t *testing.T) {
	dir := "/some/directory"
	expected := filepath.Join(dir, ".ccells-state.json")
	got := StateFilePath(dir)
	if got != expected {
		t.Errorf("StateFilePath() = %v, want %v", got, expected)
	}
}

func TestSaveAndLoadState(t *testing.T) {
	// Create a temp directory
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test workstreams
	ws1 := New("test prompt 1")
	ws1.ContainerID = "container-1"

	ws2 := New("test prompt 2")
	ws2.ContainerID = "container-2"

	workstreams := []*Workstream{ws1, ws2}
	focusedIndex := 1

	// Save state
	err = SaveState(tmpDir, workstreams, focusedIndex, 0)
	if err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	// Verify file exists
	if !StateExists(tmpDir) {
		t.Error("State file should exist after save")
	}

	// Load state
	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	// Verify loaded state
	if state.Version != 1 {
		t.Errorf("Version = %d, want 1", state.Version)
	}
	if state.FocusedIndex != focusedIndex {
		t.Errorf("FocusedIndex = %d, want %d", state.FocusedIndex, focusedIndex)
	}
	if len(state.Workstreams) != 2 {
		t.Errorf("len(Workstreams) = %d, want 2", len(state.Workstreams))
	}

	// Verify workstream data
	if state.Workstreams[0].ID != ws1.ID {
		t.Errorf("Workstream[0].ID = %v, want %v", state.Workstreams[0].ID, ws1.ID)
	}
	if state.Workstreams[0].ContainerID != "container-1" {
		t.Errorf("Workstream[0].ContainerID = %v, want container-1", state.Workstreams[0].ContainerID)
	}
	if state.Workstreams[1].Prompt != "test prompt 2" {
		t.Errorf("Workstream[1].Prompt = %v, want 'test prompt 2'", state.Workstreams[1].Prompt)
	}
}

func TestDeleteState(t *testing.T) {
	// Create a temp directory
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Save state first
	ws := New("test")
	ws.ContainerID = "test-container"
	err = SaveState(tmpDir, []*Workstream{ws}, 0, 0)
	if err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	// Verify it exists
	if !StateExists(tmpDir) {
		t.Error("State file should exist after save")
	}

	// Delete state
	err = DeleteState(tmpDir)
	if err != nil {
		t.Fatalf("DeleteState() error = %v", err)
	}

	// Verify it's gone
	if StateExists(tmpDir) {
		t.Error("State file should not exist after delete")
	}

	// Delete again should not error
	err = DeleteState(tmpDir)
	if err != nil {
		t.Errorf("DeleteState() on non-existent file error = %v", err)
	}
}

func TestStateExists(t *testing.T) {
	// Create a temp directory
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Should not exist initially
	if StateExists(tmpDir) {
		t.Error("State file should not exist initially")
	}

	// Create state file
	ws := New("test")
	err = SaveState(tmpDir, []*Workstream{ws}, 0, 0)
	if err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	// Should exist now
	if !StateExists(tmpDir) {
		t.Error("State file should exist after save")
	}
}

func TestLoadStateNonExistent(t *testing.T) {
	// Create a temp directory with no state file
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Loading from non-existent should error
	_, err = LoadState(tmpDir)
	if err == nil {
		t.Error("LoadState() should error for non-existent state")
	}
}

func TestStateSavedAtTimestamp(t *testing.T) {
	// Create a temp directory
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	before := time.Now()

	// Save state
	ws := New("test")
	err = SaveState(tmpDir, []*Workstream{ws}, 0, 0)
	if err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	after := time.Now()

	// Load and check timestamp
	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	if state.SavedAt.Before(before) || state.SavedAt.After(after) {
		t.Errorf("SavedAt = %v, should be between %v and %v", state.SavedAt, before, after)
	}
}

func TestSaveStateEmptyWorkstreams(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Save with empty workstreams list
	err = SaveState(tmpDir, []*Workstream{}, 0, 0)
	if err != nil {
		t.Fatalf("SaveState() with empty list error = %v", err)
	}

	// Load and verify
	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	if len(state.Workstreams) != 0 {
		t.Errorf("Expected 0 workstreams, got %d", len(state.Workstreams))
	}
}

func TestSaveStateOverwrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Save first state
	ws1 := New("first")
	ws1.ContainerID = "container-first"
	err = SaveState(tmpDir, []*Workstream{ws1}, 0, 0)
	if err != nil {
		t.Fatalf("First SaveState() error = %v", err)
	}

	// Save second state (overwrite)
	ws2 := New("second")
	ws2.ContainerID = "container-second"
	ws3 := New("third")
	ws3.ContainerID = "container-third"
	err = SaveState(tmpDir, []*Workstream{ws2, ws3}, 1, 0)
	if err != nil {
		t.Fatalf("Second SaveState() error = %v", err)
	}

	// Load and verify only second state exists
	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	if len(state.Workstreams) != 2 {
		t.Errorf("Expected 2 workstreams, got %d", len(state.Workstreams))
	}
	if state.Workstreams[0].Prompt != "second" {
		t.Errorf("Expected first workstream prompt 'second', got '%s'", state.Workstreams[0].Prompt)
	}
	if state.FocusedIndex != 1 {
		t.Errorf("Expected FocusedIndex 1, got %d", state.FocusedIndex)
	}
}

func TestSaveStatePreservesAllFields(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create workstream with all fields set
	ws := New("test prompt with many words")
	ws.ContainerID = "abc123def456"
	originalCreatedAt := ws.CreatedAt

	err = SaveState(tmpDir, []*Workstream{ws}, 0, 0)
	if err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	saved := state.Workstreams[0]
	if saved.ID != ws.ID {
		t.Errorf("ID mismatch: got %s, want %s", saved.ID, ws.ID)
	}
	if saved.BranchName != ws.BranchName {
		t.Errorf("BranchName mismatch: got %s, want %s", saved.BranchName, ws.BranchName)
	}
	if saved.Prompt != ws.Prompt {
		t.Errorf("Prompt mismatch: got %s, want %s", saved.Prompt, ws.Prompt)
	}
	if saved.ContainerID != ws.ContainerID {
		t.Errorf("ContainerID mismatch: got %s, want %s", saved.ContainerID, ws.ContainerID)
	}
	if !saved.CreatedAt.Equal(originalCreatedAt) {
		t.Errorf("CreatedAt mismatch: got %v, want %v", saved.CreatedAt, originalCreatedAt)
	}
}

func TestLoadStateCorruptJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write corrupt JSON
	corruptData := []byte(`{"version": 1, "workstreams": [{"id": "broken"`)
	err = os.WriteFile(StateFilePath(tmpDir), corruptData, 0644)
	if err != nil {
		t.Fatalf("Failed to write corrupt file: %v", err)
	}

	// Try to load - should error
	_, err = LoadState(tmpDir)
	if err == nil {
		t.Error("LoadState() should error on corrupt JSON")
	}
}

func TestSaveStateNilWorkstreams(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Save with nil slice
	err = SaveState(tmpDir, nil, 0, 0)
	if err != nil {
		t.Fatalf("SaveState() with nil slice error = %v", err)
	}

	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	if state.Workstreams == nil {
		// nil becomes empty slice in JSON
		if len(state.Workstreams) != 0 {
			t.Errorf("Expected empty workstreams, got %d", len(state.Workstreams))
		}
	}
}

func TestStateFilePath_DifferentDirectories(t *testing.T) {
	tests := []struct {
		dir      string
		expected string
	}{
		{"/home/user/project", "/home/user/project/.ccells-state.json"},
		{"/tmp", "/tmp/.ccells-state.json"},
		{".", ".ccells-state.json"}, // filepath.Join normalizes "." + "file" to "file"
		{"/", "/.ccells-state.json"},
	}

	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			got := StateFilePath(tt.dir)
			if got != tt.expected {
				t.Errorf("StateFilePath(%q) = %q, want %q", tt.dir, got, tt.expected)
			}
		})
	}
}

func TestSaveStateAtomic(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Save state
	ws := New("test")
	err = SaveState(tmpDir, []*Workstream{ws}, 0, 0)
	if err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	// Verify no temp file remains (check for any .tmp files)
	entries, _ := os.ReadDir(tmpDir)
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" || len(entry.Name()) > 20 && entry.Name()[len(entry.Name())-4:] == ".tmp" {
			t.Errorf("Temp file should not exist after successful save: %s", entry.Name())
		}
	}

	// Verify final file exists
	if !StateExists(tmpDir) {
		t.Error("State file should exist after save")
	}
}

func TestSaveStateConcurrent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Run multiple concurrent saves
	const numGoroutines = 10
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ws := New("concurrent test")
			ws.ContainerID = "container-" + itoa(idx)
			if err := SaveState(tmpDir, []*Workstream{ws}, 0, 0); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("SaveState() error during concurrent access: %v", err)
	}

	// State file should exist and be valid
	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error after concurrent saves: %v", err)
	}

	// Should have exactly one workstream (last one wins)
	if len(state.Workstreams) != 1 {
		t.Errorf("Expected 1 workstream after concurrent saves, got %d", len(state.Workstreams))
	}

	// No temp files should remain
	entries, _ := os.ReadDir(tmpDir)
	for _, entry := range entries {
		name := entry.Name()
		if name != ".ccells-state.json" {
			t.Errorf("Unexpected file after concurrent saves: %s", name)
		}
	}
}
