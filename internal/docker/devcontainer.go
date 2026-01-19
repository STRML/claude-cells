package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DevcontainerConfig represents the parsed devcontainer.json configuration.
type DevcontainerConfig struct {
	Name         string             `json:"name,omitempty"`
	Image        string             `json:"image,omitempty"`
	Build        *DevcontainerBuild `json:"build,omitempty"`
	ContainerEnv map[string]string  `json:"containerEnv,omitempty"`
}

// DevcontainerBuild represents the build section of devcontainer.json.
type DevcontainerBuild struct {
	Dockerfile string            `json:"dockerfile,omitempty"`
	Context    string            `json:"context,omitempty"`
	Args       map[string]string `json:"args,omitempty"`
}

// LoadDevcontainerConfig finds and parses devcontainer.json from the project.
// Returns nil, nil if no devcontainer.json is found.
// Returns error if the file exists but is invalid.
func LoadDevcontainerConfig(projectPath string) (*DevcontainerConfig, error) {
	// Check for devcontainer.json in standard locations (in priority order)
	candidates := []string{
		filepath.Join(projectPath, ".devcontainer", "devcontainer.json"),
		filepath.Join(projectPath, ".devcontainer.json"),
	}

	var configPath string
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			configPath = path
			break
		}
	}

	if configPath == "" {
		// No devcontainer.json found - not an error
		return nil, nil
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", configPath, err)
	}

	// Strip JSONC comments and trailing commas
	cleaned := stripJSONC(content)

	var cfg DevcontainerConfig
	if err := json.Unmarshal(cleaned, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", configPath, err)
	}

	return &cfg, nil
}

// stripJSONC removes // and /* */ comments and trailing commas from JSON.
// This allows parsing of JSONC format used by devcontainer.json.
func stripJSONC(input []byte) []byte {
	var result []byte
	i := 0
	inString := false
	escaped := false

	for i < len(input) {
		c := input[i]

		// Handle escape sequences in strings
		if inString {
			if escaped {
				result = append(result, c)
				escaped = false
				i++
				continue
			}
			if c == '\\' {
				escaped = true
				result = append(result, c)
				i++
				continue
			}
			if c == '"' {
				inString = false
			}
			result = append(result, c)
			i++
			continue
		}

		// Check for string start
		if c == '"' {
			inString = true
			result = append(result, c)
			i++
			continue
		}

		// Check for single-line comment
		if c == '/' && i+1 < len(input) && input[i+1] == '/' {
			// Skip until end of line
			for i < len(input) && input[i] != '\n' {
				i++
			}
			continue
		}

		// Check for multi-line comment
		if c == '/' && i+1 < len(input) && input[i+1] == '*' {
			i += 2
			// Skip until */
			for i+1 < len(input) {
				if input[i] == '*' && input[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			continue
		}

		result = append(result, c)
		i++
	}

	// Remove trailing commas before } or ]
	return removeTrailingCommas(result)
}

// removeTrailingCommas removes trailing commas before } or ].
func removeTrailingCommas(input []byte) []byte {
	var result []byte
	for i := 0; i < len(input); i++ {
		c := input[i]
		if c == ',' {
			// Look ahead for } or ] (skipping whitespace)
			j := i + 1
			for j < len(input) && isWhitespace(input[j]) {
				j++
			}
			if j < len(input) && (input[j] == '}' || input[j] == ']') {
				// Skip the comma
				continue
			}
		}
		result = append(result, c)
	}
	return result
}

func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// GetProjectImage returns the image name to use for a project and whether it needs to be built.
// Returns (imageName, needsBuild, error).
// If no devcontainer.json exists, returns the DefaultImage with needsBuild=false.
func GetProjectImage(projectPath string) (string, bool, error) {
	cfg, err := LoadDevcontainerConfig(projectPath)
	if err != nil {
		return "", false, err
	}

	// No devcontainer.json - use default image
	if cfg == nil {
		return DefaultImage, false, nil
	}

	// If image is specified directly, use it
	if cfg.Image != "" {
		return cfg.Image, false, nil
	}

	// If build is specified, we need to build
	if cfg.Build != nil && cfg.Build.Dockerfile != "" {
		// Verify dockerfile exists
		_, _, err := cfg.ResolveDockerfilePath(projectPath)
		if err != nil {
			return "", false, err
		}
		// Image name will be generated based on project
		imageName := generateProjectImageName(projectPath)
		return imageName, true, nil
	}

	// Neither image nor build specified - use default
	return DefaultImage, false, nil
}

// generateProjectImageName creates a unique image name for a project's custom build.
func generateProjectImageName(projectPath string) string {
	projectName := filepath.Base(projectPath)
	if projectName == "" || projectName == "." {
		projectName = "workspace"
	}
	// Sanitize for docker tag
	projectName = strings.ToLower(projectName)
	projectName = strings.ReplaceAll(projectName, " ", "-")
	return fmt.Sprintf("ccells-devcontainer-%s:latest", projectName)
}

// ResolveDockerfilePath resolves the full path to the Dockerfile based on the build config.
// Returns (dockerfilePath, contextPath, error).
func (cfg *DevcontainerConfig) ResolveDockerfilePath(projectPath string) (string, string, error) {
	if cfg.Build == nil || cfg.Build.Dockerfile == "" {
		return "", "", fmt.Errorf("no dockerfile specified in build config")
	}

	// Default context is .devcontainer
	contextPath := filepath.Join(projectPath, ".devcontainer")
	if cfg.Build.Context != "" {
		contextPath = filepath.Join(projectPath, ".devcontainer", cfg.Build.Context)
	}

	// Resolve dockerfile relative to context
	dockerfilePath := filepath.Join(contextPath, cfg.Build.Dockerfile)

	// Verify it exists
	if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
		// Also try relative to .devcontainer
		altPath := filepath.Join(projectPath, ".devcontainer", cfg.Build.Dockerfile)
		if _, err := os.Stat(altPath); os.IsNotExist(err) {
			return "", "", fmt.Errorf("dockerfile not found: %s (also tried %s)", dockerfilePath, altPath)
		}
		dockerfilePath = altPath
	}

	return dockerfilePath, contextPath, nil
}

// BuildProjectImage builds a Docker image from the devcontainer configuration.
// The output writer receives build progress output.
func BuildProjectImage(ctx context.Context, projectPath string, cfg *DevcontainerConfig, output io.Writer) error {
	if cfg == nil || cfg.Build == nil {
		return fmt.Errorf("no build configuration provided")
	}

	dockerfilePath, contextPath, err := cfg.ResolveDockerfilePath(projectPath)
	if err != nil {
		return err
	}

	imageName := generateProjectImageName(projectPath)

	// Build command arguments
	args := []string{"build", "-t", imageName, "-f", dockerfilePath}

	// Add build args
	for key, value := range cfg.Build.Args {
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", key, value))
	}

	// Add context
	args = append(args, contextPath)

	cmd := exec.CommandContext(ctx, "docker", args...)

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
