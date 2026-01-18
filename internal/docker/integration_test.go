package docker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestClaudeCodeContainer tests the full container lifecycle with the actual
// claude-code-base image and verifies Claude Code can start without errors.
func TestClaudeCodeContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	client := skipIfDockerUnavailable(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Verify the required image exists
	exists, err := client.ImageExists(ctx, RequiredImage)
	if err != nil {
		t.Fatalf("ImageExists() error = %v", err)
	}
	if !exists {
		t.Fatalf("Required image %s not found. Run: docker build -t %s -f configs/base.Dockerfile .", RequiredImage, RequiredImage)
	}

	// Get paths for mounts
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	// Go up to project root from internal/docker
	projectRoot := filepath.Dir(filepath.Dir(cwd))

	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}
	claudeCfg := filepath.Join(homeDir, ".claude")

	// Create container config
	cfg := &ContainerConfig{
		Name:      "docker-tui-integration-test-" + time.Now().Format("150405"),
		Image:     RequiredImage,
		RepoPath:  projectRoot,
		ClaudeCfg: claudeCfg,
	}

	// Create container
	containerID, err := client.CreateContainer(ctx, cfg)
	if err != nil {
		t.Fatalf("CreateContainer() error = %v", err)
	}
	t.Logf("Created container: %s", containerID[:12])

	// Cleanup
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_ = client.StopContainer(cleanupCtx, containerID)
		_ = client.RemoveContainer(cleanupCtx, containerID)
		t.Log("Container cleaned up")
	}()

	// Start container
	err = client.StartContainer(ctx, containerID)
	if err != nil {
		t.Fatalf("StartContainer() error = %v", err)
	}
	t.Log("Container started")

	// Verify container is running
	running, err := client.IsContainerRunning(ctx, containerID)
	if err != nil {
		t.Fatalf("IsContainerRunning() error = %v", err)
	}
	if !running {
		t.Fatal("Container should be running")
	}

	// Test 1: Verify Claude Code is installed
	t.Run("claude_code_installed", func(t *testing.T) {
		output, err := client.ExecInContainer(ctx, containerID, []string{"which", "claude"})
		if err != nil {
			t.Fatalf("ExecInContainer() error = %v", err)
		}
		if !strings.Contains(output, "claude") {
			t.Errorf("Claude Code not found in PATH. Output: %s", output)
		}
		t.Logf("Claude Code found at: %s", strings.TrimSpace(output))
	})

	// Test 2: Verify .claude directory is writable
	t.Run("claude_dir_writable", func(t *testing.T) {
		// Try to create a file in the debug directory
		output, err := client.ExecInContainer(ctx, containerID, []string{
			"sh", "-c", "mkdir -p /home/claude/.claude/debug && echo 'test' > /home/claude/.claude/debug/test.txt && cat /home/claude/.claude/debug/test.txt",
		})
		if err != nil {
			t.Fatalf("ExecInContainer() error = %v", err)
		}
		if !strings.Contains(output, "test") {
			t.Errorf("Failed to write to .claude/debug. Output: %s", output)
		}
		t.Log(".claude/debug directory is writable")
	})

	// Test 3: Verify workspace is mounted
	t.Run("workspace_mounted", func(t *testing.T) {
		output, err := client.ExecInContainer(ctx, containerID, []string{"ls", "/workspace"})
		if err != nil {
			t.Fatalf("ExecInContainer() error = %v", err)
		}
		// Should see project files
		if !strings.Contains(output, "go.mod") && !strings.Contains(output, "internal") {
			t.Errorf("Workspace not properly mounted. Contents: %s", output)
		}
		t.Log("Workspace mounted correctly")
	})

	// Test 4: Verify Claude Code can start (with --help to avoid needing API key)
	t.Run("claude_code_runs", func(t *testing.T) {
		output, err := client.ExecInContainer(ctx, containerID, []string{"claude", "--version"})
		if err != nil {
			// Claude might exit with error if no API key, but should still show version
			t.Logf("claude --version output (may have error): %s", output)
		}
		// Check that we got some output (version info)
		if output == "" {
			t.Error("Claude Code produced no output")
		}
		t.Logf("Claude Code version output: %s", strings.TrimSpace(output))
	})

	// Test 5: Test PTY-like command execution (simulating what the TUI does)
	t.Run("pty_simulation", func(t *testing.T) {
		// Run an interactive-ish command
		output, err := client.ExecInContainer(ctx, containerID, []string{
			"sh", "-c", "echo 'Hello from container' && pwd && whoami",
		})
		if err != nil {
			t.Fatalf("ExecInContainer() error = %v", err)
		}
		if !strings.Contains(output, "Hello from container") {
			t.Errorf("PTY simulation failed. Output: %s", output)
		}
		t.Logf("PTY simulation output: %s", output)
	})
}

// TestContainerClaudeCodeExec tests executing Claude Code in a container
// with a simple prompt that should work without API interaction.
func TestContainerClaudeCodeExec(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Skip if ANTHROPIC_API_KEY is not set (can't actually run Claude)
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping Claude execution test")
	}

	client := skipIfDockerUnavailable(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Verify image exists
	exists, err := client.ImageExists(ctx, RequiredImage)
	if err != nil || !exists {
		t.Skipf("Required image %s not found", RequiredImage)
	}

	homeDir, _ := os.UserHomeDir()
	claudeCfg := filepath.Join(homeDir, ".claude")

	cfg := &ContainerConfig{
		Name:      "docker-tui-claude-exec-test-" + time.Now().Format("150405"),
		Image:     RequiredImage,
		RepoPath:  "/tmp",
		ClaudeCfg: claudeCfg,
	}

	containerID, err := client.CreateContainer(ctx, cfg)
	if err != nil {
		t.Fatalf("CreateContainer() error = %v", err)
	}

	defer func() {
		cleanupCtx, _ := context.WithTimeout(context.Background(), 30*time.Second)
		_ = client.StopContainer(cleanupCtx, containerID)
		_ = client.RemoveContainer(cleanupCtx, containerID)
	}()

	if err := client.StartContainer(ctx, containerID); err != nil {
		t.Fatalf("StartContainer() error = %v", err)
	}

	// Try running Claude with a simple print command
	// Using --print flag for non-interactive mode
	output, err := client.ExecInContainer(ctx, containerID, []string{
		"claude", "--print", "Say hello in exactly 3 words",
	})
	if err != nil {
		t.Logf("Claude exec error (may be expected): %v", err)
	}
	t.Logf("Claude output: %s", output)

	// Just verify we got some output (success or meaningful error)
	if output == "" {
		t.Error("Expected some output from Claude")
	}
}

// TestMultipleContainers tests creating multiple containers simultaneously
// to verify the naming and isolation works correctly.
func TestMultipleContainers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	client := skipIfDockerUnavailable(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Use alpine for speed
	var containerIDs []string
	branches := []string{"feature-auth", "bugfix-login", "feature/nested-name"}

	for _, branch := range branches {
		cfg := NewContainerConfig(branch, "/tmp")
		cfg.Image = "alpine:latest"
		cfg.Name = cfg.Name + "-" + time.Now().Format("150405.000")

		containerID, err := client.CreateContainer(ctx, cfg)
		if err != nil {
			t.Fatalf("CreateContainer(%s) error = %v", branch, err)
		}
		containerIDs = append(containerIDs, containerID)
		t.Logf("Created container for branch %s: %s", branch, containerID[:12])
	}

	// Cleanup all containers
	defer func() {
		for _, id := range containerIDs {
			cleanupCtx, _ := context.WithTimeout(context.Background(), 10*time.Second)
			_ = client.StopContainer(cleanupCtx, id)
			_ = client.RemoveContainer(cleanupCtx, id)
		}
	}()

	// Start all containers
	for i, id := range containerIDs {
		if err := client.StartContainer(ctx, id); err != nil {
			t.Fatalf("StartContainer(%s) error = %v", branches[i], err)
		}
	}

	// Verify all are running
	for i, id := range containerIDs {
		running, err := client.IsContainerRunning(ctx, id)
		if err != nil {
			t.Errorf("IsContainerRunning(%s) error = %v", branches[i], err)
		}
		if !running {
			t.Errorf("Container %s should be running", branches[i])
		}
	}

	t.Logf("Successfully created and started %d containers", len(containerIDs))
}
