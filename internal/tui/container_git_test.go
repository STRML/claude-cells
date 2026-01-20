package tui

import (
	"context"
	"errors"
	"testing"

	"github.com/STRML/claude-cells/internal/git"
	"github.com/STRML/claude-cells/internal/workstream"
)

// TestCheckUncommittedChangesCmd_WithMock demonstrates using MockGitClient
// to test container commands without requiring a real git repository.
func TestCheckUncommittedChangesCmd_WithMock(t *testing.T) {
	tests := []struct {
		name          string
		hasChanges    bool
		expectChanges bool
		gitError      error
		expectError   bool
	}{
		{
			name:          "no uncommitted changes",
			hasChanges:    false,
			expectChanges: false,
		},
		{
			name:          "has uncommitted changes",
			hasChanges:    true,
			expectChanges: true,
		},
		{
			name:        "git error",
			gitError:    errors.New("git status failed"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock git client
			mockGit := git.NewMockGitClient()
			mockGit.SetHasUncommittedChanges(tt.hasChanges)
			if tt.gitError != nil {
				mockGit.Err = tt.gitError
			}

			// Override the factory
			restore := SetGitClientFactory(func(path string) git.GitClient {
				return mockGit
			})
			defer restore()

			// Create a test workstream
			ws := &workstream.Workstream{
				ID:           "test-ws-1",
				BranchName:   "ccells/test-branch",
				WorktreePath: "/tmp/test-worktree",
			}

			// Run the command
			cmd := CheckUncommittedChangesCmd(ws)
			msg := cmd()

			// Check the result
			result, ok := msg.(UncommittedChangesMsg)
			if !ok {
				t.Fatalf("Expected UncommittedChangesMsg, got %T", msg)
			}

			if result.WorkstreamID != ws.ID {
				t.Errorf("WorkstreamID = %q, want %q", result.WorkstreamID, ws.ID)
			}

			if tt.expectError {
				if result.Error == nil {
					t.Error("Expected error, got nil")
				}
			} else {
				if result.Error != nil {
					t.Errorf("Unexpected error: %v", result.Error)
				}
				if result.HasChanges != tt.expectChanges {
					t.Errorf("HasChanges = %v, want %v", result.HasChanges, tt.expectChanges)
				}
			}
		})
	}
}

// TestBranchOperations_WithMock tests branch-related operations using mocks.
func TestBranchOperations_WithMock(t *testing.T) {
	t.Run("branch exists check", func(t *testing.T) {
		mockGit := git.NewMockGitClient()
		mockGit.AddBranch("ccells/existing-branch")

		restore := SetGitClientFactory(func(path string) git.GitClient {
			return mockGit
		})
		defer restore()

		ctx := context.Background()
		client := GitClientFactory("/any/path")

		// Check existing branch
		exists, err := client.BranchExists(ctx, "ccells/existing-branch")
		if err != nil {
			t.Fatalf("BranchExists error: %v", err)
		}
		if !exists {
			t.Error("Expected branch to exist")
		}

		// Check non-existing branch
		exists, err = client.BranchExists(ctx, "ccells/nonexistent")
		if err != nil {
			t.Fatalf("BranchExists error: %v", err)
		}
		if exists {
			t.Error("Expected branch to not exist")
		}
	})

	t.Run("branch has no commits", func(t *testing.T) {
		mockGit := git.NewMockGitClient()
		mockGit.AddBranch("ccells/empty-branch")

		// By default, BranchHasCommits returns false (no commits)
		restore := SetGitClientFactory(func(path string) git.GitClient {
			return mockGit
		})
		defer restore()

		ctx := context.Background()
		client := GitClientFactory("/any/path")

		hasCommits, err := client.BranchHasCommits(ctx, "ccells/empty-branch")
		if err != nil {
			t.Fatalf("BranchHasCommits error: %v", err)
		}
		if hasCommits {
			t.Error("Expected branch to have no commits")
		}
	})

	t.Run("branch has commits", func(t *testing.T) {
		mockGit := git.NewMockGitClient()
		mockGit.AddBranch("ccells/feature")
		// Override to return true for this test
		mockGit.BranchHasCommitsFn = func(ctx context.Context, name string) (bool, error) {
			return true, nil
		}

		restore := SetGitClientFactory(func(path string) git.GitClient {
			return mockGit
		})
		defer restore()

		ctx := context.Background()
		client := GitClientFactory("/any/path")

		hasCommits, err := client.BranchHasCommits(ctx, "ccells/feature")
		if err != nil {
			t.Fatalf("BranchHasCommits error: %v", err)
		}
		if !hasCommits {
			t.Error("Expected branch to have commits")
		}
	})
}

// TestWorktreeOperations_WithMock tests worktree operations using mocks.
func TestWorktreeOperations_WithMock(t *testing.T) {
	t.Run("create and list worktrees", func(t *testing.T) {
		mockGit := git.NewMockGitClient()

		restore := SetGitClientFactory(func(path string) git.GitClient {
			return mockGit
		})
		defer restore()

		ctx := context.Background()
		client := GitClientFactory("/repo")

		// Create worktree
		err := client.CreateWorktree(ctx, "/tmp/worktree1", "feature-branch")
		if err != nil {
			t.Fatalf("CreateWorktree error: %v", err)
		}

		// List worktrees
		worktrees, err := client.WorktreeList(ctx)
		if err != nil {
			t.Fatalf("WorktreeList error: %v", err)
		}

		found := false
		for _, wt := range worktrees {
			if wt == "/tmp/worktree1" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected worktree /tmp/worktree1 in list, got %v", worktrees)
		}

		// Check worktree exists for branch
		path, exists := client.WorktreeExistsForBranch(ctx, "feature-branch")
		if !exists {
			t.Error("Expected worktree to exist for feature-branch")
		}
		if path != "/tmp/worktree1" {
			t.Errorf("WorktreeExistsForBranch path = %q, want %q", path, "/tmp/worktree1")
		}
	})

	t.Run("remove worktree", func(t *testing.T) {
		mockGit := git.NewMockGitClient()
		mockGit.AddWorktree("/tmp/to-remove", "old-branch")

		restore := SetGitClientFactory(func(path string) git.GitClient {
			return mockGit
		})
		defer restore()

		ctx := context.Background()
		client := GitClientFactory("/repo")

		// Verify it exists first
		_, exists := client.WorktreeExistsForBranch(ctx, "old-branch")
		if !exists {
			t.Fatal("Expected worktree to exist before removal")
		}

		// Remove it
		err := client.RemoveWorktree(ctx, "/tmp/to-remove")
		if err != nil {
			t.Fatalf("RemoveWorktree error: %v", err)
		}

		// Verify it's gone
		_, exists = client.WorktreeExistsForBranch(ctx, "old-branch")
		if exists {
			t.Error("Expected worktree to not exist after removal")
		}
	})
}

// TestGitErrorHandling_WithMock tests error scenarios using mocks.
func TestGitErrorHandling_WithMock(t *testing.T) {
	t.Run("git operations fail gracefully", func(t *testing.T) {
		mockGit := git.NewMockGitClient()
		mockGit.Err = errors.New("simulated git failure")

		restore := SetGitClientFactory(func(path string) git.GitClient {
			return mockGit
		})
		defer restore()

		ctx := context.Background()
		client := GitClientFactory("/repo")

		// All operations should return the error
		_, err := client.CurrentBranch(ctx)
		if err == nil {
			t.Error("Expected error from CurrentBranch")
		}

		_, err = client.BranchExists(ctx, "any")
		if err == nil {
			t.Error("Expected error from BranchExists")
		}

		err = client.CreateWorktree(ctx, "/tmp/wt", "branch")
		if err == nil {
			t.Error("Expected error from CreateWorktree")
		}
	})
}

// TestCustomBehavior_WithMock demonstrates custom function overrides.
func TestCustomBehavior_WithMock(t *testing.T) {
	t.Run("custom merge conflict response", func(t *testing.T) {
		mockGit := git.NewMockGitClient()
		mockGit.MergeBranchFn = func(ctx context.Context, branch string) error {
			return &git.MergeConflictError{
				Branch:        branch,
				ConflictFiles: []string{"file1.go", "file2.go"},
			}
		}

		restore := SetGitClientFactory(func(path string) git.GitClient {
			return mockGit
		})
		defer restore()

		ctx := context.Background()
		client := GitClientFactory("/repo")

		err := client.MergeBranch(ctx, "feature")
		if err == nil {
			t.Fatal("Expected merge conflict error")
		}

		conflictErr, ok := err.(*git.MergeConflictError)
		if !ok {
			t.Fatalf("Expected *git.MergeConflictError, got %T", err)
		}

		if len(conflictErr.ConflictFiles) != 2 {
			t.Errorf("Expected 2 conflict files, got %d", len(conflictErr.ConflictFiles))
		}
	})
}
