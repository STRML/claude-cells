package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Initialize git repo
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "initial"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup failed: %v: %s", err, out)
		}
	}
	return dir
}

func TestGit_CurrentBranch(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	branch, err := g.CurrentBranch(ctx)
	if err != nil {
		t.Fatalf("CurrentBranch() error = %v", err)
	}
	// Git default branch could be main or master
	if branch != "main" && branch != "master" {
		t.Errorf("CurrentBranch() = %q, want main or master", branch)
	}
}

func TestGit_CreateBranch(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	err := g.CreateBranch(ctx, "test-branch")
	if err != nil {
		t.Fatalf("CreateBranch() error = %v", err)
	}

	// Verify branch exists
	cmd := exec.Command("git", "branch", "--list", "test-branch")
	cmd.Dir = dir
	out, _ := cmd.Output()
	if len(out) == 0 {
		t.Error("Branch was not created")
	}
}

func TestGit_Checkout(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Create and checkout
	_ = g.CreateBranch(ctx, "feature")
	err := g.Checkout(ctx, "feature")
	if err != nil {
		t.Fatalf("Checkout() error = %v", err)
	}

	branch, _ := g.CurrentBranch(ctx)
	if branch != "feature" {
		t.Errorf("CurrentBranch() = %q after checkout, want 'feature'", branch)
	}
}

func TestGit_HasUncommittedChanges(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Initially clean
	hasChanges, err := g.HasUncommittedChanges(ctx)
	if err != nil {
		t.Fatalf("HasUncommittedChanges() error = %v", err)
	}
	if hasChanges {
		t.Error("HasUncommittedChanges() = true, want false for clean repo")
	}

	// Create uncommitted file
	_ = os.WriteFile(filepath.Join(dir, "test.txt"), []byte("test"), 0644)

	hasChanges, err = g.HasUncommittedChanges(ctx)
	if err != nil {
		t.Fatalf("HasUncommittedChanges() error = %v", err)
	}
	if !hasChanges {
		t.Error("HasUncommittedChanges() = false, want true after adding file")
	}
}

func TestGit_Stash(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Create a tracked file first
	_ = os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("tracked"), 0644)
	exec.Command("git", "-C", dir, "add", "tracked.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "add tracked").Run()

	// Modify tracked file
	_ = os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("modified"), 0644)

	err := g.Stash(ctx)
	if err != nil {
		t.Fatalf("Stash() error = %v", err)
	}

	hasChanges, _ := g.HasUncommittedChanges(ctx)
	if hasChanges {
		t.Error("HasUncommittedChanges() = true after Stash(), want false")
	}
}

func TestGit_DeleteBranch(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Create branch and switch back
	_ = g.CreateBranch(ctx, "to-delete")

	// Stay on main
	err := g.DeleteBranch(ctx, "to-delete")
	if err != nil {
		t.Fatalf("DeleteBranch() error = %v", err)
	}

	// Verify deleted
	cmd := exec.Command("git", "branch", "--list", "to-delete")
	cmd.Dir = dir
	out, _ := cmd.Output()
	if len(out) > 0 {
		t.Error("Branch was not deleted")
	}
}

// resolvePath resolves symlinks to handle macOS /var -> /private/var symlink
func resolvePath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return resolved
}

func TestGit_CreateWorktree(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Create a worktree directory
	worktreePath := filepath.Join(os.TempDir(), "git-worktree-test-"+filepath.Base(dir))
	defer os.RemoveAll(worktreePath)

	// Create worktree with new branch
	err := g.CreateWorktree(ctx, worktreePath, "feature-branch")
	if err != nil {
		t.Fatalf("CreateWorktree() error = %v", err)
	}

	// Verify worktree directory exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Error("Worktree directory was not created")
	}

	// Verify .git file exists in worktree (not a directory for worktrees)
	gitPath := filepath.Join(worktreePath, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		t.Fatalf("Worktree .git not found: %v", err)
	}
	if info.IsDir() {
		t.Error("Worktree .git should be a file, not a directory")
	}

	// Verify branch was created
	exists, err := g.BranchExists(ctx, "feature-branch")
	if err != nil {
		t.Fatalf("BranchExists() error = %v", err)
	}
	if !exists {
		t.Error("Branch was not created with worktree")
	}

	// Verify worktree is listed
	worktrees, err := g.WorktreeList(ctx)
	if err != nil {
		t.Fatalf("WorktreeList() error = %v", err)
	}
	found := false
	resolvedWorktreePath := resolvePath(worktreePath)
	for _, wt := range worktrees {
		if wt == resolvedWorktreePath {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("WorktreeList() does not contain %s, got %v", resolvedWorktreePath, worktrees)
	}
}

func TestGit_CreateWorktreeFromExisting(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// First create a branch
	err := g.CreateBranch(ctx, "existing-branch")
	if err != nil {
		t.Fatalf("CreateBranch() error = %v", err)
	}

	// Create a worktree directory
	worktreePath := filepath.Join(os.TempDir(), "git-worktree-existing-test-"+filepath.Base(dir))
	defer os.RemoveAll(worktreePath)

	// Create worktree from existing branch
	err = g.CreateWorktreeFromExisting(ctx, worktreePath, "existing-branch")
	if err != nil {
		t.Fatalf("CreateWorktreeFromExisting() error = %v", err)
	}

	// Verify worktree directory exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Error("Worktree directory was not created")
	}
}

func TestGit_RemoveWorktree(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Create a worktree
	worktreePath := filepath.Join(os.TempDir(), "git-worktree-remove-test-"+filepath.Base(dir))
	defer os.RemoveAll(worktreePath) // cleanup in case test fails

	err := g.CreateWorktree(ctx, worktreePath, "remove-branch")
	if err != nil {
		t.Fatalf("CreateWorktree() error = %v", err)
	}

	// Verify it exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Fatal("Worktree was not created")
	}

	// Remove the worktree
	err = g.RemoveWorktree(ctx, worktreePath)
	if err != nil {
		t.Fatalf("RemoveWorktree() error = %v", err)
	}

	// Verify worktree is no longer listed
	worktrees, err := g.WorktreeList(ctx)
	if err != nil {
		t.Fatalf("WorktreeList() error = %v", err)
	}
	for _, wt := range worktrees {
		if wt == worktreePath {
			t.Error("Worktree still appears in list after removal")
		}
	}

	// Note: git worktree remove does NOT delete the directory itself
	// The directory may still exist - this is expected git behavior
	// The cleanup code in StopContainerCmd calls os.RemoveAll separately
}

func TestGit_RemoveWorktree_AndDirectory(t *testing.T) {
	// This test mimics the cleanup behavior in StopContainerCmd
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Create a worktree
	worktreePath := filepath.Join(os.TempDir(), "git-worktree-full-cleanup-"+filepath.Base(dir))
	defer os.RemoveAll(worktreePath) // cleanup in case test fails

	err := g.CreateWorktree(ctx, worktreePath, "cleanup-branch")
	if err != nil {
		t.Fatalf("CreateWorktree() error = %v", err)
	}

	// Verify it exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Fatal("Worktree was not created")
	}

	// This is what StopContainerCmd does:
	// 1. Remove worktree from git
	err = g.RemoveWorktree(ctx, worktreePath)
	if err != nil {
		t.Fatalf("RemoveWorktree() error = %v", err)
	}

	// 2. Remove the directory
	err = os.RemoveAll(worktreePath)
	if err != nil {
		t.Fatalf("os.RemoveAll() error = %v", err)
	}

	// Verify directory is gone
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Error("Worktree directory still exists after os.RemoveAll")
	}

	// Verify worktree is no longer in git's list
	worktrees, err := g.WorktreeList(ctx)
	if err != nil {
		t.Fatalf("WorktreeList() error = %v", err)
	}
	for _, wt := range worktrees {
		if wt == worktreePath {
			t.Error("Worktree still in git list after removal")
		}
	}
}

func TestGit_WorktreeList(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Initial list should contain at least the main worktree
	worktrees, err := g.WorktreeList(ctx)
	if err != nil {
		t.Fatalf("WorktreeList() error = %v", err)
	}
	if len(worktrees) == 0 {
		t.Error("WorktreeList() should return at least the main worktree")
	}

	// The main repo should be in the list (resolve symlinks for macOS)
	resolvedDir := resolvePath(dir)
	found := false
	for _, wt := range worktrees {
		if wt == resolvedDir {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("WorktreeList() should contain main repo %s, got %v", resolvedDir, worktrees)
	}

	// Create additional worktrees
	wt1 := filepath.Join(os.TempDir(), "git-worktree-list-1-"+filepath.Base(dir))
	wt2 := filepath.Join(os.TempDir(), "git-worktree-list-2-"+filepath.Base(dir))
	defer os.RemoveAll(wt1)
	defer os.RemoveAll(wt2)

	_ = g.CreateWorktree(ctx, wt1, "list-branch-1")
	_ = g.CreateWorktree(ctx, wt2, "list-branch-2")

	// Now list should have 3 worktrees
	worktrees, err = g.WorktreeList(ctx)
	if err != nil {
		t.Fatalf("WorktreeList() error = %v", err)
	}
	if len(worktrees) != 3 {
		t.Errorf("WorktreeList() = %d worktrees, want 3", len(worktrees))
	}
}

func TestGit_WorktreeExistsForBranch(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Initially, no worktree for "test-branch"
	path, exists := g.WorktreeExistsForBranch(ctx, "test-branch")
	if exists {
		t.Errorf("WorktreeExistsForBranch() found worktree at %s for non-existent branch", path)
	}

	// Create a worktree for test-branch
	worktreePath := filepath.Join(os.TempDir(), "worktree-exists-test-"+filepath.Base(dir))
	defer os.RemoveAll(worktreePath)

	err := g.CreateWorktree(ctx, worktreePath, "test-branch")
	if err != nil {
		t.Fatalf("CreateWorktree() error = %v", err)
	}

	// Now the worktree should exist
	path, exists = g.WorktreeExistsForBranch(ctx, "test-branch")
	if !exists {
		t.Error("WorktreeExistsForBranch() did not find worktree for test-branch")
	}
	resolvedPath := resolvePath(worktreePath)
	if path != resolvedPath {
		t.Errorf("WorktreeExistsForBranch() path = %s, want %s", path, resolvedPath)
	}

	// A different branch should not exist
	_, exists = g.WorktreeExistsForBranch(ctx, "other-branch")
	if exists {
		t.Error("WorktreeExistsForBranch() found worktree for non-existent other-branch")
	}

	// Clean up
	_ = g.RemoveWorktree(ctx, worktreePath)
}

func TestGit_RemoveWorktree_SymlinkPath(t *testing.T) {
	// This test verifies that worktree removal works when the path
	// contains symlinks (e.g., /tmp -> /private/tmp on macOS)
	// This is the exact scenario that happens in production.
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Use /tmp which is symlinked to /private/tmp on macOS
	// This mimics the actual path used in production: /tmp/ccells/worktrees/<branch>
	worktreePath := "/tmp/ccells-test-worktree-" + filepath.Base(dir)
	defer os.RemoveAll(worktreePath)

	// Create worktree using the /tmp path (not resolved)
	err := g.CreateWorktree(ctx, worktreePath, "symlink-test-branch")
	if err != nil {
		t.Fatalf("CreateWorktree() error = %v", err)
	}

	// Verify it exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Fatal("Worktree was not created")
	}

	// Now try to remove using the same /tmp path
	// This is what StopContainerCmd does
	err = g.RemoveWorktree(ctx, worktreePath)
	if err != nil {
		t.Fatalf("RemoveWorktree() error = %v", err)
	}

	// Verify worktree is no longer in git's list
	worktrees, err := g.WorktreeList(ctx)
	if err != nil {
		t.Fatalf("WorktreeList() error = %v", err)
	}

	// Check both resolved and unresolved paths
	resolvedPath := resolvePath(worktreePath)
	for _, wt := range worktrees {
		if wt == worktreePath || wt == resolvedPath {
			t.Errorf("Worktree still in git list after removal: %s", wt)
		}
	}

	// Clean up directory
	_ = os.RemoveAll(worktreePath)
}

// getWorktreePathForTest mimics the getWorktreePath function from tui/container.go
func getWorktreePathForTest(branchName string) string {
	safeName := strings.ReplaceAll(branchName, "/", "-")
	safeName = strings.ReplaceAll(safeName, " ", "-")
	return fmt.Sprintf("/tmp/ccells/worktrees/%s", safeName)
}

func TestGit_CleanupFlow_MimicsStopContainerCmd(t *testing.T) {
	// This test mimics the exact cleanup flow in StopContainerCmd
	// to verify the worktree gets properly cleaned up
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	branchName := "ccells/test-cleanup-branch"

	// Ensure worktrees directory exists (as done in startContainerWithOptions)
	if err := os.MkdirAll("/tmp/ccells/worktrees", 0755); err != nil {
		t.Fatalf("Failed to create worktrees directory: %v", err)
	}

	// Compute worktree path the same way the TUI does
	worktreePath := getWorktreePathForTest(branchName)
	defer os.RemoveAll(worktreePath)

	t.Logf("Creating worktree at: %s", worktreePath)

	// Create worktree (as done in startContainerWithOptions)
	err := g.CreateWorktree(ctx, worktreePath, branchName)
	if err != nil {
		t.Fatalf("CreateWorktree() error = %v", err)
	}

	// Verify worktree exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Fatal("Worktree was not created")
	}

	// Verify branch exists
	exists, _ := g.BranchExists(ctx, branchName)
	if !exists {
		t.Fatal("Branch was not created")
	}

	t.Logf("Worktree and branch created successfully")

	// Now simulate StopContainerCmd cleanup:
	// 1. Compute path from branch name (as if WorktreePath was empty)
	cleanupPath := getWorktreePathForTest(branchName)
	t.Logf("Computed cleanup path: %s", cleanupPath)

	// 2. Remove worktree from git
	err = g.RemoveWorktree(ctx, cleanupPath)
	if err != nil {
		t.Logf("RemoveWorktree() error (ignored in prod): %v", err)
	}

	// 3. Remove the directory
	err = os.RemoveAll(cleanupPath)
	if err != nil {
		t.Logf("os.RemoveAll() error (ignored in prod): %v", err)
	}

	// 4. Check if branch has commits (it shouldn't)
	hasCommits, _ := g.BranchHasCommits(ctx, branchName)
	if !hasCommits {
		// 5. Delete the branch
		err = g.DeleteBranch(ctx, branchName)
		if err != nil {
			t.Logf("DeleteBranch() error: %v", err)
		}
	}

	// VERIFY CLEANUP WORKED:

	// Worktree directory should be gone
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Errorf("Worktree directory still exists at %s", worktreePath)
	}

	// Worktree should not be in git's list
	worktrees, err := g.WorktreeList(ctx)
	if err != nil {
		t.Fatalf("WorktreeList() error = %v", err)
	}
	resolvedPath := resolvePath(worktreePath)
	for _, wt := range worktrees {
		if wt == worktreePath || wt == resolvedPath {
			t.Errorf("Worktree still in git list: %s", wt)
		}
	}

	// Branch should be deleted (since it had no commits)
	exists, _ = g.BranchExists(ctx, branchName)
	if exists {
		t.Errorf("Branch %s still exists after cleanup", branchName)
	}

	t.Log("Cleanup completed successfully")
}

func TestGit_BranchHasCommits_NoCommits(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Create a new branch without any commits
	err := g.CreateBranch(ctx, "empty-branch")
	if err != nil {
		t.Fatalf("CreateBranch() error = %v", err)
	}

	// Should have no commits ahead of main
	hasCommits, err := g.BranchHasCommits(ctx, "empty-branch")
	if err != nil {
		t.Fatalf("BranchHasCommits() error = %v", err)
	}
	if hasCommits {
		t.Error("BranchHasCommits() = true for new branch with no commits")
	}
}

func TestGit_BranchHasCommits_WithCommits(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Create and checkout a new branch
	err := g.CreateAndCheckout(ctx, "branch-with-commits")
	if err != nil {
		t.Fatalf("CreateAndCheckout() error = %v", err)
	}

	// Make a commit
	_ = os.WriteFile(filepath.Join(dir, "newfile.txt"), []byte("content"), 0644)
	exec.Command("git", "-C", dir, "add", "newfile.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "add file").Run()

	// Should have commits ahead of main
	hasCommits, err := g.BranchHasCommits(ctx, "branch-with-commits")
	if err != nil {
		t.Fatalf("BranchHasCommits() error = %v", err)
	}
	if !hasCommits {
		t.Error("BranchHasCommits() = false for branch with commits")
	}
}

func TestGit_GetBranchCommitLogs(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Create and checkout a new branch
	err := g.CreateAndCheckout(ctx, "feature-branch")
	if err != nil {
		t.Fatalf("CreateAndCheckout() error = %v", err)
	}

	// Make multiple commits
	_ = os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content1"), 0644)
	exec.Command("git", "-C", dir, "add", "file1.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "Add first feature").Run()

	_ = os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("content2"), 0644)
	exec.Command("git", "-C", dir, "add", "file2.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "Add second feature").Run()

	// Get commit logs
	logs, err := g.GetBranchCommitLogs(ctx, "feature-branch")
	if err != nil {
		t.Fatalf("GetBranchCommitLogs() error = %v", err)
	}

	// Should contain both commit messages
	if !strings.Contains(logs, "Add first feature") {
		t.Errorf("GetBranchCommitLogs() missing 'Add first feature', got: %s", logs)
	}
	if !strings.Contains(logs, "Add second feature") {
		t.Errorf("GetBranchCommitLogs() missing 'Add second feature', got: %s", logs)
	}
}

func TestGit_GetBranchCommitLogs_Empty(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Create a new branch without any commits
	err := g.CreateBranch(ctx, "empty-branch")
	if err != nil {
		t.Fatalf("CreateBranch() error = %v", err)
	}

	// Get commit logs - should be empty
	logs, err := g.GetBranchCommitLogs(ctx, "empty-branch")
	if err != nil {
		t.Fatalf("GetBranchCommitLogs() error = %v", err)
	}

	if logs != "" {
		t.Errorf("GetBranchCommitLogs() = %q, want empty string for branch with no commits", logs)
	}
}

func TestGit_MergeBranchWithOptions_SquashPreservesCommitLogs(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Get the base branch name (main or master)
	baseBranch, _ := g.CurrentBranch(ctx)

	// Create and checkout a feature branch
	err := g.CreateAndCheckout(ctx, "feature-squash")
	if err != nil {
		t.Fatalf("CreateAndCheckout() error = %v", err)
	}

	// Make multiple commits with descriptive messages
	_ = os.WriteFile(filepath.Join(dir, "feature1.txt"), []byte("feature 1"), 0644)
	exec.Command("git", "-C", dir, "add", "feature1.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "Implement feature one").Run()

	_ = os.WriteFile(filepath.Join(dir, "feature2.txt"), []byte("feature 2"), 0644)
	exec.Command("git", "-C", dir, "add", "feature2.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "Implement feature two").Run()

	_ = os.WriteFile(filepath.Join(dir, "bugfix.txt"), []byte("bugfix"), 0644)
	exec.Command("git", "-C", dir, "add", "bugfix.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "Fix bug in feature").Run()

	// Checkout main and squash merge
	err = g.Checkout(ctx, baseBranch)
	if err != nil {
		t.Fatalf("Checkout() error = %v", err)
	}

	err = g.MergeBranchWithOptions(ctx, "feature-squash", true)
	if err != nil {
		t.Fatalf("MergeBranchWithOptions() error = %v", err)
	}

	// Get the last commit message
	cmd := exec.Command("git", "log", "-1", "--format=%B")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log error = %v", err)
	}
	commitMsg := string(out)

	// Verify the squash commit message contains all the original commit messages
	if !strings.Contains(commitMsg, "Squash merge branch 'feature-squash'") {
		t.Errorf("Squash commit missing header, got: %s", commitMsg)
	}
	if !strings.Contains(commitMsg, "Implement feature one") {
		t.Errorf("Squash commit missing 'Implement feature one', got: %s", commitMsg)
	}
	if !strings.Contains(commitMsg, "Implement feature two") {
		t.Errorf("Squash commit missing 'Implement feature two', got: %s", commitMsg)
	}
	if !strings.Contains(commitMsg, "Fix bug in feature") {
		t.Errorf("Squash commit missing 'Fix bug in feature', got: %s", commitMsg)
	}
	if !strings.Contains(commitMsg, "Squashed commits:") {
		t.Errorf("Squash commit missing 'Squashed commits:' section, got: %s", commitMsg)
	}
}
