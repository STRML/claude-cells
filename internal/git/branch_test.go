package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
