//go:build integration

package docker

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// These tests require Docker and are only run with: go test -tags=integration
// Unit tests (like TestContainerConfig) are in config_test.go

// getTestImage returns an available image for testing, or skips the test
func getTestImage(t *testing.T, client *Client, ctx context.Context) string {
	t.Helper()

	// Prefer ccells-base (built by CI)
	if exists, _ := client.ImageExists(ctx, RequiredImage); exists {
		return RequiredImage
	}

	// Fall back to alpine if available locally
	if exists, _ := client.ImageExists(ctx, "alpine:latest"); exists {
		return "alpine:latest"
	}

	t.Skip("No test image available (need ccells-base or alpine:latest)")
	return ""
}

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

	testImage := getTestImage(t, client, ctx)

	cfg := &ContainerConfig{
		Name:      "ccells-test-" + time.Now().Format("150405"),
		Image:     testImage,
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

	testImage := getTestImage(t, client, ctx)

	cfg := &ContainerConfig{
		Name:  "ccells-signal-test-" + time.Now().Format("150405"),
		Image: testImage,
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

	testImage := getTestImage(t, client, ctx)

	cfg := &ContainerConfig{
		Name:  "ccells-pause-test-" + time.Now().Format("150405"),
		Image: testImage,
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

// TestContainer_PersistSessions verifies PersistSessions is a no-op
// (sessions are now written directly to the mounted ~/.claude directory)
// Note: More comprehensive test exists in mock_client_test.go
func TestContainer_PersistSessions(t *testing.T) {
	t.Parallel()

	client := NewMockClient()
	ctx := context.Background()

	// Create a container first - PersistSessions requires container to exist
	cfg := &ContainerConfig{Name: "persist-test", Image: "test"}
	id, err := client.CreateContainer(ctx, cfg)
	if err != nil {
		t.Fatalf("CreateContainer() error = %v", err)
	}
	_ = client.StartContainer(ctx, id)

	// PersistSessions should be a no-op and return nil for existing container
	err = client.PersistSessions(ctx, id)
	if err != nil {
		t.Errorf("PersistSessions() error = %v, want nil", err)
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

	testImage := getTestImage(t, client, ctx)

	// Use a unique project name to avoid conflicts with other tests
	projectName := "cleanuptest"
	timestamp := time.Now().Format("150405")

	// Container names follow format: ccells-<projectName>-<branchName>-<timestamp>
	// So for project "cleanuptest", containers should be "ccells-cleanuptest-*"
	knownCfg := &ContainerConfig{
		Name:  fmt.Sprintf("ccells-%s-known-%s", projectName, timestamp),
		Image: testImage,
	}
	orphanCfg := &ContainerConfig{
		Name:  fmt.Sprintf("ccells-%s-orphan-%s", projectName, timestamp),
		Image: testImage,
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

	// Call cleanup with only knownID as known - orphan should be removed
	removed, err := client.CleanupOrphanedContainers(ctx, projectName, []string{knownID}, nil)
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
