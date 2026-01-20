package docker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// DevcontainerCLIStatus represents the status of the devcontainer CLI
type DevcontainerCLIStatus struct {
	Available bool
	Version   string
	Path      string
}

// CheckDevcontainerCLI checks if the devcontainer CLI is installed and returns its status
func CheckDevcontainerCLI() DevcontainerCLIStatus {
	path, err := exec.LookPath("devcontainer")
	if err != nil {
		return DevcontainerCLIStatus{Available: false}
	}

	// Get version
	cmd := exec.Command("devcontainer", "--version")
	output, err := cmd.Output()
	if err != nil {
		return DevcontainerCLIStatus{Available: true, Path: path}
	}

	version := strings.TrimSpace(string(output))
	return DevcontainerCLIStatus{
		Available: true,
		Version:   version,
		Path:      path,
	}
}

// DevcontainerCLIInstallInstructions returns instructions for installing the devcontainer CLI
func DevcontainerCLIInstallInstructions() string {
	return `To install the devcontainer CLI:
  npm install -g @devcontainers/cli

Or with Homebrew (macOS):
  brew install devcontainer`
}

// BuildWithDevcontainerCLI builds a container image using the devcontainer CLI.
// This properly processes devcontainer.json including features.
// Returns the intermediate image name (without Claude Code) on success.
// The caller should then use BuildEnhancedImage to add Claude Code.
// Uses a per-image lock to prevent concurrent builds of the same image.
func BuildWithDevcontainerCLI(ctx context.Context, projectPath string, output io.Writer) (string, error) {
	// Generate intermediate image name (will be enhanced with Claude Code later)
	baseImageName := generateProjectImageName(projectPath)
	baseImageName = strings.TrimSuffix(baseImageName, ":latest") + "-base:latest"

	// Acquire build lock for this image
	unlock := acquireBuildLock(baseImageName)
	defer unlock()

	// Check if image was built while we waited for the lock
	client, err := NewClient()
	if err == nil {
		exists, _ := client.ImageExists(ctx, baseImageName)
		client.Close()
		if exists {
			fmt.Fprintln(output, "Base image already built by another workstream")
			return baseImageName, nil
		}
	}

	// Build command: devcontainer build --workspace-folder <path> --image-name <name>
	args := []string{
		"build",
		"--workspace-folder", projectPath,
		"--image-name", baseImageName,
	}

	cmd := exec.CommandContext(ctx, "devcontainer", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start devcontainer build: %w", err)
	}

	// Stream output
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		fmt.Fprintln(output, scanner.Text())
	}

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("devcontainer build failed: %w", err)
	}

	return baseImageName, nil
}

// HasDevcontainerConfig checks if the project has a devcontainer.json
func HasDevcontainerConfig(projectPath string) bool {
	cfg, err := LoadDevcontainerConfig(projectPath)
	return err == nil && cfg != nil
}

// HasDevcontainerFeatures checks if the devcontainer.json has features defined
func HasDevcontainerFeatures(projectPath string) bool {
	cfg, err := LoadDevcontainerConfig(projectPath)
	if err != nil || cfg == nil {
		return false
	}
	// Note: Features aren't currently in our DevcontainerConfig struct
	// We'd need to add them. For now, check by re-reading the file.
	return hasFeatures(projectPath)
}

// hasFeatures checks if devcontainer.json contains a features section
func hasFeatures(projectPath string) bool {
	cfg, err := LoadDevcontainerConfig(projectPath)
	if err != nil || cfg == nil {
		return false
	}
	// Load raw JSON and check for features field
	// This is a simple heuristic - look for "features" in the raw file
	content, err := readDevcontainerJSON(projectPath)
	if err != nil {
		return false
	}
	return strings.Contains(string(content), `"features"`)
}

// readDevcontainerJSON reads the raw devcontainer.json content
func readDevcontainerJSON(projectPath string) ([]byte, error) {
	candidates := []string{
		projectPath + "/.devcontainer/devcontainer.json",
		projectPath + "/.devcontainer.json",
	}

	for _, path := range candidates {
		content, err := exec.Command("cat", path).Output()
		if err == nil {
			return content, nil
		}
	}
	return nil, fmt.Errorf("no devcontainer.json found")
}
