package tui

import (
	"fmt"
	"math"
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

// rgbToHSL converts RGB (0-255) to HSL (h: 0-360, s: 0-1, l: 0-1)
func rgbToHSL(r, g, b int) (h, s, l float64) {
	rf := float64(r) / 255.0
	gf := float64(g) / 255.0
	bf := float64(b) / 255.0

	maxC := math.Max(rf, math.Max(gf, bf))
	minC := math.Min(rf, math.Min(gf, bf))
	delta := maxC - minC

	l = (maxC + minC) / 2.0

	if delta == 0 {
		h = 0
		s = 0
	} else {
		if l < 0.5 {
			s = delta / (maxC + minC)
		} else {
			s = delta / (2.0 - maxC - minC)
		}

		switch maxC {
		case rf:
			h = (gf - bf) / delta
			if gf < bf {
				h += 6
			}
		case gf:
			h = (bf-rf)/delta + 2
		case bf:
			h = (rf-gf)/delta + 4
		}
		h *= 60
	}
	return h, s, l
}

// hslToRGB converts HSL (h: 0-360, s: 0-1, l: 0-1) to RGB (0-255)
func hslToRGB(h, s, l float64) (r, g, b int) {
	if s == 0 {
		v := int(math.Round(l * 255))
		return v, v, v
	}

	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q

	hNorm := h / 360.0

	hueToRGB := func(t float64) float64 {
		if t < 0 {
			t += 1
		}
		if t > 1 {
			t -= 1
		}
		if t < 1.0/6.0 {
			return p + (q-p)*6*t
		}
		if t < 0.5 {
			return q
		}
		if t < 2.0/3.0 {
			return p + (q-p)*(2.0/3.0-t)*6
		}
		return p
	}

	r = int(math.Round(hueToRGB(hNorm+1.0/3.0) * 255))
	g = int(math.Round(hueToRGB(hNorm) * 255))
	b = int(math.Round(hueToRGB(hNorm-1.0/3.0) * 255))

	// Clamp values
	r = clampInt(r, 0, 255)
	g = clampInt(g, 0, 255)
	b = clampInt(b, 0, 255)

	return r, g, b
}

// clampInt clamps an int to a range
func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// MuteColor reduces the saturation and optionally brightness of a color
// saturationFactor: 0.0 = grayscale, 1.0 = original saturation
// brightnessFactor: multiplier for lightness (1.0 = no change)
func MuteColor(r, g, b int, saturationFactor, brightnessFactor float64) (int, int, int) {
	h, s, l := rgbToHSL(r, g, b)
	s *= saturationFactor
	l *= brightnessFactor
	// Clamp lightness
	if l > 1.0 {
		l = 1.0
	}
	if l < 0.0 {
		l = 0.0
	}
	return hslToRGB(h, s, l)
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
