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
	result, err := orch.CreateWorkstream(ctx, ws, opts)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.ContainerID == "" {
		t.Error("expected container ID to be returned")
	}

	// Verify container was created and started
	state, err := mockDocker.GetContainerState(ctx, result.ContainerID)
	if err != nil {
		t.Fatalf("failed to get container state: %v", err)
	}
	if state != "running" {
		t.Errorf("expected container state 'running', got '%s'", state)
	}

	// Verify workstream has container ID set
	if ws.ContainerID != result.ContainerID {
		t.Errorf("expected ws.ContainerID=%s, got %s", result.ContainerID, ws.ContainerID)
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

	result, err := orch.RebuildWorkstream(ctx, ws, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// New container should exist and be running
	if result == nil || result.ContainerID == "" {
		t.Error("expected new container ID")
	}
	state, err := mockDocker.GetContainerState(ctx, result.ContainerID)
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
	if ws.ContainerID != result.ContainerID {
		t.Errorf("expected ws.ContainerID=%s, got %s", result.ContainerID, ws.ContainerID)
	}
}

func TestCheckBranchConflict_NoConflict(t *testing.T) {
	mockGit := git.NewMockGitClient()
	gitFactory := func(path string) git.GitClient {
		return mockGit
	}
	orch := New(nil, gitFactory, "/test/repo")

	ctx := context.Background()
	conflict, err := orch.CheckBranchConflict(ctx, "ccells/new-branch")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conflict != nil {
		t.Errorf("expected no conflict, got %+v", conflict)
	}
}

func TestCheckBranchConflict_BranchExists(t *testing.T) {
	mockGit := git.NewMockGitClient()
	mockGit.AddBranch("ccells/existing-branch")
	gitFactory := func(path string) git.GitClient {
		return mockGit
	}
	orch := New(nil, gitFactory, "/test/repo")

	ctx := context.Background()
	conflict, err := orch.CheckBranchConflict(ctx, "ccells/existing-branch")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conflict == nil {
		t.Fatal("expected conflict, got nil")
	}
	if conflict.HasWorktree {
		t.Error("expected no worktree")
	}
	if conflict.BranchName != "ccells/existing-branch" {
		t.Errorf("expected BranchName=ccells/existing-branch, got %s", conflict.BranchName)
	}
}

func TestCheckBranchConflict_WorktreeExists(t *testing.T) {
	mockGit := git.NewMockGitClient()
	mockGit.AddWorktree("/tmp/ccells/worktrees/ccells/worktree-branch", "ccells/worktree-branch")
	gitFactory := func(path string) git.GitClient {
		return mockGit
	}
	orch := New(nil, gitFactory, "/test/repo")

	ctx := context.Background()
	conflict, err := orch.CheckBranchConflict(ctx, "ccells/worktree-branch")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conflict == nil {
		t.Fatal("expected conflict, got nil")
	}
	if !conflict.HasWorktree {
		t.Error("expected worktree")
	}
	if conflict.WorktreePath != "/tmp/ccells/worktrees/ccells/worktree-branch" {
		t.Errorf("expected WorktreePath, got %s", conflict.WorktreePath)
	}
}
