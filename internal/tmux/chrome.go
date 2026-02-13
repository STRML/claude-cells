package tmux

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

// tmux color constants matching the old Bubble Tea TUI palette.
const (
	colorGreen    = "colour46"  // Running dot, PR merged, workstream count
	colorYellow   = "colour226" // Paused
	colorGray     = "colour240" // Exited, inactive border
	colorMagenta  = "colour201" // Workstream name, key letter
	colorHintGray = "colour244" // Key hint text
	colorCyan     = "colour38"  // Active border, PR open
	colorBarBg    = "colour236" // Status bar background
	colorWhite    = "colour255" // Default text

	// Powerline status-left colors
	colorPathBg   = "colour34"  // Green background for path segment
	colorBranchBg = "colour142" // Olive/yellow-green for branch segment
	colorDarkText = "colour234" // Dark text on colored backgrounds

	// Powerline separator character (U+E0B0)
	powerlineSep = "\ue0b0"
	// Git branch icon (U+E0A0)
	branchIcon = "\ue0a0"
)

// StatusWorkstream is the data needed to render a workstream in the status line.
type StatusWorkstream struct {
	Name     string
	Status   string // "running", "paused", "exited"
	HasPR    bool
	PRMerged bool
}

// FormatStatusLine renders the tmux status line content with color formatting.
// Returns tmux #[fg=...] style sequences for use in tmux session variables.
func FormatStatusLine(workstreams []StatusWorkstream, prefix string, multiLine bool) string {
	// Build colored workstream indicators
	var wsItems []string
	for _, ws := range workstreams {
		var indicator string

		// Status dot
		switch ws.Status {
		case "paused":
			indicator = fmt.Sprintf("#[fg=%s]●#[default] ", colorYellow)
		case "exited":
			indicator = fmt.Sprintf("#[fg=%s]●#[default] ", colorGray)
		default:
			indicator = fmt.Sprintf("#[fg=%s]●#[default] ", colorGreen)
		}

		// Workstream name in magenta
		indicator += fmt.Sprintf("#[fg=%s]%s#[default]", colorMagenta, ws.Name)

		// PR badge
		if ws.HasPR && ws.PRMerged {
			indicator += fmt.Sprintf(" #[fg=%s]✓#[default]", colorGreen)
		} else if ws.HasPR {
			indicator += fmt.Sprintf(" #[fg=%s]●#[default]", colorCyan)
		}

		wsItems = append(wsItems, indicator)
	}

	wsPart := strings.Join(wsItems, "  ")

	// Workstream count
	countPart := fmt.Sprintf("#[fg=%s]%d#[default] workstream", colorGreen, len(workstreams))
	if len(workstreams) != 1 {
		countPart += "s"
	}

	if multiLine {
		return fmt.Sprintf("%s  %s", wsPart, countPart)
	}

	// Single line: compact
	hint := FormatPrefixHint(prefix)
	keyhints := formatColoredKeyhints(hint)
	return fmt.Sprintf("%s  %s | %s", wsPart, countPart, keyhints)
}

// formatColoredKeyhints returns keyhints with tmux color codes.
func formatColoredKeyhints(hint string) string {
	keys := []struct{ key, label string }{
		{"n", "new"},
		{"x", "destroy"},
		{"m", "merge"},
		{"?", "help"},
	}
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("#[fg=%s]%s+%s#[fg=%s] %s#[default]",
			colorMagenta, hint, k.key, colorHintGray, k.label))
	}
	return strings.Join(parts, "  ")
}

// FormatPaneBorder renders the text for a pane's top border with tmux color codes.
func FormatPaneBorder(name, status string, prNumber int, synopsis string) string {
	var parts []string

	// Status dot
	switch status {
	case "paused":
		parts = append(parts, fmt.Sprintf("#[fg=%s]●#[default]", colorYellow))
	case "exited":
		parts = append(parts, fmt.Sprintf("#[fg=%s]●#[default]", colorGray))
	default:
		parts = append(parts, fmt.Sprintf("#[fg=%s]●#[default]", colorGreen))
	}

	// Workstream name in magenta
	parts = append(parts, fmt.Sprintf("#[fg=%s]%s#[default]", colorMagenta, name))

	if prNumber > 0 {
		parts = append(parts, fmt.Sprintf("#[fg=%s]PR#%d#[default]", colorCyan, prNumber))
	}

	if synopsis != "" {
		parts = append(parts, fmt.Sprintf("#[fg=%s]- %s#[default]", colorHintGray, synopsis))
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

// AbbreviatePath shortens a path by abbreviating all directory components
// except the last to their first character. Replaces $HOME with ~.
// Example: /Users/sam/git/oss/claude-cells → ~/g/o/claude-cells
func AbbreviatePath(path string) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(path, home) {
		path = "~" + path[len(home):]
	}

	parts := strings.Split(path, "/")
	if len(parts) <= 1 {
		return path
	}

	// Abbreviate all but the last component
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "" || parts[i] == "~" {
			continue
		}
		r, _ := utf8.DecodeRuneInString(parts[i])
		if r != utf8.RuneError {
			parts[i] = string(r)
		}
	}

	return strings.Join(parts, "/")
}

// FormatPowerlineLeft builds a powerline-style tmux status-left string
// with path and branch segments.
func FormatPowerlineLeft(repoPath, branch string) string {
	abbrev := AbbreviatePath(repoPath)

	var b strings.Builder
	// Path segment: green bg, white bold text
	b.WriteString(fmt.Sprintf("#[bg=%s,fg=%s,bold] %s ", colorPathBg, colorWhite, abbrev))
	// Separator: path→branch transition
	b.WriteString(fmt.Sprintf("#[fg=%s,bg=%s,nobold]%s", colorPathBg, colorBranchBg, powerlineSep))
	// Branch segment: olive bg, dark text
	b.WriteString(fmt.Sprintf("#[fg=%s,bg=%s] %s %s ", colorDarkText, colorBranchBg, branchIcon, branch))
	// Separator: branch→bar bg transition
	b.WriteString(fmt.Sprintf("#[fg=%s,bg=%s]%s", colorBranchBg, colorBarBg, powerlineSep))
	b.WriteString("#[default]")

	return b.String()
}

// ConfigureChrome sets up tmux status line, pane borders, and keybindings.
func (c *Client) ConfigureChrome(ctx context.Context, session, ccellsBin, repoPath, branch string) error {
	bin := escapeShellArg(ccellsBin)

	// Pane border styling with color support
	if _, err := c.run(ctx, "set-option", "-t", session, "-g", "pane-border-format",
		" #{@ccells-border-text} "); err != nil {
		return fmt.Errorf("set pane-border-format: %w", err)
	}
	if _, err := c.run(ctx, "set-option", "-t", session, "-g", "pane-border-status", "top"); err != nil {
		return fmt.Errorf("set pane-border-status: %w", err)
	}
	if _, err := c.run(ctx, "set-option", "-t", session, "-g", "pane-active-border-style",
		fmt.Sprintf("fg=%s", colorCyan)); err != nil {
		return fmt.Errorf("set pane-active-border-style: %w", err)
	}
	if _, err := c.run(ctx, "set-option", "-t", session, "-g", "pane-border-style",
		fmt.Sprintf("fg=%s", colorGray)); err != nil {
		return fmt.Errorf("set pane-border-style: %w", err)
	}

	// Status bar background and foreground
	if _, err := c.run(ctx, "set-option", "-t", session, "-g", "status-style",
		fmt.Sprintf("bg=%s,fg=%s", colorBarBg, colorWhite)); err != nil {
		return fmt.Errorf("set status-style: %w", err)
	}

	// Status left: powerline-style path + branch
	statusLeft := FormatPowerlineLeft(repoPath, branch)
	if _, err := c.run(ctx, "set-option", "-t", session, "-g", "status-left", statusLeft); err != nil {
		return fmt.Errorf("set status-left: %w", err)
	}
	if _, err := c.run(ctx, "set-option", "-t", session, "-g", "status-left-length", "80"); err != nil {
		return fmt.Errorf("set status-left-length: %w", err)
	}

	// Status right: uses #() command substitution which interprets #[...] color sequences in the output.
	// tmux does NOT interpret #[...] inside #{@variable} expansions, so we use #() directly.
	statusRight := fmt.Sprintf("#(%s status --format=tmux)", bin)
	if _, err := c.run(ctx, "set-option", "-t", session, "-g", "status-right", statusRight); err != nil {
		return fmt.Errorf("set status-right: %w", err)
	}
	if _, err := c.run(ctx, "set-option", "-t", session, "-g", "status-right-length", "200"); err != nil {
		return fmt.Errorf("set status-right-length: %w", err)
	}
	if _, err := c.run(ctx, "set-option", "-t", session, "-g", "status-interval", "5"); err != nil {
		return fmt.Errorf("set status-interval: %w", err)
	}

	// Hide the default tmux window list (we don't use windows, only panes)
	if _, err := c.run(ctx, "set-option", "-t", session, "-g", "window-status-format", ""); err != nil {
		return fmt.Errorf("set window-status-format: %w", err)
	}
	if _, err := c.run(ctx, "set-option", "-t", session, "-g", "window-status-current-format", ""); err != nil {
		return fmt.Errorf("set window-status-current-format: %w", err)
	}

	// Keybindings
	// Note: "d" is reserved for tmux's default detach, so destroy uses "x"
	createCmd := fmt.Sprintf("split-window %s create --interactive", bin)
	bindings := map[string]string{
		"n": createCmd,
		"N": createCmd,
		`"`: createCmd, // Override tmux split-horizontal with create
		"%": createCmd, // Override tmux split-vertical with create
		"x": fmt.Sprintf("display-popup -E -w 60 -h 15 %s rm --interactive", bin),
		"m": fmt.Sprintf("display-popup -E -w 70 -h 20 %s merge --interactive", bin),
		"p": fmt.Sprintf("run-shell \"%s pause #{@ccells-workstream}\"", bin),
		"r": fmt.Sprintf("run-shell \"%s unpause #{@ccells-workstream}\"", bin),
		"?": fmt.Sprintf("display-popup -E -w 55 -h 22 %s help --keybindings", bin),
	}

	for key, cmd := range bindings {
		if _, err := c.run(ctx, "bind-key", key, cmd); err != nil {
			return fmt.Errorf("bind-key %s: %w", key, err)
		}
	}

	// Pane resizing with Alt+arrow keys (prefix-less for convenience)
	resizeBindings := map[string]string{
		"M-Up":    "resize-pane -U 5",
		"M-Down":  "resize-pane -D 5",
		"M-Left":  "resize-pane -L 5",
		"M-Right": "resize-pane -R 5",
	}
	for key, cmd := range resizeBindings {
		if _, err := c.run(ctx, "bind-key", "-n", key, cmd); err != nil {
			return fmt.Errorf("bind-key %s: %w", key, err)
		}
	}

	return nil
}

// SetSessionOption sets a session-level option on the tmux session.
func (c *Client) SetSessionOption(ctx context.Context, session, option, value string) error {
	_, err := c.run(ctx, "set-option", "-t", session, option, value)
	return err
}

// SetHook sets a tmux hook on a session. The hookName should include the index (e.g., "client-attached[99]").
func (c *Client) SetHook(ctx context.Context, session, hookName, command string) error {
	_, err := c.run(ctx, "set-hook", "-t", session, hookName, command)
	return err
}
