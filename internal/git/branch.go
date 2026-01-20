package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Git provides git operations for a repository.
type Git struct {
	repoPath string
}

// New creates a Git instance for the given repository path.
func New(repoPath string) *Git {
	return &Git{repoPath: repoPath}
}

// run executes a git command and returns output.
func (g *Git) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.repoPath
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// CurrentBranch returns the current branch name.
func (g *Git) CurrentBranch(ctx context.Context) (string, error) {
	return g.run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
}

// CreateBranch creates a new branch at HEAD.
func (g *Git) CreateBranch(ctx context.Context, name string) error {
	_, err := g.run(ctx, "branch", name)
	return err
}

// Checkout switches to a branch.
func (g *Git) Checkout(ctx context.Context, branch string) error {
	_, err := g.run(ctx, "checkout", branch)
	return err
}

// CreateAndCheckout creates and switches to a new branch.
func (g *Git) CreateAndCheckout(ctx context.Context, name string) error {
	_, err := g.run(ctx, "checkout", "-b", name)
	return err
}

// HasUncommittedChanges returns true if there are uncommitted changes.
func (g *Git) HasUncommittedChanges(ctx context.Context) (bool, error) {
	out, err := g.run(ctx, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return len(out) > 0, nil
}

// Stash stashes uncommitted changes.
func (g *Git) Stash(ctx context.Context) error {
	_, err := g.run(ctx, "stash", "push", "-m", "ccells pairing mode")
	return err
}

// StashPop restores stashed changes.
func (g *Git) StashPop(ctx context.Context) error {
	_, err := g.run(ctx, "stash", "pop")
	return err
}

// DeleteBranch deletes a local branch.
func (g *Git) DeleteBranch(ctx context.Context, name string) error {
	_, err := g.run(ctx, "branch", "-D", name)
	return err
}

// DeleteRemoteBranch deletes a remote branch.
func (g *Git) DeleteRemoteBranch(ctx context.Context, name string) error {
	_, err := g.run(ctx, "push", "origin", "--delete", name)
	return err
}

// Push pushes a branch to origin.
func (g *Git) Push(ctx context.Context, branch string) error {
	_, err := g.run(ctx, "push", "-u", "origin", branch)
	return err
}

// MergeConflictError represents a merge conflict that needs resolution
type MergeConflictError struct {
	Branch        string
	ConflictFiles []string
}

func (e *MergeConflictError) Error() string {
	return fmt.Sprintf("merge conflict in branch %s: %d files with conflicts", e.Branch, len(e.ConflictFiles))
}

// MergeBranch merges a branch into the current branch (typically main).
// It fetches origin/main and merges the branch into main.
// Returns MergeConflictError if there are conflicts that need resolution.
func (g *Git) MergeBranch(ctx context.Context, branch string) error {
	return g.MergeBranchWithOptions(ctx, branch, false)
}

// MergeBranchWithOptions merges a branch into main with optional squash.
// If squash is true, all commits are combined into a single commit.
// Returns MergeConflictError if there are conflicts that need resolution.
func (g *Git) MergeBranchWithOptions(ctx context.Context, branch string, squash bool) error {
	// Fetch latest main from origin to ensure we're up to date
	_, _ = g.run(ctx, "fetch", "origin", "main")

	// Checkout main first
	if _, err := g.run(ctx, "checkout", "main"); err != nil {
		// Try master if main doesn't exist
		if _, err := g.run(ctx, "checkout", "master"); err != nil {
			return fmt.Errorf("failed to checkout main/master: %w", err)
		}
	}

	// Fast-forward local main to match origin/main
	_, _ = g.run(ctx, "merge", "origin/main", "--ff-only")

	// Merge the branch
	var err error
	if squash {
		// Squash merge: combines all commits into staged changes
		_, err = g.run(ctx, "merge", "--squash", branch)
		if err != nil {
			// Check if this is a conflict
			conflictFiles, conflictErr := g.GetConflictFiles(ctx)
			if conflictErr == nil && len(conflictFiles) > 0 {
				// Abort the failed merge to leave repo in clean state
				_, _ = g.run(ctx, "merge", "--abort")
				return &MergeConflictError{Branch: branch, ConflictFiles: conflictFiles}
			}
			return err
		}
		// Squash merge requires a separate commit
		_, err = g.run(ctx, "commit", "-m", fmt.Sprintf("Squash merge branch '%s'", branch))
		if err != nil {
			return fmt.Errorf("failed to commit squash merge: %w", err)
		}
	} else {
		// Regular merge commit
		_, err = g.run(ctx, "merge", branch, "--no-edit")
		if err != nil {
			// Check if this is a conflict
			conflictFiles, conflictErr := g.GetConflictFiles(ctx)
			if conflictErr == nil && len(conflictFiles) > 0 {
				// Abort the failed merge to leave repo in clean state
				_, _ = g.run(ctx, "merge", "--abort")
				return &MergeConflictError{Branch: branch, ConflictFiles: conflictFiles}
			}
			return err
		}
	}
	return nil
}

// GetConflictFiles returns list of files with merge conflicts
func (g *Git) GetConflictFiles(ctx context.Context) ([]string, error) {
	out, err := g.run(ctx, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	return lines, nil
}

// RebaseBranch rebases the specified branch onto main.
// This should be run from within the branch's worktree.
func (g *Git) RebaseBranch(ctx context.Context, branch string) error {
	// Make sure we're on the branch
	if _, err := g.run(ctx, "checkout", branch); err != nil {
		return fmt.Errorf("failed to checkout branch %s: %w", branch, err)
	}

	// Fetch latest main from origin
	_, _ = g.run(ctx, "fetch", "origin", "main")

	// Try to rebase onto origin/main
	_, err := g.run(ctx, "rebase", "origin/main")
	if err != nil {
		// Check if this is a conflict during rebase
		conflictFiles, conflictErr := g.GetConflictFiles(ctx)
		if conflictErr == nil && len(conflictFiles) > 0 {
			return &MergeConflictError{Branch: branch, ConflictFiles: conflictFiles}
		}
		return fmt.Errorf("rebase failed: %w", err)
	}
	return nil
}

// AbortRebase aborts an in-progress rebase
func (g *Git) AbortRebase(ctx context.Context) error {
	_, err := g.run(ctx, "rebase", "--abort")
	return err
}

// BranchExistsRemote checks if a branch exists on the remote.
func (g *Git) BranchExistsRemote(ctx context.Context, name string) (bool, error) {
	out, err := g.run(ctx, "ls-remote", "--heads", "origin", name)
	if err != nil {
		return false, nil
	}
	return len(out) > 0, nil
}

// BranchExists checks if a local branch exists.
func (g *Git) BranchExists(ctx context.Context, name string) (bool, error) {
	_, err := g.run(ctx, "rev-parse", "--verify", "refs/heads/"+name)
	if err != nil {
		// Branch doesn't exist
		return false, nil
	}
	return true, nil
}

// ListCCellsBranches returns all local branches matching the ccells pattern.
func (g *Git) ListCCellsBranches(ctx context.Context) ([]string, error) {
	out, err := g.run(ctx, "branch", "--list", "ccells/*")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}

	lines := strings.Split(out, "\n")
	var branches []string
	for _, line := range lines {
		// Remove leading * and whitespace
		branch := strings.TrimSpace(strings.TrimPrefix(line, "*"))
		if branch != "" {
			branches = append(branches, branch)
		}
	}
	return branches, nil
}

// GetBaseBranch returns the main/master branch name.
func (g *Git) GetBaseBranch(ctx context.Context) (string, error) {
	// Try common base branch names
	for _, name := range []string{"main", "master"} {
		exists, err := g.BranchExists(ctx, name)
		if err != nil {
			continue
		}
		if exists {
			return name, nil
		}
	}
	// Fall back to trying to get from remote HEAD
	out, err := g.run(ctx, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil && out != "" {
		// Output is like refs/remotes/origin/main
		parts := strings.Split(out, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1], nil
		}
	}
	return "main", nil // Default to main
}

// BranchHasCommits returns true if the branch has commits not in the base branch.
func (g *Git) BranchHasCommits(ctx context.Context, branchName string) (bool, error) {
	baseBranch, err := g.GetBaseBranch(ctx)
	if err != nil {
		return true, err // Assume has commits on error to be safe
	}

	// Count commits in branch that aren't in base
	out, err := g.run(ctx, "rev-list", "--count", baseBranch+".."+branchName)
	if err != nil {
		return true, err // Assume has commits on error to be safe
	}

	count := strings.TrimSpace(out)
	return count != "0", nil
}

// CreateWorktree creates a new worktree at the given path with a new branch.
// This allows each container to have its own working directory with its own branch
// without affecting the main repository's checkout.
func (g *Git) CreateWorktree(ctx context.Context, worktreePath, branchName string) error {
	_, err := g.run(ctx, "worktree", "add", "-b", branchName, worktreePath)
	return err
}

// CreateWorktreeFromExisting creates a new worktree from an existing branch.
func (g *Git) CreateWorktreeFromExisting(ctx context.Context, worktreePath, branchName string) error {
	_, err := g.run(ctx, "worktree", "add", worktreePath, branchName)
	return err
}

// RemoveWorktree removes a worktree and optionally its branch.
func (g *Git) RemoveWorktree(ctx context.Context, worktreePath string) error {
	// First prune any stale worktrees
	_, _ = g.run(ctx, "worktree", "prune")
	// Then remove the specific worktree
	_, err := g.run(ctx, "worktree", "remove", "--force", worktreePath)
	return err
}

// WorktreeList returns a list of worktree paths.
func (g *Git) WorktreeList(ctx context.Context) ([]string, error) {
	out, err := g.run(ctx, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			paths = append(paths, strings.TrimPrefix(line, "worktree "))
		}
	}
	return paths, nil
}

// WorktreeExistsForBranch checks if a worktree exists that is using the specified branch.
// Returns the worktree path if found, empty string otherwise.
func (g *Git) WorktreeExistsForBranch(ctx context.Context, branchName string) (string, bool) {
	out, err := g.run(ctx, "worktree", "list", "--porcelain")
	if err != nil {
		return "", false
	}

	// Parse porcelain output which looks like:
	// worktree /path/to/worktree
	// HEAD abc123
	// branch refs/heads/branch-name
	//
	// worktree /path/to/another
	// ...
	var currentPath string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			currentPath = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch refs/heads/") {
			branch := strings.TrimPrefix(line, "branch refs/heads/")
			if branch == branchName {
				return currentPath, true
			}
		}
	}
	return "", false
}

// FetchMain fetches the latest main/master branch from origin.
func (g *Git) FetchMain(ctx context.Context) error {
	baseBranch, err := g.GetBaseBranch(ctx)
	if err != nil {
		baseBranch = "main" // Default to main
	}
	_, err = g.run(ctx, "fetch", "origin", baseBranch)
	return err
}

// PullMain pulls the latest changes from origin into the local main/master branch.
// This should only be called when main is checked out and clean.
func (g *Git) PullMain(ctx context.Context) error {
	baseBranch, err := g.GetBaseBranch(ctx)
	if err != nil {
		baseBranch = "main"
	}

	// First fetch
	if _, err := g.run(ctx, "fetch", "origin", baseBranch); err != nil {
		return err
	}

	// Fast-forward merge (will fail if there are local changes or divergence)
	_, err = g.run(ctx, "merge", "--ff-only", "origin/"+baseBranch)
	return err
}

// UpdateMainBranch updates the local main branch to match origin without checking it out.
// Uses git fetch origin main:main to update the local ref directly.
func (g *Git) UpdateMainBranch(ctx context.Context) error {
	baseBranch, err := g.GetBaseBranch(ctx)
	if err != nil {
		baseBranch = "main"
	}

	// This updates local main to match origin/main without needing to checkout
	// Will fail if main has local commits not on origin (which is fine)
	_, err = g.run(ctx, "fetch", "origin", baseBranch+":"+baseBranch)
	return err
}

// GetBranchInfo returns a summary of commits and diff stats for a branch.
func (g *Git) GetBranchInfo(ctx context.Context, branchName string) (string, error) {
	baseBranch, err := g.GetBaseBranch(ctx)
	if err != nil {
		return "", err
	}

	var info strings.Builder

	// Get commit count
	countOut, err := g.run(ctx, "rev-list", "--count", baseBranch+".."+branchName)
	if err != nil {
		return "", err
	}
	commitCount := strings.TrimSpace(countOut)

	if commitCount == "0" {
		info.WriteString("No commits ahead of " + baseBranch)
		return info.String(), nil
	}

	// Get commit log (short format, max 5 commits)
	logOut, err := g.run(ctx, "log", "--oneline", "-5", baseBranch+".."+branchName)
	if err != nil {
		return "", err
	}

	info.WriteString("Commits (" + commitCount + "):\n")
	lines := strings.Split(logOut, "\n")
	for _, line := range lines {
		if line != "" {
			info.WriteString("  " + line + "\n")
		}
	}
	if len(lines) > 5 {
		info.WriteString("  ...\n")
	}

	// Get diff stats (files changed, insertions, deletions)
	diffOut, err := g.run(ctx, "diff", "--stat", "--stat-width=50", baseBranch+"..."+branchName)
	if err == nil && diffOut != "" {
		// Just get the summary line (last line)
		diffLines := strings.Split(diffOut, "\n")
		for i := len(diffLines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(diffLines[i])
			if line != "" && (strings.Contains(line, "changed") || strings.Contains(line, "insertion") || strings.Contains(line, "deletion")) {
				info.WriteString("\n" + line)
				break
			}
		}
	}

	return info.String(), nil
}

// RemoteURL returns the URL for a named remote (e.g., "origin").
func (g *Git) RemoteURL(ctx context.Context, remoteName string) (string, error) {
	return g.run(ctx, "remote", "get-url", remoteName)
}

// RepoID returns a stable identifier for the repository (the first commit hash).
// This ID is unique to the repository and doesn't change regardless of where it's cloned.
func (g *Git) RepoID(ctx context.Context) (string, error) {
	// Get the hash of the very first commit in the repository
	out, err := g.run(ctx, "rev-list", "--max-parents=0", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to get repo ID: %w", err)
	}
	// If there are multiple root commits (rare), take the first one
	lines := strings.Split(out, "\n")
	if len(lines) > 0 && lines[0] != "" {
		// Return first 12 characters for brevity (still unique enough)
		hash := lines[0]
		if len(hash) > 12 {
			hash = hash[:12]
		}
		return hash, nil
	}
	return "", fmt.Errorf("no commits found in repository")
}
