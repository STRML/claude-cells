package git

import (
	"context"
	"errors"
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

	// Untracked files should NOT be considered uncommitted changes
	// (they don't affect merge/rebase operations)
	_ = os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("untracked"), 0644)

	hasChanges, err = g.HasUncommittedChanges(ctx)
	if err != nil {
		t.Fatalf("HasUncommittedChanges() error = %v", err)
	}
	if hasChanges {
		t.Error("HasUncommittedChanges() = true, want false for untracked file")
	}

	// Create and commit a tracked file
	_ = os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("original"), 0644)
	cmd := exec.Command("git", "add", "tracked.txt")
	cmd.Dir = dir
	_, _ = cmd.CombinedOutput()
	cmd = exec.Command("git", "commit", "-m", "add tracked file")
	cmd.Dir = dir
	_, _ = cmd.CombinedOutput()

	// Modified tracked file SHOULD be considered uncommitted changes
	_ = os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("modified content"), 0644)

	hasChanges, err = g.HasUncommittedChanges(ctx)
	if err != nil {
		t.Fatalf("HasUncommittedChanges() error = %v", err)
	}
	if !hasChanges {
		t.Error("HasUncommittedChanges() = false, want true for modified tracked file")
	}
}

func TestGit_GetUntrackedFiles(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Initially no untracked files
	files, err := g.GetUntrackedFiles(ctx)
	if err != nil {
		t.Fatalf("GetUntrackedFiles() error = %v", err)
	}
	if len(files) != 0 {
		t.Errorf("GetUntrackedFiles() = %v, want empty for clean repo", files)
	}

	// Create untracked files
	_ = os.WriteFile(filepath.Join(dir, "untracked1.txt"), []byte("test1"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "untracked2.txt"), []byte("test2"), 0644)

	files, err = g.GetUntrackedFiles(ctx)
	if err != nil {
		t.Fatalf("GetUntrackedFiles() error = %v", err)
	}
	if len(files) != 2 {
		t.Errorf("GetUntrackedFiles() returned %d files, want 2", len(files))
	}

	// Verify the files are listed
	fileMap := make(map[string]bool)
	for _, f := range files {
		fileMap[f] = true
	}
	if !fileMap["untracked1.txt"] || !fileMap["untracked2.txt"] {
		t.Errorf("GetUntrackedFiles() = %v, missing expected files", files)
	}

	// Create a nested untracked file
	_ = os.MkdirAll(filepath.Join(dir, "subdir"), 0755)
	_ = os.WriteFile(filepath.Join(dir, "subdir", "nested.txt"), []byte("nested"), 0644)

	files, err = g.GetUntrackedFiles(ctx)
	if err != nil {
		t.Fatalf("GetUntrackedFiles() error = %v", err)
	}
	if len(files) != 3 {
		t.Errorf("GetUntrackedFiles() returned %d files after nested add, want 3", len(files))
	}

	// Verify nested file is included
	foundNested := false
	for _, f := range files {
		if strings.Contains(f, "subdir") && strings.Contains(f, "nested.txt") {
			foundNested = true
			break
		}
	}
	if !foundNested {
		t.Errorf("GetUntrackedFiles() = %v, missing nested file", files)
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

	// Use t.TempDir() for test isolation
	tempDir := t.TempDir()
	worktreePath := filepath.Join(tempDir, "symlink-test-worktree")

	// Create worktree
	err := g.CreateWorktree(ctx, worktreePath, "symlink-test-branch")
	if err != nil {
		t.Fatalf("CreateWorktree() error = %v", err)
	}

	// Verify it exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Fatal("Worktree was not created")
	}

	// Now try to remove
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
}

// getWorktreePathForTest mimics the getWorktreePath function from tui/container.go
func getWorktreePathForTest(baseDir, branchName string) string {
	safeName := strings.ReplaceAll(branchName, "/", "-")
	safeName = strings.ReplaceAll(safeName, " ", "-")
	return filepath.Join(baseDir, safeName)
}

func TestGit_CleanupFlow_MimicsStopContainerCmd(t *testing.T) {
	// This test mimics the exact cleanup flow in StopContainerCmd
	// to verify the worktree gets properly cleaned up
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	branchName := "ccells/test-cleanup-branch"

	// Use t.TempDir() for test isolation
	tempDir := t.TempDir()
	worktreesDir := filepath.Join(tempDir, "worktrees")
	if err := os.MkdirAll(worktreesDir, 0755); err != nil {
		t.Fatalf("Failed to create worktrees directory: %v", err)
	}

	// Compute worktree path the same way the TUI does
	worktreePath := getWorktreePathForTest(worktreesDir, branchName)

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
	cleanupPath := getWorktreePathForTest(worktreesDir, branchName)
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

func TestBranchNameToTitle(t *testing.T) {
	tests := []struct {
		name     string
		branch   string
		expected string
	}{
		{"hyphen separated", "add-user-auth", "Add user auth"},
		{"underscore separated", "fix_login_bug", "Fix login bug"},
		{"slash separated", "feature/new-feature", "Feature new feature"},
		{"mixed separators", "fix/user-auth_bug", "Fix user auth bug"},
		{"single word", "bugfix", "Bugfix"},
		{"already capitalized", "Add-Feature", "Add Feature"},
		{"multiple hyphens", "add--multiple---hyphens", "Add multiple hyphens"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := branchNameToTitle(tt.branch)
			if result != tt.expected {
				t.Errorf("branchNameToTitle(%q) = %q, want %q", tt.branch, result, tt.expected)
			}
		})
	}
}

func TestGenerateConventionalTitle_Fallback(t *testing.T) {
	// When Claude CLI is not available, generateConventionalTitle should return empty string
	// This tests the fallback behavior - branchNameToTitle will be used instead
	result := generateConventionalTitle("test-branch", "some commit logs")
	// Result will be empty if Claude CLI is not available (expected in test environment)
	// or a valid conventional commit title if it is available
	if result != "" {
		// If we got a result, verify it's a valid conventional commit format
		validPrefixes := []string{"feat:", "fix:", "docs:", "style:", "refactor:", "perf:", "test:", "build:", "ci:", "chore:"}
		isValid := false
		for _, prefix := range validPrefixes {
			if strings.HasPrefix(result, prefix) {
				isValid = true
				break
			}
		}
		if !isValid {
			t.Errorf("generateConventionalTitle() returned invalid format: %q", result)
		}
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

	// Verify the squash commit message has a descriptive title (either conventional commit or fallback)
	// and contains all the original commit messages
	firstLine := strings.Split(commitMsg, "\n")[0]
	hasConventionalTitle := strings.HasPrefix(firstLine, "feat:") ||
		strings.HasPrefix(firstLine, "fix:") ||
		strings.HasPrefix(firstLine, "chore:")
	hasFallbackTitle := strings.HasPrefix(firstLine, "Feature squash")
	if !hasConventionalTitle && !hasFallbackTitle {
		t.Errorf("Squash commit should start with conventional commit title or fallback 'Feature squash', got: %s", firstLine)
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
	if !strings.Contains(commitMsg, "Squashed commits from branch 'feature-squash':") {
		t.Errorf("Squash commit missing 'Squashed commits from branch' section, got: %s", commitMsg)
	}
}

func TestGit_MergeBranchWithOptions_DirtyWorktreeError(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Get the base branch name
	baseBranch, _ := g.CurrentBranch(ctx)

	// Create a feature branch with a commit
	err := g.CreateAndCheckout(ctx, "feature-dirty-test")
	if err != nil {
		t.Fatalf("CreateAndCheckout() error = %v", err)
	}

	_ = os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("feature"), 0644)
	exec.Command("git", "-C", dir, "add", "feature.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "Add feature").Run()

	// Go back to base branch
	err = g.Checkout(ctx, baseBranch)
	if err != nil {
		t.Fatalf("Checkout() error = %v", err)
	}

	// Create a tracked file with uncommitted modifications (dirty worktree)
	// First commit a file, then modify it without committing
	_ = os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("original"), 0644)
	exec.Command("git", "-C", dir, "add", "dirty.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "Add dirty file").Run()
	_ = os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("modified uncommitted"), 0644)

	// Try to merge - should fail with DirtyWorktreeError
	err = g.MergeBranchWithOptions(ctx, "feature-dirty-test", false)

	if err == nil {
		t.Fatal("MergeBranchWithOptions() should fail with dirty worktree")
	}

	var dirtyErr *DirtyWorktreeError
	if !errors.As(err, &dirtyErr) {
		t.Errorf("Expected DirtyWorktreeError, got: %T (%v)", err, err)
	}

	// Also test squash merge
	err = g.MergeBranchWithOptions(ctx, "feature-dirty-test", true)
	if err == nil {
		t.Fatal("MergeBranchWithOptions(squash) should fail with dirty worktree")
	}
	if !errors.As(err, &dirtyErr) {
		t.Errorf("Expected DirtyWorktreeError for squash, got: %T (%v)", err, err)
	}
}

func TestGit_MergeBranchWithOptions_UntrackedFilesAllowed(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Get the base branch name
	baseBranch, _ := g.CurrentBranch(ctx)

	// Create a feature branch with a commit
	err := g.CreateAndCheckout(ctx, "feature-untracked-test")
	if err != nil {
		t.Fatalf("CreateAndCheckout() error = %v", err)
	}

	_ = os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("feature"), 0644)
	exec.Command("git", "-C", dir, "add", "feature.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "Add feature").Run()

	// Go back to base branch
	err = g.Checkout(ctx, baseBranch)
	if err != nil {
		t.Fatalf("Checkout() error = %v", err)
	}

	// Create an untracked file - this should NOT block the merge
	_ = os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("untracked content"), 0644)

	// Squash merge should succeed even with untracked files
	err = g.MergeBranchWithOptions(ctx, "feature-untracked-test", true)
	if err != nil {
		t.Fatalf("MergeBranchWithOptions() should succeed with untracked files, got error: %v", err)
	}

	// Verify the merge succeeded
	if _, err := os.Stat(filepath.Join(dir, "feature.txt")); os.IsNotExist(err) {
		t.Error("feature.txt not found on main after merge")
	}

	// Verify untracked file is still there
	if _, err := os.Stat(filepath.Join(dir, "untracked.txt")); os.IsNotExist(err) {
		t.Error("untracked.txt should still exist after merge")
	}
}

func TestGit_MergeBranchWithOptions_RestoresStateOnFailure(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Get the base branch name
	baseBranch, _ := g.CurrentBranch(ctx)

	// Create a feature branch
	err := g.CreateAndCheckout(ctx, "feature-restore-test")
	if err != nil {
		t.Fatalf("CreateAndCheckout() error = %v", err)
	}

	// Add a file that will conflict
	_ = os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("feature content"), 0644)
	exec.Command("git", "-C", dir, "add", "conflict.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "Add conflict file on feature").Run()

	// Go back to base and create a conflicting change
	err = g.Checkout(ctx, baseBranch)
	if err != nil {
		t.Fatalf("Checkout() error = %v", err)
	}

	// Create a DIFFERENT feature branch (not the one we'll merge) and check it out
	// This tests that we return to the original branch on failure
	err = g.CreateAndCheckout(ctx, "working-branch")
	if err != nil {
		t.Fatalf("CreateAndCheckout(working-branch) error = %v", err)
	}

	// Add a commit so working-branch exists
	_ = os.WriteFile(filepath.Join(dir, "working.txt"), []byte("working"), 0644)
	exec.Command("git", "-C", dir, "add", "working.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "Working branch commit").Run()

	// Now attempt to merge feature-restore-test (which doesn't conflict)
	// But use a non-existent branch to force a failure
	err = g.MergeBranchWithOptions(ctx, "nonexistent-branch", false)
	if err == nil {
		t.Fatal("MergeBranchWithOptions() should fail for nonexistent branch")
	}

	// Verify we're back on the working branch (not stuck on main)
	currentBranch, err := g.CurrentBranch(ctx)
	if err != nil {
		t.Fatalf("CurrentBranch() error = %v", err)
	}
	if currentBranch != "working-branch" {
		t.Errorf("Expected to be on 'working-branch' after failed merge, got: %s", currentBranch)
	}

	// Verify worktree is clean (no staged changes)
	hasChanges, err := g.HasUncommittedChanges(ctx)
	if err != nil {
		t.Fatalf("HasUncommittedChanges() error = %v", err)
	}
	if hasChanges {
		t.Error("Worktree should be clean after failed merge")
	}
}

func TestGit_MergeBranchWithOptions_AutoRebaseAfterSquashMerge(t *testing.T) {
	// This test verifies the auto-rebase functionality when:
	// 1. A branch is squash-merged to main
	// 2. More work is done on the branch
	// 3. A second merge is attempted (which would normally conflict)
	// 4. The auto-rebase kicks in and handles it automatically
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Get the base branch name (main or master)
	baseBranch, _ := g.CurrentBranch(ctx)

	// Create and checkout a feature branch
	err := g.CreateAndCheckout(ctx, "feature-auto-rebase")
	if err != nil {
		t.Fatalf("CreateAndCheckout() error = %v", err)
	}

	// Make initial commits on the feature branch
	_ = os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("initial feature"), 0644)
	exec.Command("git", "-C", dir, "add", "feature.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "Add initial feature").Run()

	// First squash merge to main
	err = g.Checkout(ctx, baseBranch)
	if err != nil {
		t.Fatalf("Checkout() error = %v", err)
	}

	err = g.MergeBranchWithOptions(ctx, "feature-auto-rebase", true)
	if err != nil {
		t.Fatalf("First MergeBranchWithOptions() error = %v", err)
	}

	// Verify first merge succeeded - check that the feature file exists on main
	// (commit message format varies based on whether Claude CLI generates conventional commit titles)
	if _, err := os.Stat(filepath.Join(dir, "feature.txt")); os.IsNotExist(err) {
		t.Error("feature.txt not found on main after first merge")
	}

	// Continue working on the feature branch
	err = g.Checkout(ctx, "feature-auto-rebase")
	if err != nil {
		t.Fatalf("Checkout() error = %v", err)
	}

	// Add more commits (this creates the "diverged history" scenario)
	_ = os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("updated feature"), 0644)
	exec.Command("git", "-C", dir, "add", "feature.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "Update feature").Run()

	_ = os.WriteFile(filepath.Join(dir, "feature2.txt"), []byte("second feature"), 0644)
	exec.Command("git", "-C", dir, "add", "feature2.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "Add second feature").Run()

	// Second squash merge - this would normally fail with conflicts
	// because the branch still has the original commits that conflict
	// with the squashed commit on main. Auto-rebase should handle this.
	err = g.Checkout(ctx, baseBranch)
	if err != nil {
		t.Fatalf("Checkout() error = %v", err)
	}

	err = g.MergeBranchWithOptions(ctx, "feature-auto-rebase", true)
	if err != nil {
		t.Fatalf("Second MergeBranchWithOptions() should auto-rebase and succeed, got error = %v", err)
	}

	// Verify second merge succeeded - the commit body should contain the squashed commit logs
	cmd := exec.Command("git", "log", "-1", "--format=%B")
	cmd.Dir = dir
	out, _ := cmd.Output()
	commitMsg := string(out)

	// The second merge should contain only the new commits (after rebase)
	// Commit message body should reference the branch and include commit summaries
	if !strings.Contains(commitMsg, "feature-auto-rebase") {
		t.Errorf("Second squash merge should reference branch name, got: %s", commitMsg)
	}
	if !strings.Contains(commitMsg, "Update feature") {
		t.Errorf("Second squash merge missing 'Update feature', got: %s", commitMsg)
	}
	if !strings.Contains(commitMsg, "Add second feature") {
		t.Errorf("Second squash merge missing 'Add second feature', got: %s", commitMsg)
	}

	// Verify both files exist on main
	if _, err := os.Stat(filepath.Join(dir, "feature.txt")); os.IsNotExist(err) {
		t.Error("feature.txt not found on main after merge")
	}
	if _, err := os.Stat(filepath.Join(dir, "feature2.txt")); os.IsNotExist(err) {
		t.Error("feature2.txt not found on main after merge")
	}

	// Verify content is updated
	content, _ := os.ReadFile(filepath.Join(dir, "feature.txt"))
	if string(content) != "updated feature" {
		t.Errorf("feature.txt content = %q, want 'updated feature'", string(content))
	}
}

func TestGit_MergeBranchWithOptions_AutoRebaseWhenMainMoved(t *testing.T) {
	// This test verifies auto-rebase works when main has moved ahead
	// but there are no actual content conflicts (just history divergence).
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Get the base branch name (main or master)
	baseBranch, _ := g.CurrentBranch(ctx)

	// Create a feature branch
	err := g.CreateAndCheckout(ctx, "feature-main-moved")
	if err != nil {
		t.Fatalf("CreateAndCheckout() error = %v", err)
	}

	// Make a commit on the feature branch (touches file1.txt)
	_ = os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("feature content"), 0644)
	exec.Command("git", "-C", dir, "add", "file1.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "Add feature file").Run()

	// Go back to main and make a commit (touches different file - file2.txt)
	err = g.Checkout(ctx, baseBranch)
	if err != nil {
		t.Fatalf("Checkout() error = %v", err)
	}

	_ = os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("main content"), 0644)
	exec.Command("git", "-C", dir, "add", "file2.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "Add main file").Run()

	// Try to merge - should auto-rebase and succeed (no content conflicts)
	err = g.MergeBranchWithOptions(ctx, "feature-main-moved", true)
	if err != nil {
		t.Fatalf("MergeBranchWithOptions() should auto-rebase and succeed, got error = %v", err)
	}

	// Verify both files exist on main
	if _, err := os.Stat(filepath.Join(dir, "file1.txt")); os.IsNotExist(err) {
		t.Error("file1.txt (from feature) not found on main after merge")
	}
	if _, err := os.Stat(filepath.Join(dir, "file2.txt")); os.IsNotExist(err) {
		t.Error("file2.txt (from main) not found on main after merge")
	}

	// Verify merge succeeded - commit message format varies based on Claude CLI availability
	// Just verify files and content are correct (done above)
}

func TestGit_MergeBranchWithOptions_AutoRebaseWithRealConflicts(t *testing.T) {
	// This test verifies that when there are REAL conflicts (not just
	// diverged history from squash merge), the auto-rebase properly
	// returns a MergeConflictError for manual resolution.
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Get the base branch name (main or master)
	baseBranch, _ := g.CurrentBranch(ctx)

	// Create and checkout a feature branch
	err := g.CreateAndCheckout(ctx, "feature-real-conflict")
	if err != nil {
		t.Fatalf("CreateAndCheckout() error = %v", err)
	}

	// Make a commit on the feature branch
	_ = os.WriteFile(filepath.Join(dir, "shared.txt"), []byte("feature version"), 0644)
	exec.Command("git", "-C", dir, "add", "shared.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "Add feature version").Run()

	// Go back to main and make a conflicting commit
	err = g.Checkout(ctx, baseBranch)
	if err != nil {
		t.Fatalf("Checkout() error = %v", err)
	}

	_ = os.WriteFile(filepath.Join(dir, "shared.txt"), []byte("main version"), 0644)
	exec.Command("git", "-C", dir, "add", "shared.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "Add main version").Run()

	// Try to merge - should get a conflict error since the rebase will also fail
	err = g.MergeBranchWithOptions(ctx, "feature-real-conflict", true)
	if err == nil {
		t.Fatal("MergeBranchWithOptions() should fail with conflict, got nil")
	}

	// Should be a MergeConflictError
	mergeErr, ok := err.(*MergeConflictError)
	if !ok {
		t.Fatalf("Expected MergeConflictError, got %T: %v", err, err)
	}

	// Verify the conflict file is reported
	found := false
	for _, f := range mergeErr.ConflictFiles {
		if f == "shared.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("MergeConflictError.ConflictFiles = %v, should contain 'shared.txt'", mergeErr.ConflictFiles)
	}

	// Verify repo is in clean state (no ongoing merge or rebase)
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	out, _ := cmd.Output()
	if len(out) > 0 {
		t.Errorf("Repository should be clean after aborted merge, got: %s", string(out))
	}
}
