package docker

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// buildLocks manages per-image build locks to prevent concurrent builds
var (
	buildLocksMu sync.Mutex
	buildLocks   = make(map[string]*sync.Mutex)
)

// acquireBuildLock gets or creates a lock for a specific image and acquires it.
// Returns a function to release the lock.
func acquireBuildLock(imageName string) func() {
	buildLocksMu.Lock()
	lock, ok := buildLocks[imageName]
	if !ok {
		lock = &sync.Mutex{}
		buildLocks[imageName] = lock
	}
	buildLocksMu.Unlock()

	lock.Lock()
	return lock.Unlock
}

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
// Returns (imageName, needsBuild, baseImage, error).
// If no devcontainer.json exists, returns the DefaultImage with needsBuild=false.
// When a devcontainer.json exists, we always need to build to include Claude Code.
func GetProjectImage(projectPath string) (string, bool, error) {
	cfg, err := LoadDevcontainerConfig(projectPath)
	if err != nil {
		return "", false, err
	}

	// No devcontainer.json - use default image (which has Claude Code pre-installed)
	// Use hash-tagged name so Dockerfile changes trigger rebuilds
	if cfg == nil {
		return GetBaseImageName(), false, nil
	}

	// Always generate our own image name - we'll build a derived image with Claude Code
	imageName := generateProjectImageName(projectPath)

	// If image is specified directly, we still need to build a derived image with Claude Code
	if cfg.Image != "" {
		return imageName, true, nil
	}

	// If build is specified, we need to build
	if cfg.Build != nil && cfg.Build.Dockerfile != "" {
		// Verify dockerfile exists
		_, _, err := cfg.ResolveDockerfilePath(projectPath)
		if err != nil {
			return "", false, err
		}
		return imageName, true, nil
	}

	// Neither image nor build specified - use default
	// Use hash-tagged name so Dockerfile changes trigger rebuilds
	return GetBaseImageName(), false, nil
}

// GetBaseImage returns the base image specified in devcontainer.json (if any).
// This is used when we need to build a derived image with Claude Code.
func GetBaseImage(projectPath string) (string, error) {
	cfg, err := LoadDevcontainerConfig(projectPath)
	if err != nil {
		return "", err
	}
	if cfg == nil || cfg.Image == "" {
		return "", nil
	}
	return cfg.Image, nil
}

// computeConfigHash computes a hash of the devcontainer.json content and any
// referenced Dockerfile. This is used to detect when the config has changed
// and the image needs rebuilding. Returns empty string if no devcontainer.json exists.
func computeConfigHash(projectPath string) string {
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
		return ""
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}

	// Strip comments and whitespace for consistent hashing
	// (so formatting changes don't trigger rebuilds)
	cleaned := stripJSONC(content)

	// Parse and re-marshal to normalize the JSON
	var cfg map[string]interface{}
	if err := json.Unmarshal(cleaned, &cfg); err != nil {
		// Fall back to hashing the cleaned content directly
		hash := sha256.Sum256(cleaned)
		return hex.EncodeToString(hash[:])[:12]
	}

	// Re-marshal with sorted keys for deterministic output
	normalized, err := json.Marshal(cfg)
	if err != nil {
		hash := sha256.Sum256(cleaned)
		return hex.EncodeToString(hash[:])[:12]
	}

	// Start with devcontainer.json content
	hashInput := normalized

	// If there's a Dockerfile reference, include its content in the hash
	if build, ok := cfg["build"].(map[string]interface{}); ok {
		if dockerfile, ok := build["dockerfile"].(string); ok && dockerfile != "" {
			dockerfileContent := readDockerfileForHash(projectPath, configPath, build)
			if dockerfileContent != nil {
				hashInput = append(hashInput, dockerfileContent...)
			}
		}
	}

	hash := sha256.Sum256(hashInput)
	return hex.EncodeToString(hash[:])[:12]
}

// readDockerfileForHash reads the Dockerfile content for inclusion in the config hash.
// Returns nil if the Dockerfile cannot be read.
func readDockerfileForHash(projectPath, configPath string, build map[string]interface{}) []byte {
	dockerfile, ok := build["dockerfile"].(string)
	if !ok || dockerfile == "" {
		return nil
	}

	// Determine the context directory
	configDir := filepath.Dir(configPath)
	contextDir := configDir
	if context, ok := build["context"].(string); ok && context != "" {
		contextDir = filepath.Join(configDir, context)
	}

	// Try to find the Dockerfile
	candidates := []string{
		filepath.Join(contextDir, dockerfile),
		filepath.Join(configDir, dockerfile),
	}

	for _, path := range candidates {
		content, err := os.ReadFile(path)
		if err == nil {
			return content
		}
	}

	return nil
}

// generateProjectImageName creates a unique image name for a project's custom build.
// The image tag includes a hash of the devcontainer.json content and any referenced
// Dockerfile, so that config or Dockerfile changes automatically trigger a rebuild.
func generateProjectImageName(projectPath string) string {
	projectName := filepath.Base(projectPath)
	if projectName == "" || projectName == "." {
		projectName = "workspace"
	}
	// Sanitize for docker tag
	projectName = strings.ToLower(projectName)
	projectName = strings.ReplaceAll(projectName, " ", "-")

	// Include config hash in tag to detect changes
	configHash := computeConfigHash(projectPath)
	if configHash == "" {
		// No devcontainer.json, use latest (shouldn't normally happen
		// since we only call this when devcontainer.json exists)
		return fmt.Sprintf("ccells-devcontainer-%s:latest", projectName)
	}

	return fmt.Sprintf("ccells-devcontainer-%s:%s", projectName, configHash)
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
// Uses a per-image lock to prevent concurrent builds of the same image.
func BuildProjectImage(ctx context.Context, projectPath string, cfg *DevcontainerConfig, output io.Writer) error {
	if cfg == nil {
		return fmt.Errorf("no devcontainer configuration provided")
	}

	imageName := generateProjectImageName(projectPath)

	// Acquire build lock for this image
	unlock := acquireBuildLock(imageName)
	defer unlock()

	// Check if image was built while we waited for the lock
	client, err := NewClient()
	if err == nil {
		exists, _ := client.ImageExists(ctx, imageName)
		client.Close()
		if exists {
			fmt.Fprintln(output, "Image already built by another workstream")
			return nil
		}
	}

	// If we have a build config with Dockerfile, use it
	if cfg.Build != nil && cfg.Build.Dockerfile != "" {
		dockerfilePath, contextPath, err := cfg.ResolveDockerfilePath(projectPath)
		if err != nil {
			return err
		}

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

	// If we only have an image specified, build a derived image with Claude Code
	if cfg.Image != "" {
		return BuildEnhancedImage(ctx, cfg.Image, imageName, output)
	}

	return fmt.Errorf("no image or build configuration found")
}

// BuildEnhancedImage builds a derived image from a base image with Claude Code pre-installed.
// This allows containers to start faster since Claude Code is already available.
// Uses a per-image lock to prevent concurrent builds of the same image.
func BuildEnhancedImage(ctx context.Context, baseImage, targetImage string, output io.Writer) error {
	// Acquire build lock for this image
	unlock := acquireBuildLock(targetImage)
	defer unlock()

	// Check if image was built while we waited for the lock
	client, err := NewClient()
	if err == nil {
		exists, _ := client.ImageExists(ctx, targetImage)
		client.Close()
		if exists {
			fmt.Fprintln(output, "Image already built by another workstream")
			return nil
		}
	}

	// Create a temporary Dockerfile
	dockerfile := fmt.Sprintf(`FROM %s

# Install curl and bash if not present (needed for Claude Code installer)
RUN command -v curl || (apt-get update && apt-get install -y curl bash && rm -rf /var/lib/apt/lists/*) || (apk add --no-cache curl bash) || true

# Install Claude Code using native installer
RUN curl -fsSL https://claude.ai/install.sh | sh
`, baseImage)

	// Add injections from config
	dfCfg := LoadDockerfileConfig("")
	if len(dfCfg.Inject) > 0 {
		dockerfile += "\n# Injected from ~/.claude-cells/config.yaml\n"
		for _, cmd := range dfCfg.Inject {
			dockerfile += fmt.Sprintf("RUN %s\n", cmd)
		}
	}

	// Create temp directory for build context
	tmpDir, err := os.MkdirTemp("", "ccells-build-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write Dockerfile
	dockerfilePath := filepath.Join(tmpDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0644); err != nil {
		return fmt.Errorf("failed to write Dockerfile: %w", err)
	}

	fmt.Fprintf(output, "Building enhanced image with Claude Code from %s...\n", baseImage)

	// Build the image
	args := []string{"build", "-t", targetImage, "-f", dockerfilePath, tmpDir}
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

	fmt.Fprintln(output, "Enhanced image built successfully!")
	return nil
}
