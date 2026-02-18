package orchestrator

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/STRML/claude-cells/internal/docker"
	"github.com/STRML/claude-cells/internal/git"
	"github.com/STRML/claude-cells/internal/workstream"
)

// setupTestDirs creates temp directories for tests and returns a cleanup function.
// Sets both orchestrator worktree base dir and docker cells dir to temp locations.
func setupTestDirs(t *testing.T, orch *Orchestrator) func() {
	t.Helper()

	// Create temp directories
	tempDir := t.TempDir()
	worktreeDir := filepath.Join(tempDir, "worktrees")
	cellsDir := filepath.Join(tempDir, "claude-cells")

	// Set orchestrator to use temp worktree dir
	if orch != nil {
		orch.SetWorktreeBaseDir(worktreeDir)
	}

	// Set docker to use temp cells dir
	docker.SetTestCellsDir(cellsDir)

	// Return cleanup function
	return func() {
		docker.SetTestCellsDir("")
	}
}

func TestSanitizeBranchName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple name", "my-branch", "my-branch"},
		{"forward slash", "feature/foo", "feature-foo"},
		{"multiple slashes", "feature/sub/foo", "feature-sub-foo"},
		{"backslash", "feature\\foo", "feature-foo"},
		{"spaces", "my branch", "my-branch"},
		{"mixed", "feature/my branch", "feature-my-branch"},
		{"leading slash", "/foo", "foo"},
		{"trailing slash", "foo/", "foo"},
		{"only slashes", "///", "unnamed"},
		{"empty string", "", "unnamed"},
		{"ccells prefix", "ccells/test-branch", "ccells-test-branch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeBranchName(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeBranchName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

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
	cleanup := setupTestDirs(t, orch)
	defer cleanup()

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

	// Verify worktree was created (path uses sanitized branch name)
	worktrees := mockGit.GetWorktrees()
	expectedSuffix := "ccells-test-branch"
	found := false
	for path := range worktrees {
		if filepath.Base(path) == expectedSuffix {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected worktree ending with %s, got worktrees: %v", expectedSuffix, worktrees)
	}

	// Verify the workstream has the worktree path set
	if filepath.Base(ws.WorktreePath) != expectedSuffix {
		t.Errorf("expected ws.WorktreePath to end with %s, got %s", expectedSuffix, ws.WorktreePath)
	}
}

func TestCreateWorkstream_CreatesContainer(t *testing.T) {
	mockDocker := docker.NewMockClient()
	mockGit := git.NewMockGitClient()
	gitFactory := func(path string) git.GitClient {
		return mockGit
	}

	orch := New(mockDocker, gitFactory, "/test/repo")
	cleanup := setupTestDirs(t, orch)
	defer cleanup()

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
	cleanup := setupTestDirs(t, orch)
	defer cleanup()

	ctx := context.Background()
	cfg := &docker.ContainerConfig{Name: "test", Image: "test:latest"}
	containerID, _ := mockDocker.CreateContainer(ctx, cfg)

	ws := &workstream.Workstream{
		ID:           "test-id",
		ContainerID:  containerID,
		BranchName:   "ccells/test",
		WorktreePath: filepath.Join(orch.getWorktreeBaseDir(), "ccells-test"),
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
	cleanup := setupTestDirs(t, orch)
	defer cleanup()

	ctx := context.Background()
	cfg := &docker.ContainerConfig{Name: "test", Image: "test:latest"}
	oldContainerID, _ := mockDocker.CreateContainer(ctx, cfg)
	_ = mockDocker.StartContainer(ctx, oldContainerID)

	// Pre-create the worktree in the mock (simulating existing worktree with sanitized path)
	worktreePath := filepath.Join(orch.getWorktreeBaseDir(), "ccells-test")
	mockGit.AddWorktree(worktreePath, "ccells/test")

	ws := &workstream.Workstream{
		ID:           "test-id",
		ContainerID:  oldContainerID,
		BranchName:   "ccells/test",
		WorktreePath: worktreePath,
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
	// Use a temp dir path for the test
	tempDir := t.TempDir()
	worktreePath := filepath.Join(tempDir, "worktrees", "ccells-worktree-branch")
	// Note: The worktree path is returned by git, not constructed by orchestrator
	// So the mock stores the actual path that git would return
	mockGit.AddWorktree(worktreePath, "ccells/worktree-branch")
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
	if conflict.WorktreePath != worktreePath {
		t.Errorf("expected WorktreePath %s, got %s", worktreePath, conflict.WorktreePath)
	}
}

// Error path tests

func TestPauseWorkstream_EmptyContainerID(t *testing.T) {
	mockDocker := docker.NewMockClient()
	orch := New(mockDocker, nil, "/test/repo")

	ws := &workstream.Workstream{
		ID:          "test-id",
		ContainerID: "", // empty
	}

	ctx := context.Background()
	err := orch.PauseWorkstream(ctx, ws)

	if err == nil {
		t.Error("expected error for empty container ID")
	}
}

func TestResumeWorkstream_EmptyContainerID(t *testing.T) {
	mockDocker := docker.NewMockClient()
	orch := New(mockDocker, nil, "/test/repo")

	ws := &workstream.Workstream{
		ID:          "test-id",
		ContainerID: "", // empty
	}

	ctx := context.Background()
	err := orch.ResumeWorkstream(ctx, ws)

	if err == nil {
		t.Error("expected error for empty container ID")
	}
}

func TestCreateWorkstream_WorktreeCreationError(t *testing.T) {
	mockDocker := docker.NewMockClient()
	mockGit := git.NewMockGitClient()

	// Configure git mock to fail on worktree creation
	mockGit.CreateWorktreeFn = func(ctx context.Context, worktreePath, branchName string) error {
		return context.DeadlineExceeded // simulate timeout
	}

	gitFactory := func(path string) git.GitClient {
		return mockGit
	}

	orch := New(mockDocker, gitFactory, "/test/repo")
	cleanup := setupTestDirs(t, orch)
	defer cleanup()

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

	if err == nil {
		t.Error("expected error when worktree creation fails")
	}
	if result != nil {
		t.Error("expected nil result on error")
	}
	// Workstream fields should not be mutated on failure
	if ws.WorktreePath != "" {
		t.Errorf("expected ws.WorktreePath to remain empty, got %s", ws.WorktreePath)
	}
	if ws.ContainerID != "" {
		t.Errorf("expected ws.ContainerID to remain empty, got %s", ws.ContainerID)
	}
}

func TestCreateWorkstream_ContainerCreationError(t *testing.T) {
	mockDocker := docker.NewMockClient()
	mockGit := git.NewMockGitClient()

	// Configure docker mock to fail on container creation
	mockDocker.CreateContainerFn = func(ctx context.Context, cfg *docker.ContainerConfig) (string, error) {
		return "", context.DeadlineExceeded // simulate error
	}

	gitFactory := func(path string) git.GitClient {
		return mockGit
	}

	orch := New(mockDocker, gitFactory, "/test/repo")
	cleanup := setupTestDirs(t, orch)
	defer cleanup()

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

	if err == nil {
		t.Error("expected error when container creation fails")
	}
	if result != nil {
		t.Error("expected nil result on error")
	}
	// Container ID should not be set on failure
	if ws.ContainerID != "" {
		t.Errorf("expected ws.ContainerID to remain empty, got %s", ws.ContainerID)
	}
}

func TestCheckBranchConflict_BranchExistsError(t *testing.T) {
	mockGit := git.NewMockGitClient()

	// Configure git mock to fail on BranchExists
	mockGit.BranchExistsFn = func(ctx context.Context, name string) (bool, error) {
		return false, context.DeadlineExceeded
	}

	gitFactory := func(path string) git.GitClient {
		return mockGit
	}
	orch := New(nil, gitFactory, "/test/repo")

	ctx := context.Background()
	conflict, err := orch.CheckBranchConflict(ctx, "ccells/new-branch")

	if err == nil {
		t.Error("expected error when BranchExists fails")
	}
	if conflict != nil {
		t.Error("expected nil conflict on error")
	}
}

func TestDestroyWorkstream_RemoveWorktreeError(t *testing.T) {
	mockDocker := docker.NewMockClient()
	mockGit := git.NewMockGitClient()

	// Configure git mock to fail on worktree removal
	mockGit.RemoveWorktreeFn = func(ctx context.Context, worktreePath string) error {
		return context.DeadlineExceeded
	}

	gitFactory := func(path string) git.GitClient {
		return mockGit
	}
	orch := New(mockDocker, gitFactory, "/test/repo")
	cleanup := setupTestDirs(t, orch)
	defer cleanup()

	ctx := context.Background()
	cfg := &docker.ContainerConfig{Name: "test", Image: "test:latest"}
	containerID, _ := mockDocker.CreateContainer(ctx, cfg)

	ws := &workstream.Workstream{
		ID:           "test-id",
		ContainerID:  containerID,
		BranchName:   "ccells/test",
		WorktreePath: filepath.Join(orch.getWorktreeBaseDir(), "ccells-test"),
	}

	err := orch.DestroyWorkstream(ctx, ws, DestroyOptions{})

	if err == nil {
		t.Error("expected error when worktree removal fails")
	}
	// Container should still be removed despite worktree error
	_, containerErr := mockDocker.GetContainerState(ctx, containerID)
	if containerErr == nil {
		t.Error("expected container to be removed even when worktree removal fails")
	}
}

func TestRebuildWorkstream_DestroyError(t *testing.T) {
	mockDocker := docker.NewMockClient()
	mockGit := git.NewMockGitClient()

	gitFactory := func(path string) git.GitClient {
		return mockGit
	}
	orch := New(mockDocker, gitFactory, "/test/repo")
	cleanup := setupTestDirs(t, orch)
	defer cleanup()

	ctx := context.Background()

	// Create workstream with non-existent container (will cause stop error)
	ws := &workstream.Workstream{
		ID:           "test-id",
		ContainerID:  "non-existent-container",
		BranchName:   "ccells/test",
		WorktreePath: filepath.Join(orch.getWorktreeBaseDir(), "ccells-test"),
	}

	opts := CreateOptions{
		RepoPath:  "/test/repo",
		ImageName: "ccells-test:latest",
	}

	result, err := orch.RebuildWorkstream(ctx, ws, opts)

	// The destroy step may have errors but rebuild continues
	// The real test is whether it handles errors gracefully
	if result == nil && err == nil {
		t.Error("expected either result or error")
	}
}
