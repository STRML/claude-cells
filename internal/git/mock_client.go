package git

import (
	"context"
	"fmt"
	"sync"
)

// MockGitClient is a mock implementation of GitClient for testing.
// It provides configurable function fields for custom behavior and
// tracks state in memory to simulate a real git repository.
type MockGitClient struct {
	mu sync.Mutex

	// Internal state
	currentBranch  string
	branches       map[string]bool
	worktrees      map[string]string // path -> branch
	hasChanges     bool
	untrackedFiles []string
	stashed        bool
	baseBranch     string
	repoID         string
	remoteURL      string

	// Configurable function fields - set these to override default behavior
	CurrentBranchFn              func(ctx context.Context) (string, error)
	CreateBranchFn               func(ctx context.Context, name string) error
	CheckoutFn                   func(ctx context.Context, branch string) error
	CreateAndCheckoutFn          func(ctx context.Context, name string) error
	DeleteBranchFn               func(ctx context.Context, name string) error
	DeleteRemoteBranchFn         func(ctx context.Context, name string) error
	BranchExistsFn               func(ctx context.Context, name string) (bool, error)
	BranchExistsRemoteFn         func(ctx context.Context, name string) (bool, error)
	BranchHasCommitsFn           func(ctx context.Context, branchName string) (bool, error)
	ListCCellsBranchesFn         func(ctx context.Context) ([]string, error)
	GetBaseBranchFn              func(ctx context.Context) (string, error)
	GetBranchInfoFn              func(ctx context.Context, branchName string) (string, error)
	GetBranchCommitLogsFn        func(ctx context.Context, branchName string) (string, error)
	HasUncommittedChangesFn      func(ctx context.Context) (bool, error)
	GetUntrackedFilesFn          func(ctx context.Context) ([]string, error)
	StashFn                      func(ctx context.Context) error
	StashPopFn                   func(ctx context.Context) error
	PushFn                       func(ctx context.Context, branch string) error
	ForcePushFn                  func(ctx context.Context, branch string) error
	FetchMainFn                  func(ctx context.Context) error
	PullMainFn                   func(ctx context.Context) error
	UpdateMainBranchFn           func(ctx context.Context) error
	RemoteURLFn                  func(ctx context.Context, remoteName string) (string, error)
	MergeBranchFn                func(ctx context.Context, branch string) error
	MergeBranchWithOptionsFn     func(ctx context.Context, branch string, squash bool) error
	RebaseBranchFn               func(ctx context.Context, branch string) error
	AbortRebaseFn                func(ctx context.Context) error
	GetConflictFilesFn           func(ctx context.Context) ([]string, error)
	CreateWorktreeFn             func(ctx context.Context, worktreePath, branchName string) error
	CreateWorktreeFromExistingFn func(ctx context.Context, worktreePath, branchName string) error
	RemoveWorktreeFn             func(ctx context.Context, worktreePath string) error
	WorktreeListFn               func(ctx context.Context) ([]string, error)
	WorktreeExistsForBranchFn    func(ctx context.Context, branchName string) (string, bool)
	RepoIDFn                     func(ctx context.Context) (string, error)

	// Error injection for testing error paths
	Err error // Set this to make all operations return this error
}

// NewMockGitClient creates a new mock git client for testing.
// The mock starts with "main" as the current and base branch.
func NewMockGitClient() *MockGitClient {
	return &MockGitClient{
		currentBranch: "main",
		branches:      map[string]bool{"main": true},
		worktrees:     make(map[string]string),
		baseBranch:    "main",
		repoID:        "mock-repo-id",
		remoteURL:     "https://github.com/mock/repo.git",
	}
}

// Ensure MockGitClient implements GitClient
var _ GitClient = (*MockGitClient)(nil)

// SetCurrentBranch sets the current branch for the mock.
func (m *MockGitClient) SetCurrentBranch(branch string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentBranch = branch
	m.branches[branch] = true
}

// SetHasUncommittedChanges sets whether the mock reports uncommitted changes.
func (m *MockGitClient) SetHasUncommittedChanges(hasChanges bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hasChanges = hasChanges
}

// SetUntrackedFiles sets the list of untracked files to return.
func (m *MockGitClient) SetUntrackedFiles(files []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.untrackedFiles = files
}

// SetBaseBranch sets the base branch (main/master).
func (m *MockGitClient) SetBaseBranch(branch string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.baseBranch = branch
}

// AddBranch adds a branch to the mock's known branches.
func (m *MockGitClient) AddBranch(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.branches[name] = true
}

// AddWorktree adds a worktree to the mock's known worktrees.
func (m *MockGitClient) AddWorktree(path, branch string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.worktrees[path] = branch
	m.branches[branch] = true
}

// GetBranches returns all known branches (for test assertions).
func (m *MockGitClient) GetBranches() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var branches []string
	for b := range m.branches {
		branches = append(branches, b)
	}
	return branches
}

// GetWorktrees returns all known worktrees (for test assertions).
func (m *MockGitClient) GetWorktrees() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make(map[string]string)
	for k, v := range m.worktrees {
		result[k] = v
	}
	return result
}

// Branch operations

func (m *MockGitClient) CurrentBranch(ctx context.Context) (string, error) {
	if m.Err != nil {
		return "", m.Err
	}
	if m.CurrentBranchFn != nil {
		return m.CurrentBranchFn(ctx)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.currentBranch, nil
}

func (m *MockGitClient) CreateBranch(ctx context.Context, name string) error {
	if m.Err != nil {
		return m.Err
	}
	if m.CreateBranchFn != nil {
		return m.CreateBranchFn(ctx, name)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.branches[name] {
		return fmt.Errorf("branch %s already exists", name)
	}
	m.branches[name] = true
	return nil
}

func (m *MockGitClient) Checkout(ctx context.Context, branch string) error {
	if m.Err != nil {
		return m.Err
	}
	if m.CheckoutFn != nil {
		return m.CheckoutFn(ctx, branch)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.branches[branch] {
		return fmt.Errorf("branch %s does not exist", branch)
	}
	m.currentBranch = branch
	return nil
}

func (m *MockGitClient) CreateAndCheckout(ctx context.Context, name string) error {
	if m.Err != nil {
		return m.Err
	}
	if m.CreateAndCheckoutFn != nil {
		return m.CreateAndCheckoutFn(ctx, name)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.branches[name] {
		return fmt.Errorf("branch %s already exists", name)
	}
	m.branches[name] = true
	m.currentBranch = name
	return nil
}

func (m *MockGitClient) DeleteBranch(ctx context.Context, name string) error {
	if m.Err != nil {
		return m.Err
	}
	if m.DeleteBranchFn != nil {
		return m.DeleteBranchFn(ctx, name)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.branches[name] {
		return fmt.Errorf("branch %s does not exist", name)
	}
	if m.currentBranch == name {
		return fmt.Errorf("cannot delete current branch %s", name)
	}
	delete(m.branches, name)
	return nil
}

func (m *MockGitClient) DeleteRemoteBranch(ctx context.Context, name string) error {
	if m.Err != nil {
		return m.Err
	}
	if m.DeleteRemoteBranchFn != nil {
		return m.DeleteRemoteBranchFn(ctx, name)
	}
	// Default: succeed silently (remote branches not tracked in mock)
	return nil
}

func (m *MockGitClient) BranchExists(ctx context.Context, name string) (bool, error) {
	if m.Err != nil {
		return false, m.Err
	}
	if m.BranchExistsFn != nil {
		return m.BranchExistsFn(ctx, name)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.branches[name], nil
}

func (m *MockGitClient) BranchExistsRemote(ctx context.Context, name string) (bool, error) {
	if m.Err != nil {
		return false, m.Err
	}
	if m.BranchExistsRemoteFn != nil {
		return m.BranchExistsRemoteFn(ctx, name)
	}
	// Default: remote branches mirror local
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.branches[name], nil
}

func (m *MockGitClient) BranchHasCommits(ctx context.Context, branchName string) (bool, error) {
	if m.Err != nil {
		return false, m.Err
	}
	if m.BranchHasCommitsFn != nil {
		return m.BranchHasCommitsFn(ctx, branchName)
	}
	// Default: no commits (safe for deletion)
	return false, nil
}

func (m *MockGitClient) ListCCellsBranches(ctx context.Context) ([]string, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.ListCCellsBranchesFn != nil {
		return m.ListCCellsBranchesFn(ctx)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []string
	for b := range m.branches {
		if len(b) > 7 && b[:7] == "ccells/" {
			result = append(result, b)
		}
	}
	return result, nil
}

func (m *MockGitClient) GetBaseBranch(ctx context.Context) (string, error) {
	if m.Err != nil {
		return "", m.Err
	}
	if m.GetBaseBranchFn != nil {
		return m.GetBaseBranchFn(ctx)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.baseBranch, nil
}

func (m *MockGitClient) GetBranchInfo(ctx context.Context, branchName string) (string, error) {
	if m.Err != nil {
		return "", m.Err
	}
	if m.GetBranchInfoFn != nil {
		return m.GetBranchInfoFn(ctx, branchName)
	}
	return fmt.Sprintf("Branch: %s\nNo commits ahead of %s", branchName, m.baseBranch), nil
}

func (m *MockGitClient) GetBranchCommitLogs(ctx context.Context, branchName string) (string, error) {
	if m.Err != nil {
		return "", m.Err
	}
	if m.GetBranchCommitLogsFn != nil {
		return m.GetBranchCommitLogsFn(ctx, branchName)
	}
	// Default: return empty (no commits)
	return "", nil
}

// Working directory operations

func (m *MockGitClient) HasUncommittedChanges(ctx context.Context) (bool, error) {
	if m.Err != nil {
		return false, m.Err
	}
	if m.HasUncommittedChangesFn != nil {
		return m.HasUncommittedChangesFn(ctx)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hasChanges, nil
}

func (m *MockGitClient) GetUntrackedFiles(ctx context.Context) ([]string, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.GetUntrackedFilesFn != nil {
		return m.GetUntrackedFilesFn(ctx)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.untrackedFiles, nil
}

func (m *MockGitClient) Stash(ctx context.Context) error {
	if m.Err != nil {
		return m.Err
	}
	if m.StashFn != nil {
		return m.StashFn(ctx)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.hasChanges {
		m.stashed = true
		m.hasChanges = false
	}
	return nil
}

func (m *MockGitClient) StashPop(ctx context.Context) error {
	if m.Err != nil {
		return m.Err
	}
	if m.StashPopFn != nil {
		return m.StashPopFn(ctx)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stashed {
		m.stashed = false
		m.hasChanges = true
	}
	return nil
}

// Remote operations

func (m *MockGitClient) Push(ctx context.Context, branch string) error {
	if m.Err != nil {
		return m.Err
	}
	if m.PushFn != nil {
		return m.PushFn(ctx, branch)
	}
	// Default: succeed
	return nil
}

func (m *MockGitClient) ForcePush(ctx context.Context, branch string) error {
	if m.Err != nil {
		return m.Err
	}
	if m.ForcePushFn != nil {
		return m.ForcePushFn(ctx, branch)
	}
	// Default: succeed
	return nil
}

func (m *MockGitClient) FetchMain(ctx context.Context) error {
	if m.Err != nil {
		return m.Err
	}
	if m.FetchMainFn != nil {
		return m.FetchMainFn(ctx)
	}
	return nil
}

func (m *MockGitClient) PullMain(ctx context.Context) error {
	if m.Err != nil {
		return m.Err
	}
	if m.PullMainFn != nil {
		return m.PullMainFn(ctx)
	}
	return nil
}

func (m *MockGitClient) UpdateMainBranch(ctx context.Context) error {
	if m.Err != nil {
		return m.Err
	}
	if m.UpdateMainBranchFn != nil {
		return m.UpdateMainBranchFn(ctx)
	}
	return nil
}

func (m *MockGitClient) RemoteURL(ctx context.Context, remoteName string) (string, error) {
	if m.Err != nil {
		return "", m.Err
	}
	if m.RemoteURLFn != nil {
		return m.RemoteURLFn(ctx, remoteName)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.remoteURL, nil
}

// Merge/rebase operations

func (m *MockGitClient) MergeBranch(ctx context.Context, branch string) error {
	if m.Err != nil {
		return m.Err
	}
	if m.MergeBranchFn != nil {
		return m.MergeBranchFn(ctx, branch)
	}
	return nil
}

func (m *MockGitClient) MergeBranchWithOptions(ctx context.Context, branch string, squash bool) error {
	if m.Err != nil {
		return m.Err
	}
	if m.MergeBranchWithOptionsFn != nil {
		return m.MergeBranchWithOptionsFn(ctx, branch, squash)
	}
	return nil
}

func (m *MockGitClient) RebaseBranch(ctx context.Context, branch string) error {
	if m.Err != nil {
		return m.Err
	}
	if m.RebaseBranchFn != nil {
		return m.RebaseBranchFn(ctx, branch)
	}
	return nil
}

func (m *MockGitClient) AbortRebase(ctx context.Context) error {
	if m.Err != nil {
		return m.Err
	}
	if m.AbortRebaseFn != nil {
		return m.AbortRebaseFn(ctx)
	}
	return nil
}

func (m *MockGitClient) GetConflictFiles(ctx context.Context) ([]string, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.GetConflictFilesFn != nil {
		return m.GetConflictFilesFn(ctx)
	}
	return nil, nil
}

// Worktree operations

func (m *MockGitClient) CreateWorktree(ctx context.Context, worktreePath, branchName string) error {
	if m.Err != nil {
		return m.Err
	}
	if m.CreateWorktreeFn != nil {
		return m.CreateWorktreeFn(ctx, worktreePath, branchName)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.branches[branchName] {
		return fmt.Errorf("branch %s already exists", branchName)
	}
	m.branches[branchName] = true
	m.worktrees[worktreePath] = branchName
	return nil
}

func (m *MockGitClient) CreateWorktreeFromExisting(ctx context.Context, worktreePath, branchName string) error {
	if m.Err != nil {
		return m.Err
	}
	if m.CreateWorktreeFromExistingFn != nil {
		return m.CreateWorktreeFromExistingFn(ctx, worktreePath, branchName)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.branches[branchName] {
		return fmt.Errorf("branch %s does not exist", branchName)
	}
	m.worktrees[worktreePath] = branchName
	return nil
}

func (m *MockGitClient) RemoveWorktree(ctx context.Context, worktreePath string) error {
	if m.Err != nil {
		return m.Err
	}
	if m.RemoveWorktreeFn != nil {
		return m.RemoveWorktreeFn(ctx, worktreePath)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.worktrees, worktreePath)
	return nil
}

func (m *MockGitClient) WorktreeList(ctx context.Context) ([]string, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.WorktreeListFn != nil {
		return m.WorktreeListFn(ctx)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var paths []string
	for path := range m.worktrees {
		paths = append(paths, path)
	}
	return paths, nil
}

func (m *MockGitClient) WorktreeExistsForBranch(ctx context.Context, branchName string) (string, bool) {
	if m.WorktreeExistsForBranchFn != nil {
		return m.WorktreeExistsForBranchFn(ctx, branchName)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for path, branch := range m.worktrees {
		if branch == branchName {
			return path, true
		}
	}
	return "", false
}

// Repository info

func (m *MockGitClient) RepoID(ctx context.Context) (string, error) {
	if m.Err != nil {
		return "", m.Err
	}
	if m.RepoIDFn != nil {
		return m.RepoIDFn(ctx)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.repoID, nil
}
