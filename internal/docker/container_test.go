//go:build integration

package docker

import (
	"context"
	"strings"
	"testing"
	"time"
)

// These tests require Docker and are only run with: go test -tags=integration
// Unit tests (like TestContainerConfig) are in config_test.go

// Integration test - requires Docker
func TestContainer_Lifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	t.Parallel()

	client := skipIfDockerUnavailable(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Use alpine for fast testing
	cfg := &ContainerConfig{
		Name:      "ccells-test-" + time.Now().Format("150405"),
		Image:     "alpine:latest",
		RepoPath:  "/tmp",
		ClaudeCfg: "/tmp",
	}

	// Create container
	containerID, err := client.CreateContainer(ctx, cfg)
	if err != nil {
		t.Fatalf("CreateContainer() error = %v", err)
	}
	if containerID == "" {
		t.Fatal("CreateContainer() returned empty ID")
	}

	// Cleanup
	defer func() {
		_ = client.StopContainer(ctx, containerID)
		_ = client.RemoveContainer(ctx, containerID)
	}()

	// Start container
	err = client.StartContainer(ctx, containerID)
	if err != nil {
		t.Fatalf("StartContainer() error = %v", err)
	}

	// Check it's running
	running, err := client.IsContainerRunning(ctx, containerID)
	if err != nil {
		t.Fatalf("IsContainerRunning() error = %v", err)
	}
	if !running {
		t.Error("Container should be running")
	}

	// Execute command
	output, err := client.ExecInContainer(ctx, containerID, []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("ExecInContainer() error = %v", err)
	}
	if !strings.Contains(output, "hello") {
		t.Errorf("ExecInContainer() output = %q, want to contain 'hello'", output)
	}

	// Stop container
	err = client.StopContainer(ctx, containerID)
	if err != nil {
		t.Fatalf("StopContainer() error = %v", err)
	}

	// Verify stopped
	running, err = client.IsContainerRunning(ctx, containerID)
	if err != nil {
		t.Fatalf("IsContainerRunning() error = %v", err)
	}
	if running {
		t.Error("Container should be stopped")
	}
}

// Integration test - requires Docker
func TestContainer_SignalProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	t.Parallel()

	client := skipIfDockerUnavailable(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Use alpine for testing
	cfg := &ContainerConfig{
		Name:  "ccells-signal-test-" + time.Now().Format("150405"),
		Image: "alpine:latest",
	}

	// Create and start container
	containerID, err := client.CreateContainer(ctx, cfg)
	if err != nil {
		t.Fatalf("CreateContainer() error = %v", err)
	}
	defer func() {
		_ = client.StopContainer(ctx, containerID)
		_ = client.RemoveContainer(ctx, containerID)
	}()

	err = client.StartContainer(ctx, containerID)
	if err != nil {
		t.Fatalf("StartContainer() error = %v", err)
	}

	// Start a background sleep process
	_, err = client.ExecInContainer(ctx, containerID, []string{"sh", "-c", "sleep 300 &"})
	if err != nil {
		t.Fatalf("Failed to start background process: %v", err)
	}

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Send SIGTERM to sleep process
	err = client.SignalProcess(ctx, containerID, "sleep", "TERM")
	if err != nil {
		t.Errorf("SignalProcess() error = %v", err)
	}

	// Signal to non-existent process should not error (pkill returns 1 but we ignore)
	err = client.SignalProcess(ctx, containerID, "nonexistent", "TERM")
	// This might error because pkill returns 1 when no process found, but exec start doesn't check exit code
	// Either way, it shouldn't panic
}

// Integration test for pause/unpause
func TestContainer_PauseUnpause(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	t.Parallel()

	client := skipIfDockerUnavailable(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cfg := &ContainerConfig{
		Name:  "ccells-pause-test-" + time.Now().Format("150405"),
		Image: "alpine:latest",
	}

	containerID, err := client.CreateContainer(ctx, cfg)
	if err != nil {
		t.Fatalf("CreateContainer() error = %v", err)
	}
	defer func() {
		_ = client.UnpauseContainer(ctx, containerID)
		_ = client.StopContainer(ctx, containerID)
		_ = client.RemoveContainer(ctx, containerID)
	}()

	err = client.StartContainer(ctx, containerID)
	if err != nil {
		t.Fatalf("StartContainer() error = %v", err)
	}

	// Pause
	err = client.PauseContainer(ctx, containerID)
	if err != nil {
		t.Fatalf("PauseContainer() error = %v", err)
	}

	// Check state
	state, err := client.GetContainerState(ctx, containerID)
	if err != nil {
		t.Fatalf("GetContainerState() error = %v", err)
	}
	if state != "paused" {
		t.Errorf("State = %q, want 'paused'", state)
	}

	// Unpause
	err = client.UnpauseContainer(ctx, containerID)
	if err != nil {
		t.Fatalf("UnpauseContainer() error = %v", err)
	}

	// Check running again
	state, err = client.GetContainerState(ctx, containerID)
	if err != nil {
		t.Fatalf("GetContainerState() error = %v", err)
	}
	if state != "running" {
		t.Errorf("State = %q, want 'running'", state)
	}
}

// Integration test for cleanup orphaned containers
// NOTE: This test cannot run in parallel as it removes containers globally
func TestContainer_CleanupOrphanedContainers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	// NOT parallel - this test removes containers globally

	client := skipIfDockerUnavailable(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create two containers - one "known" and one "orphan"
	knownCfg := &ContainerConfig{
		Name:  "ccells-known-test-" + time.Now().Format("150405"),
		Image: "alpine:latest",
	}
	orphanCfg := &ContainerConfig{
		Name:  "ccells-orphan-test-" + time.Now().Format("150405"),
		Image: "alpine:latest",
	}

	knownID, err := client.CreateContainer(ctx, knownCfg)
	if err != nil {
		t.Fatalf("CreateContainer(known) error = %v", err)
	}

	orphanID, err := client.CreateContainer(ctx, orphanCfg)
	if err != nil {
		_ = client.RemoveContainer(ctx, knownID)
		t.Fatalf("CreateContainer(orphan) error = %v", err)
	}

	// Cleanup known container at end
	defer func() {
		_ = client.StopContainer(ctx, knownID)
		_ = client.RemoveContainer(ctx, knownID)
	}()

	// Start both containers
	if err := client.StartContainer(ctx, knownID); err != nil {
		t.Fatalf("StartContainer(known) error = %v", err)
	}
	if err := client.StartContainer(ctx, orphanID); err != nil {
		t.Fatalf("StartContainer(orphan) error = %v", err)
	}

	// Call cleanup with only knownID as known
	removed, err := client.CleanupOrphanedContainers(ctx, []string{knownID})
	if err != nil {
		t.Fatalf("CleanupOrphanedContainers() error = %v", err)
	}

	// Should have removed the orphan
	if removed < 1 {
		t.Errorf("CleanupOrphanedContainers() removed = %d, want >= 1", removed)
	}

	// Verify orphan is gone
	_, err = client.GetContainerState(ctx, orphanID)
	if err == nil {
		t.Error("Orphan container should have been removed")
		_ = client.StopContainer(ctx, orphanID)
		_ = client.RemoveContainer(ctx, orphanID)
	}

	// Verify known is still there
	state, err := client.GetContainerState(ctx, knownID)
	if err != nil {
		t.Fatalf("GetContainerState(known) error = %v", err)
	}
	if state != "running" {
		t.Errorf("Known container state = %q, want 'running'", state)
	}
}
