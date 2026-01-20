package tui

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/lipgloss"
)

// hexToRGB converts a hex color string to RGB values
func hexToRGB(hex string) (r, g, b int) {
	if len(hex) == 0 {
		return 0, 0, 0
	}
	if hex[0] == '#' {
		hex = hex[1:]
	}
	if len(hex) != 6 {
		return 0, 0, 0
	}
	rVal, _ := strconv.ParseInt(hex[0:2], 16, 64)
	gVal, _ := strconv.ParseInt(hex[2:4], 16, 64)
	bVal, _ := strconv.ParseInt(hex[4:6], 16, 64)
	return int(rVal), int(gVal), int(bVal)
}

// rgbToHex converts RGB values to a hex color string
func rgbToHex(r, g, b int) string {
	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}

// easeOutCubic provides smooth deceleration for animations
func easeOutCubic(t float64) float64 {
	return 1 - (1-t)*(1-t)*(1-t)
}

// LerpColor interpolates between two hex colors based on progress (0.0 to 1.0)
// Uses easeOutCubic for smooth animation feel
func LerpColor(from, to string, progress float64) lipgloss.Color {
	// Apply easing
	t := easeOutCubic(progress)

	r1, g1, b1 := hexToRGB(from)
	r2, g2, b2 := hexToRGB(to)

	r := int(float64(r1) + t*(float64(r2)-float64(r1)))
	g := int(float64(g1) + t*(float64(g2)-float64(g1)))
	b := int(float64(b1) + t*(float64(b2)-float64(b1)))

	return lipgloss.Color(rgbToHex(r, g, b))
}

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

	// Enhanced input styling for text inputs
	DialogInputFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorAccent).
				Padding(0, 1)

	DialogInputPrompt = lipgloss.NewStyle().
				Foreground(ColorAccent).
				Bold(true)

	DialogInputCursor = lipgloss.NewStyle().
				Foreground(ColorAccent)

	DialogInputText = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF"))

	DialogInputPlaceholder = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#666666")).
				Italic(true)
)

// KeyHint renders a key hint like "[n]ew"
func KeyHint(key, action string) string {
	return KeyStyle.Render("["+key+"]") + KeyHintStyle.Render(action)
}
