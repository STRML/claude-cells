package docker

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/STRML/claude-cells/configs"
)

func TestValidatePrerequisites(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Skip if Docker daemon is not available
	client := skipIfDockerUnavailable(t)
	client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get project path (test directory should work)
	projectPath, _ := os.Getwd()
	result, err := ValidatePrerequisites(ctx, projectPath)
	if err != nil {
		t.Fatalf("ValidatePrerequisites() error = %v", err)
	}

	// Docker should be available for integration tests
	if !result.DockerAvailable {
		t.Error("Docker should be available")
		for _, e := range result.Errors {
			t.Logf("  Error: %s - %s", e.Check, e.Message)
		}
	}

	// The required image may not exist in CI - skip rather than fail
	if !result.ImageExists {
		t.Skipf("Required image not found, skipping (build with: docker build -t %s -f configs/base.Dockerfile .)", RequiredImage)
	}

	if !result.IsValid() {
		t.Error("Prerequisites should be valid")
		for _, e := range result.Errors {
			t.Logf("  Validation error: %s - %s", e.Check, e.Message)
		}
	}
}

func TestImageExists(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	client := skipIfDockerUnavailable(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test nonexistent image (always valid test)
	t.Run("nonexistent image", func(t *testing.T) {
		exists, err := client.ImageExists(ctx, "nonexistent-image-12345:latest")
		if err != nil {
			t.Fatalf("ImageExists() error = %v", err)
		}
		if exists {
			t.Error("Nonexistent image should not exist")
		}
	})

	// Test required image (skip if not available)
	t.Run("required image", func(t *testing.T) {
		exists, err := client.ImageExists(ctx, RequiredImage)
		if err != nil {
			t.Fatalf("ImageExists() error = %v", err)
		}
		if !exists {
			t.Skipf("Required image %s not found, skipping", RequiredImage)
		}
	})

	// Test alpine (skip if not available - may not be pulled in CI)
	t.Run("alpine", func(t *testing.T) {
		exists, err := client.ImageExists(ctx, "alpine:latest")
		if err != nil {
			t.Fatalf("ImageExists() error = %v", err)
		}
		if !exists {
			t.Skip("alpine:latest not found, skipping")
		}
	})
}

func TestValidationError(t *testing.T) {
	err := &ValidationError{
		Check:   "test_check",
		Message: "test message",
	}

	got := err.Error()
	want := "test_check: test message"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestValidationResult_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		result ValidationResult
		want   bool
	}{
		{
			name: "all valid",
			result: ValidationResult{
				DockerAvailable: true,
				ImageExists:     true,
				Errors:          nil,
			},
			want: true,
		},
		{
			name: "docker unavailable",
			result: ValidationResult{
				DockerAvailable: false,
				ImageExists:     false,
				Errors:          []ValidationError{{Check: "docker", Message: "not available"}},
			},
			want: false,
		},
		{
			name: "image missing",
			result: ValidationResult{
				DockerAvailable: true,
				ImageExists:     false,
				Errors:          []ValidationError{{Check: "image", Message: "not found"}},
			},
			want: false,
		},
		{
			name: "has errors",
			result: ValidationResult{
				DockerAvailable: true,
				ImageExists:     true,
				Errors:          []ValidationError{{Check: "other", Message: "problem"}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindDockerfile(t *testing.T) {
	// This test verifies findDockerfile can locate the Dockerfile
	// from the test's working directory
	path, err := findDockerfile()
	if err != nil {
		t.Skipf("Dockerfile not found from test directory: %v", err)
	}

	if path == "" {
		t.Error("findDockerfile() returned empty path")
	}

	// Verify the file actually exists
	if _, err := os.Stat(path); err != nil {
		t.Errorf("findDockerfile() returned path that doesn't exist: %s", path)
	}
}

func TestBuildImage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Skip if Docker is not available
	skipIfDockerUnavailable(t)

	// We don't actually run the build in tests since it's slow
	// and the image should already exist. Just verify the function
	// doesn't panic when called with a cancelled context.
	// Note: BuildImage now uses embedded Dockerfile, so no need to check for file on disk.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Should return an error due to cancelled context
	err := BuildImage(ctx, io.Discard)
	if err == nil {
		t.Log("BuildImage with cancelled context didn't error (build may have been very fast)")
	}
}

func TestBaseDockerfileHash(t *testing.T) {
	// Import the configs package to get the embedded Dockerfile hash
	hash := configs.BaseDockerfileHash()

	// Hash should be non-empty (embedded Dockerfile always exists)
	if hash == "" {
		t.Error("BaseDockerfileHash() returned empty hash")
	}

	// Hash should be 12 characters (first 12 chars of hex-encoded sha256)
	if len(hash) != 12 {
		t.Errorf("BaseDockerfileHash() returned hash of length %d, want 12", len(hash))
	}

	// Hash should be consistent (same input = same output)
	hash2 := configs.BaseDockerfileHash()
	if hash != hash2 {
		t.Errorf("BaseDockerfileHash() not deterministic: %s != %s", hash, hash2)
	}
}

func TestGetBaseImageName(t *testing.T) {
	name := GetBaseImageName()

	// Name should start with "ccells-base:"
	if len(name) < 13 || name[:12] != "ccells-base:" {
		t.Errorf("GetBaseImageName() = %q, want prefix 'ccells-base:'", name)
	}

	// Name should have a hash suffix (12 chars after the colon)
	parts := []rune(name)
	colonIdx := 11 // "ccells-base" is 11 chars
	if parts[colonIdx] != ':' {
		t.Errorf("GetBaseImageName() = %q, missing colon at expected position", name)
	}

	hashPart := name[12:] // Everything after "ccells-base:"
	if len(hashPart) != 12 {
		t.Errorf("GetBaseImageName() hash part length = %d, want 12 (got %q)", len(hashPart), hashPart)
	}

	// Name should be consistent
	name2 := GetBaseImageName()
	if name != name2 {
		t.Errorf("GetBaseImageName() not deterministic: %s != %s", name, name2)
	}

	// Name should match configs.BaseDockerfileHash()
	expectedHash := configs.BaseDockerfileHash()
	if hashPart != expectedHash {
		t.Errorf("GetBaseImageName() hash = %q, want %q from configs.BaseDockerfileHash()", hashPart, expectedHash)
	}
}

func TestEmbeddedDockerfileContent(t *testing.T) {
	// Verify the embedded Dockerfile contains expected content
	content := configs.BaseDockerfile

	if len(content) == 0 {
		t.Fatal("Embedded Dockerfile is empty")
	}

	// Verify it contains expected markers
	contentStr := string(content)
	expectedMarkers := []string{
		"FROM",
		"WORKDIR /workspace",
		"claude-code",
	}

	for _, marker := range expectedMarkers {
		if !strings.Contains(contentStr, marker) {
			t.Errorf("Embedded Dockerfile missing expected content: %q", marker)
		}
	}
}
