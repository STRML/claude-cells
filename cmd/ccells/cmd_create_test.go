package main

import (
	"strings"
	"testing"
)

func TestValidateBranchName(t *testing.T) {
	tests := []struct {
		name    string
		branch  string
		wantErr bool
		errMsg  string
	}{
		{"valid simple", "my-branch", false, ""},
		{"valid with slash", "feat/auth", false, ""},
		{"valid with dots", "release.1.0", false, ""},
		{"valid with underscore", "my_branch", false, ""},
		{"valid mixed", "feat/my-branch_v2.0", false, ""},
		{"empty", "", true, "cannot be empty"},
		{"too long", strings.Repeat("a", 201), true, "too long"},
		{"shell metachar semicolon", "my;branch", true, "invalid character"},
		{"shell metachar pipe", "my|branch", true, "invalid character"},
		{"shell metachar ampersand", "my&branch", true, "invalid character"},
		{"shell metachar backtick", "my`branch", true, "invalid character"},
		{"shell metachar dollar", "my$branch", true, "invalid character"},
		{"space", "my branch", true, "invalid character"},
		{"double dots", "my..branch", true, "invalid sequence"},
		{"double slashes", "my//branch", true, "invalid sequence"},
		{"starts with slash", "/my-branch", true, "start or end"},
		{"ends with slash", "my-branch/", true, "start or end"},
		{"starts with dash", "-my-branch", true, "start with '-'"},
		{"ends with .lock", "my-branch.lock", true, ".lock"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBranchName(tt.branch)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateBranchName(%q) error = %v, wantErr %v", tt.branch, err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("validateBranchName(%q) error = %q, want to contain %q", tt.branch, err, tt.errMsg)
			}
		})
	}
}
