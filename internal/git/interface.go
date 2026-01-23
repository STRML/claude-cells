package git

import "context"

// GitClient defines the interface for git operations.
// This allows mocking git operations in tests without requiring a real git repository.
type GitClient interface {
	// Branch operations
	CurrentBranch(ctx context.Context) (string, error)
	CreateBranch(ctx context.Context, name string) error
	Checkout(ctx context.Context, branch string) error
	CreateAndCheckout(ctx context.Context, name string) error
	DeleteBranch(ctx context.Context, name string) error
	DeleteRemoteBranch(ctx context.Context, name string) error
	BranchExists(ctx context.Context, name string) (bool, error)
	BranchExistsRemote(ctx context.Context, name string) (bool, error)
	BranchHasCommits(ctx context.Context, branchName string) (bool, error)
	ListCCellsBranches(ctx context.Context) ([]string, error)
	GetBaseBranch(ctx context.Context) (string, error)
	GetBranchInfo(ctx context.Context, branchName string) (string, error)
	GetBranchCommitLogs(ctx context.Context, branchName string) (string, error)

	// Working directory operations
	HasUncommittedChanges(ctx context.Context) (bool, error)
	GetUntrackedFiles(ctx context.Context) ([]string, error)
	Stash(ctx context.Context) error
	StashPop(ctx context.Context) error

	// Remote operations
	Push(ctx context.Context, branch string) error
	ForcePush(ctx context.Context, branch string) error
	FetchMain(ctx context.Context) error
	PullMain(ctx context.Context) error
	UpdateMainBranch(ctx context.Context) error
	RemoteURL(ctx context.Context, remoteName string) (string, error)
	GetUnpushedCommitCount(ctx context.Context, branch string) (int, error)
	GetDivergedCommitCount(ctx context.Context, branch string) (int, error)
	FetchAndRebase(ctx context.Context) error

	// Merge/rebase operations
	MergeBranch(ctx context.Context, branch string) error
	MergeBranchWithOptions(ctx context.Context, branch string, squash bool) error
	RebaseBranch(ctx context.Context, branch string) error
	AbortRebase(ctx context.Context) error
	GetConflictFiles(ctx context.Context) ([]string, error)

	// Worktree operations
	CreateWorktree(ctx context.Context, worktreePath, branchName string) error
	CreateWorktreeFromExisting(ctx context.Context, worktreePath, branchName string) error
	RemoveWorktree(ctx context.Context, worktreePath string) error
	WorktreeList(ctx context.Context) ([]string, error)
	WorktreeExistsForBranch(ctx context.Context, branchName string) (string, bool)

	// Repository info
	RepoID(ctx context.Context) (string, error)
}

// Ensure Git implements GitClient
var _ GitClient = (*Git)(nil)
