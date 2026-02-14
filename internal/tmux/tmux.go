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

// NewSession creates a new detached tmux session.
func (c *Client) NewSession(ctx context.Context, name string) error {
	_, err := c.run(ctx, "new-session", "-d", "-s", name, "-x", "200", "-y", "50")
	return err
}

// NewSessionWithCommand creates a new detached tmux session whose initial pane
// runs the given shell command instead of the default shell.
func (c *Client) NewSessionWithCommand(ctx context.Context, name, command string) error {
	_, err := c.run(ctx, "new-session", "-d", "-s", name, "-x", "200", "-y", "50", command)
	return err
}

// HasSession checks if a named session exists.
func (c *Client) HasSession(ctx context.Context, name string) (bool, error) {
	_, err := c.run(ctx, "has-session", "-t", name)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// KillServer kills the tmux server on this socket.
func (c *Client) KillServer(ctx context.Context) error {
	_, err := c.run(ctx, "kill-server")
	return err
}

// KillSession kills a specific session.
func (c *Client) KillSession(ctx context.Context, name string) error {
	_, err := c.run(ctx, "kill-session", "-t", name)
	return err
}

// AttachCommand returns an exec.Cmd that attaches to a session.
// This should be the last call — it exec's tmux attach.
func (c *Client) AttachCommand(name string) *exec.Cmd {
	return exec.Command("tmux", "-L", c.socket, "attach-session", "-t", name)
}
