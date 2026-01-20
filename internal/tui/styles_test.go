package tui

import (
	"math"
	"strings"
	"testing"
)

func TestStatusStyle(t *testing.T) {
	tests := []struct {
		state    string
		contains string
	}{
		{"running", IndicatorRunning},
		{"idle", IndicatorIdle},
		{"pairing", IndicatorPairing},
		{"stopped", IndicatorStopped},
		{"unknown", IndicatorStopped}, // defaults to stopped
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			result := StatusStyle(tt.state)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("StatusStyle(%q) = %q, want to contain %q", tt.state, result, tt.contains)
			}
		})
	}
}

func TestKeyHint(t *testing.T) {
	result := KeyHint("n", "ew")
	if !strings.Contains(result, "n") {
		t.Error("KeyHint should contain the key")
	}
	if !strings.Contains(result, "ew") {
		t.Error("KeyHint should contain the action")
	}
}

func TestStatusIndicatorConstants(t *testing.T) {
	// Verify the indicators match the design doc
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"running", IndicatorRunning, "●"},
		{"idle", IndicatorIdle, "○"},
		{"stopped", IndicatorStopped, "◌"},
		{"pairing", IndicatorPairing, "⟳"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("Indicator%s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestPaneStyles(t *testing.T) {
	// Verify pane styles are defined and render without panic
	t.Run("active pane", func(t *testing.T) {
		result := PaneBorderActive.Render("test content")
		if result == "" {
			t.Error("PaneBorderActive should render content")
		}
	})

	t.Run("inactive pane", func(t *testing.T) {
		result := PaneBorderInactive.Render("test content")
		if result == "" {
			t.Error("PaneBorderInactive should render content")
		}
	})

	t.Run("pane title", func(t *testing.T) {
		result := PaneTitle.Render("branch-name")
		if !strings.Contains(result, "branch-name") {
			t.Error("PaneTitle should render the branch name")
		}
	})
}

func TestStatusBarStyles(t *testing.T) {
	t.Run("status bar", func(t *testing.T) {
		result := StatusBarStyle.Render("status text")
		if !strings.Contains(result, "status text") {
			t.Error("StatusBarStyle should render the text")
		}
	})

	t.Run("key hint style", func(t *testing.T) {
		result := KeyHintStyle.Render("hint")
		if !strings.Contains(result, "hint") {
			t.Error("KeyHintStyle should render the hint")
		}
	})

	t.Run("key style", func(t *testing.T) {
		result := KeyStyle.Render("k")
		if !strings.Contains(result, "k") {
			t.Error("KeyStyle should render the key")
		}
	})
}

func TestDialogStyles(t *testing.T) {
	t.Run("dialog box", func(t *testing.T) {
		result := DialogBox.Render("dialog content")
		if result == "" {
			t.Error("DialogBox should render content")
		}
	})

	t.Run("dialog title", func(t *testing.T) {
		result := DialogTitle.Render("Title")
		if !strings.Contains(result, "Title") {
			t.Error("DialogTitle should render the title")
		}
	})

	t.Run("dialog input", func(t *testing.T) {
		result := DialogInput.Render("input text")
		if result == "" {
			t.Error("DialogInput should render content")
		}
	})
}

func TestHexToRGB(t *testing.T) {
	tests := []struct {
		hex     string
		r, g, b int
	}{
		{"#FF0000", 255, 0, 0},
		{"#00FF00", 0, 255, 0},
		{"#0000FF", 0, 0, 255},
		{"#FFFFFF", 255, 255, 255},
		{"#000000", 0, 0, 0},
		{"FF0000", 255, 0, 0}, // without #
		{"", 0, 0, 0},         // empty
		{"#FFF", 0, 0, 0},     // too short
	}

	for _, tt := range tests {
		t.Run(tt.hex, func(t *testing.T) {
			r, g, b := hexToRGB(tt.hex)
			if r != tt.r || g != tt.g || b != tt.b {
				t.Errorf("hexToRGB(%q) = (%d, %d, %d), want (%d, %d, %d)",
					tt.hex, r, g, b, tt.r, tt.g, tt.b)
			}
		})
	}
}

func TestRGBToHex(t *testing.T) {
	tests := []struct {
		r, g, b int
		hex     string
	}{
		{255, 0, 0, "#FF0000"},
		{0, 255, 0, "#00FF00"},
		{0, 0, 255, "#0000FF"},
		{255, 255, 255, "#FFFFFF"},
		{0, 0, 0, "#000000"},
	}

	for _, tt := range tests {
		t.Run(tt.hex, func(t *testing.T) {
			result := rgbToHex(tt.r, tt.g, tt.b)
			if result != tt.hex {
				t.Errorf("rgbToHex(%d, %d, %d) = %q, want %q",
					tt.r, tt.g, tt.b, result, tt.hex)
			}
		})
	}
}

func TestRGBToHSLAndBack(t *testing.T) {
	tests := []struct {
		name    string
		r, g, b int
	}{
		{"red", 255, 0, 0},
		{"green", 0, 255, 0},
		{"blue", 0, 0, 255},
		{"white", 255, 255, 255},
		{"black", 0, 0, 0},
		{"gray", 128, 128, 128},
		{"cyan", 0, 255, 255},
		{"magenta", 255, 0, 255},
		{"yellow", 255, 255, 0},
		{"orange", 255, 165, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, s, l := rgbToHSL(tt.r, tt.g, tt.b)
			r2, g2, b2 := hslToRGB(h, s, l)
			// Allow small rounding differences
			if math.Abs(float64(tt.r-r2)) > 1 ||
				math.Abs(float64(tt.g-g2)) > 1 ||
				math.Abs(float64(tt.b-b2)) > 1 {
				t.Errorf("RGB->HSL->RGB conversion failed: (%d,%d,%d) -> (%f,%f,%f) -> (%d,%d,%d)",
					tt.r, tt.g, tt.b, h, s, l, r2, g2, b2)
			}
		})
	}
}

func TestMuteColor(t *testing.T) {
	tests := []struct {
		name        string
		r, g, b     int
		satFactor   float64
		briteFactor float64
	}{
		{"red full", 255, 0, 0, 1.0, 1.0},
		{"red muted", 255, 0, 0, 0.25, 0.6},
		{"green muted", 0, 255, 0, 0.25, 0.6},
		{"blue muted", 0, 0, 255, 0.25, 0.6},
		{"grayscale", 255, 0, 0, 0.0, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr, mg, mb := MuteColor(tt.r, tt.g, tt.b, tt.satFactor, tt.briteFactor)
			// Basic sanity checks
			if mr < 0 || mr > 255 || mg < 0 || mg > 255 || mb < 0 || mb > 255 {
				t.Errorf("MuteColor returned out of range values: (%d, %d, %d)", mr, mg, mb)
			}

			// If saturation is 0, result should be grayscale
			if tt.satFactor == 0.0 {
				if mr != mg || mg != mb {
					t.Errorf("Saturation 0 should produce grayscale, got (%d, %d, %d)", mr, mg, mb)
				}
			}

			// Full saturation/brightness should roughly preserve the color
			if tt.satFactor == 1.0 && tt.briteFactor == 1.0 {
				if math.Abs(float64(tt.r-mr)) > 2 ||
					math.Abs(float64(tt.g-mg)) > 2 ||
					math.Abs(float64(tt.b-mb)) > 2 {
					t.Errorf("Full saturation should preserve color: (%d,%d,%d) -> (%d,%d,%d)",
						tt.r, tt.g, tt.b, mr, mg, mb)
				}
			}
		})
	}
}

func TestLerpColor(t *testing.T) {
	tests := []struct {
		name     string
		from     string
		to       string
		progress float64
	}{
		{"start", "#FF0000", "#0000FF", 0.0},
		{"middle", "#FF0000", "#0000FF", 0.5},
		{"end", "#FF0000", "#0000FF", 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := LerpColor(tt.from, tt.to, tt.progress)
			if result == nil {
				t.Error("LerpColor should return a color")
			}
		})
	}
}
