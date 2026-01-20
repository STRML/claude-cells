package git

import (
	"context"
	"errors"
	"testing"
)

func TestNewMockGitClient(t *testing.T) {
	client := NewMockGitClient()

	if client == nil {
		t.Fatal("NewMockGitClient() returned nil")
	}

	ctx := context.Background()

	// Check default state
	branch, err := client.CurrentBranch(ctx)
	if err != nil {
		t.Fatalf("CurrentBranch() error = %v", err)
	}
	if branch != "main" {
		t.Errorf("CurrentBranch() = %q, want %q", branch, "main")
	}

	// Check main branch exists
	exists, err := client.BranchExists(ctx, "main")
	if err != nil {
		t.Fatalf("BranchExists() error = %v", err)
	}
	if !exists {
		t.Error("BranchExists(main) = false, want true")
	}
}

func TestMockGitClient_BranchOperations(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	// Create a branch
	err := client.CreateBranch(ctx, "feature")
	if err != nil {
		t.Fatalf("CreateBranch() error = %v", err)
	}

	// Verify it exists
	exists, err := client.BranchExists(ctx, "feature")
	if err != nil {
		t.Fatalf("BranchExists() error = %v", err)
	}
	if !exists {
		t.Error("BranchExists(feature) = false after CreateBranch")
	}

	// Checkout the branch
	err = client.Checkout(ctx, "feature")
	if err != nil {
		t.Fatalf("Checkout() error = %v", err)
	}

	branch, _ := client.CurrentBranch(ctx)
	if branch != "feature" {
		t.Errorf("CurrentBranch() = %q after checkout, want %q", branch, "feature")
	}

	// Switch back to main to delete feature
	_ = client.Checkout(ctx, "main")

	// Delete the branch
	err = client.DeleteBranch(ctx, "feature")
	if err != nil {
		t.Fatalf("DeleteBranch() error = %v", err)
	}

	exists, _ = client.BranchExists(ctx, "feature")
	if exists {
		t.Error("BranchExists(feature) = true after DeleteBranch")
	}
}

func TestMockGitClient_CreateBranch_AlreadyExists(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	// main already exists
	err := client.CreateBranch(ctx, "main")
	if err == nil {
		t.Error("CreateBranch(main) should fail when branch exists")
	}
}

func TestMockGitClient_Checkout_NonExistent(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	err := client.Checkout(ctx, "nonexistent")
	if err == nil {
		t.Error("Checkout(nonexistent) should fail")
	}
}

func TestMockGitClient_DeleteBranch_CurrentBranch(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	err := client.DeleteBranch(ctx, "main")
	if err == nil {
		t.Error("DeleteBranch(main) should fail when it's the current branch")
	}
}

func TestMockGitClient_CreateAndCheckout(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	err := client.CreateAndCheckout(ctx, "new-feature")
	if err != nil {
		t.Fatalf("CreateAndCheckout() error = %v", err)
	}

	branch, _ := client.CurrentBranch(ctx)
	if branch != "new-feature" {
		t.Errorf("CurrentBranch() = %q, want %q", branch, "new-feature")
	}

	exists, _ := client.BranchExists(ctx, "new-feature")
	if !exists {
		t.Error("BranchExists(new-feature) = false after CreateAndCheckout")
	}
}

func TestMockGitClient_WorktreeOperations(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	// Create worktree with new branch
	err := client.CreateWorktree(ctx, "/tmp/worktree1", "wt-branch")
	if err != nil {
		t.Fatalf("CreateWorktree() error = %v", err)
	}

	// Verify branch was created
	exists, _ := client.BranchExists(ctx, "wt-branch")
	if !exists {
		t.Error("Branch should exist after CreateWorktree")
	}

	// Verify worktree is listed
	worktrees, err := client.WorktreeList(ctx)
	if err != nil {
		t.Fatalf("WorktreeList() error = %v", err)
	}
	found := false
	for _, wt := range worktrees {
		if wt == "/tmp/worktree1" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("WorktreeList() = %v, should contain /tmp/worktree1", worktrees)
	}

	// Check WorktreeExistsForBranch
	path, exists := client.WorktreeExistsForBranch(ctx, "wt-branch")
	if !exists {
		t.Error("WorktreeExistsForBranch() = false, want true")
	}
	if path != "/tmp/worktree1" {
		t.Errorf("WorktreeExistsForBranch() path = %q, want %q", path, "/tmp/worktree1")
	}

	// Remove worktree
	err = client.RemoveWorktree(ctx, "/tmp/worktree1")
	if err != nil {
		t.Fatalf("RemoveWorktree() error = %v", err)
	}

	_, exists = client.WorktreeExistsForBranch(ctx, "wt-branch")
	if exists {
		t.Error("WorktreeExistsForBranch() = true after removal")
	}
}

func TestMockGitClient_CreateWorktreeFromExisting(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	// Create branch first
	_ = client.CreateBranch(ctx, "existing-branch")

	// Create worktree from existing branch
	err := client.CreateWorktreeFromExisting(ctx, "/tmp/worktree2", "existing-branch")
	if err != nil {
		t.Fatalf("CreateWorktreeFromExisting() error = %v", err)
	}

	path, exists := client.WorktreeExistsForBranch(ctx, "existing-branch")
	if !exists {
		t.Error("WorktreeExistsForBranch() = false after CreateWorktreeFromExisting")
	}
	if path != "/tmp/worktree2" {
		t.Errorf("path = %q, want %q", path, "/tmp/worktree2")
	}
}

func TestMockGitClient_CreateWorktreeFromExisting_NonExistent(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	err := client.CreateWorktreeFromExisting(ctx, "/tmp/wt", "nonexistent")
	if err == nil {
		t.Error("CreateWorktreeFromExisting() should fail for nonexistent branch")
	}
}

func TestMockGitClient_StashOperations(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	// Set up uncommitted changes
	client.SetHasUncommittedChanges(true)

	hasChanges, _ := client.HasUncommittedChanges(ctx)
	if !hasChanges {
		t.Error("HasUncommittedChanges() = false after SetHasUncommittedChanges(true)")
	}

	// Stash
	err := client.Stash(ctx)
	if err != nil {
		t.Fatalf("Stash() error = %v", err)
	}

	hasChanges, _ = client.HasUncommittedChanges(ctx)
	if hasChanges {
		t.Error("HasUncommittedChanges() = true after Stash()")
	}

	// Stash pop
	err = client.StashPop(ctx)
	if err != nil {
		t.Fatalf("StashPop() error = %v", err)
	}

	hasChanges, _ = client.HasUncommittedChanges(ctx)
	if !hasChanges {
		t.Error("HasUncommittedChanges() = false after StashPop()")
	}
}

func TestMockGitClient_ListCCellsBranches(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	// Add some ccells branches
	client.AddBranch("ccells/feature-1")
	client.AddBranch("ccells/feature-2")
	client.AddBranch("other-branch")

	branches, err := client.ListCCellsBranches(ctx)
	if err != nil {
		t.Fatalf("ListCCellsBranches() error = %v", err)
	}

	if len(branches) != 2 {
		t.Errorf("ListCCellsBranches() returned %d branches, want 2", len(branches))
	}

	for _, b := range branches {
		if len(b) < 7 || b[:7] != "ccells/" {
			t.Errorf("ListCCellsBranches() returned non-ccells branch: %s", b)
		}
	}
}

func TestMockGitClient_ErrorInjection(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	expectedErr := errors.New("injected error")
	client.Err = expectedErr

	// All operations should return the injected error
	_, err := client.CurrentBranch(ctx)
	if !errors.Is(err, expectedErr) {
		t.Errorf("CurrentBranch() error = %v, want %v", err, expectedErr)
	}

	err = client.CreateBranch(ctx, "test")
	if !errors.Is(err, expectedErr) {
		t.Errorf("CreateBranch() error = %v, want %v", err, expectedErr)
	}

	_, err = client.BranchExists(ctx, "main")
	if !errors.Is(err, expectedErr) {
		t.Errorf("BranchExists() error = %v, want %v", err, expectedErr)
	}
}

func TestMockGitClient_CustomFunctions(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	// Override CurrentBranch
	client.CurrentBranchFn = func(ctx context.Context) (string, error) {
		return "custom-branch", nil
	}

	branch, err := client.CurrentBranch(ctx)
	if err != nil {
		t.Fatalf("CurrentBranch() error = %v", err)
	}
	if branch != "custom-branch" {
		t.Errorf("CurrentBranch() = %q, want %q", branch, "custom-branch")
	}

	// Override BranchHasCommits
	client.BranchHasCommitsFn = func(ctx context.Context, name string) (bool, error) {
		return true, nil
	}

	hasCommits, err := client.BranchHasCommits(ctx, "any")
	if err != nil {
		t.Fatalf("BranchHasCommits() error = %v", err)
	}
	if !hasCommits {
		t.Error("BranchHasCommits() = false, want true (from custom function)")
	}
}

func TestMockGitClient_GetBaseBranch(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	base, err := client.GetBaseBranch(ctx)
	if err != nil {
		t.Fatalf("GetBaseBranch() error = %v", err)
	}
	if base != "main" {
		t.Errorf("GetBaseBranch() = %q, want %q", base, "main")
	}

	// Change base branch
	client.SetBaseBranch("master")
	base, _ = client.GetBaseBranch(ctx)
	if base != "master" {
		t.Errorf("GetBaseBranch() = %q after SetBaseBranch, want %q", base, "master")
	}
}

func TestMockGitClient_RepoID(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	id, err := client.RepoID(ctx)
	if err != nil {
		t.Fatalf("RepoID() error = %v", err)
	}
	if id != "mock-repo-id" {
		t.Errorf("RepoID() = %q, want %q", id, "mock-repo-id")
	}
}

func TestMockGitClient_RemoteURL(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	url, err := client.RemoteURL(ctx, "origin")
	if err != nil {
		t.Fatalf("RemoteURL() error = %v", err)
	}
	if url != "https://github.com/mock/repo.git" {
		t.Errorf("RemoteURL() = %q, unexpected", url)
	}
}

func TestMockGitClient_MergeAndRebase(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	// Default implementations should succeed
	if err := client.MergeBranch(ctx, "feature"); err != nil {
		t.Errorf("MergeBranch() error = %v", err)
	}

	if err := client.MergeBranchWithOptions(ctx, "feature", true); err != nil {
		t.Errorf("MergeBranchWithOptions() error = %v", err)
	}

	if err := client.RebaseBranch(ctx, "feature"); err != nil {
		t.Errorf("RebaseBranch() error = %v", err)
	}

	if err := client.AbortRebase(ctx); err != nil {
		t.Errorf("AbortRebase() error = %v", err)
	}

	// Test conflict simulation
	client.GetConflictFilesFn = func(ctx context.Context) ([]string, error) {
		return []string{"file1.go", "file2.go"}, nil
	}

	conflicts, err := client.GetConflictFiles(ctx)
	if err != nil {
		t.Fatalf("GetConflictFiles() error = %v", err)
	}
	if len(conflicts) != 2 {
		t.Errorf("GetConflictFiles() returned %d files, want 2", len(conflicts))
	}
}

func TestMockGitClient_RemoteOperations(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	// All should succeed by default
	if err := client.Push(ctx, "feature"); err != nil {
		t.Errorf("Push() error = %v", err)
	}

	if err := client.FetchMain(ctx); err != nil {
		t.Errorf("FetchMain() error = %v", err)
	}

	if err := client.PullMain(ctx); err != nil {
		t.Errorf("PullMain() error = %v", err)
	}

	if err := client.UpdateMainBranch(ctx); err != nil {
		t.Errorf("UpdateMainBranch() error = %v", err)
	}

	if err := client.DeleteRemoteBranch(ctx, "feature"); err != nil {
		t.Errorf("DeleteRemoteBranch() error = %v", err)
	}
}

func TestMockGitClient_GetBranchInfo(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	info, err := client.GetBranchInfo(ctx, "feature")
	if err != nil {
		t.Fatalf("GetBranchInfo() error = %v", err)
	}
	if info == "" {
		t.Error("GetBranchInfo() returned empty string")
	}
}

func TestMockGitClient_ThreadSafety(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	// Run concurrent operations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			branchName := "branch-" + string(rune('a'+n))
			_ = client.CreateBranch(ctx, branchName)
			_, _ = client.BranchExists(ctx, branchName)
			_, _ = client.CurrentBranch(ctx)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestMockGitClient_SetCurrentBranch(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	client.SetCurrentBranch("develop")

	branch, _ := client.CurrentBranch(ctx)
	if branch != "develop" {
		t.Errorf("CurrentBranch() = %q after SetCurrentBranch, want %q", branch, "develop")
	}

	// Setting current branch also adds it to known branches
	exists, _ := client.BranchExists(ctx, "develop")
	if !exists {
		t.Error("SetCurrentBranch should add branch to known branches")
	}
}

func TestMockGitClient_GetBranches(t *testing.T) {
	client := NewMockGitClient()

	client.AddBranch("feature-1")
	client.AddBranch("feature-2")

	branches := client.GetBranches()
	if len(branches) != 3 { // main + 2 features
		t.Errorf("GetBranches() returned %d branches, want 3", len(branches))
	}
}

func TestMockGitClient_GetWorktrees(t *testing.T) {
	client := NewMockGitClient()
	ctx := context.Background()

	_ = client.CreateWorktree(ctx, "/path/1", "branch-1")
	_ = client.CreateWorktree(ctx, "/path/2", "branch-2")

	worktrees := client.GetWorktrees()
	if len(worktrees) != 2 {
		t.Errorf("GetWorktrees() returned %d worktrees, want 2", len(worktrees))
	}

	if worktrees["/path/1"] != "branch-1" {
		t.Errorf("worktrees[/path/1] = %q, want %q", worktrees["/path/1"], "branch-1")
	}
}
