package orchestrator

import (
	"testing"

	"github.com/STRML/claude-cells/internal/docker"
	"github.com/STRML/claude-cells/internal/git"
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
