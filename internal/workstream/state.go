package workstream

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const stateFileName = ".docker-tui-state.json"

// SavedWorkstream represents a workstream saved to disk
type SavedWorkstream struct {
	ID          string    `json:"id"`
	BranchName  string    `json:"branch_name"`
	Prompt      string    `json:"prompt"`
	ContainerID string    `json:"container_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// AppState represents the saved application state
type AppState struct {
	Version      int               `json:"version"`
	Workstreams  []SavedWorkstream `json:"workstreams"`
	FocusedIndex int               `json:"focused_index"`
	SavedAt      time.Time         `json:"saved_at"`
}

// StateFilePath returns the path to the state file in the given directory
func StateFilePath(dir string) string {
	return filepath.Join(dir, stateFileName)
}

// SaveState saves the application state to a file
func SaveState(dir string, workstreams []*Workstream, focusedIndex int) error {
	state := AppState{
		Version:      1,
		FocusedIndex: focusedIndex,
		SavedAt:      time.Now(),
	}

	for _, ws := range workstreams {
		state.Workstreams = append(state.Workstreams, SavedWorkstream{
			ID:          ws.ID,
			BranchName:  ws.BranchName,
			Prompt:      ws.Prompt,
			ContainerID: ws.ContainerID,
			CreatedAt:   ws.CreatedAt,
		})
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(StateFilePath(dir), data, 0644)
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
