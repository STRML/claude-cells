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

// ContainerConfig holds configuration for creating a workstream container.
type ContainerConfig struct {
	Name        string // Container name (ccells-<project>-<timestamp>)
	Image       string // Docker image to use
	RepoPath    string // Path to repo on host
	ClaudeCfg   string // Path to ~/.claude directory on host
	ClaudeJSON  string // Path to ~/.claude.json file on host (session state)
	GitConfig   string // Path to ~/.gitconfig file on host (git identity)
	Credentials string // Path to credentials file (OAuth tokens from keychain)
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
	containerCfg := &container.Config{
		Image: cfg.Image,
		Cmd:   []string{"sleep", "infinity"},
		Tty:   true,
	}

	var mounts []mount.Mount
	if cfg.RepoPath != "" {
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: cfg.RepoPath,
			Target: "/workspace",
		})
	}
	if cfg.ClaudeCfg != "" {
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: cfg.ClaudeCfg,
			Target: "/home/claude/.claude",
			// Not read-only: Claude Code needs to write debug logs to ~/.claude/debug/
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

	hostCfg := &container.HostConfig{
		Mounts: mounts,
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

// CleanupOrphanedContainers removes ccells containers that aren't in the known list.
// This is used to clean up containers from crashed sessions.
func (c *Client) CleanupOrphanedContainers(ctx context.Context, knownContainerIDs []string) (int, error) {
	containers, err := c.ListDockerTUIContainers(ctx)
	if err != nil {
		return 0, err
	}

	// Build a set of known IDs for fast lookup
	known := make(map[string]bool)
	for _, id := range knownContainerIDs {
		known[id] = true
	}

	removed := 0
	for _, cont := range containers {
		// Skip if this container is known (managed by current or resumable session)
		if known[cont.ID] {
			continue
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
