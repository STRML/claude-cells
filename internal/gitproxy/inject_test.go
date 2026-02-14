package gitproxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInjectProxyConfig(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("Failed to create claude dir: %v", err)
	}

	// Run the injection
	if err := InjectProxyConfig(claudeDir); err != nil {
		t.Fatalf("InjectProxyConfig failed: %v", err)
	}

	// Verify the proxy script was written
	proxyScriptPath := filepath.Join(claudeDir, "bin", "ccells-git-proxy")
	if _, err := os.Stat(proxyScriptPath); os.IsNotExist(err) {
		t.Error("Proxy script was not created")
	}

	// Verify the hook script was written
	hookScriptPath := filepath.Join(claudeDir, "bin", "ccells-git-hook")
	if _, err := os.Stat(hookScriptPath); os.IsNotExist(err) {
		t.Error("Hook script was not created")
	}

	// Read and parse the settings.json
	settingsPath := filepath.Join(claudeDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read settings.json: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("Failed to parse settings.json: %v", err)
	}

	// Verify the hook structure uses PreToolUse, not Bash
	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("settings.hooks is not a map")
	}

	// Should NOT have a "Bash" key (that's the bug we're fixing)
	if _, hasBash := hooks["Bash"]; hasBash {
		t.Error("settings.json has invalid 'Bash' hook event type - should be 'PreToolUse' with matcher 'Bash'")
	}

	// Should have PreToolUse with Bash matcher
	preToolUse, ok := hooks["PreToolUse"].([]interface{})
	if !ok {
		t.Fatal("settings.hooks.PreToolUse is not an array")
	}

	// Find the Bash matcher hook
	var foundBashMatcher bool
	for _, h := range preToolUse {
		hookMap, ok := h.(map[string]interface{})
		if !ok {
			continue
		}
		if matcher, ok := hookMap["matcher"].(string); ok && matcher == "Bash" {
			foundBashMatcher = true
			// Verify the hook has a command
			hooksArray, ok := hookMap["hooks"].([]interface{})
			if !ok || len(hooksArray) == 0 {
				t.Error("Bash matcher hook has no hooks array")
			}
			break
		}
	}

	if !foundBashMatcher {
		t.Error("No PreToolUse hook with matcher 'Bash' found")
	}

	// Verify bypassPermissions mode is set (skips workspace trust dialog in containers)
	if mode, ok := settings["defaultMode"].(string); !ok || mode != "bypassPermissions" {
		t.Errorf("defaultMode = %v, want 'bypassPermissions'", settings["defaultMode"])
	}
}

func TestInjectProxyConfig_RemovesInvalidBashKey(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("Failed to create claude dir: %v", err)
	}

	// Write a settings.json with the invalid "Bash" hook event type (the old bug)
	oldSettings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"Bash": []interface{}{
				map[string]interface{}{
					"matcher": "^git\\s+(fetch|pull|push)",
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": "/some/old/path",
						},
					},
				},
			},
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Edit|Write",
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": "some-other-hook",
						},
					},
				},
			},
		},
	}

	oldData, _ := json.MarshalIndent(oldSettings, "", "  ")
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, oldData, 0644); err != nil {
		t.Fatalf("Failed to write old settings: %v", err)
	}

	// Run the injection
	if err := InjectProxyConfig(claudeDir); err != nil {
		t.Fatalf("InjectProxyConfig failed: %v", err)
	}

	// Read and parse the updated settings.json
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read settings.json: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("Failed to parse settings.json: %v", err)
	}

	hooks := settings["hooks"].(map[string]interface{})

	// The invalid "Bash" key should be removed
	if _, hasBash := hooks["Bash"]; hasBash {
		t.Error("Invalid 'Bash' hook event type was not removed")
	}

	// PreToolUse should still exist and contain the existing hook plus our new one
	preToolUse, ok := hooks["PreToolUse"].([]interface{})
	if !ok {
		t.Fatal("settings.hooks.PreToolUse is not an array")
	}

	// Should have both the existing Edit|Write hook and our new Bash hook
	var foundEditWrite, foundBash bool
	for _, h := range preToolUse {
		hookMap, ok := h.(map[string]interface{})
		if !ok {
			continue
		}
		matcher, _ := hookMap["matcher"].(string)
		if matcher == "Edit|Write" {
			foundEditWrite = true
		}
		if matcher == "Bash" {
			foundBash = true
		}
	}

	if !foundEditWrite {
		t.Error("Existing Edit|Write hook was not preserved")
	}
	if !foundBash {
		t.Error("New Bash matcher hook was not added")
	}
}

func TestInjectProxyConfig_MergesWithExistingBashMatcher(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("Failed to create claude dir: %v", err)
	}

	// Write a settings.json with an existing Bash matcher hook (like block-amend-pushed.sh)
	existingSettings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Bash",
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": "/some/existing/hook.sh",
						},
					},
				},
			},
		},
	}

	data, _ := json.MarshalIndent(existingSettings, "", "  ")
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatalf("Failed to write existing settings: %v", err)
	}

	// Run the injection
	if err := InjectProxyConfig(claudeDir); err != nil {
		t.Fatalf("InjectProxyConfig failed: %v", err)
	}

	// Read and parse the updated settings.json
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read settings.json: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("Failed to parse settings.json: %v", err)
	}

	hooks := settings["hooks"].(map[string]interface{})
	preToolUse := hooks["PreToolUse"].([]interface{})

	// Should have exactly one Bash matcher
	var bashMatcherCount int
	var bashHooks []interface{}
	for _, h := range preToolUse {
		hookMap, ok := h.(map[string]interface{})
		if !ok {
			continue
		}
		if matcher, ok := hookMap["matcher"].(string); ok && matcher == "Bash" {
			bashMatcherCount++
			bashHooks = hookMap["hooks"].([]interface{})
		}
	}

	if bashMatcherCount != 1 {
		t.Errorf("Expected exactly 1 Bash matcher, got %d", bashMatcherCount)
	}

	// The Bash matcher should have TWO hook commands - existing + git proxy
	if len(bashHooks) != 2 {
		t.Errorf("Expected 2 hooks in Bash matcher (existing + git proxy), got %d", len(bashHooks))
	}

	// Verify both hooks are present
	var foundExisting, foundGitProxy bool
	for _, h := range bashHooks {
		hookMap, ok := h.(map[string]interface{})
		if !ok {
			continue
		}
		cmd, _ := hookMap["command"].(string)
		if cmd == "/some/existing/hook.sh" {
			foundExisting = true
		}
		if cmd == "/root/.claude/bin/ccells-git-hook" {
			foundGitProxy = true
		}
	}

	if !foundExisting {
		t.Error("Existing hook was not preserved")
	}
	if !foundGitProxy {
		t.Error("Git proxy hook was not added")
	}
}

func TestInjectProxyConfig_Idempotent(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("Failed to create claude dir: %v", err)
	}

	// Run the injection twice
	if err := InjectProxyConfig(claudeDir); err != nil {
		t.Fatalf("First InjectProxyConfig failed: %v", err)
	}

	if err := InjectProxyConfig(claudeDir); err != nil {
		t.Fatalf("Second InjectProxyConfig failed: %v", err)
	}

	// Read and parse the settings.json
	settingsPath := filepath.Join(claudeDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read settings.json: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("Failed to parse settings.json: %v", err)
	}

	hooks := settings["hooks"].(map[string]interface{})
	preToolUse := hooks["PreToolUse"].([]interface{})

	// Count Bash matchers - should only be one
	var bashCount int
	for _, h := range preToolUse {
		hookMap, ok := h.(map[string]interface{})
		if !ok {
			continue
		}
		if matcher, ok := hookMap["matcher"].(string); ok && matcher == "Bash" {
			bashCount++
		}
	}

	if bashCount != 1 {
		t.Errorf("Expected exactly 1 Bash matcher hook, got %d (not idempotent)", bashCount)
	}
}
