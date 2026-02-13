package tmux

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// StatusWorkstream is the data needed to render a workstream in the status line.
type StatusWorkstream struct {
	Name     string
	Status   string // "running", "paused", "exited"
	HasPR    bool
	PRMerged bool
}

// FormatStatusLine renders the tmux status line content.
// If multiLine is true, returns a two-line format (workstream tabs + keyhints).
func FormatStatusLine(workstreams []StatusWorkstream, prefix string, multiLine bool) string {
	hint := FormatPrefixHint(prefix)

	// Build workstream indicators
	var wsItems []string
	for _, ws := range workstreams {
		indicator := ws.Name
		switch ws.Status {
		case "paused":
			indicator += " [paused]"
		case "exited":
			indicator += " [exited]"
		}
		if ws.HasPR && ws.PRMerged {
			indicator += " ✓"
		} else if ws.HasPR {
			indicator += " ●"
		}
		wsItems = append(wsItems, indicator)
	}

	wsPart := strings.Join(wsItems, " | ")
	keyhints := fmt.Sprintf("%s+n new  %s+d destroy  %s+m merge  %s+? help", hint, hint, hint, hint)

	if multiLine {
		return fmt.Sprintf("[ccells] %s\n%s", wsPart, keyhints)
	}

	// Single line: compact
	return fmt.Sprintf("[ccells] %s | %s", wsPart, keyhints)
}

// FormatPaneBorder renders the text for a pane's top border.
func FormatPaneBorder(name, status string, prNumber int, synopsis string) string {
	parts := []string{name}

	if status != "running" {
		parts = append(parts, fmt.Sprintf("[%s]", status))
	}

	if prNumber > 0 {
		parts = append(parts, fmt.Sprintf("PR#%d", prNumber))
	}

	if synopsis != "" {
		parts = append(parts, "- "+synopsis)
	}

	return "─── " + strings.Join(parts, " ") + " ───"
}

// FormatPrefixHint converts a tmux prefix like "C-b" to a display hint like "^b".
func FormatPrefixHint(prefix string) string {
	if strings.HasPrefix(prefix, "C-") {
		return "^" + strings.TrimPrefix(prefix, "C-")
	}
	return prefix
}

// tmuxVersionAtLeast returns true if the tmux version is >= major.minor.
func tmuxVersionAtLeast(c *Client, ctx context.Context, major, minor int) bool {
	ver, err := c.Version(ctx)
	if err != nil {
		return false
	}
	parts := strings.SplitN(ver, ".", 3)
	if len(parts) < 1 {
		return false
	}
	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	if maj > major {
		return true
	}
	if maj < major {
		return false
	}
	if len(parts) < 2 {
		return minor == 0
	}
	// Strip non-numeric suffix (e.g., "4a" → "4")
	minStr := strings.TrimRight(parts[1], "abcdefghijklmnopqrstuvwxyz")
	min, err := strconv.Atoi(minStr)
	if err != nil {
		return false
	}
	return min >= minor
}

// escapeShellArg quotes a string for safe embedding in shell commands.
// Uses single-quoting with proper escaping of embedded single quotes.
func escapeShellArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// ConfigureChrome sets up tmux status line, pane borders, and keybindings.
func (c *Client) ConfigureChrome(ctx context.Context, session, ccellsBin string) error {
	bin := escapeShellArg(ccellsBin)

	// Pane border styling
	if _, err := c.run(ctx, "set-option", "-t", session, "-g", "pane-border-format",
		" #{@ccells-border-text} "); err != nil {
		return fmt.Errorf("set pane-border-format: %w", err)
	}
	if _, err := c.run(ctx, "set-option", "-t", session, "-g", "pane-border-status", "top"); err != nil {
		return fmt.Errorf("set pane-border-status: %w", err)
	}
	if _, err := c.run(ctx, "set-option", "-t", session, "-g", "pane-active-border-style", "fg=cyan"); err != nil {
		return fmt.Errorf("set pane-active-border-style: %w", err)
	}
	if _, err := c.run(ctx, "set-option", "-t", session, "-g", "pane-border-style", "fg=colour240"); err != nil {
		return fmt.Errorf("set pane-border-style: %w", err)
	}

	// Status line — polls ccells status every 5s
	if _, err := c.run(ctx, "set-option", "-t", session, "-g", "status-right",
		fmt.Sprintf("#(%s status --format=tmux)", bin)); err != nil {
		return fmt.Errorf("set status-right: %w", err)
	}
	if _, err := c.run(ctx, "set-option", "-t", session, "-g", "status-right-length", "120"); err != nil {
		return fmt.Errorf("set status-right-length: %w", err)
	}
	if _, err := c.run(ctx, "set-option", "-t", session, "-g", "status-interval", "5"); err != nil {
		return fmt.Errorf("set status-interval: %w", err)
	}
	if _, err := c.run(ctx, "set-option", "-t", session, "-g", "status-left", "[ccells] "); err != nil {
		return fmt.Errorf("set status-left: %w", err)
	}

	// Multi-line status if tmux >= 3.4
	if tmuxVersionAtLeast(c, ctx, 3, 4) {
		if _, err := c.run(ctx, "set-option", "-t", session, "-g", "status", "2"); err != nil {
			return fmt.Errorf("set status lines: %w", err)
		}
	}

	// Keybindings
	bindings := map[string]string{
		"n": fmt.Sprintf("display-popup -E -w 60 -h 20 %s create --interactive", bin),
		"d": fmt.Sprintf("display-popup -E -w 60 -h 15 %s rm --interactive", bin),
		"m": fmt.Sprintf("display-popup -E -w 70 -h 20 %s merge --interactive", bin),
		"p": fmt.Sprintf("run-shell \"%s pause #{@ccells-workstream}\"", bin),
		"r": fmt.Sprintf("run-shell \"%s unpause #{@ccells-workstream}\"", bin),
		"s": "refresh-client -S",
		"?": fmt.Sprintf("display-popup -E -w 50 -h 15 %s help --keybindings", bin),
	}

	for key, cmd := range bindings {
		if _, err := c.run(ctx, "bind-key", key, cmd); err != nil {
			return fmt.Errorf("bind-key %s: %w", key, err)
		}
	}

	return nil
}
