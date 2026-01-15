package docker

import (
	"bytes"
	"context"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/pkg/stdcopy"
)

// ContainerConfig holds configuration for creating a workstream container.
type ContainerConfig struct {
	Name      string // Container name (docker-tui-<branch>)
	Image     string // Docker image to use
	RepoPath  string // Path to repo on host
	ClaudeCfg string // Path to ~/.claude on host
}

// NewContainerConfig creates a container config for a workstream.
func NewContainerConfig(branchName, repoPath string) *ContainerConfig {
	// Sanitize branch name for container naming
	safeName := strings.ReplaceAll(branchName, "/", "-")
	return &ContainerConfig{
		Name:     "docker-tui-" + safeName,
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
			Type:     mount.TypeBind,
			Source:   cfg.ClaudeCfg,
			Target:   "/root/.claude",
			ReadOnly: true,
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
