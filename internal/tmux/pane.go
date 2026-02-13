package tmux

import (
	"context"
	"fmt"
	"strings"
)

// PaneInfo describes a tmux pane.
type PaneInfo struct {
	ID      string // e.g., "%0"
	Index   int
	Active  bool
	Dead    bool
	Width   int
	Height  int
	Command string
}

// SplitWindow creates a new pane in the session running the given command.
// Returns the pane ID (e.g., "%3").
func (c *Client) SplitWindow(ctx context.Context, session, command string) (string, error) {
	out, err := c.run(ctx, "split-window", "-t", session, "-P", "-F", "#{pane_id}", command)
	if err != nil {
		return "", fmt.Errorf("split-window: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// ListPanes returns all panes in a session.
func (c *Client) ListPanes(ctx context.Context, session string) ([]PaneInfo, error) {
	format := "#{pane_id}\t#{pane_index}\t#{pane_active}\t#{pane_width}\t#{pane_height}\t#{pane_current_command}\t#{pane_dead}"
	out, err := c.run(ctx, "list-panes", "-t", session, "-F", format)
	if err != nil {
		return nil, fmt.Errorf("list-panes: %w", err)
	}
	var panes []PaneInfo
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 7)
		if len(fields) < 6 {
			continue
		}
		p := PaneInfo{
			ID:      fields[0],
			Active:  fields[2] == "1",
			Command: fields[5],
		}
		if len(fields) >= 7 {
			p.Dead = fields[6] == "1"
		}
		fmt.Sscanf(fields[1], "%d", &p.Index)
		fmt.Sscanf(fields[3], "%d", &p.Width)
		fmt.Sscanf(fields[4], "%d", &p.Height)
		panes = append(panes, p)
	}
	return panes, nil
}

// KillPane destroys a pane by ID.
func (c *Client) KillPane(ctx context.Context, paneID string) error {
	_, err := c.run(ctx, "kill-pane", "-t", paneID)
	return err
}

// SelectPane focuses a pane by ID.
func (c *Client) SelectPane(ctx context.Context, paneID string) error {
	_, err := c.run(ctx, "select-pane", "-t", paneID)
	return err
}

// SetPaneOption sets a user-defined pane option (e.g., @ccells-workstream).
func (c *Client) SetPaneOption(ctx context.Context, paneID, key, value string) error {
	_, err := c.run(ctx, "set-option", "-p", "-t", paneID, key, value)
	return err
}

// GetPaneOption reads a user-defined pane option.
func (c *Client) GetPaneOption(ctx context.Context, paneID, key string) (string, error) {
	out, err := c.run(ctx, "show-option", "-p", "-t", paneID, "-v", key)
	if err != nil {
		return "", err
	}
	return out, nil
}

// SelectLayout sets the layout for panes in a session.
// Layouts: "tiled", "even-horizontal", "even-vertical", "main-horizontal", "main-vertical"
func (c *Client) SelectLayout(ctx context.Context, session, layout string) error {
	_, err := c.run(ctx, "select-layout", "-t", session, layout)
	return err
}

// SendKeys sends keystrokes to a pane.
func (c *Client) SendKeys(ctx context.Context, paneID string, keys ...string) error {
	args := append([]string{"send-keys", "-t", paneID}, keys...)
	_, err := c.run(ctx, args...)
	return err
}

// DisplayPopup shows a popup window running a command.
func (c *Client) DisplayPopup(ctx context.Context, session string, width, height int, command string) error {
	_, err := c.run(ctx, "display-popup", "-t", session,
		"-E", "-w", fmt.Sprintf("%d", width), "-h", fmt.Sprintf("%d", height), command)
	return err
}

// RespawnPane replaces a dead pane's command.
func (c *Client) RespawnPane(ctx context.Context, paneID, command string) error {
	_, err := c.run(ctx, "respawn-pane", "-t", paneID, "-k", command)
	return err
}
