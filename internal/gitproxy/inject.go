package gitproxy

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// InjectProxyConfig injects the git proxy script and hooks configuration
// into a container's Claude settings directory.
func InjectProxyConfig(claudeDir string) error {
	// Write the proxy script to a bin directory
	binDir := filepath.Join(claudeDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return err
	}

	proxyScriptPath := filepath.Join(binDir, "ccells-git-proxy")
	if err := os.WriteFile(proxyScriptPath, []byte(ProxyScript), 0755); err != nil {
		return err
	}

	// Write the hook script that intercepts git/gh commands
	hookScriptPath := filepath.Join(binDir, "ccells-git-hook")
	if err := os.WriteFile(hookScriptPath, []byte(GitHookScript), 0755); err != nil {
		return err
	}

	// Load existing settings or create new
	settingsPath := filepath.Join(claudeDir, "settings.json")
	settings := make(map[string]interface{})

	if data, err := os.ReadFile(settingsPath); err == nil {
		if unmarshalErr := json.Unmarshal(data, &settings); unmarshalErr != nil {
			log.Printf("[gitproxy] Warning: failed to parse existing settings.json at %s: %v (starting fresh)", settingsPath, unmarshalErr)
			settings = make(map[string]interface{})
		}
	}

	// Merge our hooks with any existing hooks
	// Use PreToolUse event type with "Bash" matcher (not "Bash" as event type)
	hooks := getOrCreateMap(settings, "hooks")
	preToolUseHooks := getOrCreateSlice(hooks, "PreToolUse")

	// Add a single hook that intercepts all Bash commands and filters git/gh
	// The hook script parses the command from JSON stdin and decides what to do
	preToolUseHooks = appendHookIfMatcherNotExists(preToolUseHooks, "Bash", map[string]interface{}{
		"matcher": "Bash",
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": "/root/.claude/bin/ccells-git-hook",
			},
		},
	})

	hooks["PreToolUse"] = preToolUseHooks

	// Remove any invalid "Bash" hook event type that may have been added by older versions
	delete(hooks, "Bash")

	settings["hooks"] = hooks

	// Write updated settings
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(settingsPath, data, 0644)
}

// getOrCreateMap gets a nested map from a parent map, creating it if it doesn't exist.
func getOrCreateMap(parent map[string]interface{}, key string) map[string]interface{} {
	if v, ok := parent[key]; ok {
		if m, ok := v.(map[string]interface{}); ok {
			return m
		}
	}
	m := make(map[string]interface{})
	parent[key] = m
	return m
}

// getOrCreateSlice gets a slice from a map, creating it if it doesn't exist.
// If the key doesn't exist or isn't a slice, creates a new slice and stores it.
func getOrCreateSlice(parent map[string]interface{}, key string) []interface{} {
	if v, ok := parent[key]; ok {
		if s, ok := v.([]interface{}); ok {
			return s
		}
	}
	s := []interface{}{}
	parent[key] = s
	return s
}

// appendHookIfMatcherNotExists adds a hook to the slice if a hook with the specified matcher doesn't exist.
func appendHookIfMatcherNotExists(hooks []interface{}, matcherToFind string, newHook map[string]interface{}) []interface{} {
	for _, h := range hooks {
		if m, ok := h.(map[string]interface{}); ok {
			if matcher, ok := m["matcher"].(string); ok && matcher == matcherToFind {
				// Hook with same matcher already exists, skip
				return hooks
			}
		}
	}

	return append(hooks, newHook)
}
