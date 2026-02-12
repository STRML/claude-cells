package main

import "testing"

func TestGenerateBranchName(t *testing.T) {
	tests := []struct {
		prompt string
		want   string
	}{
		{"Add user authentication", "add-user-authentication"},
		{"Fix the login bug in auth module", "fix-the-login-bug"},
		{"", "workstream"},
		{"Simple", "simple"},
		{"Use special chars! @#$", "use-special-chars"},
		{"UPPERCASE words", "uppercase-words"},
	}
	for _, tt := range tests {
		t.Run(tt.prompt, func(t *testing.T) {
			got := generateBranchName(tt.prompt)
			if got != tt.want {
				t.Errorf("generateBranchName(%q) = %q, want %q", tt.prompt, got, tt.want)
			}
		})
	}
}
