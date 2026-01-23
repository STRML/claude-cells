package gitproxy

import (
	"testing"
)

func TestBuildCommand(t *testing.T) {
	e := NewExecutor()

	tests := []struct {
		name     string
		op       Operation
		args     []string
		expected []string
	}{
		{
			name:     "git fetch",
			op:       OpGitFetch,
			args:     []string{"origin"},
			expected: []string{"git", "fetch", "origin"},
		},
		{
			name:     "git pull",
			op:       OpGitPull,
			args:     []string{"--rebase"},
			expected: []string{"git", "pull", "--rebase"},
		},
		{
			name:     "git push",
			op:       OpGitPush,
			args:     []string{"origin", "feature-branch"},
			expected: []string{"git", "push", "origin", "feature-branch"},
		},
		{
			name:     "gh pr view",
			op:       OpGHPRView,
			args:     []string{"123"},
			expected: []string{"gh", "pr", "view", "123"},
		},
		{
			name:     "gh pr checks",
			op:       OpGHPRChecks,
			args:     []string{},
			expected: []string{"gh", "pr", "checks"},
		},
		{
			name:     "gh pr diff",
			op:       OpGHPRDiff,
			args:     []string{},
			expected: []string{"gh", "pr", "diff"},
		},
		{
			name:     "gh pr list",
			op:       OpGHPRList,
			args:     []string{"--state", "open"},
			expected: []string{"gh", "pr", "list", "--state", "open"},
		},
		{
			name:     "gh pr create",
			op:       OpGHPRCreate,
			args:     []string{"--title", "Test PR"},
			expected: []string{"gh", "pr", "create", "--title", "Test PR"},
		},
		{
			name:     "gh pr merge",
			op:       OpGHPRMerge,
			args:     []string{"--squash"},
			expected: []string{"gh", "pr", "merge", "--squash"},
		},
		{
			name:     "gh issue view",
			op:       OpGHIssueView,
			args:     []string{"42"},
			expected: []string{"gh", "issue", "view", "42"},
		},
		{
			name:     "gh issue list",
			op:       OpGHIssueList,
			args:     []string{},
			expected: []string{"gh", "issue", "list"},
		},
		{
			name:     "unknown operation",
			op:       Operation("unknown"),
			args:     []string{},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.buildCommand(tt.op, tt.args)
			if len(result) != len(tt.expected) {
				t.Errorf("buildCommand() = %v, want %v", result, tt.expected)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("buildCommand()[%d] = %q, want %q", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestExtractPRCreateResult(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		wantNumber int
		wantURL    string
		wantNil    bool
	}{
		{
			name:       "simple URL",
			output:     "https://github.com/owner/repo/pull/123",
			wantNumber: 123,
			wantURL:    "https://github.com/owner/repo/pull/123",
		},
		{
			name:       "URL with surrounding text",
			output:     "Creating PR...\nhttps://github.com/owner/repo/pull/456\nDone!",
			wantNumber: 456,
			wantURL:    "https://github.com/owner/repo/pull/456",
		},
		{
			name:       "URL in verbose output",
			output:     "remote: Create a pull request for 'feature' on GitHub by visiting:\nremote:      https://github.com/owner/repo/pull/new/feature\n\nhttps://github.com/owner/repo/pull/789",
			wantNumber: 789,
			wantURL:    "https://github.com/owner/repo/pull/789",
		},
		{
			name:       "large PR number",
			output:     "https://github.com/owner/repo/pull/99999",
			wantNumber: 99999,
			wantURL:    "https://github.com/owner/repo/pull/99999",
		},
		{
			name:    "no URL",
			output:  "Error: failed to create PR",
			wantNil: true,
		},
		{
			name:    "empty output",
			output:  "",
			wantNil: true,
		},
		{
			name:    "invalid URL format",
			output:  "https://github.com/owner/repo/issues/123",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPRCreateResult(tt.output)
			if tt.wantNil {
				if result != nil {
					t.Errorf("extractPRCreateResult() = %+v, want nil", result)
				}
				return
			}
			if result == nil {
				t.Errorf("extractPRCreateResult() = nil, want {Number: %d, URL: %q}", tt.wantNumber, tt.wantURL)
				return
			}
			if result.Number != tt.wantNumber {
				t.Errorf("extractPRCreateResult().Number = %d, want %d", result.Number, tt.wantNumber)
			}
			if result.URL != tt.wantURL {
				t.Errorf("extractPRCreateResult().URL = %q, want %q", result.URL, tt.wantURL)
			}
		})
	}
}
