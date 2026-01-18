package git

import (
	"context"
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
