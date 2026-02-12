package tmux

import (
	"context"
	"fmt"
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

// ConfigureChrome sets up tmux status line, pane borders, and keybindings.
func (c *Client) ConfigureChrome(ctx context.Context, session, ccellsBin string) error {
	// Pane border styling
	c.run(ctx, "set-option", "-t", session, "-g", "pane-border-format",
		" #{@ccells-border-text} ")
	c.run(ctx, "set-option", "-t", session, "-g", "pane-border-status", "top")
	c.run(ctx, "set-option", "-t", session, "-g", "pane-active-border-style", "fg=cyan")
	c.run(ctx, "set-option", "-t", session, "-g", "pane-border-style", "fg=colour240")

	// Status line — polls ccells status every 5s
	c.run(ctx, "set-option", "-t", session, "-g", "status-right",
		fmt.Sprintf("#(%s status --format=tmux)", ccellsBin))
	c.run(ctx, "set-option", "-t", session, "-g", "status-right-length", "120")
	c.run(ctx, "set-option", "-t", session, "-g", "status-interval", "5")
	c.run(ctx, "set-option", "-t", session, "-g", "status-left", "[ccells] ")

	// Multi-line status if tmux >= 3.4
	ver, _ := c.Version(ctx)
	if ver >= "3.4" {
		c.run(ctx, "set-option", "-t", session, "-g", "status", "2")
	}

	// Keybindings
	bindings := map[string]string{
		"n": fmt.Sprintf("display-popup -E -w 60 -h 20 '%s create --interactive'", ccellsBin),
		"d": fmt.Sprintf("display-popup -E -w 60 -h 15 '%s rm --interactive'", ccellsBin),
		"m": fmt.Sprintf("display-popup -E -w 70 -h 20 '%s merge --interactive'", ccellsBin),
		"p": fmt.Sprintf("run-shell '%s pause #{@ccells-workstream}'", ccellsBin),
		"r": fmt.Sprintf("run-shell '%s unpause #{@ccells-workstream}'", ccellsBin),
		"s": "refresh-client -S",
		"?": fmt.Sprintf("display-popup -E -w 50 -h 15 '%s help --keybindings'", ccellsBin),
	}

	for key, cmd := range bindings {
		c.run(ctx, "bind-key", key, cmd)
	}

	return nil
}
