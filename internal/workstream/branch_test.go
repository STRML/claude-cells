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
			name:     "limits to 5 meaningful words",
			prompt:   "implement a very long feature that has way too many words to fit in a branch name",
			expected: "implement-very-long-feature-has",
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

func TestGenerateUniqueBranchName(t *testing.T) {
	tests := []struct {
		name             string
		prompt           string
		existingBranches []string
		expected         string
	}{
		{
			name:             "no conflicts",
			prompt:           "add authentication",
			existingBranches: []string{},
			expected:         "add-authentication",
		},
		{
			name:             "no conflicts with other branches",
			prompt:           "add authentication",
			existingBranches: []string{"fix-bug", "add-feature"},
			expected:         "add-authentication",
		},
		{
			name:             "conflict adds suffix -2",
			prompt:           "add authentication",
			existingBranches: []string{"add-authentication"},
			expected:         "add-authentication-2",
		},
		{
			name:             "conflict with -2 adds -3",
			prompt:           "add authentication",
			existingBranches: []string{"add-authentication", "add-authentication-2"},
			expected:         "add-authentication-3",
		},
		{
			name:             "gaps in sequence are filled",
			prompt:           "add authentication",
			existingBranches: []string{"add-authentication", "add-authentication-3"},
			expected:         "add-authentication-2",
		},
		{
			name:             "multiple gaps",
			prompt:           "add feature",
			existingBranches: []string{"add-feature", "add-feature-2", "add-feature-5"},
			expected:         "add-feature-3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateUniqueBranchName(tt.prompt, tt.existingBranches)
			if got != tt.expected {
				t.Errorf("GenerateUniqueBranchName(%q, %v) = %q, want %q",
					tt.prompt, tt.existingBranches, got, tt.expected)
			}
		})
	}
}

func TestGenerateUniqueBranchName_MaxLength(t *testing.T) {
	// Long branch name that's near the limit (with 5-word limit)
	prompt := "implement a very long feature that has way too many words"
	existing := []string{
		"implement-very-long-feature-has", // First 5 words after stripping stop words
	}
	got := GenerateUniqueBranchName(prompt, existing)
	if len(got) > 50 {
		t.Errorf("GenerateUniqueBranchName() returned %d chars, want <= 50: %q", len(got), got)
	}
	// Should still be unique (different from existing) - will get -2 suffix
	if got == existing[0] {
		t.Errorf("GenerateUniqueBranchName() returned same as existing: %q", got)
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{123, "123"},
		{-5, "-5"},
	}
	for _, tt := range tests {
		got := itoa(tt.input)
		if got != tt.expected {
			t.Errorf("itoa(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
