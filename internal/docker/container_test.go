package docker

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestContainerConfig(t *testing.T) {
	tests := []struct {
		name       string
		branchName string
		repoPath   string
		wantName   string
	}{
		{
			name:       "creates config with branch name",
			branchName: "add-auth",
			repoPath:   "/path/to/repo",
			wantName:   "docker-tui-add-auth",
		},
		{
			name:       "sanitizes special chars in name",
			branchName: "feature/add-auth",
			repoPath:   "/path/to/repo",
			wantName:   "docker-tui-feature-add-auth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewContainerConfig(tt.branchName, tt.repoPath)
			if cfg.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", cfg.Name, tt.wantName)
			}
			if cfg.RepoPath != tt.repoPath {
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

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
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
