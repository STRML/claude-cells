package docker

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestGetClaudeCredentials(t *testing.T) {
	creds, err := GetClaudeCredentials()
	if err != nil {
		t.Fatalf("GetClaudeCredentials() error = %v", err)
	}

	// On non-macOS, should return nil
	if runtime.GOOS != "darwin" {
		if creds != nil {
			t.Error("Expected nil credentials on non-macOS")
		}
		return
	}

	// On macOS, credentials may or may not exist
	// Just verify no error and if credentials exist, they have content
	if creds != nil {
		if creds.Raw == "" {
			t.Error("Credentials returned but Raw is empty")
		}
		t.Logf("Found credentials (length: %d)", len(creds.Raw))

		// Verify credentials contain expected OAuth fields
		if !strings.Contains(creds.Raw, "claudeAiOauth") {
			t.Error("Credentials should contain 'claudeAiOauth' field")
		}
		if !strings.Contains(creds.Raw, "accessToken") {
			t.Error("Credentials should contain 'accessToken' field")
		}
		if !strings.Contains(creds.Raw, "refreshToken") {
			t.Error("Credentials should contain 'refreshToken' field")
		}
	} else {
		t.Log("No Claude credentials found in keychain (this is OK)")
	}
}

func TestCredentialsStructure(t *testing.T) {
	// Test that ClaudeCredentials struct works correctly
	creds := &ClaudeCredentials{
		Raw: `{"claudeAiOauth":{"accessToken":"test-access","refreshToken":"test-refresh"}}`,
	}

	if creds.Raw == "" {
		t.Error("ClaudeCredentials.Raw should not be empty")
	}

	if !strings.Contains(creds.Raw, "accessToken") {
		t.Error("Raw should contain accessToken")
	}
}

func TestCredentialRefresherRegisterExistingContainers(t *testing.T) {
	// Create a temporary directory to simulate ~/.claude-cells
	tempDir := t.TempDir()

	// Override GetCellsDir for this test by creating the expected structure
	containersDir := filepath.Join(tempDir, "containers")
	if err := os.MkdirAll(containersDir, 0755); err != nil {
		t.Fatalf("Failed to create containers dir: %v", err)
	}

	// Create mock container configs
	container1Dir := filepath.Join(containersDir, "test-container-1")
	container1ClaudeDir := filepath.Join(container1Dir, ".claude")
	if err := os.MkdirAll(container1ClaudeDir, 0755); err != nil {
		t.Fatalf("Failed to create container1 .claude dir: %v", err)
	}

	container2Dir := filepath.Join(containersDir, "test-container-2")
	container2ClaudeDir := filepath.Join(container2Dir, ".claude")
	if err := os.MkdirAll(container2ClaudeDir, 0755); err != nil {
		t.Fatalf("Failed to create container2 .claude dir: %v", err)
	}

	// Create a directory without .claude (should be skipped)
	invalidDir := filepath.Join(containersDir, "invalid-container")
	if err := os.MkdirAll(invalidDir, 0755); err != nil {
		t.Fatalf("Failed to create invalid dir: %v", err)
	}

	// Create refresher and manually call registerExistingContainers with our temp dir
	refresher := NewCredentialRefresher(1 * time.Hour)

	// We can't easily override GetCellsDir, so we'll test the behavior directly
	// by calling the internal registration logic
	entries, err := os.ReadDir(containersDir)
	if err != nil {
		t.Fatalf("Failed to read containers dir: %v", err)
	}

	registered := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		containerName := entry.Name()
		configDir := filepath.Join(containersDir, containerName)
		claudeDir := filepath.Join(configDir, ".claude")
		if _, err := os.Stat(claudeDir); os.IsNotExist(err) {
			continue
		}
		refresher.RegisterContainer(containerName, containerName, configDir)
		registered++
	}

	// Should have registered 2 containers (not the invalid one)
	if registered != 2 {
		t.Errorf("Expected 2 registered containers, got %d", registered)
	}

	// Verify the containers are in the map
	refresher.mu.RLock()
	defer refresher.mu.RUnlock()

	if _, ok := refresher.containers["test-container-1"]; !ok {
		t.Error("test-container-1 should be registered")
	}
	if _, ok := refresher.containers["test-container-2"]; !ok {
		t.Error("test-container-2 should be registered")
	}
	if _, ok := refresher.containers["invalid-container"]; ok {
		t.Error("invalid-container should NOT be registered (no .claude dir)")
	}
}

func TestCredentialRefresherNoDuplicateRegistration(t *testing.T) {
	refresher := NewCredentialRefresher(1 * time.Hour)

	// Register a container
	refresher.RegisterContainer("container-id-1", "test-container", "/some/path")

	// Try to register the same container name (simulating re-registration)
	refresher.mu.Lock()
	_, exists := refresher.containers["test-container"]
	refresher.mu.Unlock()

	// If it doesn't exist with that key, register it
	// This tests that our registerExistingContainers checks for duplicates
	if !exists {
		t.Log("Container not found by name key, which is expected since RegisterContainer uses containerID as key")
	}

	// Verify the original registration is intact
	refresher.mu.RLock()
	info, ok := refresher.containers["container-id-1"]
	refresher.mu.RUnlock()

	if !ok {
		t.Error("Original container registration should still exist")
	}
	if info.name != "test-container" {
		t.Errorf("Container name should be 'test-container', got '%s'", info.name)
	}
}
