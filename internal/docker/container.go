package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/pkg/stdcopy"
)

// ContainerPrefix is used to identify ccells containers
const ContainerPrefix = "ccells-"

// Default resource limits for containers
const (
	DefaultCPULimit    = 2.0                    // Number of CPUs
	DefaultMemoryLimit = 4 * 1024 * 1024 * 1024 // 4GB in bytes
)

// ContainerConfig holds configuration for creating a workstream container.
type ContainerConfig struct {
	Name        string            // Container name (ccells-<project>-<timestamp>)
	Image       string            // Docker image to use
	RepoPath    string            // Path to worktree on host (mounted at /workspace)
	HostGitDir  string            // Path to host repo's .git directory (mounted at same path for worktree support)
	ClaudeCfg   string            // Path to ~/.claude directory on host
	ClaudeJSON  string            // Path to ~/.claude.json file on host (session state)
	GitConfig   string            // Path to ~/.gitconfig file on host (git identity)
	GitIdentity *GitIdentity      // Git user identity (name/email) for commits
	Credentials string            // Path to credentials file (OAuth tokens from keychain)
	Timezone    string            // Host timezone (e.g., "America/New_York") for consistent commit timestamps
	ExtraEnv    map[string]string // Additional environment variables from devcontainer.json
	ExtraMounts []mount.Mount     // Additional mounts from devcontainer.json

	// Resource limits (optional - defaults applied if zero)
	CPULimit    float64 // Number of CPUs (e.g., 2.0 for 2 CPUs)
	MemoryLimit int64   // Memory limit in bytes (e.g., 4*1024*1024*1024 for 4GB)
}

// NewContainerConfig creates a container config for a workstream.
func NewContainerConfig(branchName, repoPath string) *ContainerConfig {
	// Sanitize branch name for container naming
	safeName := strings.ReplaceAll(branchName, "/", "-")
	safeName = strings.ReplaceAll(safeName, " ", "-")

	// Get project name from repo path
	projectName := filepath.Base(repoPath)
	if projectName == "" || projectName == "." {
		projectName = "workspace"
	}

	// Create unique name with timestamp
	timestamp := time.Now().Format("20060102-150405")
	name := fmt.Sprintf("%s%s-%s-%s", ContainerPrefix, projectName, safeName, timestamp)

	return &ContainerConfig{
		Name:     name,
		RepoPath: repoPath,
	}
}

// CreateContainer creates a new container but doesn't start it.
func (c *Client) CreateContainer(ctx context.Context, cfg *ContainerConfig) (string, error) {
	// Build environment variables from ExtraEnv
	var env []string
	for k, v := range cfg.ExtraEnv {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Add claude to PATH (npm global installs to ~/.local/bin)
	env = append(env, "PATH=/root/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")

	// Add git identity environment variables if provided
	// These ensure commits are attributed to the user even without .gitconfig
	if cfg.GitIdentity != nil {
		if cfg.GitIdentity.Name != "" {
			env = append(env, fmt.Sprintf("GIT_AUTHOR_NAME=%s", cfg.GitIdentity.Name))
			env = append(env, fmt.Sprintf("GIT_COMMITTER_NAME=%s", cfg.GitIdentity.Name))
		}
		if cfg.GitIdentity.Email != "" {
			env = append(env, fmt.Sprintf("GIT_AUTHOR_EMAIL=%s", cfg.GitIdentity.Email))
			env = append(env, fmt.Sprintf("GIT_COMMITTER_EMAIL=%s", cfg.GitIdentity.Email))
		}
	}

	// Add timezone environment variable if provided
	// This ensures commits have the same timezone as the host
	if cfg.Timezone != "" {
		env = append(env, fmt.Sprintf("TZ=%s", cfg.Timezone))
	}

	// Set CLAUDE_CONFIG_DIR to tell Claude Code where to find/store all config
	// This is the recommended approach for Docker containers (see claude-code#1736)
	// Claude Code will read/write .claude.json, .credentials.json, and other config here
	if cfg.ClaudeCfg != "" {
		// Point to the parent of .claude directory (e.g., /home/claude)
		// Claude Code expects CLAUDE_CONFIG_DIR to contain .claude/ subdirectory
		env = append(env, "CLAUDE_CONFIG_DIR=/home/claude")
	}

	containerCfg := &container.Config{
		Image: cfg.Image,
		Cmd:   []string{"sleep", "infinity"},
		Tty:   true,
		Env:   env,
	}

	var mounts []mount.Mount
	if cfg.RepoPath != "" {
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: cfg.RepoPath,
			Target: "/workspace",
		})
	}
	// Mount host repo's .git directory at the same path for worktree support
	// Worktrees have a .git file that points to the main repo's .git/worktrees/<name>
	// By mounting .git at the same host path, those references resolve correctly
	// NOT read-only: commits need to write objects to .git/objects/
	// Each worktree has its own index in .git/worktrees/<name>/, so no locking conflicts
	if cfg.HostGitDir != "" {
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: cfg.HostGitDir,
			Target: cfg.HostGitDir, // Same path as on host
		})
	}
	if cfg.ClaudeCfg != "" {
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: cfg.ClaudeCfg,
			Target: "/home/claude/.claude",
			// Not read-only: Claude Code needs to write debug logs to ~/.claude/debug/
		})
		// Also mount root-level .credentials.json for CLAUDE_CONFIG_DIR (claude-code#1736)
		// Claude Code with CLAUDE_CONFIG_DIR looks for credentials at $CLAUDE_CONFIG_DIR/.credentials.json
		rootCredsPath := filepath.Join(filepath.Dir(cfg.ClaudeCfg), ".credentials.json")
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: rootCredsPath,
			Target: "/home/claude/.credentials.json",
			// Not read-only: Claude Code needs to refresh tokens
		})
	}
	if cfg.ClaudeJSON != "" {
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: cfg.ClaudeJSON,
			Target: "/home/claude/.claude.json",
			// Not read-only: Claude Code updates session state
		})
	}
	if cfg.GitConfig != "" {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   cfg.GitConfig,
			Target:   "/home/claude/.gitconfig",
			ReadOnly: true, // Git identity should not be modified
		})
	}
	if cfg.Credentials != "" {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   cfg.Credentials,
			Target:   "/home/claude/.claude-credentials",
			ReadOnly: true, // Credentials should not be modified by container
		})
	}

	// Add extra mounts from devcontainer.json
	mounts = append(mounts, cfg.ExtraMounts...)

	// Apply resource limits with defaults
	cpuLimit := cfg.CPULimit
	if cpuLimit == 0 {
		cpuLimit = DefaultCPULimit
	}
	memoryLimit := cfg.MemoryLimit
	if memoryLimit == 0 {
		memoryLimit = DefaultMemoryLimit
	}

	hostCfg := &container.HostConfig{
		Mounts: mounts,
		Resources: container.Resources{
			// CPU limit: NanoCPUs is CPUs * 1e9 (e.g., 2.0 CPUs = 2e9 NanoCPUs)
			NanoCPUs: int64(cpuLimit * 1e9),
			// Memory limit in bytes
			Memory: memoryLimit,
			// Memory swap equal to memory (disables swap)
			MemorySwap: memoryLimit,
		},
	}

	resp, err := c.cli.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, cfg.Name)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// StartContainer starts a created container.
func (c *Client) StartContainer(ctx context.Context, containerID string) error {
	return c.cli.ContainerStart(ctx, containerID, container.StartOptions{})
}

// StopContainer stops a running container.
func (c *Client) StopContainer(ctx context.Context, containerID string) error {
	timeout := 10 // seconds
	return c.cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
}

// RemoveContainer removes a container.
func (c *Client) RemoveContainer(ctx context.Context, containerID string) error {
	return c.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
}

// GetContainerName returns the name of a container by ID.
func (c *Client) GetContainerName(ctx context.Context, containerID string) (string, error) {
	info, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", err
	}
	// Docker container names have a leading slash
	return strings.TrimPrefix(info.Name, "/"), nil
}

// RemoveContainerAndConfig removes a container and its associated config directory.
// This should be called when destroying a workstream to clean up all resources.
func (c *Client) RemoveContainerAndConfig(ctx context.Context, containerID string) error {
	// Get container name before removing (needed for config cleanup)
	containerName, _ := c.GetContainerName(ctx, containerID)

	// Remove the container
	if err := c.RemoveContainer(ctx, containerID); err != nil {
		return err
	}

	// Clean up config directory if we got the name
	if containerName != "" {
		_ = CleanupContainerConfig(containerName)
	}

	return nil
}

// PauseContainer pauses a running container.
func (c *Client) PauseContainer(ctx context.Context, containerID string) error {
	return c.cli.ContainerPause(ctx, containerID)
}

// UnpauseContainer unpauses a paused container.
func (c *Client) UnpauseContainer(ctx context.Context, containerID string) error {
	return c.cli.ContainerUnpause(ctx, containerID)
}

// GetContainerState returns the state of a container (running, paused, exited, etc.)
func (c *Client) GetContainerState(ctx context.Context, containerID string) (string, error) {
	info, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", err
	}
	return info.State.Status, nil
}

// IsContainerRunning checks if a container is running.
func (c *Client) IsContainerRunning(ctx context.Context, containerID string) (bool, error) {
	info, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return false, err
	}
	return info.State.Running, nil
}

// ExecInContainer runs a command in a container and returns output.
func (c *Client) ExecInContainer(ctx context.Context, containerID string, cmd []string) (string, error) {
	execCfg := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := c.cli.ContainerExecCreate(ctx, containerID, execCfg)
	if err != nil {
		return "", err
	}

	resp, err := c.cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return "", err
	}
	defer resp.Close()

	var stdout, stderr bytes.Buffer
	_, err = stdcopy.StdCopy(&stdout, &stderr, resp.Reader)
	if err != nil && err != io.EOF {
		return "", err
	}

	return stdout.String() + stderr.String(), nil
}

// ContainerInfo holds basic info about a container
type ContainerInfo struct {
	ID      string
	Name    string
	State   string
	Created time.Time
}

// ListDockerTUIContainers lists all containers created by ccells
func (c *Client) ListDockerTUIContainers(ctx context.Context) ([]ContainerInfo, error) {
	// Filter by name prefix
	filterArgs := filters.NewArgs()
	filterArgs.Add("name", ContainerPrefix)

	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filterArgs,
	})
	if err != nil {
		return nil, err
	}

	var result []ContainerInfo
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		result = append(result, ContainerInfo{
			ID:      c.ID,
			Name:    name,
			State:   c.State,
			Created: time.Unix(c.Created, 0),
		})
	}
	return result, nil
}

// PruneDockerTUIContainers removes all stopped ccells containers
func (c *Client) PruneDockerTUIContainers(ctx context.Context) (int, error) {
	containers, err := c.ListDockerTUIContainers(ctx)
	if err != nil {
		return 0, err
	}

	pruned := 0
	for _, cont := range containers {
		if cont.State != "running" {
			if err := c.RemoveContainer(ctx, cont.ID); err == nil {
				pruned++
			}
		}
	}
	return pruned, nil
}

// PruneAllDockerTUIContainers removes ALL ccells containers (including running ones)
func (c *Client) PruneAllDockerTUIContainers(ctx context.Context) (int, error) {
	containers, err := c.ListDockerTUIContainers(ctx)
	if err != nil {
		return 0, err
	}

	pruned := 0
	for _, cont := range containers {
		// Stop if running
		if cont.State == "running" {
			_ = c.StopContainer(ctx, cont.ID)
		}
		if err := c.RemoveContainer(ctx, cont.ID); err == nil {
			pruned++
		}
	}
	return pruned, nil
}

// ListDockerTUIContainersForProject lists containers created by ccells for a specific project
func (c *Client) ListDockerTUIContainersForProject(ctx context.Context, projectName string) ([]ContainerInfo, error) {
	// Filter by project-specific prefix: ccells-<projectName>-
	projectPrefix := fmt.Sprintf("%s%s-", ContainerPrefix, projectName)
	filterArgs := filters.NewArgs()
	filterArgs.Add("name", projectPrefix)

	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filterArgs,
	})
	if err != nil {
		return nil, err
	}

	var result []ContainerInfo
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		result = append(result, ContainerInfo{
			ID:      c.ID,
			Name:    name,
			State:   c.State,
			Created: time.Unix(c.Created, 0),
		})
	}
	return result, nil
}

// PruneAllDockerTUIContainersForProject removes ALL ccells containers for a specific project
func (c *Client) PruneAllDockerTUIContainersForProject(ctx context.Context, projectName string) (int, error) {
	containers, err := c.ListDockerTUIContainersForProject(ctx, projectName)
	if err != nil {
		return 0, err
	}

	pruned := 0
	for _, cont := range containers {
		// Stop if running
		if cont.State == "running" {
			_ = c.StopContainer(ctx, cont.ID)
		}
		if err := c.RemoveContainer(ctx, cont.ID); err == nil {
			pruned++
		}
	}
	return pruned, nil
}

// SignalProcess sends a signal to processes matching a pattern inside a container.
// This uses pkill to find and signal processes by name.
func (c *Client) SignalProcess(ctx context.Context, containerID, processName, signal string) error {
	// Use pkill to send signal to process by name
	// pkill -TERM claude will send SIGTERM to all processes matching "claude"
	cmd := []string{"pkill", fmt.Sprintf("-%s", signal), processName}

	execCfg := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := c.cli.ContainerExecCreate(ctx, containerID, execCfg)
	if err != nil {
		return err
	}

	// Start but don't wait for output - we just want to send the signal
	return c.cli.ContainerExecStart(ctx, execID.ID, container.ExecStartOptions{})
}

// CleanupOrphanedContainers removes ccells containers for a specific project that aren't in the known list
// and don't have corresponding worktrees.
// This is used to clean up containers from crashed sessions.
// IMPORTANT: Only cleans up containers for the specified project to avoid affecting other projects.
// IMPORTANT: Never removes containers that have a corresponding worktree (worktree = work in progress).
func (c *Client) CleanupOrphanedContainers(ctx context.Context, projectName string, knownContainerIDs []string, existingWorktrees []string) (int, error) {
	// Only list containers for THIS project - don't touch other projects' containers
	containers, err := c.ListDockerTUIContainersForProject(ctx, projectName)
	if err != nil {
		return 0, err
	}

	// Build a set of known IDs for fast lookup
	known := make(map[string]bool)
	for _, id := range knownContainerIDs {
		known[id] = true
	}

	// Build a set of existing worktree names for fast lookup
	// Worktrees indicate work in progress - NEVER delete these containers
	worktrees := make(map[string]bool)
	for _, wt := range existingWorktrees {
		worktrees[wt] = true
	}

	removed := 0
	for _, cont := range containers {
		// Skip if this container is known (managed by current or resumable session)
		if known[cont.ID] {
			continue
		}

		// Skip if this container has a corresponding worktree
		// Container names are like "ccells-projectname-branchname"
		// Extract the branch name and check if worktree exists
		branchName := extractBranchFromContainerName(cont.Name, projectName)
		if branchName != "" && worktrees[branchName] {
			continue // Worktree exists - don't delete!
		}

		// Stop if running
		if cont.State == "running" || cont.State == "paused" {
			_ = c.StopContainer(ctx, cont.ID)
		}

		// Remove the orphaned container
		if err := c.RemoveContainer(ctx, cont.ID); err == nil {
			removed++
		}
	}

	return removed, nil
}

// extractBranchFromContainerName extracts the branch name from a container name.
// Container names follow the pattern: ccells-<projectname>-<branchname>
func extractBranchFromContainerName(containerName, projectName string) string {
	prefix := fmt.Sprintf("%s%s-", ContainerPrefix, projectName)
	if strings.HasPrefix(containerName, prefix) {
		return strings.TrimPrefix(containerName, prefix)
	}
	return ""
}

// PersistSessions copies Claude session files from the container's runtime location
// to the persistent mount point, so sessions survive container rebuilds.
//
// Sessions created inside containers are stored at $HOME/.claude/projects/-workspace/
// (e.g., /root/.claude/projects/-workspace/ when running as root).
// The .claude directory is mounted at /home/claude/.claude from the host.
// To persist sessions, we copy them from the runtime location to the mount point.
func (c *Client) PersistSessions(ctx context.Context, containerID string) error {
	// Shell script that:
	// 1. Finds where Claude stored sessions (varies by user: /root/.claude or /home/<user>/.claude)
	// 2. Copies session files to the mount point at /home/claude/.claude/projects/-workspace/
	// 3. Handles the case where sessions might be in multiple locations
	script := `
# Find source session directories (where Claude wrote them)
# Check $HOME/.claude/projects/-workspace/ first
SRC_DIR=""
if [ -d "$HOME/.claude/projects/-workspace" ] && [ "$HOME" != "/home/claude" ]; then
    SRC_DIR="$HOME/.claude/projects/-workspace"
elif [ -d "/root/.claude/projects/-workspace" ]; then
    SRC_DIR="/root/.claude/projects/-workspace"
fi

# Mount point where sessions should persist (this is mounted from host)
DEST_DIR="/home/claude/.claude/projects/-workspace"

# Only copy if source exists and has files, and is different from destination
if [ -n "$SRC_DIR" ] && [ "$SRC_DIR" != "$DEST_DIR" ] && [ -d "$SRC_DIR" ]; then
    # Create destination directory
    mkdir -p "$DEST_DIR" 2>/dev/null

    # Copy all session files (.jsonl files and session directories)
    # Use -n to not overwrite existing files (preserve original sessions)
    if ls "$SRC_DIR"/*.jsonl >/dev/null 2>&1; then
        for f in "$SRC_DIR"/*.jsonl; do
            if [ -f "$f" ]; then
                cp -n "$f" "$DEST_DIR/" 2>/dev/null
            fi
        done
    fi

    # Copy any subdirectories (tool results, etc.)
    for d in "$SRC_DIR"/*/; do
        if [ -d "$d" ]; then
            dirname=$(basename "$d")
            mkdir -p "$DEST_DIR/$dirname" 2>/dev/null
            cp -rn "$d"* "$DEST_DIR/$dirname/" 2>/dev/null
        fi
    done
fi
`
	cmd := []string{"/bin/sh", "-c", script}
	_, err := c.ExecInContainer(ctx, containerID, cmd)
	return err
}
