package tui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/samuelreed/docker-tui/internal/docker"
	"github.com/samuelreed/docker-tui/internal/workstream"
)

// skipIfDockerUnavailable skips the test if Docker daemon is not accessible.
func skipIfDockerUnavailable(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := docker.ValidatePrerequisites(ctx)
	if err != nil {
		t.Skipf("Docker validation failed: %v", err)
	}
	if !result.DockerAvailable {
		t.Skip("Docker daemon not available")
	}
	if !result.ImageExists {
		t.Skipf("Required image %s not found. Run: docker build -t %s -f configs/base.Dockerfile .",
			docker.RequiredImage, docker.RequiredImage)
	}
}

// setupTestRepo creates a temporary git repository for testing.
// It returns the repo path and a cleanup function.
func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "docker-tui-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	// Initialize git repo
	if err := exec.Command("git", "-C", tmpDir, "init").Run(); err != nil {
		cleanup()
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git user (required for commits)
	if err := exec.Command("git", "-C", tmpDir, "config", "user.email", "test@test.com").Run(); err != nil {
		cleanup()
		t.Fatalf("Failed to config git email: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run(); err != nil {
		cleanup()
		t.Fatalf("Failed to config git name: %v", err)
	}

	// Create initial commit (needed for branches)
	readmePath := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test Repo\n"), 0644); err != nil {
		cleanup()
		t.Fatalf("Failed to create README: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "add", "README.md").Run(); err != nil {
		cleanup()
		t.Fatalf("Failed to git add: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "commit", "-m", "Initial commit").Run(); err != nil {
		cleanup()
		t.Fatalf("Failed to git commit: %v", err)
	}

	return tmpDir, cleanup
}

func TestStartContainerCmd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	skipIfDockerUnavailable(t)

	// Set up a temporary git repo to avoid polluting the main repo
	tmpDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Change to temp directory for test (restore after)
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// Create a test workstream
	ws := workstream.New("test container creation")

	// Execute the command
	cmd := StartContainerCmd(ws)
	msg := cmd()

	// Should get either ContainerStartedMsg or ContainerErrorMsg
	switch m := msg.(type) {
	case ContainerStartedMsg:
		if m.WorkstreamID != ws.ID {
			t.Errorf("WorkstreamID = %q, want %q", m.WorkstreamID, ws.ID)
		}
		if m.ContainerID == "" {
			t.Error("ContainerID should not be empty")
		}

		// Cleanup: stop and remove the container
		ws.SetContainerID(m.ContainerID)
		stopCmd := StopContainerCmd(ws)
		stopCmd()

	case ContainerErrorMsg:
		t.Errorf("Container creation failed: %v", m.Error)

	default:
		t.Errorf("Unexpected message type: %T", msg)
	}
}

func TestStopContainerCmd_NoContainer(t *testing.T) {
	ws := workstream.New("test no container")
	// ContainerID is empty

	cmd := StopContainerCmd(ws)
	msg := cmd()

	switch m := msg.(type) {
	case ContainerStoppedMsg:
		if m.WorkstreamID != ws.ID {
			t.Errorf("WorkstreamID = %q, want %q", m.WorkstreamID, ws.ID)
		}
	default:
		t.Errorf("Expected ContainerStoppedMsg, got %T", msg)
	}
}

func TestFetchContainerLogsCmd_NoContainer(t *testing.T) {
	ws := workstream.New("test no container logs")
	// ContainerID is empty

	cmd := FetchContainerLogsCmd(ws)
	msg := cmd()

	switch m := msg.(type) {
	case ContainerLogsMsg:
		if m.WorkstreamID != ws.ID {
			t.Errorf("WorkstreamID = %q, want %q", m.WorkstreamID, ws.ID)
		}
		if m.Logs != "" {
			t.Errorf("Logs should be empty for no container, got %q", m.Logs)
		}
		if m.Error != nil {
			t.Errorf("Error should be nil, got %v", m.Error)
		}
	default:
		t.Errorf("Expected ContainerLogsMsg, got %T", msg)
	}
}

func TestContainerLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	skipIfDockerUnavailable(t)

	// Set up a temporary git repo to avoid polluting the main repo
	tmpDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Change to temp directory for test (restore after)
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// Create workstream
	ws := workstream.New("integration test lifecycle")

	// Step 1: Start container
	startCmd := StartContainerCmd(ws)
	startMsg := startCmd()

	started, ok := startMsg.(ContainerStartedMsg)
	if !ok {
		if errMsg, isErr := startMsg.(ContainerErrorMsg); isErr {
			t.Fatalf("Container start failed: %v", errMsg.Error)
		}
		t.Fatalf("Expected ContainerStartedMsg, got %T", startMsg)
	}

	ws.SetContainerID(started.ContainerID)
	defer func() {
		// Cleanup
		stopCmd := StopContainerCmd(ws)
		stopCmd()
	}()

	// Step 2: Verify we can fetch logs
	logsCmd := FetchContainerLogsCmd(ws)
	logsMsg := logsCmd()

	logs, ok := logsMsg.(ContainerLogsMsg)
	if !ok {
		t.Fatalf("Expected ContainerLogsMsg, got %T", logsMsg)
	}
	if logs.Error != nil {
		t.Errorf("Logs fetch error: %v", logs.Error)
	}

	// Step 3: Stop container
	stopCmd := StopContainerCmd(ws)
	stopMsg := stopCmd()

	_, ok = stopMsg.(ContainerStoppedMsg)
	if !ok {
		if errMsg, isErr := stopMsg.(ContainerErrorMsg); isErr {
			t.Errorf("Container stop error: %v", errMsg.Error)
		}
	}
}

func TestContainerMessages(t *testing.T) {
	tests := []struct {
		name string
		msg  interface{}
	}{
		{
			name: "ContainerStartedMsg",
			msg:  ContainerStartedMsg{WorkstreamID: "ws1", ContainerID: "c1"},
		},
		{
			name: "ContainerErrorMsg",
			msg:  ContainerErrorMsg{WorkstreamID: "ws1", Error: nil},
		},
		{
			name: "ContainerOutputMsg",
			msg:  ContainerOutputMsg{WorkstreamID: "ws1", Output: []byte("test")},
		},
		{
			name: "ContainerStoppedMsg",
			msg:  ContainerStoppedMsg{WorkstreamID: "ws1"},
		},
		{
			name: "PTYReadyMsg",
			msg:  PTYReadyMsg{WorkstreamID: "ws1", Session: nil},
		},
		{
			name: "ContainerLogsMsg",
			msg:  ContainerLogsMsg{WorkstreamID: "ws1", Logs: "test", Error: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify the types exist and can be created
			if tt.msg == nil {
				t.Error("Message should not be nil")
			}
		})
	}
}
