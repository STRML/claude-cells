package workstream

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// stateMu protects concurrent SaveState calls from racing on temp file
var stateMu sync.Mutex

const stateFileName = ".claude-cells-state.json"

// GetStateDir returns the directory for storing state files for a given repo.
// State is stored in ~/.claude-cells/state/<repo-id>/ to avoid polluting the repo.
// The repoID should be obtained from git.Git.RepoID().
func GetStateDir(repoID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	stateDir := filepath.Join(home, ".claude-cells", "state", repoID)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create state directory: %w", err)
	}
	return stateDir, nil
}

// MigrateStateFile moves a state file from the old location (in repo) to the new location.
// Returns true if migration occurred, false if no migration was needed.
func MigrateStateFile(oldDir, newDir string) (bool, error) {
	oldPath := filepath.Join(oldDir, stateFileName)
	newPath := filepath.Join(newDir, stateFileName)

	// Check if old file exists
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		return false, nil // No migration needed
	}

	// Check if new file already exists (don't overwrite)
	if _, err := os.Stat(newPath); err == nil {
		// New file exists, just delete the old one
		_ = os.Remove(oldPath)
		return false, nil
	}

	// Read old file
	data, err := os.ReadFile(oldPath)
	if err != nil {
		return false, fmt.Errorf("failed to read old state file: %w", err)
	}

	// Write to new location
	if err := os.WriteFile(newPath, data, 0644); err != nil {
		return false, fmt.Errorf("failed to write new state file: %w", err)
	}

	// Delete old file
	if err := os.Remove(oldPath); err != nil {
		// Not critical, just log it
		return true, nil
	}

	return true, nil
}

// RepoInfo contains human-readable metadata about the repository.
// This is written once when the state file is created and preserved on updates.
type RepoInfo struct {
	Name      string    `json:"name"`                 // Repository name (directory name)
	Path      string    `json:"path"`                 // Local path when first opened
	Remote    string    `json:"remote,omitempty"`     // Remote URL (origin)
	RepoID    string    `json:"repo_id"`              // Stable identifier (first commit hash)
	CreatedAt time.Time `json:"created_at,omitempty"` // When this state file was first created
}

// SavedWorkstream represents a workstream saved to disk
type SavedWorkstream struct {
	ID              string    `json:"id"`
	BranchName      string    `json:"branch_name"`
	Prompt          string    `json:"prompt"`
	Title           string    `json:"title,omitempty"`    // Short summary title
	Synopsis        string    `json:"synopsis,omitempty"` // Brief description of work accomplished
	ContainerID     string    `json:"container_id"`
	ClaudeSessionID string    `json:"claude_session_id,omitempty"` // Claude Code session ID for --resume
	Runtime         string    `json:"runtime,omitempty"`           // Runtime: "claude" or "claudesp"
	WasInterrupted  bool      `json:"was_interrupted,omitempty"`   // True if Claude was working when session ended
	HasBeenPushed   bool      `json:"has_been_pushed,omitempty"`   // True if branch has been pushed to remote
	PRNumber        int       `json:"pr_number,omitempty"`         // GitHub PR number if created
	PRURL           string    `json:"pr_url,omitempty"`            // GitHub PR URL if created
	CreatedAt       time.Time `json:"created_at"`
}

// AppState represents the saved application state
type AppState struct {
	Version      int               `json:"version"`
	Repo         *RepoInfo         `json:"repo,omitempty"` // Human-readable repo metadata
	Workstreams  []SavedWorkstream `json:"workstreams"`
	FocusedIndex int               `json:"focused_index"`
	Layout       int               `json:"layout"` // LayoutType as int for JSON
	SavedAt      time.Time         `json:"saved_at"`
}

// StateFilePath returns the path to the state file in the given directory
func StateFilePath(dir string) string {
	return filepath.Join(dir, stateFileName)
}

// SaveState saves the application state to a file.
// Uses atomic write (write to temp file, then rename) to prevent corruption.
// Thread-safe: protected by mutex to prevent concurrent write races.
// Preserves existing RepoInfo if present in the file.
func SaveState(dir string, workstreams []*Workstream, focusedIndex int, layout int) error {
	return SaveStateWithRepoInfo(dir, workstreams, focusedIndex, layout, nil)
}

// SaveStateWithRepoInfo saves state with optional repo info.
// If repoInfo is nil and a state file exists, the existing RepoInfo is preserved.
// If repoInfo is provided, it overwrites any existing RepoInfo.
func SaveStateWithRepoInfo(dir string, workstreams []*Workstream, focusedIndex int, layout int, repoInfo *RepoInfo) error {
	stateMu.Lock()
	defer stateMu.Unlock()

	// Try to load existing state to preserve RepoInfo
	var existingRepoInfo *RepoInfo
	if existing, err := loadStateUnsafe(dir); err == nil && existing.Repo != nil {
		existingRepoInfo = existing.Repo
	}

	// Use provided repoInfo, or fall back to existing
	finalRepoInfo := repoInfo
	if finalRepoInfo == nil {
		finalRepoInfo = existingRepoInfo
	}

	state := AppState{
		Version:      1,
		Repo:         finalRepoInfo,
		FocusedIndex: focusedIndex,
		Layout:       layout,
		SavedAt:      time.Now(),
	}

	for _, ws := range workstreams {
		// Skip title-generation sessions (empty BranchName means still generating title)
		if ws.BranchName == "" {
			continue
		}
		state.Workstreams = append(state.Workstreams, SavedWorkstream{
			ID:              ws.ID,
			BranchName:      ws.BranchName,
			Prompt:          ws.Prompt,
			Title:           ws.Title,
			Synopsis:        ws.Synopsis,
			ContainerID:     ws.ContainerID,
			ClaudeSessionID: ws.ClaudeSessionID,
			Runtime:         ws.Runtime,
			WasInterrupted:  ws.WasInterrupted,
			HasBeenPushed:   ws.HasBeenPushed,
			PRNumber:        ws.PRNumber,
			PRURL:           ws.PRURL,
			CreatedAt:       ws.CreatedAt,
		})
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write: write to temp file with unique name, then rename
	finalPath := StateFilePath(dir)
	tempPath := fmt.Sprintf("%s.tmp.%d", finalPath, time.Now().UnixNano())

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}

	if err := os.Rename(tempPath, finalPath); err != nil {
		// Clean up temp file on rename failure
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename temp state file: %w", err)
	}

	return nil
}

// LoadState loads the application state from a file
func LoadState(dir string) (*AppState, error) {
	return loadStateUnsafe(dir)
}

// loadStateUnsafe loads state without acquiring the mutex.
// Used internally by SaveStateWithRepoInfo which already holds the lock.
func loadStateUnsafe(dir string) (*AppState, error) {
	data, err := os.ReadFile(StateFilePath(dir))
	if err != nil {
		return nil, err
	}

	var state AppState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

// StateExists checks if a state file exists in the directory
func StateExists(dir string) bool {
	_, err := os.Stat(StateFilePath(dir))
	return err == nil
}

// DeleteState removes the state file
func DeleteState(dir string) error {
	path := StateFilePath(dir)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // Already deleted
	}
	return os.Remove(path)
}
