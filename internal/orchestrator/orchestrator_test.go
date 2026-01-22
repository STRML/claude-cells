package orchestrator

import (
	"context"
	"testing"

	"github.com/STRML/claude-cells/internal/docker"
	"github.com/STRML/claude-cells/internal/git"
	"github.com/STRML/claude-cells/internal/workstream"
)

func TestNew(t *testing.T) {
	mockDocker := docker.NewMockClient()
	gitFactory := func(path string) git.GitClient {
		return git.NewMockGitClient()
	}

	orch := New(mockDocker, gitFactory, "/test/repo")

	if orch == nil {
		t.Fatal("expected non-nil orchestrator")
	}
	if orch.repoPath != "/test/repo" {
		t.Errorf("expected repoPath /test/repo, got %s", orch.repoPath)
	}
	if orch.dockerClient == nil {
		t.Error("expected non-nil dockerClient")
	}
	if orch.gitFactory == nil {
		t.Error("expected non-nil gitFactory")
	}
}

func TestCreateWorkstream_CreatesWorktree(t *testing.T) {
	mockDocker := docker.NewMockClient()
	mockGit := git.NewMockGitClient()
	gitFactory := func(path string) git.GitClient {
		return mockGit
	}

	orch := New(mockDocker, gitFactory, "/test/repo")

	ws := &workstream.Workstream{
		ID:         "test-id",
		BranchName: "ccells/test-branch",
	}

	opts := CreateOptions{
		RepoPath: "/test/repo",
	}

	ctx := context.Background()
	_, err := orch.CreateWorkstream(ctx, ws, opts)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify worktree was created
	worktrees := mockGit.GetWorktrees()
	expectedPath := "/tmp/ccells/worktrees/ccells/test-branch"
	if _, ok := worktrees[expectedPath]; !ok {
		t.Errorf("expected worktree at %s, got worktrees: %v", expectedPath, worktrees)
	}

	// Verify the workstream has the worktree path set
	if ws.WorktreePath != expectedPath {
		t.Errorf("expected ws.WorktreePath=%s, got %s", expectedPath, ws.WorktreePath)
	}
}

func TestCreateWorkstream_CreatesContainer(t *testing.T) {
	mockDocker := docker.NewMockClient()
	mockGit := git.NewMockGitClient()
	gitFactory := func(path string) git.GitClient {
		return mockGit
	}

	orch := New(mockDocker, gitFactory, "/test/repo")

	ws := &workstream.Workstream{
		ID:         "test-id",
		BranchName: "ccells/test-branch",
	}

	opts := CreateOptions{
		RepoPath:  "/test/repo",
		ImageName: "ccells-test:latest",
	}

	ctx := context.Background()
	containerID, err := orch.CreateWorkstream(ctx, ws, opts)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if containerID == "" {
		t.Error("expected container ID to be returned")
	}

	// Verify container was created and started
	state, err := mockDocker.GetContainerState(ctx, containerID)
	if err != nil {
		t.Fatalf("failed to get container state: %v", err)
	}
	if state != "running" {
		t.Errorf("expected container state 'running', got '%s'", state)
	}

	// Verify workstream has container ID set
	if ws.ContainerID != containerID {
		t.Errorf("expected ws.ContainerID=%s, got %s", containerID, ws.ContainerID)
	}
}

func TestPauseWorkstream(t *testing.T) {
	mockDocker := docker.NewMockClient()
	orch := New(mockDocker, nil, "/test/repo")

	// Create a "running" container in mock
	ctx := context.Background()
	cfg := &docker.ContainerConfig{Name: "test", Image: "test:latest"}
	containerID, _ := mockDocker.CreateContainer(ctx, cfg)
	_ = mockDocker.StartContainer(ctx, containerID)

	ws := &workstream.Workstream{
		ID:          "test-id",
		ContainerID: containerID,
	}

	err := orch.PauseWorkstream(ctx, ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	state, _ := mockDocker.GetContainerState(ctx, containerID)
	if state != "paused" {
		t.Errorf("expected state 'paused', got '%s'", state)
	}
}

func TestResumeWorkstream(t *testing.T) {
	mockDocker := docker.NewMockClient()
	orch := New(mockDocker, nil, "/test/repo")

	ctx := context.Background()
	cfg := &docker.ContainerConfig{Name: "test", Image: "test:latest"}
	containerID, _ := mockDocker.CreateContainer(ctx, cfg)
	_ = mockDocker.StartContainer(ctx, containerID)
	_ = mockDocker.PauseContainer(ctx, containerID)

	ws := &workstream.Workstream{
		ID:          "test-id",
		ContainerID: containerID,
	}

	err := orch.ResumeWorkstream(ctx, ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	state, _ := mockDocker.GetContainerState(ctx, containerID)
	if state != "running" {
		t.Errorf("expected state 'running', got '%s'", state)
	}
}

func TestDestroyWorkstream(t *testing.T) {
	mockDocker := docker.NewMockClient()
	mockGit := git.NewMockGitClient()
	gitFactory := func(path string) git.GitClient {
		return mockGit
	}
	orch := New(mockDocker, gitFactory, "/test/repo")

	ctx := context.Background()
	cfg := &docker.ContainerConfig{Name: "test", Image: "test:latest"}
	containerID, _ := mockDocker.CreateContainer(ctx, cfg)

	ws := &workstream.Workstream{
		ID:           "test-id",
		ContainerID:  containerID,
		BranchName:   "ccells/test",
		WorktreePath: "/tmp/ccells/worktrees/ccells/test",
	}

	err := orch.DestroyWorkstream(ctx, ws, DestroyOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Container should be removed
	_, err = mockDocker.GetContainerState(ctx, containerID)
	if err == nil {
		t.Error("expected container to be removed")
	}
}

func TestRebuildWorkstream(t *testing.T) {
	mockDocker := docker.NewMockClient()
	mockGit := git.NewMockGitClient()
	gitFactory := func(path string) git.GitClient {
		return mockGit
	}
	orch := New(mockDocker, gitFactory, "/test/repo")

	ctx := context.Background()
	cfg := &docker.ContainerConfig{Name: "test", Image: "test:latest"}
	oldContainerID, _ := mockDocker.CreateContainer(ctx, cfg)
	_ = mockDocker.StartContainer(ctx, oldContainerID)

	// Pre-create the worktree in the mock (simulating existing worktree)
	mockGit.AddWorktree("/tmp/ccells/worktrees/ccells/test", "ccells/test")

	ws := &workstream.Workstream{
		ID:           "test-id",
		ContainerID:  oldContainerID,
		BranchName:   "ccells/test",
		WorktreePath: "/tmp/ccells/worktrees/ccells/test",
	}

	opts := CreateOptions{
		RepoPath:  "/test/repo",
		ImageName: "ccells-test:latest",
	}

	newContainerID, err := orch.RebuildWorkstream(ctx, ws, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// New container should exist and be running
	if newContainerID == "" {
		t.Error("expected new container ID")
	}
	state, err := mockDocker.GetContainerState(ctx, newContainerID)
	if err != nil {
		t.Fatalf("failed to get container state: %v", err)
	}
	if state != "running" {
		t.Errorf("expected state 'running', got '%s'", state)
	}

	// Old container should be removed
	_, err = mockDocker.GetContainerState(ctx, oldContainerID)
	if err == nil {
		t.Error("expected old container to be removed")
	}

	// Workstream should have new container ID
	if ws.ContainerID != newContainerID {
		t.Errorf("expected ws.ContainerID=%s, got %s", newContainerID, ws.ContainerID)
	}
}
