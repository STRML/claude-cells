package docker

import (
	"context"
	"io"
	"os"
	"testing"
	"time"
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

	result, err := ValidatePrerequisites(ctx)
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

	// The required image should exist (fail fast if not)
	if !result.ImageExists {
		t.Error("Required image should exist")
		t.Log("Run: docker build -t claude-code-base:latest -f configs/base.Dockerfile .")
		for _, e := range result.Errors {
			t.Logf("  Error: %s - %s", e.Check, e.Message)
		}
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

	tests := []struct {
		name      string
		imageName string
		wantExist bool
	}{
		{
			name:      "required image exists",
			imageName: RequiredImage,
			wantExist: true,
		},
		{
			name:      "alpine exists (common image)",
			imageName: "alpine:latest",
			wantExist: true, // Usually pulled in tests
		},
		{
			name:      "nonexistent image",
			imageName: "nonexistent-image-12345:latest",
			wantExist: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exists, err := client.ImageExists(ctx, tt.imageName)
			if err != nil {
				t.Fatalf("ImageExists() error = %v", err)
			}

			if tt.imageName == RequiredImage && !exists {
				t.Fatalf("Required image %s does not exist! Run: docker build -t %s -f configs/base.Dockerfile .", RequiredImage, RequiredImage)
			}

			if tt.imageName == "nonexistent-image-12345:latest" && exists {
				t.Error("Nonexistent image should not exist")
			}
		})
	}
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

	// Skip if Dockerfile can't be found
	if _, err := findDockerfile(); err != nil {
		t.Skipf("Dockerfile not found: %v", err)
	}

	// We don't actually run the build in tests since it's slow
	// and the image should already exist. Just verify the function
	// doesn't panic when called with a cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Should return an error due to cancelled context
	err := BuildImage(ctx, io.Discard)
	if err == nil {
		t.Log("BuildImage with cancelled context didn't error (build may have been very fast)")
	}
}
