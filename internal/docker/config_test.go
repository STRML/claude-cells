package docker

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestCreateContainerConfig_CopiesSneakpeekForClaudesp(t *testing.T) {
	// Setup temp home directory
	tmpHome := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", oldHome)

	// Create .claude-sneakpeek directory with a test file
	sneakpeekDir := filepath.Join(tmpHome, ".claude-sneakpeek")
	if err := os.MkdirAll(sneakpeekDir, 0755); err != nil {
		t.Fatalf("Failed to create .claude-sneakpeek: %v", err)
	}
	testFile := filepath.Join(sneakpeekDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("sneakpeek test"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create container config with claudesp runtime
	cfg, err := CreateContainerConfig("test-container", "claudesp")
	if err != nil {
		t.Fatalf("CreateContainerConfig() error = %v", err)
	}

	// Verify SneakpeekDir is set
	if cfg.SneakpeekDir == "" {
		t.Error("SneakpeekDir should be set for claudesp runtime")
	}

	// Verify .claude-sneakpeek was copied
	copiedFile := filepath.Join(cfg.SneakpeekDir, "test.txt")
	data, err := os.ReadFile(copiedFile)
	if err != nil {
		t.Fatalf("Failed to read copied sneakpeek file: %v", err)
	}
	if string(data) != "sneakpeek test" {
		t.Errorf("Copied file content = %q, want %q", data, "sneakpeek test")
	}
}

func TestCreateContainerConfig_NoSneakpeekForClaude(t *testing.T) {
	// Setup temp home directory
	tmpHome := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", oldHome)

	// Create container config with claude runtime (default)
	cfg, err := CreateContainerConfig("test-container", "claude")
	if err != nil {
		t.Fatalf("CreateContainerConfig() error = %v", err)
	}

	// Verify SneakpeekDir is empty for standard claude runtime
	if cfg.SneakpeekDir != "" {
		t.Errorf("SneakpeekDir should be empty for claude runtime, got %q", cfg.SneakpeekDir)
	}
}

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
// NOTE: This test is slow (~4s) because it copies the entire ~/.claude directory.
// Run with: go test -tags=integration ./internal/docker/...
func TestInitClaudeConfig_CreatesCredentialsInClaudeDir(t *testing.T) {
	t.Skip("slow test - copies ~/.claude directory; run with -tags=integration")

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

// TestContainerConfig tests the NewContainerConfig function (no Docker required).
func TestContainerConfig(t *testing.T) {
	tests := []struct {
		name            string
		branchName      string
		repoPath        string
		wantPrefix      string
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:         "creates config with branch name and project",
			branchName:   "add-auth",
			repoPath:     "/path/to/repo",
			wantPrefix:   "ccells-",
			wantContains: []string{"repo", "add-auth"},
		},
		{
			name:            "sanitizes slashes in branch name",
			branchName:      "feature/add-auth",
			repoPath:        "/path/to/repo",
			wantPrefix:      "ccells-",
			wantContains:    []string{"repo", "feature-add-auth"},
			wantNotContains: []string{"/"},
		},
		{
			name:         "sanitizes spaces in branch name",
			branchName:   "my feature",
			repoPath:     "/path/to/project",
			wantPrefix:   "ccells-",
			wantContains: []string{"project", "my-feature"},
		},
		{
			name:         "handles empty repo path",
			branchName:   "test",
			repoPath:     "",
			wantPrefix:   "ccells-",
			wantContains: []string{"workspace", "test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewContainerConfig(tt.branchName, tt.repoPath)

			if !strings.HasPrefix(cfg.Name, tt.wantPrefix) {
				t.Errorf("Name = %q, want prefix %q", cfg.Name, tt.wantPrefix)
			}

			for _, part := range tt.wantContains {
				if !strings.Contains(cfg.Name, part) {
					t.Errorf("Name = %q, want to contain %q", cfg.Name, part)
				}
			}

			for _, part := range tt.wantNotContains {
				if strings.Contains(cfg.Name, part) {
					t.Errorf("Name = %q, should not contain %q", cfg.Name, part)
				}
			}

			if !strings.Contains(cfg.Name, "-202") {
				t.Errorf("Name = %q, should contain timestamp", cfg.Name)
			}

			if tt.repoPath != "" && cfg.RepoPath != tt.repoPath {
				t.Errorf("RepoPath = %q, want %q", cfg.RepoPath, tt.repoPath)
			}
		})
	}
}

// TestGetHostTimezone tests the timezone detection function.
func TestGetHostTimezone(t *testing.T) {
	// This test verifies GetHostTimezone doesn't crash and returns sensible values.
	// The actual timezone depends on the host system's configuration.
	tz := GetHostTimezone()

	// GetHostTimezone can return empty string if timezone cannot be determined
	if tz == "" {
		t.Log("No timezone detected on this system (TZ env not set and /etc/localtime not a symlink to zoneinfo)")
		return
	}

	// Verify it looks like a valid timezone (contains / for region/city format)
	// e.g., "America/New_York", "Europe/London", "UTC"
	t.Logf("Detected timezone: %q", tz)

	// Most timezones have the format Region/City, but some like "UTC" are single words
	// Just verify it's not suspiciously short or containing path separators beyond the timezone format
	if len(tz) < 2 {
		t.Errorf("Timezone %q seems too short to be valid", tz)
	}
}

// TestGetHostTimezoneFromEnv tests that TZ env var takes precedence.
func TestGetHostTimezoneFromEnv(t *testing.T) {
	// Save and restore original TZ
	originalTZ := os.Getenv("TZ")
	defer func() {
		if originalTZ == "" {
			os.Unsetenv("TZ")
		} else {
			os.Setenv("TZ", originalTZ)
		}
	}()

	// Set TZ environment variable
	os.Setenv("TZ", "America/Los_Angeles")

	tz := GetHostTimezone()
	if tz != "America/Los_Angeles" {
		t.Errorf("GetHostTimezone() = %q, want %q", tz, "America/Los_Angeles")
	}
}

// TestGetGitIdentity tests the git identity detection function.
func TestGetGitIdentity(t *testing.T) {
	// This test verifies GetGitIdentity doesn't crash and returns sensible values.
	// The actual identity values depend on the host system's git configuration.
	identity := GetGitIdentity()

	// GetGitIdentity can return nil if git is not configured
	if identity == nil {
		t.Log("No git identity configured on this system")
		return
	}

	// If we got an identity, at least one of name or email should be set
	if identity.Name == "" && identity.Email == "" {
		t.Error("GetGitIdentity returned non-nil but both Name and Email are empty")
	}

	t.Logf("Git identity: Name=%q, Email=%q", identity.Name, identity.Email)
}

// TestGitIdentityStruct tests the GitIdentity struct fields.
func TestGitIdentityStruct(t *testing.T) {
	identity := &GitIdentity{
		Name:  "Test User",
		Email: "test@example.com",
	}

	if identity.Name != "Test User" {
		t.Errorf("Name = %q, want %q", identity.Name, "Test User")
	}
	if identity.Email != "test@example.com" {
		t.Errorf("Email = %q, want %q", identity.Email, "test@example.com")
	}
}
