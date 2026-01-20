package docker

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDevcontainerCLIStatus_Fields(t *testing.T) {
	// Test struct fields
	status := DevcontainerCLIStatus{
		Available: true,
		Version:   "0.50.0",
		Path:      "/usr/local/bin/devcontainer",
	}

	if !status.Available {
		t.Error("Available should be true")
	}
	if status.Version != "0.50.0" {
		t.Errorf("Version = %q, want %q", status.Version, "0.50.0")
	}
	if status.Path != "/usr/local/bin/devcontainer" {
		t.Errorf("Path = %q, want %q", status.Path, "/usr/local/bin/devcontainer")
	}
}

func TestCheckDevcontainerCLI(t *testing.T) {
	status := CheckDevcontainerCLI()

	// We can't know if devcontainer is installed, but we can verify the struct is valid
	if status.Available {
		// If available, path should be set
		if status.Path == "" {
			t.Error("Path should be set when Available is true")
		}
		// Version might be empty if --version fails but command exists
	} else {
		// If not available, path should be empty
		if status.Path != "" {
			t.Errorf("Path should be empty when not available, got %q", status.Path)
		}
	}
}

func TestDevcontainerCLIInstallInstructions(t *testing.T) {
	instructions := DevcontainerCLIInstallInstructions()

	// Verify instructions contain expected content
	if !strings.Contains(instructions, "npm install") {
		t.Error("Instructions should mention npm install")
	}
	if !strings.Contains(instructions, "@devcontainers/cli") {
		t.Error("Instructions should mention @devcontainers/cli package")
	}
	if !strings.Contains(instructions, "brew") {
		t.Error("Instructions should mention Homebrew option")
	}

	// Verify it's a non-empty string
	if len(instructions) < 50 {
		t.Errorf("Instructions seem too short: %q", instructions)
	}
}

func TestHasDevcontainerConfig(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "devcontainer-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test without devcontainer.json
	if HasDevcontainerConfig(tmpDir) {
		t.Error("Should return false when no devcontainer.json exists")
	}

	// Create .devcontainer directory and devcontainer.json
	devcontainerDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("Failed to create .devcontainer dir: %v", err)
	}

	devcontainerJSON := `{
		"name": "test",
		"image": "ubuntu:22.04"
	}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(devcontainerJSON), 0644); err != nil {
		t.Fatalf("Failed to write devcontainer.json: %v", err)
	}

	// Test with devcontainer.json
	if !HasDevcontainerConfig(tmpDir) {
		t.Error("Should return true when devcontainer.json exists")
	}
}

func TestHasDevcontainerConfig_RootLevel(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "devcontainer-root-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create root-level .devcontainer.json
	devcontainerJSON := `{
		"name": "test",
		"image": "ubuntu:22.04"
	}`
	if err := os.WriteFile(filepath.Join(tmpDir, ".devcontainer.json"), []byte(devcontainerJSON), 0644); err != nil {
		t.Fatalf("Failed to write .devcontainer.json: %v", err)
	}

	// Test with root-level .devcontainer.json
	if !HasDevcontainerConfig(tmpDir) {
		t.Error("Should return true when root-level .devcontainer.json exists")
	}
}

func TestHasDevcontainerFeatures(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "devcontainer-features-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test without any config
	if HasDevcontainerFeatures(tmpDir) {
		t.Error("Should return false when no devcontainer.json exists")
	}

	// Create .devcontainer directory
	devcontainerDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("Failed to create .devcontainer dir: %v", err)
	}

	// Create devcontainer.json WITHOUT features
	devcontainerJSON := `{
		"name": "test",
		"image": "ubuntu:22.04"
	}`
	devcontainerPath := filepath.Join(devcontainerDir, "devcontainer.json")
	if err := os.WriteFile(devcontainerPath, []byte(devcontainerJSON), 0644); err != nil {
		t.Fatalf("Failed to write devcontainer.json: %v", err)
	}

	if HasDevcontainerFeatures(tmpDir) {
		t.Error("Should return false when devcontainer.json has no features")
	}

	// Update with features
	devcontainerJSONWithFeatures := `{
		"name": "test",
		"image": "ubuntu:22.04",
		"features": {
			"ghcr.io/devcontainers/features/go:1": {}
		}
	}`
	if err := os.WriteFile(devcontainerPath, []byte(devcontainerJSONWithFeatures), 0644); err != nil {
		t.Fatalf("Failed to update devcontainer.json: %v", err)
	}

	if !HasDevcontainerFeatures(tmpDir) {
		t.Error("Should return true when devcontainer.json has features")
	}
}

func TestHasFeatures(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "features-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test without config
	if hasFeatures(tmpDir) {
		t.Error("Should return false when no config exists")
	}

	// Create .devcontainer directory
	devcontainerDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("Failed to create .devcontainer dir: %v", err)
	}

	// Test with config but no features key
	configNoFeatures := `{"name": "test", "image": "ubuntu:22.04"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(configNoFeatures), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	if hasFeatures(tmpDir) {
		t.Error("Should return false when features key is missing")
	}

	// Test with features key
	configWithFeatures := `{"name": "test", "image": "ubuntu:22.04", "features": {}}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(configWithFeatures), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	if !hasFeatures(tmpDir) {
		t.Error("Should return true when features key exists")
	}
}

func TestReadDevcontainerJSON(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "read-devcontainer-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test without config
	_, err = readDevcontainerJSON(tmpDir)
	if err == nil {
		t.Error("Should return error when no devcontainer.json exists")
	}

	// Create .devcontainer directory
	devcontainerDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("Failed to create .devcontainer dir: %v", err)
	}

	// Create devcontainer.json
	expectedContent := `{"name": "test", "image": "ubuntu:22.04"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(expectedContent), 0644); err != nil {
		t.Fatalf("Failed to write devcontainer.json: %v", err)
	}

	// Read it
	content, err := readDevcontainerJSON(tmpDir)
	if err != nil {
		t.Errorf("Should successfully read devcontainer.json: %v", err)
	}
	if string(content) != expectedContent {
		t.Errorf("Content = %q, want %q", string(content), expectedContent)
	}
}

func TestReadDevcontainerJSON_RootLevel(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "read-devcontainer-root-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create root-level .devcontainer.json
	expectedContent := `{"name": "root-test", "image": "ubuntu:22.04"}`
	if err := os.WriteFile(filepath.Join(tmpDir, ".devcontainer.json"), []byte(expectedContent), 0644); err != nil {
		t.Fatalf("Failed to write .devcontainer.json: %v", err)
	}

	// Read it
	content, err := readDevcontainerJSON(tmpDir)
	if err != nil {
		t.Errorf("Should successfully read root-level .devcontainer.json: %v", err)
	}
	if string(content) != expectedContent {
		t.Errorf("Content = %q, want %q", string(content), expectedContent)
	}
}

func TestBuildWithDevcontainerCLI_NoCLI(t *testing.T) {
	// This test verifies behavior when devcontainer CLI is not available
	status := CheckDevcontainerCLI()
	if status.Available {
		t.Skip("Skipping test because devcontainer CLI is available")
	}

	// Create temp directory with devcontainer config
	tmpDir, err := os.MkdirTemp("", "build-no-cli-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var output bytes.Buffer
	_, err = BuildWithDevcontainerCLI(ctx, tmpDir, &output)
	if err == nil {
		t.Error("Should return error when devcontainer CLI is not available")
	}
}

func TestBuildWithDevcontainerCLI_ContextCancellation(t *testing.T) {
	// Test that BuildWithDevcontainerCLI respects context cancellation
	status := CheckDevcontainerCLI()
	if !status.Available {
		t.Skip("Skipping test because devcontainer CLI is not available")
	}

	tmpDir, err := os.MkdirTemp("", "build-cancel-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a minimal devcontainer.json
	devcontainerDir := filepath.Join(tmpDir, ".devcontainer")
	os.MkdirAll(devcontainerDir, 0755)
	os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(`{"name":"test","image":"ubuntu:22.04"}`), 0644)

	// Create an already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var output bytes.Buffer
	_, err = BuildWithDevcontainerCLI(ctx, tmpDir, &output)
	// Should fail due to cancelled context (either during exec or start)
	if err == nil {
		t.Error("Should return error when context is cancelled")
	}
}

// Integration test - only runs if devcontainer CLI is available
func TestBuildWithDevcontainerCLI_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	status := CheckDevcontainerCLI()
	if !status.Available {
		t.Skip("Skipping integration test because devcontainer CLI is not available")
	}

	// Create temp directory with valid devcontainer config
	tmpDir, err := os.MkdirTemp("", "build-integration-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .devcontainer directory
	devcontainerDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("Failed to create .devcontainer dir: %v", err)
	}

	// Create a minimal devcontainer.json
	devcontainerJSON := `{
		"name": "test-build",
		"image": "ubuntu:22.04"
	}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(devcontainerJSON), 0644); err != nil {
		t.Fatalf("Failed to write devcontainer.json: %v", err)
	}

	// Initialize git repo (required by devcontainer CLI)
	exec.Command("git", "-C", tmpDir, "init").Run()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var output bytes.Buffer
	imageName, err := BuildWithDevcontainerCLI(ctx, tmpDir, &output)

	// The build might fail due to Docker not being available, but the command should start
	if err != nil {
		// Check if it's a "devcontainer not found" error vs Docker error
		if strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "docker") {
			t.Errorf("Unexpected error: %v", err)
		}
		// Docker-related errors are acceptable in test environment
	} else {
		if imageName == "" {
			t.Error("Image name should not be empty on success")
		}
	}
}

func TestDevcontainerConfigPaths(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "config-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test both possible locations are checked
	// .devcontainer/devcontainer.json takes precedence

	// Create root-level .devcontainer.json first
	rootConfig := `{"name": "root-config"}`
	if err := os.WriteFile(filepath.Join(tmpDir, ".devcontainer.json"), []byte(rootConfig), 0644); err != nil {
		t.Fatalf("Failed to write root config: %v", err)
	}

	content, err := readDevcontainerJSON(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read root config: %v", err)
	}
	if !strings.Contains(string(content), "root-config") {
		t.Error("Should read root-level config when it's the only one")
	}

	// Now create .devcontainer/devcontainer.json - it should take precedence
	devcontainerDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("Failed to create .devcontainer dir: %v", err)
	}

	dirConfig := `{"name": "dir-config"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(dirConfig), 0644); err != nil {
		t.Fatalf("Failed to write dir config: %v", err)
	}

	content, err = readDevcontainerJSON(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}
	// .devcontainer/devcontainer.json should take precedence (it's checked first)
	if !strings.Contains(string(content), "dir-config") {
		t.Error(".devcontainer/devcontainer.json should take precedence over root-level")
	}
}

func TestHasDevcontainerConfig_InvalidJSON(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "invalid-json-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .devcontainer directory
	devcontainerDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("Failed to create .devcontainer dir: %v", err)
	}

	// Create invalid JSON
	invalidJSON := `{invalid json`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(invalidJSON), 0644); err != nil {
		t.Fatalf("Failed to write invalid json: %v", err)
	}

	// HasDevcontainerConfig uses LoadDevcontainerConfig which parses JSON
	// It should return false for invalid JSON
	result := HasDevcontainerConfig(tmpDir)
	if result {
		t.Error("Should return false for invalid JSON")
	}
}

func TestHasDevcontainerFeatures_EmptyFeaturesObject(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "empty-features-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .devcontainer directory
	devcontainerDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("Failed to create .devcontainer dir: %v", err)
	}

	// Create config with empty features object
	// The check looks for the string "features", so empty object still counts
	emptyFeatures := `{"name": "test", "features": {}}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(emptyFeatures), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	if !HasDevcontainerFeatures(tmpDir) {
		t.Error("Should return true even for empty features object (string match)")
	}
}

// BenchmarkCheckDevcontainerCLI benchmarks the CLI check
func BenchmarkCheckDevcontainerCLI(b *testing.B) {
	for i := 0; i < b.N; i++ {
		CheckDevcontainerCLI()
	}
}

// BenchmarkHasDevcontainerConfig benchmarks the config check
func BenchmarkHasDevcontainerConfig(b *testing.B) {
	// Create temp directory with config
	tmpDir, _ := os.MkdirTemp("", "bench-config-*")
	defer os.RemoveAll(tmpDir)

	devcontainerDir := filepath.Join(tmpDir, ".devcontainer")
	os.MkdirAll(devcontainerDir, 0755)
	os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(`{"name":"test"}`), 0644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HasDevcontainerConfig(tmpDir)
	}
}
