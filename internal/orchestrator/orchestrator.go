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
	// Returns CreateResult with container info on success.
	CreateWorkstream(ctx context.Context, ws *workstream.Workstream, opts CreateOptions) (*CreateResult, error)

	// PauseWorkstream pauses a running workstream's container.
	PauseWorkstream(ctx context.Context, ws *workstream.Workstream) error

	// ResumeWorkstream resumes a paused workstream's container.
	ResumeWorkstream(ctx context.Context, ws *workstream.Workstream) error

	// DestroyWorkstream removes container, worktree, and cleans up state.
	DestroyWorkstream(ctx context.Context, ws *workstream.Workstream, opts DestroyOptions) error

	// RebuildWorkstream destroys and recreates the container.
	RebuildWorkstream(ctx context.Context, ws *workstream.Workstream, opts CreateOptions) (*CreateResult, error)

	// CheckBranchConflict checks if a branch already exists and returns info about it.
	// Returns nil if no conflict.
	CheckBranchConflict(ctx context.Context, branchName string) (*BranchConflict, error)
}

// BranchConflict contains information about an existing branch conflict.
type BranchConflict struct {
	BranchName   string
	HasWorktree  bool
	WorktreePath string
	BranchInfo   string // Commit info for the branch
}

// CreateOptions configures workstream creation.
type CreateOptions struct {
	RepoPath          string
	CopyUntracked     bool
	UntrackedFiles    []string
	ImageName         string // Empty = auto-detect from devcontainer or default
	IsResume          bool   // Resuming existing session (use existing branch)
	UseExistingBranch bool   // Use existing branch without creating new one
	UpdateMain        bool   // Auto-pull main before creating branch
}

// CreateResult contains the result of workstream creation.
type CreateResult struct {
	ContainerID       string
	ContainerName     string
	ConfigDir         string // Container config directory for credential registration
	WorktreePath      string
	GitProxySocketDir string // Directory containing git.sock for git proxy
}

// DestroyOptions configures workstream destruction.
type DestroyOptions struct {
	DeleteBranch bool
	KeepWorktree bool
	Force        bool // Force even if container is running
}

// Orchestrator implements WorkstreamOrchestrator.
type Orchestrator struct {
	dockerClient    docker.DockerClient
	gitFactory      func(repoPath string) git.GitClient
	repoPath        string
	worktreeBaseDir string // Override for testing; empty uses DefaultWorktreeBaseDir
}

// New creates a new Orchestrator.
func New(dockerClient docker.DockerClient, gitFactory func(string) git.GitClient, repoPath string) *Orchestrator {
	return &Orchestrator{
		dockerClient: dockerClient,
		gitFactory:   gitFactory,
		repoPath:     repoPath,
	}
}

// SetWorktreeBaseDir sets a custom worktree base directory (for testing).
func (o *Orchestrator) SetWorktreeBaseDir(dir string) {
	o.worktreeBaseDir = dir
}

// getWorktreeBaseDir returns the worktree base directory to use.
func (o *Orchestrator) getWorktreeBaseDir() string {
	if o.worktreeBaseDir != "" {
		return o.worktreeBaseDir
	}
	return DefaultWorktreeBaseDir
}

// Verify Orchestrator implements WorkstreamOrchestrator at compile time
var _ WorkstreamOrchestrator = (*Orchestrator)(nil)
