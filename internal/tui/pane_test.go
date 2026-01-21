package tui

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/STRML/claude-cells/internal/workstream"
)

func TestNewPaneModel(t *testing.T) {
	ws := workstream.New("test prompt")
	pane := NewPaneModel(ws)

	if pane.workstream != ws {
		t.Error("Pane should reference the workstream")
	}
	if pane.focused {
		t.Error("Pane should not be focused initially")
	}
}

func TestPaneModel_SetFocused(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	pane.SetFocused(true)
	if !pane.focused {
		t.Error("Pane should be focused after SetFocused(true)")
	}

	pane.SetFocused(false)
	if pane.focused {
		t.Error("Pane should not be focused after SetFocused(false)")
	}
}

func TestPaneModel_SetSize(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	pane.SetSize(80, 24)
	if pane.width != 80 {
		t.Errorf("width = %d, want 80", pane.width)
	}
	if pane.height != 24 {
		t.Errorf("height = %d, want 24", pane.height)
	}
}

func TestPaneModel_AppendOutput(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	pane.AppendOutput("Hello")
	pane.AppendOutput(" World")

	if !strings.Contains(pane.output.String(), "Hello World") {
		t.Errorf("output = %q, want to contain 'Hello World'", pane.output.String())
	}
}

func TestPaneModel_View(t *testing.T) {
	ws := workstream.New("test prompt")
	ws.SetState(workstream.StateRunning)
	pane := NewPaneModel(ws)
	pane.SetSize(40, 10)

	view := pane.View()

	// Should contain the branch name
	if !strings.Contains(view, ws.BranchName) {
		t.Error("View should contain branch name")
	}
	// Should contain status indicator
	if !strings.Contains(view, IndicatorRunning) {
		t.Error("View should contain running indicator")
	}
}

func TestPaneModel_Workstream(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	if pane.Workstream() != ws {
		t.Error("Workstream() should return the workstream")
	}
}

func TestPaneModel_VTerm_Initialized(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	if pane.vterm == nil {
		t.Error("vterm should be initialized")
	}
}

func TestPaneModel_WritePTYOutput(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)

	// Write some data to vterm
	pane.WritePTYOutput([]byte("Hello from PTY\r\n"))

	// Render and check
	output := pane.renderVTerm()
	if !strings.Contains(output, "Hello from PTY") {
		t.Errorf("renderVTerm() = %q, want to contain 'Hello from PTY'", output)
	}
}

func TestPaneModel_WritePTYOutput_WithCarriageReturn(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)

	// Write with carriage return (should overwrite line)
	pane.WritePTYOutput([]byte("First line\r\n"))
	pane.WritePTYOutput([]byte("Overwrite\r"))
	pane.WritePTYOutput([]byte("NewText"))

	output := pane.renderVTerm()

	if !strings.Contains(output, "First line") {
		t.Errorf("renderVTerm() should contain 'First line', got: %q", output)
	}
	if !strings.Contains(output, "NewText") {
		t.Errorf("renderVTerm() should contain 'NewText', got: %q", output)
	}
}

func TestPaneModel_renderVTerm_AfterResize(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	// Start with one size
	pane.SetSize(80, 24)
	pane.WritePTYOutput([]byte("Test content\r\n"))

	output1 := pane.renderVTerm()
	if output1 == "" {
		t.Error("renderVTerm() should not be empty after write")
	}

	// Resize to different sizes - should not panic
	pane.SetSize(120, 40)
	output2 := pane.renderVTerm()
	_ = output2 // Just ensure no panic

	pane.SetSize(40, 10)
	output3 := pane.renderVTerm()
	_ = output3 // Just ensure no panic
}

func TestPaneModel_renderVTerm_EmptyVterm(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)

	// Render empty vterm - should not panic
	output := pane.renderVTerm()
	_ = output
}

func TestPaneModel_renderVTerm_CachesLastRender(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)

	pane.WritePTYOutput([]byte("Cached content\r\n"))
	output := pane.renderVTerm()

	if pane.lastVtermRender == "" {
		t.Error("lastVtermRender should be set after successful render")
	}
	if pane.lastVtermRender != output {
		t.Error("lastVtermRender should match the returned output")
	}
}

func TestPaneModel_HasPTY(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	if pane.HasPTY() {
		t.Error("HasPTY() should be false initially")
	}

	pane.SetPTY(nil)
	if pane.HasPTY() {
		t.Error("HasPTY() should be false with nil PTY")
	}
}

func TestPaneModel_ScrollMode(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)

	// Initially not in scroll mode
	if pane.IsScrollMode() {
		t.Error("Pane should not be in scroll mode initially")
	}

	// Enter scroll mode
	pane.EnterScrollMode()
	if !pane.IsScrollMode() {
		t.Error("Pane should be in scroll mode after EnterScrollMode()")
	}

	// Exit scroll mode
	pane.ScrollToBottom()
	if pane.IsScrollMode() {
		t.Error("Pane should not be in scroll mode after ScrollToBottom()")
	}
}

func TestPaneModel_ScrollLineUp(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)

	// Add some content so we have something to scroll
	for i := 0; i < 50; i++ {
		pane.AppendOutput("Line content\n")
	}

	// ScrollLineUp should enter scroll mode
	pane.ScrollLineUp()
	if !pane.IsScrollMode() {
		t.Error("ScrollLineUp() should enter scroll mode")
	}
}

func TestPaneModel_ScrollLineDown(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)

	// Add content and enter scroll mode
	for i := 0; i < 50; i++ {
		pane.AppendOutput("Line content\n")
	}
	pane.ScrollPageUp() // Enter scroll mode and scroll up

	if !pane.IsScrollMode() {
		t.Error("Should be in scroll mode after ScrollPageUp()")
	}

	// ScrollLineDown should stay in scroll mode until at bottom
	pane.ScrollLineDown()
	// We're still not at bottom, so should still be in scroll mode
	if !pane.IsScrollMode() {
		t.Error("ScrollLineDown() should stay in scroll mode when not at bottom")
	}
}

func TestPaneModel_ScrollPageUp(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)

	// ScrollPageUp should enter scroll mode
	pane.ScrollPageUp()
	if !pane.IsScrollMode() {
		t.Error("ScrollPageUp() should enter scroll mode")
	}
}

func TestPaneModel_View_ScrollModeIndicator(t *testing.T) {
	ws := workstream.New("test prompt")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)
	pane.SetIndex(1)
	pane.SetFocused(true)

	// Not in scroll mode - should show NAV or INPUT
	pane.SetInputMode(false)
	view := pane.View()
	if !strings.Contains(view, "NAV") {
		t.Error("Focused pane not in scroll mode should show NAV indicator")
	}

	// Enter scroll mode - should show SCROLL
	pane.EnterScrollMode()
	view = pane.View()
	if !strings.Contains(view, "SCROLL") {
		t.Error("Focused pane in scroll mode should show SCROLL indicator")
	}
	if strings.Contains(view, "NAV") {
		t.Error("Pane in scroll mode should not show NAV indicator")
	}
	if strings.Contains(view, "INPUT") {
		t.Error("Pane in scroll mode should not show INPUT indicator")
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no ansi", "Hello World", "Hello World"},
		{"simple color", "\x1b[31mRed\x1b[0m", "Red"},
		{"multiple colors", "\x1b[31mRed\x1b[32mGreen\x1b[0m", "RedGreen"},
		{"256 color", "\x1b[38;5;196mRed\x1b[0m", "Red"},
		{"true color", "\x1b[38;2;255;0;0mRed\x1b[0m", "Red"},
		{"bold and color", "\x1b[1;31mBold Red\x1b[0m", "Bold Red"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripANSI(tt.input)
			if result != tt.expected {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMuteANSI(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"no ansi", "Hello World"},
		{"basic red", "\x1b[31mRed\x1b[0m"},
		{"basic green", "\x1b[32mGreen\x1b[0m"},
		{"bright red", "\x1b[91mBright Red\x1b[0m"},
		{"256 color", "\x1b[38;5;196mRed 256\x1b[0m"},
		{"true color", "\x1b[38;2;255;0;0mTrue Red\x1b[0m"},
		{"background color", "\x1b[44mBlue BG\x1b[0m"},
		{"bold with color", "\x1b[1;31mBold Red\x1b[0m"},
		{"multiple codes", "\x1b[1;4;31mBold Underline Red\x1b[0m"},
		{"mixed content", "Normal \x1b[31mRed\x1b[0m Normal \x1b[32mGreen\x1b[0m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that muteANSI doesn't panic and returns something
			result := muteANSI(tt.input, 0.25, 0.6, [3]int{85, 85, 85})
			if result == "" && tt.input != "" {
				t.Errorf("muteANSI(%q) returned empty string", tt.input)
			}

			// The plain text should still be present
			plain := stripANSI(tt.input)
			resultPlain := stripANSI(result)
			if resultPlain != plain {
				t.Errorf("muteANSI changed text content: %q -> %q", plain, resultPlain)
			}
		})
	}
}

func TestMuteANSI_PreservesNonColorCodes(t *testing.T) {
	// Test that non-color codes like bold, underline, reset are preserved
	tests := []struct {
		name  string
		input string
	}{
		{"reset", "\x1b[0m"},
		{"bold", "\x1b[1mBold\x1b[0m"},
		{"underline", "\x1b[4mUnderline\x1b[0m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := muteANSI(tt.input, 0.25, 0.6, [3]int{85, 85, 85})
			// Should not crash and should preserve text
			plain := stripANSI(tt.input)
			resultPlain := stripANSI(result)
			if resultPlain != plain {
				t.Errorf("muteANSI changed text content: %q -> %q", plain, resultPlain)
			}
		})
	}
}

func TestColor256ToRGB(t *testing.T) {
	tests := []struct {
		index   int
		r, g, b int
	}{
		// Standard colors (first 16)
		{0, 0, 0, 0},        // Black
		{1, 205, 49, 49},    // Red
		{15, 255, 255, 255}, // Bright White
		// 216 color cube
		{16, 0, 0, 0},    // Start of cube (black)
		{21, 0, 0, 255},  // Blue in cube
		{196, 255, 0, 0}, // Red in cube
		// Grayscale
		{232, 8, 8, 8},       // Near black
		{255, 238, 238, 238}, // Near white
	}

	for _, tt := range tests {
		t.Run(string(rune('0'+tt.index)), func(t *testing.T) {
			r, g, b := color256ToRGB(tt.index)
			if r != tt.r || g != tt.g || b != tt.b {
				t.Errorf("color256ToRGB(%d) = (%d, %d, %d), want (%d, %d, %d)",
					tt.index, r, g, b, tt.r, tt.g, tt.b)
			}
		})
	}
}

// TestMuteANSI_ColorTransformation verifies that muteANSI actually transforms
// explicit ANSI color codes to muted versions (reduced saturation and brightness).
// This is important for showing unfocused panes with visually distinct, dimmed colors.
func TestMuteANSI_ColorTransformation(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		saturation  float64
		brightness  float64
		description string
	}{
		{
			name:        "basic red to muted",
			input:       "\x1b[31mRed Text\x1b[0m",
			saturation:  0.25,
			brightness:  0.6,
			description: "Basic ANSI red (31) should be converted to true color with reduced saturation",
		},
		{
			name:        "256-color green to muted",
			input:       "\x1b[38;5;46mGreen\x1b[0m", // Bright green in 256-color palette
			saturation:  0.25,
			brightness:  0.6,
			description: "256-color mode should be converted to muted true color",
		},
		{
			name:        "true color blue to muted",
			input:       "\x1b[38;2;0;0;255mBlue\x1b[0m",
			saturation:  0.25,
			brightness:  0.6,
			description: "True color RGB should have saturation and brightness reduced",
		},
		{
			name:        "background color muted",
			input:       "\x1b[44mBlue Background\x1b[0m",
			saturation:  0.25,
			brightness:  0.6,
			description: "Background colors should also be muted",
		},
		{
			name:        "bright foreground muted",
			input:       "\x1b[92mBright Green\x1b[0m", // Bright green (90-97 range)
			saturation:  0.25,
			brightness:  0.6,
			description: "Bright ANSI colors (90-97) should be muted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := muteANSI(tt.input, tt.saturation, tt.brightness, [3]int{85, 85, 85})

			// The result should contain true color sequences (38;2; for fg, 48;2; for bg)
			// because muteANSI converts all colors to true color for precise muting
			if strings.Contains(tt.input, "[31m") || strings.Contains(tt.input, "[32m") ||
				strings.Contains(tt.input, "[92m") || strings.Contains(tt.input, "[38;5;") {
				// Foreground colors should be converted to 38;2;R;G;B format
				if !strings.Contains(result, "38;2;") {
					t.Errorf("%s: expected true color foreground (38;2;), got %q", tt.description, result)
				}
			}
			if strings.Contains(tt.input, "[44m") {
				// Background colors should be converted to 48;2;R;G;B format
				if !strings.Contains(result, "48;2;") {
					t.Errorf("%s: expected true color background (48;2;), got %q", tt.description, result)
				}
			}

			// Text content must be preserved
			inputPlain := stripANSI(tt.input)
			resultPlain := stripANSI(result)
			if resultPlain != inputPlain {
				t.Errorf("muteANSI altered text content: %q -> %q", inputPlain, resultPlain)
			}
		})
	}
}

// TestMuteANSI_PlainTextPassthrough verifies that muteANSI does NOT modify
// text that has no explicit ANSI color codes. This is important because
// such text uses the terminal's default foreground color (e.g., user's green-on-black
// theme), which muteANSI cannot detect or transform.
//
// The fix for this is handled at the pane View level by wrapping the output
// in a muted default foreground style AFTER calling muteANSI.
func TestMuteANSI_PlainTextPassthrough(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"plain text", "Hello World"},
		{"multiline plain", "Line 1\nLine 2\nLine 3"},
		{"plain with whitespace", "  indented  \ttabbed  "},
		{"unicode plain", "Hello 世界 🌍"},
		{"empty string", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := muteANSI(tt.input, 0.25, 0.6, [3]int{85, 85, 85})

			// Plain text should pass through completely unchanged
			// (no ANSI codes to transform)
			if result != tt.input {
				t.Errorf("muteANSI modified plain text: %q -> %q", tt.input, result)
			}
		})
	}
}

// TestMuteANSI_MixedContent verifies behavior with mixed ANSI and plain text.
// The ANSI-colored portions should be muted, while plain text portions
// should pass through unchanged (they'll be handled by the pane's default style).
func TestMuteANSI_MixedContent(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "plain then colored",
			input: "Plain text \x1b[31mRed text\x1b[0m more plain",
		},
		{
			name:  "colored then plain",
			input: "\x1b[32mGreen\x1b[0m then plain text",
		},
		{
			name:  "multiple color spans",
			input: "A \x1b[31mB\x1b[0m C \x1b[32mD\x1b[0m E",
		},
		{
			name:  "nested attributes",
			input: "\x1b[1m\x1b[31mBold Red\x1b[0m Plain \x1b[4mUnderline\x1b[0m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := muteANSI(tt.input, 0.25, 0.6, [3]int{85, 85, 85})

			// Plain text content must be preserved
			inputPlain := stripANSI(tt.input)
			resultPlain := stripANSI(result)
			if resultPlain != inputPlain {
				t.Errorf("muteANSI altered text content: %q -> %q", inputPlain, resultPlain)
			}

			// Any basic color codes (30-37, 90-97) should be converted to true color
			// Check that the result doesn't contain basic color codes that were in input
			if strings.Contains(tt.input, "[31m") && strings.Contains(result, "[31m") {
				t.Errorf("muteANSI should convert basic colors to true color, but [31m still present")
			}
			if strings.Contains(tt.input, "[32m") && strings.Contains(result, "[32m") {
				t.Errorf("muteANSI should convert basic colors to true color, but [32m still present")
			}
		})
	}
}

// TestMuteANSI_GrayscaleMode verifies that saturation=0 produces grayscale output.
func TestMuteANSI_GrayscaleMode(t *testing.T) {
	input := "\x1b[31mRed\x1b[0m \x1b[32mGreen\x1b[0m \x1b[34mBlue\x1b[0m"
	result := muteANSI(input, 0.0, 1.0, [3]int{85, 85, 85}) // Zero saturation = grayscale

	// Should still preserve text
	inputPlain := stripANSI(input)
	resultPlain := stripANSI(result)
	if resultPlain != inputPlain {
		t.Errorf("grayscale mode altered text: %q -> %q", inputPlain, resultPlain)
	}

	// Result should contain true color codes (converted from basic colors)
	if !strings.Contains(result, "38;2;") {
		t.Error("expected grayscale mode to produce true color output")
	}
}

// TestMuteANSI_ResetAndDefaultForeground verifies that reset codes and default
// foreground codes are replaced with the muted default color. This is critical
// for handling terminal output that resets to the user's configured default
// foreground color (e.g., bright green on dark background).
func TestMuteANSI_ResetAndDefaultForeground(t *testing.T) {
	mutedDefault := [3]int{85, 85, 85} // #555555

	tests := []struct {
		name           string
		input          string
		shouldContain  string
		shouldNotMatch string
		description    string
	}{
		{
			name:          "empty reset sequence",
			input:         "\x1b[mText after reset",
			shouldContain: "38;2;85;85;85",
			description:   "Empty reset \\x1b[m should be replaced with muted default foreground",
		},
		{
			name:          "full reset code 0",
			input:         "\x1b[0mText after reset",
			shouldContain: "38;2;85;85;85",
			description:   "Reset code 0 should add muted default foreground after reset",
		},
		{
			name:          "default foreground code 39",
			input:         "\x1b[39mText with default fg",
			shouldContain: "38;2;85;85;85",
			description:   "Default foreground code 39 should be replaced with muted default",
		},
		{
			name:          "color then reset",
			input:         "\x1b[31mRed\x1b[0mDefault",
			shouldContain: "38;2;85;85;85",
			description:   "Text after reset should use muted default, not terminal default",
		},
		{
			name:          "color then default fg",
			input:         "\x1b[32mGreen\x1b[39mDefault",
			shouldContain: "38;2;85;85;85",
			description:   "Switching to default fg should use muted default color",
		},
		{
			name:          "bold with reset preserves reset",
			input:         "\x1b[1mBold\x1b[0mNormal",
			shouldContain: "0;38;2;85;85;85",
			description:   "Reset should still reset attributes but add muted default fg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := muteANSI(tt.input, 0.25, 0.6, mutedDefault)

			// Should contain the muted default color
			if !strings.Contains(result, tt.shouldContain) {
				t.Errorf("%s\nInput:    %q\nExpected to contain: %s\nGot:      %q",
					tt.description, tt.input, tt.shouldContain, result)
			}

			// Plain text should be preserved
			inputPlain := stripANSI(tt.input)
			resultPlain := stripANSI(result)
			if resultPlain != inputPlain {
				t.Errorf("muteANSI altered text content: %q -> %q", inputPlain, resultPlain)
			}
		})
	}
}

// TestMuteANSI_DefaultForegroundNotPassedThrough verifies that code 39 (default fg)
// is NOT passed through unchanged, which would cause the terminal to use its
// configured default color (e.g., user's bright green).
func TestMuteANSI_DefaultForegroundNotPassedThrough(t *testing.T) {
	mutedDefault := [3]int{100, 100, 100}
	input := "\x1b[39mText"
	result := muteANSI(input, 0.25, 0.6, mutedDefault)

	// Should NOT contain the original [39m sequence
	if strings.Contains(result, "[39m") {
		t.Errorf("muteANSI should replace [39m with muted color, but [39m still present in: %q", result)
	}

	// Should contain the muted default color
	if !strings.Contains(result, "38;2;100;100;100") {
		t.Errorf("muteANSI should use muted default (100,100,100), got: %q", result)
	}
}

func TestPaneModel_IsClaudeWorking(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected bool
	}{
		{
			name:     "no output",
			output:   "",
			expected: false,
		},
		{
			name:     "regular output",
			output:   "Hello World\r\nSome text\r\n",
			expected: false,
		},
		{
			name:     "claude working indicator with dot",
			output:   "Processing...\r\n(ctrl+c to interrupt · 5s)\r\n",
			expected: true,
		},
		{
			name:     "claude working indicator hyphen with dot",
			output:   "Working on task\r\n(ctrl-c to interrupt · 10s)\r\n",
			expected: true,
		},
		{
			name:     "indicator without dot - not matched",
			output:   "Fake indicator\r\n(ctrl+c to interrupt)\r\n",
			expected: false,
		},
		{
			name:     "indicator in middle of output",
			output:   "Line 1\r\nLine 2\r\n(ctrl+c to interrupt · 3s)\r\nLine 4\r\n",
			expected: true,
		},
		{
			name:     "user typed similar text without dot",
			output:   "User said: press ctrl+c to interrupt the process\r\n",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := workstream.New("test")
			pane := NewPaneModel(ws)
			pane.SetSize(80, 24)

			if tt.output != "" {
				pane.WritePTYOutput([]byte(tt.output))
			}

			result := pane.IsClaudeWorking()
			if result != tt.expected {
				t.Errorf("IsClaudeWorking() = %v, want %v for output %q", result, tt.expected, tt.output)
			}
		})
	}
}

func TestPaneModel_IsClaudeWorking_NilVterm(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	// Don't initialize vterm size - vterm exists but is empty

	// Should not panic and should return false
	result := pane.IsClaudeWorking()
	if result {
		t.Error("IsClaudeWorking() should return false when vterm has no content")
	}
}

// mockPTYStdin is a mock io.WriteCloser for testing PTY input
type mockPTYStdin struct {
	buf    bytes.Buffer
	closed bool
	mu     sync.Mutex
}

func (m *mockPTYStdin) Write(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, io.ErrClosedPipe
	}
	return m.buf.Write(p)
}

func (m *mockPTYStdin) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockPTYStdin) Bytes() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.buf.Bytes()
}

// TestPaneModel_EnterKey_SendsCarriageReturn verifies that pressing Enter sends \r (carriage return)
// to the PTY, not \n (line feed). This is crucial for proper terminal input handling.
func TestPaneModel_EnterKey_SendsCarriageReturn(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)
	pane.SetFocused(true) // Pane must be focused to process key events

	// Create a mock PTY session with a mock stdin
	mockStdin := &mockPTYStdin{}
	pty := &PTYSession{
		workstreamID: ws.ID,
		closed:       false,
		done:         make(chan struct{}),
		stdin:        mockStdin,
	}
	pane.SetPTY(pty)

	// Send Enter key
	enterMsg := tea.KeyPressMsg{Code: tea.KeyEnter}
	pane.Update(enterMsg)

	// Verify that \r was sent, not \n
	written := mockStdin.Bytes()
	if string(written) != "\r" {
		t.Errorf("Enter key should send \\r (carriage return), got %q", string(written))
	}
}
