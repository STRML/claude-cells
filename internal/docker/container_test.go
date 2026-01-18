package docker

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestContainerConfig(t *testing.T) {
	tests := []struct {
		name              string
		branchName        string
		repoPath          string
		wantPrefix        string
		wantContains      []string // parts that should be in the name
		wantNotContains   []string // parts that should NOT be in the name
	}{
		{
			name:         "creates config with branch name and project",
			branchName:   "add-auth",
			repoPath:     "/path/to/repo",
			wantPrefix:   "docker-tui-",
			wantContains: []string{"repo", "add-auth"},
		},
		{
			name:            "sanitizes slashes in branch name",
			branchName:      "feature/add-auth",
			repoPath:        "/path/to/repo",
			wantPrefix:      "docker-tui-",
			wantContains:    []string{"repo", "feature-add-auth"},
			wantNotContains: []string{"/"},
		},
		{
			name:         "sanitizes spaces in branch name",
			branchName:   "my feature",
			repoPath:     "/path/to/project",
			wantPrefix:   "docker-tui-",
			wantContains: []string{"project", "my-feature"},
		},
		{
			name:         "handles empty repo path",
			branchName:   "test",
			repoPath:     "",
			wantPrefix:   "docker-tui-",
			wantContains: []string{"workspace", "test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewContainerConfig(tt.branchName, tt.repoPath)

			if !strings.HasPrefix(cfg.Name, tt.wantPrefix) {
				t.Errorf("Name = %q, want prefix %q", cfg.Name, tt.wantPrefix)
			}

			for _, part := range tt.wantContains {
				if !strings.Contains(cfg.Name, part) {
					t.Errorf("Name = %q, want to contain %q", cfg.Name, part)
				}
			}

			for _, part := range tt.wantNotContains {
				if strings.Contains(cfg.Name, part) {
					t.Errorf("Name = %q, should not contain %q", cfg.Name, part)
				}
			}

			// Name should contain a timestamp (8 digits for date, 6 for time)
			// Format: docker-tui-project-branch-YYYYMMDD-HHMMSS
			if !strings.Contains(cfg.Name, "-202") { // 202X year check
				t.Errorf("Name = %q, should contain timestamp", cfg.Name)
			}

			if tt.repoPath != "" && cfg.RepoPath != tt.repoPath {
				t.Errorf("RepoPath = %q, want %q", cfg.RepoPath, tt.repoPath)
			}
		})
	}
}

// Integration test - requires Docker
func TestContainer_Lifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	client := skipIfDockerUnavailable(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Use alpine for fast testing
	cfg := &ContainerConfig{
		Name:      "docker-tui-test-" + time.Now().Format("150405"),
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

	client := skipIfDockerUnavailable(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Use alpine for testing
	cfg := &ContainerConfig{
		Name:  "docker-tui-signal-test-" + time.Now().Format("150405"),
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

	client := skipIfDockerUnavailable(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cfg := &ContainerConfig{
		Name:  "docker-tui-pause-test-" + time.Now().Format("150405"),
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
