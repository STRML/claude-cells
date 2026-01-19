package docker

import (
	"context"
	"testing"
)

// Unit tests using MockClient - these run instantly without Docker

func TestMockClient_ContainerLifecycle(t *testing.T) {
	t.Parallel()

	client := NewMockClient()
	ctx := context.Background()

	cfg := &ContainerConfig{
		Name:     "test-container",
		Image:    "alpine:latest",
		RepoPath: "/tmp",
	}

	// Create
	id, err := client.CreateContainer(ctx, cfg)
	if err != nil {
		t.Fatalf("CreateContainer() error = %v", err)
	}
	if id == "" {
		t.Fatal("CreateContainer() returned empty ID")
	}

	// Check state
	state, err := client.GetContainerState(ctx, id)
	if err != nil {
		t.Fatalf("GetContainerState() error = %v", err)
	}
	if state != "created" {
		t.Errorf("State = %q, want 'created'", state)
	}

	// Start
	if err := client.StartContainer(ctx, id); err != nil {
		t.Fatalf("StartContainer() error = %v", err)
	}

	// Check running
	running, err := client.IsContainerRunning(ctx, id)
	if err != nil {
		t.Fatalf("IsContainerRunning() error = %v", err)
	}
	if !running {
		t.Error("Container should be running")
	}

	// Stop
	if err := client.StopContainer(ctx, id); err != nil {
		t.Fatalf("StopContainer() error = %v", err)
	}

	running, _ = client.IsContainerRunning(ctx, id)
	if running {
		t.Error("Container should not be running")
	}

	// Remove
	if err := client.RemoveContainer(ctx, id); err != nil {
		t.Fatalf("RemoveContainer() error = %v", err)
	}

	// Verify gone
	_, err = client.GetContainerState(ctx, id)
	if err == nil {
		t.Error("Container should not exist after removal")
	}
}

func TestMockClient_PauseUnpause(t *testing.T) {
	t.Parallel()

	client := NewMockClient()
	ctx := context.Background()

	cfg := &ContainerConfig{Name: "pause-test", Image: "alpine"}
	id, _ := client.CreateContainer(ctx, cfg)
	_ = client.StartContainer(ctx, id)

	// Pause
	if err := client.PauseContainer(ctx, id); err != nil {
		t.Fatalf("PauseContainer() error = %v", err)
	}

	state, _ := client.GetContainerState(ctx, id)
	if state != "paused" {
		t.Errorf("State = %q, want 'paused'", state)
	}

	// Unpause
	if err := client.UnpauseContainer(ctx, id); err != nil {
		t.Fatalf("UnpauseContainer() error = %v", err)
	}

	state, _ = client.GetContainerState(ctx, id)
	if state != "running" {
		t.Errorf("State = %q, want 'running'", state)
	}
}

func TestMockClient_ExecInContainer(t *testing.T) {
	t.Parallel()

	client := NewMockClient()
	ctx := context.Background()

	cfg := &ContainerConfig{Name: "exec-test", Image: "alpine"}
	id, _ := client.CreateContainer(ctx, cfg)
	_ = client.StartContainer(ctx, id)

	output, err := client.ExecInContainer(ctx, id, []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("ExecInContainer() error = %v", err)
	}
	if output != "hello" {
		t.Errorf("Output = %q, want 'hello'", output)
	}
}

func TestMockClient_ListContainers(t *testing.T) {
	t.Parallel()

	client := NewMockClient()
	ctx := context.Background()

	// Create multiple containers with unique names
	names := []string{"list-test-1", "list-test-2", "list-test-3"}
	for _, name := range names {
		cfg := &ContainerConfig{
			Name:  name,
			Image: "alpine",
		}
		id, _ := client.CreateContainer(ctx, cfg)
		_ = client.StartContainer(ctx, id)
	}

	containers, err := client.ListDockerTUIContainers(ctx)
	if err != nil {
		t.Fatalf("ListDockerTUIContainers() error = %v", err)
	}
	if len(containers) != 3 {
		t.Errorf("Got %d containers, want 3", len(containers))
	}
}

func TestMockClient_CleanupOrphaned(t *testing.T) {
	t.Parallel()

	client := NewMockClient()
	ctx := context.Background()

	// Create known and orphan containers
	knownCfg := &ContainerConfig{Name: "known", Image: "alpine"}
	knownID, _ := client.CreateContainer(ctx, knownCfg)
	_ = client.StartContainer(ctx, knownID)

	orphanCfg := &ContainerConfig{Name: "orphan", Image: "alpine"}
	orphanID, _ := client.CreateContainer(ctx, orphanCfg)
	_ = client.StartContainer(ctx, orphanID)

	// Cleanup with only known ID
	removed, err := client.CleanupOrphanedContainers(ctx, []string{knownID})
	if err != nil {
		t.Fatalf("CleanupOrphanedContainers() error = %v", err)
	}
	if removed != 1 {
		t.Errorf("Removed = %d, want 1", removed)
	}

	// Verify known still exists
	_, err = client.GetContainerState(ctx, knownID)
	if err != nil {
		t.Error("Known container should still exist")
	}

	// Verify orphan is gone
	_, err = client.GetContainerState(ctx, orphanID)
	if err == nil {
		t.Error("Orphan container should be removed")
	}
}

func TestMockClient_ImageExists(t *testing.T) {
	t.Parallel()

	client := NewMockClient()
	ctx := context.Background()

	// Default: all images exist
	exists, err := client.ImageExists(ctx, "alpine:latest")
	if err != nil {
		t.Fatalf("ImageExists() error = %v", err)
	}
	if !exists {
		t.Error("Default should return true")
	}

	// Custom behavior
	client.ImageExistsFn = func(ctx context.Context, imageName string) (bool, error) {
		return imageName == "exists:latest", nil
	}

	exists, _ = client.ImageExists(ctx, "exists:latest")
	if !exists {
		t.Error("Should exist")
	}

	exists, _ = client.ImageExists(ctx, "notexists:latest")
	if exists {
		t.Error("Should not exist")
	}
}

func TestMockClient_Prune(t *testing.T) {
	t.Parallel()

	client := NewMockClient()
	ctx := context.Background()

	// Create running and stopped containers
	cfg1 := &ContainerConfig{Name: "running", Image: "alpine"}
	id1, _ := client.CreateContainer(ctx, cfg1)
	_ = client.StartContainer(ctx, id1)

	cfg2 := &ContainerConfig{Name: "stopped", Image: "alpine"}
	id2, _ := client.CreateContainer(ctx, cfg2)
	_ = client.StartContainer(ctx, id2)
	_ = client.StopContainer(ctx, id2)

	// Prune stopped only
	removed, err := client.PruneDockerTUIContainers(ctx)
	if err != nil {
		t.Fatalf("PruneDockerTUIContainers() error = %v", err)
	}
	if removed != 1 {
		t.Errorf("Removed = %d, want 1", removed)
	}

	// Running should still exist
	_, err = client.GetContainerState(ctx, id1)
	if err != nil {
		t.Error("Running container should still exist")
	}
}
