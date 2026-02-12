package tmux

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Client wraps tmux CLI commands for a specific server socket.
type Client struct {
	socket string
}

// NewClient creates a tmux client targeting the given socket name.
func NewClient(socket string) *Client {
	return &Client{socket: socket}
}

// Socket returns the socket name.
func (c *Client) Socket() string {
	return c.socket
}

// run executes a tmux command with the socket flag.
func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	fullArgs := append([]string{"-L", c.socket}, args...)
	cmd := exec.CommandContext(ctx, "tmux", fullArgs...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// ServerRunning checks if a tmux server is running on this socket.
func (c *Client) ServerRunning(ctx context.Context) (bool, error) {
	_, err := c.run(ctx, "list-sessions")
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Exit code 1 = no server running
			if exitErr.ExitCode() == 1 {
				return false, nil
			}
		}
		// tmux not installed or other error
		if strings.Contains(err.Error(), "executable file not found") {
			return false, fmt.Errorf("tmux not installed: %w", err)
		}
		return false, nil
	}
	return true, nil
}

// Version returns the tmux version string (e.g., "3.4").
func (c *Client) Version(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "tmux", "-V").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux not installed: %w", err)
	}
	// "tmux 3.4" -> "3.4"
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) >= 2 {
		return parts[1], nil
	}
	return strings.TrimSpace(string(out)), nil
}

// Prefix returns the user's configured tmux prefix key (e.g., "C-b").
func (c *Client) Prefix(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "show-option", "-gv", "prefix")
	if err != nil {
		return "C-b", nil // default fallback
	}
	return out, nil
}
