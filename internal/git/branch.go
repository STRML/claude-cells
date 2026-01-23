package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/STRML/claude-cells/internal/claude"
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
	output := strings.TrimSpace(string(out))
	if err != nil && output != "" {
		// Include git's output in the error message for better diagnostics
		return output, fmt.Errorf("%s: %w", output, err)
	}
	return output, err
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

// HasUncommittedChanges returns true if there are uncommitted changes to tracked files.
// Untracked files are not considered uncommitted changes since they don't affect
// operations like merge or rebase (unless a file with the same name is being added).
func (g *Git) HasUncommittedChanges(ctx context.Context) (bool, error) {
	out, err := g.run(ctx, "status", "--porcelain", "-uno")
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

// ForcePush force pushes a branch to origin using --force-with-lease for safety.
// This is safer than --force as it will fail if the remote has commits we don't have.
func (g *Git) ForcePush(ctx context.Context, branch string) error {
	_, err := g.run(ctx, "push", "-u", "--force-with-lease", "origin", branch)
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

// DirtyWorktreeError represents uncommitted changes blocking an operation
type DirtyWorktreeError struct {
	Operation string
}

func (e *DirtyWorktreeError) Error() string {
	return fmt.Sprintf("cannot %s: worktree has uncommitted changes", e.Operation)
}

// branchNameToTitle converts a branch name to a readable commit title.
// Examples: "add-user-auth" -> "Add user auth", "fix_login_bug" -> "Fix login bug"
func branchNameToTitle(branch string) string {
	// Replace common separators with spaces
	title := strings.ReplaceAll(branch, "-", " ")
	title = strings.ReplaceAll(title, "_", " ")
	title = strings.ReplaceAll(title, "/", " ")

	// Trim and collapse multiple spaces
	title = strings.TrimSpace(title)
	parts := strings.Fields(title)
	if len(parts) == 0 {
		return branch
	}

	// Capitalize first word only
	parts[0] = strings.ToUpper(parts[0][:1]) + parts[0][1:]
	return strings.Join(parts, " ")
}

// generateConventionalTitle uses Claude CLI to generate a conventional commit title.
// Returns empty string if generation fails (caller should fall back to branchNameToTitle).
// See: https://www.conventionalcommits.org/en/v1.0.0/
func generateConventionalTitle(branch, commitLogs string) string {
	prompt := fmt.Sprintf(`Generate a conventional commit title for a squash merge. Follow https://www.conventionalcommits.org/en/v1.0.0/

Format: <type>: <description>
Types: feat, fix, docs, style, refactor, perf, test, build, ci, chore

Rules:
- Use lowercase for type and description
- No period at the end
- Keep under 72 characters
- Output ONLY the commit title, nothing else

Branch: %s

Commits being squashed:
%s`, branch, commitLogs)

	// Use ephemeral query to avoid polluting the resume log
	ctx := context.Background()
	title, err := claude.QueryWithTimeout(ctx, prompt, claude.DefaultTimeout)
	if err != nil {
		return ""
	}

	// Strip markdown code blocks if Claude wrapped the output
	title = stripMarkdownCodeBlock(title)

	// Basic validation - should start with a conventional type
	validPrefixes := []string{"feat:", "fix:", "docs:", "style:", "refactor:", "perf:", "test:", "build:", "ci:", "chore:"}
	isValid := false
	for _, prefix := range validPrefixes {
		if strings.HasPrefix(title, prefix) {
			isValid = true
			break
		}
	}
	if !isValid {
		return ""
	}

	// Limit length
	if len(title) > 72 {
		title = title[:72]
	}

	return title
}

// MergeBranch merges a branch into the current branch (typically main).
// It fetches origin/main and merges the branch into main.
// Returns MergeConflictError if there are conflicts that need resolution.
func (g *Git) MergeBranch(ctx context.Context, branch string) error {
	return g.MergeBranchWithOptions(ctx, branch, false)
}

// MergeBranchWithOptions merges a branch into main with optional squash.
// If squash is true, all commits are combined into a single commit.
// If a merge conflict occurs (common after a previous squash merge), this will
// automatically attempt to rebase the branch onto main and retry the merge.
// Returns MergeConflictError if there are conflicts that need manual resolution.
// Returns DirtyWorktreeError if there are uncommitted changes.
func (g *Git) MergeBranchWithOptions(ctx context.Context, branch string, squash bool) error {
	// Check for uncommitted changes first - fail early to avoid leaving worktree dirty
	hasChanges, err := g.HasUncommittedChanges(ctx)
	if err != nil {
		return fmt.Errorf("failed to check for uncommitted changes: %w", err)
	}
	if hasChanges {
		return &DirtyWorktreeError{Operation: "merge"}
	}

	return g.mergeBranchInternal(ctx, branch, squash, true)
}

// mergeBranchInternal is the internal merge implementation.
// If autoRebase is true and a merge conflict occurs, it will attempt to rebase
// the branch onto main and retry the merge once.
func (g *Git) mergeBranchInternal(ctx context.Context, branch string, squash bool, autoRebase bool) error {
	// Save the original branch so we can restore on failure
	originalBranch, err := g.CurrentBranch(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	// Determine the base branch (main or master)
	baseBranch, err := g.GetBaseBranch(ctx)
	if err != nil {
		baseBranch = "main" // Default to main
	}

	// restoreOriginalState returns to the original branch and cleans up any staged changes.
	// This is called on non-conflict failures to leave the worktree clean.
	restoreOriginalState := func() {
		// Reset any staged changes (from failed squash merge)
		_, _ = g.run(ctx, "reset", "--hard", "HEAD")
		// Return to original branch if we moved
		if originalBranch != baseBranch {
			_, _ = g.run(ctx, "checkout", originalBranch)
		}
	}

	// Fetch latest base branch from origin to ensure we're up to date
	_, _ = g.run(ctx, "fetch", "origin", baseBranch)

	// Checkout base branch
	if _, err := g.run(ctx, "checkout", baseBranch); err != nil {
		return fmt.Errorf("failed to checkout %s: %w", baseBranch, err)
	}

	// Fast-forward local base branch to match origin
	_, _ = g.run(ctx, "merge", "origin/"+baseBranch, "--ff-only")

	// Merge the branch
	if squash {
		// Get commit logs before squashing so we can preserve them
		commitLogs, _ := g.GetBranchCommitLogs(ctx, branch)

		// Squash merge: combines all commits into staged changes
		_, err := g.run(ctx, "merge", "--squash", branch)
		if err != nil {
			// Check if this is a conflict
			conflictFiles, conflictErr := g.GetConflictFiles(ctx)
			if conflictErr == nil && len(conflictFiles) > 0 {
				// Abort the failed merge to leave repo in clean state
				_, _ = g.run(ctx, "merge", "--abort")

				// If autoRebase is enabled, try rebasing and retrying
				if autoRebase {
					rebaseErr := g.rebaseAndRetryMerge(ctx, branch, squash)
					if rebaseErr != nil {
						return rebaseErr
					}
					return nil
				}
				return &MergeConflictError{Branch: branch, ConflictFiles: conflictFiles}
			}
			// Non-conflict error - restore original state
			restoreOriginalState()
			return err
		}

		// Build commit message with conventional commit title via Claude, with fallback
		title := generateConventionalTitle(branch, commitLogs)
		if title == "" {
			title = branchNameToTitle(branch)
		}
		commitMsg := title
		if commitLogs != "" {
			commitMsg = fmt.Sprintf("%s\n\nSquashed commits from branch '%s':\n\n%s", title, branch, commitLogs)
		}

		// Squash merge requires a separate commit
		_, err = g.run(ctx, "commit", "-m", commitMsg) //nolint:govet // err is shadowed but only used within this block
		if err != nil {
			// Commit failed - restore original state to clean up staged changes
			restoreOriginalState()
			return fmt.Errorf("failed to commit squash merge: %w", err)
		}
	} else {
		// Regular merge commit
		_, err := g.run(ctx, "merge", branch, "--no-edit")
		if err != nil {
			// Check if this is a conflict
			conflictFiles, conflictErr := g.GetConflictFiles(ctx)
			if conflictErr == nil && len(conflictFiles) > 0 {
				// Abort the failed merge to leave repo in clean state
				_, _ = g.run(ctx, "merge", "--abort")

				// If autoRebase is enabled, try rebasing and retrying
				if autoRebase {
					rebaseErr := g.rebaseAndRetryMerge(ctx, branch, squash)
					if rebaseErr != nil {
						return rebaseErr
					}
					return nil
				}
				return &MergeConflictError{Branch: branch, ConflictFiles: conflictFiles}
			}
			// Non-conflict error - restore original state
			restoreOriginalState()
			return err
		}
	}
	return nil
}

// rebaseAndRetryMerge attempts to rebase the branch onto the base branch and retry the merge.
// This handles the common case where a branch was previously squash-merged, then
// more work was done on it, causing conflicts on the next merge attempt.
// Returns nil on success, MergeConflictError if rebase has conflicts.
func (g *Git) rebaseAndRetryMerge(ctx context.Context, branch string, squash bool) error {
	// Determine the base branch
	baseBranch, err := g.GetBaseBranch(ctx)
	if err != nil {
		baseBranch = "main"
	}

	// Ensure working directory is clean before checkout
	// (merge --abort should have done this, but be safe)
	_, _ = g.run(ctx, "reset", "--hard", "HEAD")

	// Checkout the branch to rebase it
	if _, err := g.run(ctx, "checkout", branch); err != nil {
		return fmt.Errorf("failed to checkout branch for rebase: %w", err)
	}

	// Attempt to rebase onto origin/baseBranch, falling back to local if no remote
	_, err = g.run(ctx, "rebase", "origin/"+baseBranch)
	if err != nil && strings.Contains(err.Error(), "invalid upstream") {
		// No remote - try local base branch
		_, err = g.run(ctx, "rebase", baseBranch)
	}
	if err != nil {
		// Check if rebase has conflicts
		conflictFiles, conflictErr := g.GetConflictFiles(ctx)
		if conflictErr == nil && len(conflictFiles) > 0 {
			// Abort the rebase to leave repo in clean state
			_, _ = g.run(ctx, "rebase", "--abort")
			return &MergeConflictError{Branch: branch, ConflictFiles: conflictFiles}
		}
		// Abort any partial rebase
		_, _ = g.run(ctx, "rebase", "--abort")
		return fmt.Errorf("rebase failed: %w", err)
	}

	// Rebase succeeded - retry the merge (without auto-rebase to prevent infinite loop)
	return g.mergeBranchInternal(ctx, branch, squash, false)
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

// GetBranchCommitLogs returns all commit messages from a branch that are not in the base branch.
// The format includes the full commit message (subject and body) for each commit.
func (g *Git) GetBranchCommitLogs(ctx context.Context, branchName string) (string, error) {
	baseBranch, err := g.GetBaseBranch(ctx)
	if err != nil {
		baseBranch = "main"
	}

	// Get all commit messages with full body, separated by a delimiter
	// Format: hash (short) followed by subject and body
	out, err := g.run(ctx, "log", "--format=%h %s%n%n%b", baseBranch+".."+branchName)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(out), nil
}

// GetUntrackedFiles returns a list of untracked files in the repository.
// These are files that are not ignored and not added to the index.
func (g *Git) GetUntrackedFiles(ctx context.Context) ([]string, error) {
	// Use --others to list untracked files, --exclude-standard to respect .gitignore
	out, err := g.run(ctx, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var files []string
	for _, line := range lines {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
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
