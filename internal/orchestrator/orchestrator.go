package orchestrator

import (
	"context"

	"github.com/STRML/claude-cells/internal/docker"
	"github.com/STRML/claude-cells/internal/git"
	"github.com/STRML/claude-cells/internal/workstream"
)

// WorkstreamOrchestrator manages workstream lifecycle operations.
// It coordinates between Docker, Git, and Workstream packages.
type WorkstreamOrchestrator interface {
	// CreateWorkstream creates a new workstream with container and worktree.
	// Returns the container ID on success.
	CreateWorkstream(ctx context.Context, ws *workstream.Workstream, opts CreateOptions) (string, error)

	// PauseWorkstream pauses a running workstream's container.
	PauseWorkstream(ctx context.Context, ws *workstream.Workstream) error

	// ResumeWorkstream resumes a paused workstream's container.
	ResumeWorkstream(ctx context.Context, ws *workstream.Workstream) error

	// DestroyWorkstream removes container, worktree, and cleans up state.
	DestroyWorkstream(ctx context.Context, ws *workstream.Workstream, opts DestroyOptions) error

	// RebuildWorkstream destroys and recreates the container.
	RebuildWorkstream(ctx context.Context, ws *workstream.Workstream, opts CreateOptions) (string, error)
}

// CreateOptions configures workstream creation.
type CreateOptions struct {
	RepoPath       string
	CopyUntracked  bool
	UntrackedFiles []string
	ImageName      string // Empty = auto-detect
	IsResume       bool   // Resuming existing session
}

// DestroyOptions configures workstream destruction.
type DestroyOptions struct {
	DeleteBranch bool
	KeepWorktree bool
	Force        bool // Force even if container is running
}

// Orchestrator implements WorkstreamOrchestrator.
type Orchestrator struct {
	dockerClient docker.DockerClient
	gitFactory   func(repoPath string) git.GitClient
	repoPath     string
}

// New creates a new Orchestrator.
func New(dockerClient docker.DockerClient, gitFactory func(string) git.GitClient, repoPath string) *Orchestrator {
	return &Orchestrator{
		dockerClient: dockerClient,
		gitFactory:   gitFactory,
		repoPath:     repoPath,
	}
}

// Verify Orchestrator implements WorkstreamOrchestrator at compile time
var _ WorkstreamOrchestrator = (*Orchestrator)(nil)
