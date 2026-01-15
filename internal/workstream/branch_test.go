package workstream

import "testing"

func TestGenerateBranchName(t *testing.T) {
	tests := []struct {
		name     string
		prompt   string
		expected string
	}{
		{
			name:     "empty input",
			prompt:   "",
			expected: "workstream",
		},
		{
			name:     "whitespace only",
			prompt:   "   ",
			expected: "workstream",
		},
		{
			name:     "simple prompt",
			prompt:   "add user authentication",
			expected: "add-user-authentication",
		},
		{
			name:     "strips common words",
			prompt:   "add the user authentication to the app",
			expected: "add-user-authentication-app",
		},
		{
			name:     "converts to lowercase",
			prompt:   "Add User Authentication",
			expected: "add-user-authentication",
		},
		{
			name:     "replaces spaces with hyphens",
			prompt:   "fix   multiple   spaces",
			expected: "fix-multiple-spaces",
		},
		{
			name:     "truncates to 50 chars",
			prompt:   "implement a very long feature that has way too many words to fit in a branch name",
			expected: "implement-very-long-feature-has-way-too-many-words",
		},
		{
			name:     "handles special characters",
			prompt:   "add JWT/OAuth authentication!",
			expected: "add-jwt-oauth-authentication",
		},
		{
			name:     "strips leading/trailing hyphens",
			prompt:   "the authentication",
			expected: "authentication",
		},
		{
			name:     "with keyword",
			prompt:   "add user authentication with JWT tokens",
			expected: "add-user-authentication-jwt-tokens",
		},
		{
			name:     "for keyword",
			prompt:   "create tests for the API",
			expected: "create-tests-api",
		},
		{
			name:     "handles empty after stripping",
			prompt:   "the a an to for with",
			expected: "workstream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateBranchName(tt.prompt)
			if got != tt.expected {
				t.Errorf("GenerateBranchName(%q) = %q, want %q", tt.prompt, got, tt.expected)
			}
		})
	}
}

func TestGenerateBranchName_MaxLength(t *testing.T) {
	prompt := "this is an extremely long prompt that should definitely be truncated because it exceeds the maximum allowed length for a git branch name"
	got := GenerateBranchName(prompt)
	if len(got) > 50 {
		t.Errorf("GenerateBranchName() returned %d chars, want <= 50", len(got))
	}
}

func TestGenerateBranchName_NoTrailingHyphen(t *testing.T) {
	// Edge case: truncation might leave a trailing hyphen
	prompt := "add something that ends at exactly fifty chars here"
	got := GenerateBranchName(prompt)
	if got[len(got)-1] == '-' {
		t.Errorf("GenerateBranchName() has trailing hyphen: %q", got)
	}
}
