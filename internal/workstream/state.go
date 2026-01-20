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

const stateFileName = ".ccells-state.json"

// SavedWorkstream represents a workstream saved to disk
type SavedWorkstream struct {
	ID              string    `json:"id"`
	BranchName      string    `json:"branch_name"`
	Prompt          string    `json:"prompt"`
	Title           string    `json:"title,omitempty"` // Short summary title
	ContainerID     string    `json:"container_id"`
	ClaudeSessionID string    `json:"claude_session_id,omitempty"` // Claude Code session ID for --resume
	CreatedAt       time.Time `json:"created_at"`
}

// AppState represents the saved application state
type AppState struct {
	Version      int               `json:"version"`
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
func SaveState(dir string, workstreams []*Workstream, focusedIndex int, layout int) error {
	stateMu.Lock()
	defer stateMu.Unlock()

	state := AppState{
		Version:      1,
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
			ContainerID:     ws.ContainerID,
			ClaudeSessionID: ws.ClaudeSessionID,
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
