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
