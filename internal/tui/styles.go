package tui

import "github.com/charmbracelet/lipgloss"

// Colors
var (
	ColorRunning = lipgloss.Color("#00FF00") // Green
	ColorIdle    = lipgloss.Color("#FFFF00") // Yellow
	ColorStopped = lipgloss.Color("#808080") // Gray
	ColorPairing = lipgloss.Color("#00FFFF") // Cyan
	ColorAccent  = lipgloss.Color("#FF00FF") // Magenta
	ColorBorder  = lipgloss.Color("#444444") // Dark gray
	ColorFocus   = lipgloss.Color("#FFFFFF") // White
)

// Status indicators
const (
	IndicatorRunning = "●"
	IndicatorIdle    = "○"
	IndicatorStopped = "◌"
	IndicatorPairing = "⟳"
)

// StatusStyle returns the styled status indicator for a state.
func StatusStyle(state string) string {
	switch state {
	case "running":
		return lipgloss.NewStyle().Foreground(ColorRunning).Render(IndicatorRunning)
	case "idle":
		return lipgloss.NewStyle().Foreground(ColorIdle).Render(IndicatorIdle)
	case "pairing":
		return lipgloss.NewStyle().Foreground(ColorPairing).Render(IndicatorPairing)
	default:
		return lipgloss.NewStyle().Foreground(ColorStopped).Render(IndicatorStopped)
	}
}

// Pane styles
var (
	PaneBorderActive = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorFocus).
		Padding(0, 1)

	PaneBorderInactive = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1)

	PaneTitle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorAccent)
)

// Status bar styles
var (
	StatusBarStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("#333333")).
		Foreground(lipgloss.Color("#FFFFFF")).
		Padding(0, 1)

	KeyHintStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888"))

	KeyStyle = lipgloss.NewStyle().
		Foreground(ColorAccent).
		Bold(true)
)

// Dialog styles
var (
	DialogBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorAccent).
		Padding(1, 2).
		Width(50)

	DialogTitle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorAccent).
		MarginBottom(1)

	DialogInput = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1)
)

// KeyHint renders a key hint like "[n]ew"
func KeyHint(key, action string) string {
	return KeyStyle.Render("["+key+"]") + KeyHintStyle.Render(action)
}
