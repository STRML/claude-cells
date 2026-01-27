package main

import (
	"fmt"
	"strings"

	"github.com/STRML/claude-cells/internal/docker"
)

// AllowedRuntimes lists valid runtime identifiers
var AllowedRuntimes = []string{"claude", "claudesp"}

// ResolveRuntime determines the runtime to use based on CLI flag and config files.
// Priority: CLI flag > project config > global config > default ("claude")
// Returns normalized (lowercase, trimmed) runtime or error if invalid.
func ResolveRuntime(runtimeFlag string, projectPath string) (string, error) {
	// Normalize the flag input
	runtimeFlag = strings.ToLower(strings.TrimSpace(runtimeFlag))

	// If flag provided, validate and return it
	if runtimeFlag != "" {
		if !isValidRuntime(runtimeFlag) {
			return "", fmt.Errorf("invalid runtime %q: must be one of %v", runtimeFlag, AllowedRuntimes)
		}
		return runtimeFlag, nil
	}

	// No flag - load from config
	cfg := docker.LoadConfig(projectPath)

	// Normalize config runtime
	runtime := strings.ToLower(strings.TrimSpace(cfg.Runtime))

	// Validate or use default
	if runtime == "" || !isValidRuntime(runtime) {
		return "claude", nil // Default
	}

	return runtime, nil
}

// isValidRuntime checks if a runtime is in the allowed list
func isValidRuntime(runtime string) bool {
	for _, allowed := range AllowedRuntimes {
		if runtime == allowed {
			return true
		}
	}
	return false
}
