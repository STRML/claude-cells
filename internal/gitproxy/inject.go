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
	hooks := getOrCreateMap(settings, "hooks")
	bashHooks := getOrCreateSlice(hooks, "Bash")

	// Add our git/gh intercepting hooks
	bashHooks = appendHookIfNotExists(bashHooks, map[string]interface{}{
		"matcher": "^git\\s+(fetch|pull|push)",
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": "/root/.claude/bin/ccells-git-proxy \"$@\"",
			},
		},
	})

	bashHooks = appendHookIfNotExists(bashHooks, map[string]interface{}{
		"matcher": "^git\\s+remote",
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "block",
				"message": "git remote commands are blocked in ccells containers",
			},
		},
	})

	bashHooks = appendHookIfNotExists(bashHooks, map[string]interface{}{
		"matcher": "^gh\\s+pr\\s+(view|checks|diff|list|create|merge)",
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": "/root/.claude/bin/ccells-git-proxy \"$@\"",
			},
		},
	})

	bashHooks = appendHookIfNotExists(bashHooks, map[string]interface{}{
		"matcher": "^gh\\s+issue\\s+(view|list)",
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": "/root/.claude/bin/ccells-git-proxy \"$@\"",
			},
		},
	})

	hooks["Bash"] = bashHooks
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
func getOrCreateSlice(parent map[string]interface{}, key string) []interface{} {
	if v, ok := parent[key]; ok {
		if s, ok := v.([]interface{}); ok {
			return s
		}
	}
	return []interface{}{}
}

// appendHookIfNotExists adds a hook to the slice if a hook with the same matcher doesn't exist.
func appendHookIfNotExists(hooks []interface{}, newHook map[string]interface{}) []interface{} {
	newMatcher, _ := newHook["matcher"].(string)

	for _, h := range hooks {
		if m, ok := h.(map[string]interface{}); ok {
			if matcher, ok := m["matcher"].(string); ok && matcher == newMatcher {
				// Hook with same matcher already exists, skip
				return hooks
			}
		}
	}

	return append(hooks, newHook)
}
