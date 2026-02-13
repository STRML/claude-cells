package main

import (
	"encoding/json"
	"fmt"
	"strings"
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
	if strings.Contains(branch, "..") {
		return fmt.Errorf("branch name cannot contain '..'")
	}
	if strings.HasPrefix(branch, "/") || strings.HasSuffix(branch, "/") {
		return fmt.Errorf("branch name cannot start or end with '/'")
	}
	return nil
}

// runCreate sends a create request to the daemon.
func runCreate(stateDir, branch, prompt, runtime string) error {
	if err := validateBranchName(branch); err != nil {
		return err
	}

	daemonSock := stateDir + "/daemon.sock"

	params, _ := json.Marshal(map[string]string{
		"branch":  branch,
		"prompt":  prompt,
		"runtime": runtime,
	})

	resp, err := sendDaemonRequestWithResponse(daemonSock, "create", params)
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("create failed: %s", resp.Error)
	}

	fmt.Printf("Workstream created on branch: %s\n", branch)
	return nil
}
