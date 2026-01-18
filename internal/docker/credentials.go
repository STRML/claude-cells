package docker

import (
	"os/exec"
	"runtime"
	"strings"
)

// ClaudeCredentials holds the OAuth credentials from Claude Code
type ClaudeCredentials struct {
	Raw string // The raw JSON from keychain
}

// GetClaudeCredentials retrieves Claude Code OAuth credentials from the system keychain.
// Returns nil if credentials are not found or on non-macOS systems.
func GetClaudeCredentials() (*ClaudeCredentials, error) {
	// Only works on macOS
	if runtime.GOOS != "darwin" {
		return nil, nil
	}

	// Try to get credentials from keychain
	cmd := exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w")
	output, err := cmd.Output()
	if err != nil {
		// Credentials not found or access denied - not an error, just not available
		return nil, nil
	}

	raw := strings.TrimSpace(string(output))
	if raw == "" {
		return nil, nil
	}

	return &ClaudeCredentials{
		Raw: raw,
	}, nil
}
