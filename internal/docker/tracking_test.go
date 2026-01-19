package docker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestContainerTracker_TrackUntrack(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "tracking-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create tracker with custom base dir
	tracker := &ContainerTracker{
		baseDir:    tmpDir,
		containers: make(map[string]TrackedContainer),
	}

	// Track a container
	err = tracker.Track("container-1", "ws-1", "feature-branch", "/repo/path")
	if err != nil {
		t.Fatalf("Track() error: %v", err)
	}

	// Verify it's tracked
	tracked := tracker.GetTracked()
	if len(tracked) != 1 {
		t.Errorf("Expected 1 tracked container, got %d", len(tracked))
	}
	if tracked[0].ContainerID != "container-1" {
		t.Errorf("Expected container-1, got %s", tracked[0].ContainerID)
	}

	// Track another
	err = tracker.Track("container-2", "ws-2", "another-branch", "/other/path")
	if err != nil {
		t.Fatalf("Track() second container error: %v", err)
	}

	tracked = tracker.GetTracked()
	if len(tracked) != 2 {
		t.Errorf("Expected 2 tracked containers, got %d", len(tracked))
	}

	// Untrack first
	err = tracker.Untrack("container-1")
	if err != nil {
		t.Fatalf("Untrack() error: %v", err)
	}

	tracked = tracker.GetTracked()
	if len(tracked) != 1 {
		t.Errorf("Expected 1 tracked container after untrack, got %d", len(tracked))
	}
	if tracked[0].ContainerID != "container-2" {
		t.Errorf("Expected container-2 to remain, got %s", tracked[0].ContainerID)
	}
}

func TestContainerTracker_Persistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tracking-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create tracker and track containers
	tracker1 := &ContainerTracker{
		baseDir:    tmpDir,
		containers: make(map[string]TrackedContainer),
	}

	err = tracker1.Track("container-1", "ws-1", "branch-1", "/path/1")
	if err != nil {
		t.Fatalf("Track() error: %v", err)
	}
	err = tracker1.Track("container-2", "ws-2", "branch-2", "/path/2")
	if err != nil {
		t.Fatalf("Track() second error: %v", err)
	}

	// Create new tracker instance that loads from disk
	tracker2 := &ContainerTracker{
		baseDir:    tmpDir,
		containers: make(map[string]TrackedContainer),
	}
	err = tracker2.load()
	if err != nil {
		t.Fatalf("load() error: %v", err)
	}

	tracked := tracker2.GetTracked()
	if len(tracked) != 2 {
		t.Errorf("Expected 2 tracked containers after reload, got %d", len(tracked))
	}
}

func TestContainerTracker_Heartbeat(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tracking-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tracker := &ContainerTracker{
		baseDir:    tmpDir,
		containers: make(map[string]TrackedContainer),
	}

	// Initially stale (no heartbeat file)
	if !tracker.IsHeartbeatStale() {
		t.Error("Expected stale heartbeat when file doesn't exist")
	}

	// Write heartbeat
	err = tracker.WriteHeartbeat(12345)
	if err != nil {
		t.Fatalf("WriteHeartbeat() error: %v", err)
	}

	// Should not be stale now
	if tracker.IsHeartbeatStale() {
		t.Error("Expected non-stale heartbeat after writing")
	}

	// Verify file exists
	_, err = os.Stat(tracker.heartbeatFilePath())
	if err != nil {
		t.Errorf("Heartbeat file should exist: %v", err)
	}

	// Remove heartbeat
	err = tracker.RemoveHeartbeat()
	if err != nil {
		t.Fatalf("RemoveHeartbeat() error: %v", err)
	}

	// Should be stale again
	if !tracker.IsHeartbeatStale() {
		t.Error("Expected stale heartbeat after removal")
	}
}

func TestContainerTracker_GetOrphanedContainers(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tracking-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tracker := &ContainerTracker{
		baseDir:    tmpDir,
		containers: make(map[string]TrackedContainer),
	}

	// Track some containers
	tracker.Track("container-1", "ws-1", "branch-1", "/path/1")
	tracker.Track("container-2", "ws-2", "branch-2", "/path/2")

	// With no heartbeat (stale), all containers are orphaned
	orphaned := tracker.GetOrphanedContainers()
	if len(orphaned) != 2 {
		t.Errorf("Expected 2 orphaned containers with stale heartbeat, got %d", len(orphaned))
	}

	// Write heartbeat - no longer orphaned
	tracker.WriteHeartbeat(12345)
	orphaned = tracker.GetOrphanedContainers()
	if len(orphaned) != 0 {
		t.Errorf("Expected 0 orphaned containers with fresh heartbeat, got %d", len(orphaned))
	}
}

func TestContainerTracker_GetTrackedIDs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tracking-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tracker := &ContainerTracker{
		baseDir:    tmpDir,
		containers: make(map[string]TrackedContainer),
	}

	tracker.Track("container-1", "ws-1", "branch-1", "/path/1")
	tracker.Track("container-2", "ws-2", "branch-2", "/path/2")

	ids := tracker.GetTrackedIDs()
	if len(ids) != 2 {
		t.Errorf("Expected 2 IDs, got %d", len(ids))
	}

	// Check both IDs are present (order not guaranteed)
	hasContainer1 := false
	hasContainer2 := false
	for _, id := range ids {
		if id == "container-1" {
			hasContainer1 = true
		}
		if id == "container-2" {
			hasContainer2 = true
		}
	}
	if !hasContainer1 || !hasContainer2 {
		t.Errorf("Missing expected container IDs: got %v", ids)
	}
}

func TestContainerTracker_Clear(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tracking-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tracker := &ContainerTracker{
		baseDir:    tmpDir,
		containers: make(map[string]TrackedContainer),
	}

	tracker.Track("container-1", "ws-1", "branch-1", "/path/1")
	tracker.Track("container-2", "ws-2", "branch-2", "/path/2")

	if len(tracker.GetTracked()) != 2 {
		t.Error("Expected 2 containers before clear")
	}

	err = tracker.Clear()
	if err != nil {
		t.Fatalf("Clear() error: %v", err)
	}

	if len(tracker.GetTracked()) != 0 {
		t.Error("Expected 0 containers after clear")
	}

	// Verify persistence
	tracker2 := &ContainerTracker{
		baseDir:    tmpDir,
		containers: make(map[string]TrackedContainer),
	}
	tracker2.load()
	if len(tracker2.GetTracked()) != 0 {
		t.Error("Expected 0 containers in reloaded tracker")
	}
}

func TestContainerTracker_TrackedContainerFields(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tracking-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tracker := &ContainerTracker{
		baseDir:    tmpDir,
		containers: make(map[string]TrackedContainer),
	}

	before := time.Now()
	tracker.Track("container-1", "ws-123", "feature/test-branch", "/home/user/repo")
	after := time.Now()

	tracked := tracker.GetTracked()
	if len(tracked) != 1 {
		t.Fatalf("Expected 1 tracked container")
	}

	c := tracked[0]
	if c.ContainerID != "container-1" {
		t.Errorf("ContainerID = %s, want container-1", c.ContainerID)
	}
	if c.WorkstreamID != "ws-123" {
		t.Errorf("WorkstreamID = %s, want ws-123", c.WorkstreamID)
	}
	if c.BranchName != "feature/test-branch" {
		t.Errorf("BranchName = %s, want feature/test-branch", c.BranchName)
	}
	if c.RepoPath != "/home/user/repo" {
		t.Errorf("RepoPath = %s, want /home/user/repo", c.RepoPath)
	}
	if c.CreatedAt.Before(before) || c.CreatedAt.After(after) {
		t.Errorf("CreatedAt = %v, should be between %v and %v", c.CreatedAt, before, after)
	}
}

func TestContainerTracker_AtomicSave(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tracking-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tracker := &ContainerTracker{
		baseDir:    tmpDir,
		containers: make(map[string]TrackedContainer),
	}

	tracker.Track("container-1", "ws-1", "branch-1", "/path/1")

	// Verify no temp files remain
	entries, _ := os.ReadDir(tmpDir)
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Errorf("Temp file should not remain: %s", entry.Name())
		}
	}
}
