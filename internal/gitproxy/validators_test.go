package gitproxy

import (
	"testing"
)

func TestValidatePush(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		ws      WorkstreamInfo
		wantErr bool
		errMsg  string
	}{
		{
			name: "push to correct branch",
			args: []string{"origin", "feature-branch"},
			ws:   WorkstreamInfo{Branch: "feature-branch"},
		},
		{
			name: "push to wrong branch",
			args: []string{"origin", "other-branch"},
			ws:   WorkstreamInfo{Branch: "feature-branch"},
			wantErr: true,
			errMsg:  "can only push to branch",
		},
		{
			name: "push with HEAD:branch syntax - correct",
			args: []string{"origin", "HEAD:feature-branch"},
			ws:   WorkstreamInfo{Branch: "feature-branch"},
		},
		{
			name: "push with HEAD:branch syntax - wrong",
			args: []string{"origin", "HEAD:other-branch"},
			ws:   WorkstreamInfo{Branch: "feature-branch"},
			wantErr: true,
			errMsg:  "can only push to branch",
		},
		{
			name: "push with --force flag",
			args: []string{"--force", "origin", "feature-branch"},
			ws:   WorkstreamInfo{Branch: "feature-branch"},
			wantErr: true,
			errMsg:  "force push not allowed",
		},
		{
			name: "push with -f flag",
			args: []string{"-f", "origin", "feature-branch"},
			ws:   WorkstreamInfo{Branch: "feature-branch"},
			wantErr: true,
			errMsg:  "force push not allowed",
		},
		{
			name: "push with --force-with-lease",
			args: []string{"--force-with-lease", "origin", "feature-branch"},
			ws:   WorkstreamInfo{Branch: "feature-branch"},
			wantErr: true,
			errMsg:  "force push not allowed",
		},
		{
			name:    "push with --force=true",
			args:    []string{"--force=true", "origin", "feature-branch"},
			ws:      WorkstreamInfo{Branch: "feature-branch"},
			wantErr: true,
			errMsg:  "force push not allowed",
		},
		{
			name:    "push with -f=true",
			args:    []string{"-f=true", "origin", "feature-branch"},
			ws:      WorkstreamInfo{Branch: "feature-branch"},
			wantErr: true,
			errMsg:  "force push not allowed",
		},
		{
			name: "push with -u flag (set upstream)",
			args: []string{"-u", "origin", "feature-branch"},
			ws:   WorkstreamInfo{Branch: "feature-branch"},
		},
		{
			name: "push with no explicit branch (uses current)",
			args: []string{"origin"},
			ws:   WorkstreamInfo{Branch: "feature-branch"},
		},
		{
			name: "push with no args (uses defaults)",
			args: []string{},
			ws:   WorkstreamInfo{Branch: "feature-branch"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePush(tt.args, tt.ws)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateMerge(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		ws      WorkstreamInfo
		wantErr bool
		errMsg  string
	}{
		{
			name: "merge current branch's PR - no args",
			args: []string{},
			ws:   WorkstreamInfo{Branch: "feature-branch", PRNumber: 123},
		},
		{
			name: "merge by PR number - correct",
			args: []string{"123"},
			ws:   WorkstreamInfo{Branch: "feature-branch", PRNumber: 123},
		},
		{
			name: "merge by PR number - wrong",
			args: []string{"456"},
			ws:   WorkstreamInfo{Branch: "feature-branch", PRNumber: 123},
			wantErr: true,
			errMsg:  "can only merge PR #123",
		},
		{
			name: "merge by URL - correct",
			args: []string{"https://github.com/owner/repo/pull/123"},
			ws:   WorkstreamInfo{Branch: "feature-branch", PRNumber: 123},
		},
		{
			name: "merge by URL - wrong",
			args: []string{"https://github.com/owner/repo/pull/456"},
			ws:   WorkstreamInfo{Branch: "feature-branch", PRNumber: 123},
			wantErr: true,
			errMsg:  "can only merge PR #123",
		},
		{
			name: "merge with no PR associated",
			args: []string{},
			ws:   WorkstreamInfo{Branch: "feature-branch", PRNumber: 0},
			wantErr: true,
			errMsg:  "no PR associated",
		},
		{
			name: "merge with flags",
			args: []string{"--squash", "123"},
			ws:   WorkstreamInfo{Branch: "feature-branch", PRNumber: 123},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMerge(tt.args, tt.ws)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseOperation(t *testing.T) {
	tests := []struct {
		name       string
		cmd        string
		args       []string
		wantOp     Operation
		wantArgs   []string
		wantErr    bool
	}{
		{
			name:     "git fetch",
			cmd:      "git",
			args:     []string{"fetch", "origin"},
			wantOp:   OpGitFetch,
			wantArgs: []string{"origin"},
		},
		{
			name:     "git pull",
			cmd:      "git",
			args:     []string{"pull", "--rebase"},
			wantOp:   OpGitPull,
			wantArgs: []string{"--rebase"},
		},
		{
			name:     "git push",
			cmd:      "git",
			args:     []string{"push", "origin", "main"},
			wantOp:   OpGitPush,
			wantArgs: []string{"origin", "main"},
		},
		{
			name:    "git remote - blocked",
			cmd:     "git",
			args:    []string{"remote", "-v"},
			wantErr: true,
		},
		{
			name:    "git status - not proxied",
			cmd:     "git",
			args:    []string{"status"},
			wantErr: true,
		},
		{
			name:     "gh pr view",
			cmd:      "gh",
			args:     []string{"pr", "view", "123"},
			wantOp:   OpGHPRView,
			wantArgs: []string{"123"},
		},
		{
			name:     "gh pr create",
			cmd:      "gh",
			args:     []string{"pr", "create", "--title", "Test"},
			wantOp:   OpGHPRCreate,
			wantArgs: []string{"--title", "Test"},
		},
		{
			name:     "gh pr merge",
			cmd:      "gh",
			args:     []string{"pr", "merge", "--squash"},
			wantOp:   OpGHPRMerge,
			wantArgs: []string{"--squash"},
		},
		{
			name:     "gh issue view",
			cmd:      "gh",
			args:     []string{"issue", "view", "42"},
			wantOp:   OpGHIssueView,
			wantArgs: []string{"42"},
		},
		{
			name:    "gh pr close - not proxied",
			cmd:     "gh",
			args:    []string{"pr", "close"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op, args, err := ParseOperation(tt.cmd, tt.args)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if op != tt.wantOp {
				t.Errorf("got op %v, want %v", op, tt.wantOp)
			}
			if len(args) != len(tt.wantArgs) {
				t.Errorf("got %d args, want %d", len(args), len(tt.wantArgs))
			}
		})
	}
}

func TestIsAllowedOperation(t *testing.T) {
	allowed := []Operation{
		OpGitFetch, OpGitPull, OpGitPush,
		OpGHPRView, OpGHPRChecks, OpGHPRDiff, OpGHPRList,
		OpGHPRCreate, OpGHPRMerge,
		OpGHIssueView, OpGHIssueList,
	}

	for _, op := range allowed {
		if !IsAllowedOperation(op) {
			t.Errorf("expected %q to be allowed", op)
		}
	}

	// Test an unknown operation
	if IsAllowedOperation("git-status") {
		t.Error("expected git-status to be disallowed")
	}
}

func TestExtractPushBranch(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "simple push",
			args: []string{"origin", "feature"},
			want: "feature",
		},
		{
			name: "with HEAD:branch",
			args: []string{"origin", "HEAD:feature"},
			want: "feature",
		},
		{
			name: "no branch specified",
			args: []string{"origin"},
			want: "",
		},
		{
			name: "empty args",
			args: []string{},
			want: "",
		},
		{
			name: "with -u flag",
			args: []string{"-u", "origin", "feature"},
			want: "feature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPushBranch(tt.args)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractPRNumber(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{
			name: "number only",
			args: []string{"123"},
			want: 123,
		},
		{
			name: "URL with number",
			args: []string{"https://github.com/owner/repo/pull/456"},
			want: 456,
		},
		{
			name: "no number",
			args: []string{"--squash"},
			want: 0,
		},
		{
			name: "number with flags",
			args: []string{"--squash", "789"},
			want: 789,
		},
		{
			name: "empty args",
			args: []string{},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPRNumber(tt.args)
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	if start+len(substr) > len(s) {
		return false
	}
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
