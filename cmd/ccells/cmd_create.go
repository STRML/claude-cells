package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// validateBranchName checks a branch name for unsafe characters.
func validateBranchName(branch string) error {
	if branch == "" {
		return fmt.Errorf("branch name cannot be empty")
	}
	if len(branch) > 200 {
		return fmt.Errorf("branch name too long (max 200 chars)")
	}
	for _, r := range branch {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '/' || r == '-') {
			return fmt.Errorf("branch name contains invalid character: %q", r)
		}
	}
	if strings.Contains(branch, "..") || strings.Contains(branch, "//") {
		return fmt.Errorf("branch name contains invalid sequence")
	}
	if strings.HasPrefix(branch, "/") || strings.HasSuffix(branch, "/") {
		return fmt.Errorf("branch name cannot start or end with '/'")
	}
	if strings.HasPrefix(branch, "-") {
		return fmt.Errorf("branch name cannot start with '-'")
	}
	if strings.HasSuffix(branch, ".lock") {
		return fmt.Errorf("branch name cannot end with '.lock'")
	}
	return nil
}

// createResult holds the response from a successful create operation.
type createResult struct {
	Branch        string
	ContainerName string
}

// runCreate sends a create request to the daemon.
// When skipPane is true, the daemon skips tmux pane management (caller handles it).
// Returns the container name on success.
func runCreate(stateDir, branch, prompt, runtime string, skipPane bool) (*createResult, error) {
	if err := validateBranchName(branch); err != nil {
		return nil, err
	}

	daemonSock := filepath.Join(stateDir, "daemon.sock")

	type createParams struct {
		Branch   string `json:"branch"`
		Prompt   string `json:"prompt"`
		Runtime  string `json:"runtime"`
		SkipPane bool   `json:"skip_pane,omitempty"`
	}
	params, _ := json.Marshal(createParams{
		Branch:   branch,
		Prompt:   prompt,
		Runtime:  runtime,
		SkipPane: skipPane,
	})

	// Create operations can take minutes (image build + container start).
	resp, err := sendDaemonRequestWithResponse(daemonSock, "create", params, 3*time.Minute)
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("create failed: %s", resp.Error)
	}

	// Parse response data for container name
	var data map[string]string
	if resp.Data != nil {
		json.Unmarshal(resp.Data, &data)
	}

	return &createResult{
		Branch:        branch,
		ContainerName: data["container"],
	}, nil
}
