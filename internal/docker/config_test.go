package docker

import (
	"os"
	"path/filepath"
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

// Note: TestInitClaudeConfig_OverwritesExisting is skipped because it requires writing
// to ~/.ccells which may be outside the sandbox.
