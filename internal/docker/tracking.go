package docker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	trackingFileName  = "containers.json"
	heartbeatFileName = "heartbeat"
	heartbeatStaleAge = 30 * time.Second
)

// TrackedContainer holds metadata about a running container
type TrackedContainer struct {
	ContainerID  string    `json:"container_id"`
	WorkstreamID string    `json:"workstream_id"`
	BranchName   string    `json:"branch_name"`
	RepoPath     string    `json:"repo_path"`
	CreatedAt    time.Time `json:"created_at"`
}

// ContainerTracker manages tracking of running containers for crash recovery
type ContainerTracker struct {
	mu         sync.Mutex
	baseDir    string
	containers map[string]TrackedContainer // keyed by container ID
}

// NewContainerTracker creates a new container tracker
func NewContainerTracker() (*ContainerTracker, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	baseDir := filepath.Join(homeDir, ".claude-cells")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create tracking directory: %w", err)
	}

	tracker := &ContainerTracker{
		baseDir:    baseDir,
		containers: make(map[string]TrackedContainer),
	}

	// Load existing tracked containers
	if err := tracker.load(); err != nil {
		// If file doesn't exist, that's fine - start fresh
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load tracking data: %w", err)
		}
	}

	return tracker, nil
}

// trackingFilePath returns the path to the tracking file
func (t *ContainerTracker) trackingFilePath() string {
	return filepath.Join(t.baseDir, trackingFileName)
}

// heartbeatFilePath returns the path to the heartbeat file
func (t *ContainerTracker) heartbeatFilePath() string {
	return filepath.Join(t.baseDir, heartbeatFileName)
}

// load reads tracked containers from disk
func (t *ContainerTracker) load() error {
	data, err := os.ReadFile(t.trackingFilePath())
	if err != nil {
		return err
	}

	var containers []TrackedContainer
	if err := json.Unmarshal(data, &containers); err != nil {
		return err
	}

	t.containers = make(map[string]TrackedContainer)
	for _, c := range containers {
		t.containers[c.ContainerID] = c
	}

	return nil
}

// save writes tracked containers to disk atomically
func (t *ContainerTracker) save() error {
	containers := make([]TrackedContainer, 0, len(t.containers))
	for _, c := range t.containers {
		containers = append(containers, c)
	}

	data, err := json.MarshalIndent(containers, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write
	finalPath := t.trackingFilePath()
	tempPath := fmt.Sprintf("%s.tmp.%d", finalPath, time.Now().UnixNano())

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}

	if err := os.Rename(tempPath, finalPath); err != nil {
		os.Remove(tempPath)
		return err
	}

	return nil
}

// Track adds a container to the tracking list
func (t *ContainerTracker) Track(containerID, workstreamID, branchName, repoPath string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.containers[containerID] = TrackedContainer{
		ContainerID:  containerID,
		WorkstreamID: workstreamID,
		BranchName:   branchName,
		RepoPath:     repoPath,
		CreatedAt:    time.Now(),
	}

	return t.save()
}

// Untrack removes a container from the tracking list
func (t *ContainerTracker) Untrack(containerID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.containers, containerID)
	return t.save()
}

// GetTracked returns all tracked containers
func (t *ContainerTracker) GetTracked() []TrackedContainer {
	t.mu.Lock()
	defer t.mu.Unlock()

	result := make([]TrackedContainer, 0, len(t.containers))
	for _, c := range t.containers {
		result = append(result, c)
	}
	return result
}

// GetTrackedIDs returns just the container IDs
func (t *ContainerTracker) GetTrackedIDs() []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	ids := make([]string, 0, len(t.containers))
	for id := range t.containers {
		ids = append(ids, id)
	}
	return ids
}

// Clear removes all tracked containers
func (t *ContainerTracker) Clear() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.containers = make(map[string]TrackedContainer)
	return t.save()
}

// WriteHeartbeat writes the current PID and timestamp to the heartbeat file
func (t *ContainerTracker) WriteHeartbeat(pid int) error {
	data := fmt.Sprintf("%d\n%d", pid, time.Now().Unix())
	return os.WriteFile(t.heartbeatFilePath(), []byte(data), 0644)
}

// IsHeartbeatStale returns true if the heartbeat file is missing or stale
func (t *ContainerTracker) IsHeartbeatStale() bool {
	info, err := os.Stat(t.heartbeatFilePath())
	if err != nil {
		// No heartbeat file means previous session crashed or never started
		return true
	}

	// Check if modification time is stale
	return time.Since(info.ModTime()) > heartbeatStaleAge
}

// RemoveHeartbeat removes the heartbeat file (called on clean shutdown)
func (t *ContainerTracker) RemoveHeartbeat() error {
	err := os.Remove(t.heartbeatFilePath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// GetOrphanedContainers returns container IDs that are tracked but have a stale heartbeat.
// These are containers from a crashed session that need cleanup.
func (t *ContainerTracker) GetOrphanedContainers() []TrackedContainer {
	if !t.IsHeartbeatStale() {
		// Session is still alive
		return nil
	}

	return t.GetTracked()
}
