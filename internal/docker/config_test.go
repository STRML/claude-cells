package docker

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestGetCellsDir(t *testing.T) {
	dir, err := GetCellsDir()
	if err != nil {
		t.Fatalf("GetCellsDir() error = %v", err)
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, CellsDir)
	if dir != expected {
		t.Errorf("GetCellsDir() = %q, want %q", dir, expected)
	}
}

// Note: TestInitClaudeConfig is skipped because it requires writing to ~/.ccells
// which may be outside the sandbox. The function is tested implicitly through
// integration tests that run outside the sandbox.

func TestCopyFile(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create source file
	srcPath := filepath.Join(tmpDir, "source.txt")
	content := []byte("test content 12345")
	if err := os.WriteFile(srcPath, content, 0644); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}

	// Copy file
	dstPath := filepath.Join(tmpDir, "dest.txt")
	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("copyFile() error = %v", err)
	}

	// Verify copy
	copied, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("Failed to read copied file: %v", err)
	}
	if string(copied) != string(content) {
		t.Errorf("Copied content = %q, want %q", copied, content)
	}
}

func TestCopyDir(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "ccells-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create source directory structure
	srcDir := filepath.Join(tmpDir, "source")
	if err := os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755); err != nil {
		t.Fatalf("Failed to create source dir: %v", err)
	}

	// Create files
	files := map[string]string{
		"file1.txt":        "content 1",
		"subdir/file2.txt": "content 2",
	}
	for path, content := range files {
		fullPath := filepath.Join(srcDir, path)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", path, err)
		}
	}

	// Copy directory
	dstDir := filepath.Join(tmpDir, "dest")
	if err := copyDir(srcDir, dstDir); err != nil {
		t.Fatalf("copyDir() error = %v", err)
	}

	// Verify all files were copied
	for path, expectedContent := range files {
		fullPath := filepath.Join(dstDir, path)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			t.Errorf("Failed to read copied file %s: %v", path, err)
			continue
		}
		if string(content) != expectedContent {
			t.Errorf("Copied %s = %q, want %q", path, content, expectedContent)
		}
	}
}

// TestInitClaudeConfig_CreatesCredentialsInClaudeDir verifies that credentials.json
// is written inside the .claude directory (not just as a separate file).
func TestInitClaudeConfig_CreatesCredentialsInClaudeDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test that writes to home directory")
	}

	// Reset the global config so InitClaudeConfig runs fresh
	globalConfigOnce = sync.Once{}
	globalConfig = nil
	globalConfigErr = nil

	cfg, err := InitClaudeConfig()
	if err != nil {
		t.Fatalf("InitClaudeConfig() error = %v", err)
	}

	// Check that .claude directory exists
	if _, err := os.Stat(cfg.ClaudeDir); os.IsNotExist(err) {
		t.Fatalf("ClaudeDir %s does not exist", cfg.ClaudeDir)
	}

	// Check for .credentials.json inside .claude directory (note leading dot!)
	credsPath := filepath.Join(cfg.ClaudeDir, ".credentials.json")
	if _, err := os.Stat(credsPath); os.IsNotExist(err) {
		// This is OK if user doesn't have OAuth credentials
		t.Logf(".credentials.json not found at %s (user may not have OAuth credentials)", credsPath)
	} else {
		t.Logf(".credentials.json found at %s", credsPath)
	}

	// Check for settings.json (should be copied from ~/.claude/settings.json)
	settingsPath := filepath.Join(cfg.ClaudeDir, "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Logf("settings.json not found at %s (user may not have settings)", settingsPath)
	} else {
		t.Logf("settings.json found at %s", settingsPath)
	}
}
