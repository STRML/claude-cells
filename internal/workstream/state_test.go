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
	expected := filepath.Join(dir, ".claude-cells-state.json")
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
		{"/home/user/project", "/home/user/project/.claude-cells-state.json"},
		{"/tmp", "/tmp/.claude-cells-state.json"},
		{".", ".claude-cells-state.json"}, // filepath.Join normalizes "." + "file" to "file"
		{"/", "/.claude-cells-state.json"},
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
		if name != ".claude-cells-state.json" {
			t.Errorf("Unexpected file after concurrent saves: %s", name)
		}
	}
}

func TestSaveStatePreservesClaudeSessionID(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create workstream with ClaudeSessionID set
	ws := New("test prompt")
	ws.ContainerID = "container-123"
	ws.ClaudeSessionID = "01HZ8Y3QPXKJNM5VG2DTCW9RAE"

	err = SaveState(tmpDir, []*Workstream{ws}, 0, 0)
	if err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	saved := state.Workstreams[0]
	if saved.ClaudeSessionID != ws.ClaudeSessionID {
		t.Errorf("ClaudeSessionID mismatch: got %q, want %q", saved.ClaudeSessionID, ws.ClaudeSessionID)
	}
}

func TestSaveStateEmptyClaudeSessionID(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create workstream without ClaudeSessionID
	ws := New("test prompt")
	ws.ContainerID = "container-123"
	// ClaudeSessionID is intentionally left empty

	err = SaveState(tmpDir, []*Workstream{ws}, 0, 0)
	if err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	saved := state.Workstreams[0]
	if saved.ClaudeSessionID != "" {
		t.Errorf("ClaudeSessionID should be empty, got %q", saved.ClaudeSessionID)
	}
}

func TestSaveStateExcludesTitleGenerationSessions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a normal workstream with branch name
	normalWs := New("implement user auth")
	normalWs.ContainerID = "container-normal"

	// Create a title-generation workstream (empty BranchName)
	titleGenWs := NewForSummarizing("fix the login bug")
	titleGenWs.ContainerID = "container-title-gen"

	// Save both
	err = SaveState(tmpDir, []*Workstream{normalWs, titleGenWs}, 0, 0)
	if err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	// Load and verify only normal session is saved
	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	if len(state.Workstreams) != 1 {
		t.Errorf("Expected 1 workstream (title-gen excluded), got %d", len(state.Workstreams))
	}

	if state.Workstreams[0].ID != normalWs.ID {
		t.Errorf("Expected normal workstream ID %s, got %s", normalWs.ID, state.Workstreams[0].ID)
	}
}

func TestSaveStateAllTitleGenerationSessionsExcluded(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create only title-generation workstreams
	ws1 := NewForSummarizing("task 1")
	ws2 := NewForSummarizing("task 2")

	err = SaveState(tmpDir, []*Workstream{ws1, ws2}, 0, 0)
	if err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	if len(state.Workstreams) != 0 {
		t.Errorf("Expected 0 workstreams (all title-gen excluded), got %d", len(state.Workstreams))
	}
}

func TestSaveStatePreservesWasInterrupted(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create workstream that was interrupted
	ws := New("test prompt")
	ws.ContainerID = "container-123"
	ws.WasInterrupted = true

	err = SaveState(tmpDir, []*Workstream{ws}, 0, 0)
	if err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	saved := state.Workstreams[0]
	if !saved.WasInterrupted {
		t.Error("WasInterrupted should be true after load")
	}
}

func TestMigrateStateFile_NoOldFile(t *testing.T) {
	oldDir, err := os.MkdirTemp("", "migrate-old-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(oldDir)

	newDir, err := os.MkdirTemp("", "migrate-new-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(newDir)

	migrated, err := MigrateStateFile(oldDir, newDir)
	if err != nil {
		t.Fatalf("MigrateStateFile() error = %v", err)
	}
	if migrated {
		t.Error("MigrateStateFile() should return false when no old file exists")
	}
}

func TestMigrateStateFile_NewFileExists(t *testing.T) {
	oldDir, err := os.MkdirTemp("", "migrate-old-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(oldDir)

	newDir, err := os.MkdirTemp("", "migrate-new-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(newDir)

	// Create both old and new files
	oldData := []byte(`{"version":1,"workstreams":[]}`)
	newData := []byte(`{"version":1,"workstreams":[{"id":"new"}]}`)
	os.WriteFile(filepath.Join(oldDir, stateFileName), oldData, 0644)
	os.WriteFile(filepath.Join(newDir, stateFileName), newData, 0644)

	migrated, err := MigrateStateFile(oldDir, newDir)
	if err != nil {
		t.Fatalf("MigrateStateFile() error = %v", err)
	}
	if migrated {
		t.Error("should not migrate when new file already exists")
	}

	// Old file should be deleted
	if _, err := os.Stat(filepath.Join(oldDir, stateFileName)); !os.IsNotExist(err) {
		t.Error("old file should be deleted when new file exists")
	}

	// New file should be preserved (not overwritten)
	data, _ := os.ReadFile(filepath.Join(newDir, stateFileName))
	if string(data) != string(newData) {
		t.Error("new file should not be overwritten")
	}
}

func TestMigrateStateFile_Success(t *testing.T) {
	oldDir, err := os.MkdirTemp("", "migrate-old-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(oldDir)

	newDir, err := os.MkdirTemp("", "migrate-new-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(newDir)

	// Create old file only
	oldData := []byte(`{"version":1,"workstreams":[{"id":"migrated"}]}`)
	os.WriteFile(filepath.Join(oldDir, stateFileName), oldData, 0644)

	migrated, err := MigrateStateFile(oldDir, newDir)
	if err != nil {
		t.Fatalf("MigrateStateFile() error = %v", err)
	}
	if !migrated {
		t.Error("should return true when migration occurred")
	}

	// New file should have the old data
	data, err := os.ReadFile(filepath.Join(newDir, stateFileName))
	if err != nil {
		t.Fatalf("failed to read new file: %v", err)
	}
	if string(data) != string(oldData) {
		t.Error("new file should contain migrated data")
	}

	// Old file should be deleted
	if _, err := os.Stat(filepath.Join(oldDir, stateFileName)); !os.IsNotExist(err) {
		t.Error("old file should be deleted after migration")
	}
}

func TestGetStateDir(t *testing.T) {
	dir, err := GetStateDir("test-repo-id-123")
	if err != nil {
		t.Fatalf("GetStateDir() error = %v", err)
	}
	if dir == "" {
		t.Error("GetStateDir() returned empty string")
	}
	// Should contain the repo ID
	if !filepath.IsAbs(dir) {
		t.Errorf("GetStateDir() should return absolute path, got %q", dir)
	}
	// Directory should exist
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("GetStateDir() should create the directory")
	}
	// Clean up
	os.RemoveAll(dir)
}

func TestSaveStateWithRepoInfo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	ws := New("test prompt")
	ws.ContainerID = "container-123"

	repoInfo := &RepoInfo{
		Name:      "my-repo",
		Path:      "/home/user/my-repo",
		Remote:    "https://github.com/user/my-repo",
		RepoID:    "abc123",
		CreatedAt: time.Now(),
	}

	err = SaveStateWithRepoInfo(tmpDir, []*Workstream{ws}, 0, 0, repoInfo)
	if err != nil {
		t.Fatalf("SaveStateWithRepoInfo() error = %v", err)
	}

	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	if state.Repo == nil {
		t.Fatal("Repo should not be nil")
	}
	if state.Repo.Name != "my-repo" {
		t.Errorf("Repo.Name = %q, want %q", state.Repo.Name, "my-repo")
	}
	if state.Repo.RepoID != "abc123" {
		t.Errorf("Repo.RepoID = %q, want %q", state.Repo.RepoID, "abc123")
	}
}

func TestSaveStateWithRepoInfo_PreservesExisting(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	ws := New("test")
	ws.ContainerID = "c1"

	// First save with repo info
	repoInfo := &RepoInfo{Name: "original-repo", RepoID: "orig123"}
	err = SaveStateWithRepoInfo(tmpDir, []*Workstream{ws}, 0, 0, repoInfo)
	if err != nil {
		t.Fatal(err)
	}

	// Second save with nil repo info — should preserve original
	err = SaveStateWithRepoInfo(tmpDir, []*Workstream{ws}, 0, 0, nil)
	if err != nil {
		t.Fatal(err)
	}

	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	if state.Repo == nil {
		t.Fatal("Repo should be preserved from first save")
	}
	if state.Repo.Name != "original-repo" {
		t.Errorf("Repo.Name = %q, want %q", state.Repo.Name, "original-repo")
	}
}

func TestSaveStatePreservesPRInfo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	ws := New("test")
	ws.ContainerID = "c1"
	ws.PRNumber = 42
	ws.PRURL = "https://github.com/org/repo/pull/42"
	ws.HasBeenPushed = true
	ws.Synopsis = "Added auth flow"

	err = SaveState(tmpDir, []*Workstream{ws}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	saved := state.Workstreams[0]
	if saved.PRNumber != 42 {
		t.Errorf("PRNumber = %d, want 42", saved.PRNumber)
	}
	if saved.PRURL != "https://github.com/org/repo/pull/42" {
		t.Errorf("PRURL = %q, want correct URL", saved.PRURL)
	}
	if !saved.HasBeenPushed {
		t.Error("HasBeenPushed should be true")
	}
	if saved.Synopsis != "Added auth flow" {
		t.Errorf("Synopsis = %q, want %q", saved.Synopsis, "Added auth flow")
	}
}

func TestSaveStateWasInterruptedFalse(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create workstream that was NOT interrupted (default)
	ws := New("test prompt")
	ws.ContainerID = "container-123"
	// WasInterrupted is false by default

	err = SaveState(tmpDir, []*Workstream{ws}, 0, 0)
	if err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	state, err := LoadState(tmpDir)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	saved := state.Workstreams[0]
	if saved.WasInterrupted {
		t.Error("WasInterrupted should be false when not set")
	}
}
