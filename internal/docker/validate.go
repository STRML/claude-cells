package docker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/docker/docker/client"
)

// RequiredImage is the Docker image required for workstreams
const RequiredImage = "claude-code-base:latest"

// ValidationError represents a Docker validation failure
type ValidationError struct {
	Check   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Check, e.Message)
}

// ValidationResult contains all validation results
type ValidationResult struct {
	DockerAvailable bool
	ImageExists     bool
	Errors          []ValidationError
}

// IsValid returns true if all validations passed
func (v *ValidationResult) IsValid() bool {
	return v.DockerAvailable && v.ImageExists && len(v.Errors) == 0
}

// ValidatePrerequisites checks all Docker prerequisites
func ValidatePrerequisites(ctx context.Context) (*ValidationResult, error) {
	result := &ValidationResult{}

	// Check Docker daemon
	client, err := NewClient()
	if err != nil {
		result.Errors = append(result.Errors, ValidationError{
			Check:   "docker_connection",
			Message: fmt.Sprintf("failed to connect to Docker: %v", err),
		})
		return result, nil
	}
	defer client.Close()

	// Ping Docker daemon
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx); err != nil {
		result.Errors = append(result.Errors, ValidationError{
			Check:   "docker_ping",
			Message: fmt.Sprintf("Docker daemon not responding: %v", err),
		})
		return result, nil
	}
	result.DockerAvailable = true

	// Check required image exists
	exists, err := client.ImageExists(ctx, RequiredImage)
	if err != nil {
		result.Errors = append(result.Errors, ValidationError{
			Check:   "image_check",
			Message: fmt.Sprintf("failed to check image: %v", err),
		})
		return result, nil
	}

	if !exists {
		result.Errors = append(result.Errors, ValidationError{
			Check:   "required_image",
			Message: fmt.Sprintf("required image '%s' not found. Run: docker build -t %s -f configs/base.Dockerfile .", RequiredImage, RequiredImage),
		})
	} else {
		result.ImageExists = true
	}

	return result, nil
}

// ImageExists checks if a Docker image exists locally
func (c *Client) ImageExists(ctx context.Context, imageName string) (bool, error) {
	// Use ImageInspect instead of listing all images - much faster
	_, _, err := c.cli.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		// Check if it's a "not found" error
		if client.IsErrNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// BuildImage builds the required Docker image from the Dockerfile.
// It streams build output to the provided writer (can be os.Stdout).
// Returns an error if the build fails.
func BuildImage(ctx context.Context, output io.Writer) error {
	// Find the project root by looking for configs/base.Dockerfile
	dockerfilePath, err := findDockerfile()
	if err != nil {
		return fmt.Errorf("failed to find Dockerfile: %w", err)
	}

	buildContext := filepath.Dir(filepath.Dir(dockerfilePath)) // Go up from configs/ to project root

	cmd := exec.CommandContext(ctx, "docker", "build",
		"-t", RequiredImage,
		"-f", dockerfilePath,
		buildContext,
	)

	// Combine stdout and stderr for build output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start docker build: %w", err)
	}

	// Stream output
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		fmt.Fprintln(output, scanner.Text())
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("docker build failed: %w", err)
	}

	return nil
}

// findDockerfile locates the base.Dockerfile in the project
func findDockerfile() (string, error) {
	// Try relative to current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Check common locations
	candidates := []string{
		filepath.Join(cwd, "configs", "base.Dockerfile"),
		filepath.Join(cwd, "..", "configs", "base.Dockerfile"),
		filepath.Join(cwd, "..", "..", "configs", "base.Dockerfile"),
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return filepath.Abs(path)
		}
	}

	// Try to find by walking up from executable location
	execPath, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(execPath)
		candidates := []string{
			filepath.Join(execDir, "configs", "base.Dockerfile"),
			filepath.Join(execDir, "..", "configs", "base.Dockerfile"),
		}
		for _, path := range candidates {
			if _, err := os.Stat(path); err == nil {
				return filepath.Abs(path)
			}
		}
	}

	return "", fmt.Errorf("could not find configs/base.Dockerfile")
}
