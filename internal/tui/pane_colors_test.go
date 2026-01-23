package tui

import (
	"strings"
	"testing"
)

// TestClampByte tests the clampByte helper function
func TestClampByte(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{"negative clamped to 0", -100, 0},
		{"negative one clamped to 0", -1, 0},
		{"zero unchanged", 0, 0},
		{"mid-range unchanged", 128, 128},
		{"max valid unchanged", 255, 255},
		{"256 clamped to 255", 256, 255},
		{"large value clamped to 255", 1000, 255},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clampByte(tt.input); got != tt.want {
				t.Errorf("clampByte(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestStripANSI_PrivateCSI tests that stripANSI handles private CSI sequences
// like cursor visibility (\x1b[?25l, \x1b[?25h) that the basic tests don't cover.
func TestStripANSI_PrivateCSI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Private CSI sequences (cursor visibility, etc.)
		{"hide cursor", "\x1b[?25l", ""},
		{"show cursor", "\x1b[?25h", ""},
		{"text with hide cursor", "hello\x1b[?25lworld", "helloworld"},
		{"private mode set", "\x1b[?1049h", ""}, // alternate screen buffer
		{"private mode reset", "\x1b[?1049l", ""},

		// Cursor movement
		{"cursor up", "\x1b[5A", ""},
		{"cursor down", "\x1b[3B", ""},
		{"cursor forward", "\x1b[10C", ""},
		{"cursor back", "\x1b[2D", ""},
		{"cursor position", "\x1b[10;20H", ""},

		// Erase sequences
		{"erase display", "\x1b[2J", ""},
		{"erase line", "\x1b[K", ""},
		{"erase to end", "\x1b[0K", ""},

		// Mixed content with private sequences
		{"mixed with text", "start\x1b[31m\x1b[?25lmiddle\x1b[0m\x1b[?25hend", "startmiddleend"},

		// Edge cases
		{"just reset", "\x1b[0m", ""},
		{"incomplete sequence preserved", "\x1b[", "\x1b["}, // incomplete sequence preserved
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripANSI(tt.input); got != tt.want {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestColor256ToRGB_OutOfRange tests that color256ToRGB clamps out-of-range indices
func TestColor256ToRGB_OutOfRange(t *testing.T) {
	tests := []struct {
		name  string
		index int
		wantR int
		wantG int
		wantB int
	}{
		// Out of range - should clamp
		{"negative clamped to 0 (black)", -1, 0, 0, 0},
		{"negative large clamped to 0", -100, 0, 0, 0},
		{"256 clamped to 255 (grayscale)", 256, 238, 238, 238},
		{"large clamped to 255", 1000, 238, 238, 238},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotR, gotG, gotB := color256ToRGB(tt.index)
			if gotR != tt.wantR || gotG != tt.wantG || gotB != tt.wantB {
				t.Errorf("color256ToRGB(%d) = (%d, %d, %d), want (%d, %d, %d)",
					tt.index, gotR, gotG, gotB, tt.wantR, tt.wantG, tt.wantB)
			}
		})
	}
}

// TestMuteANSI_MalformedInput tests that muteANSI handles malformed sequences gracefully
func TestMuteANSI_MalformedInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"incomplete 256 color missing index", "\x1b[38;5m"},
		{"incomplete truecolor missing B", "\x1b[38;2;255;0m"},
		{"non-numeric color index", "\x1b[38;5;abcm"},
		{"invalid color type", "\x1b[38;9;100m"},
		{"out of range 256 color", "\x1b[38;5;300m"},
		{"negative in truecolor", "\x1b[38;2;-10;100;100m"},
		{"non-numeric color type", "\x1b[38;xm"},
	}

	mutedDefault := [3]int{128, 128, 128}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic on malformed input
			result := muteANSI(tt.input, 0.5, 0.8, mutedDefault)
			// Result should be non-empty (preserves something)
			if result == "" {
				t.Errorf("muteANSI(%q) returned empty string", tt.input)
			}
		})
	}
}

// TestMuteANSI_256ColorValidation tests that 256-color mode validates indices
func TestMuteANSI_256ColorValidation(t *testing.T) {
	mutedDefault := [3]int{128, 128, 128}

	// Valid 256-color should be converted to truecolor
	validInput := "\x1b[38;5;196mtext\x1b[0m"
	validResult := muteANSI(validInput, 0.5, 0.8, mutedDefault)
	if !strings.Contains(validResult, "38;2;") {
		t.Errorf("valid 256-color should be converted to truecolor, got %q", validResult)
	}

	// Invalid 256-color (out of range) should preserve original code
	invalidInput := "\x1b[38;5;300mtext\x1b[0m"
	invalidResult := muteANSI(invalidInput, 0.5, 0.8, mutedDefault)
	// Should still have some output and not panic
	if invalidResult == "" {
		t.Error("invalid 256-color should not return empty string")
	}
}

// TestMuteANSI_TruecolorValidation tests that truecolor mode validates and clamps RGB values
func TestMuteANSI_TruecolorValidation(t *testing.T) {
	mutedDefault := [3]int{128, 128, 128}

	// Valid truecolor should be processed
	validInput := "\x1b[38;2;255;128;0mtext\x1b[0m"
	validResult := muteANSI(validInput, 0.5, 0.8, mutedDefault)
	if !strings.Contains(validResult, "38;2;") {
		t.Errorf("valid truecolor should remain truecolor, got %q", validResult)
	}

	// Out of range values should be clamped, not error
	outOfRangeInput := "\x1b[38;2;300;400;500mtext\x1b[0m"
	outOfRangeResult := muteANSI(outOfRangeInput, 0.5, 0.8, mutedDefault)
	if outOfRangeResult == "" {
		t.Error("out-of-range truecolor should not return empty string")
	}
	// Should still contain truecolor format with clamped values
	if !strings.Contains(outOfRangeResult, "38;2;") {
		t.Errorf("out-of-range truecolor should be converted to valid truecolor, got %q", outOfRangeResult)
	}
}
