package tui

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/STRML/claude-cells/internal/git"
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

func TestPaneModel_SendInput(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	// Test error when no PTY
	err := pane.SendInput("test", true)
	if err == nil {
		t.Error("SendInput() should return error when no PTY")
	}

	// Create a mock PTY with a mock stdin
	mockStdin := &mockWriteCloser{}
	pty := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
		stdin:        mockStdin,
	}
	pane.SetPTY(pty)

	// Test sending text with enter
	err = pane.SendInput("hello world", true)
	if err != nil {
		t.Errorf("SendInput() unexpected error: %v", err)
	}

	// Verify the output: should be "hello world" followed by Kitty Enter key
	expected := "hello world\x1b[13u"
	if string(mockStdin.Bytes()) != expected {
		t.Errorf("Written data = %q, want %q", string(mockStdin.Bytes()), expected)
	}
}

func TestPaneModel_SendInput_WithoutEnter(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	mockStdin := &mockWriteCloser{}
	pty := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
		stdin:        mockStdin,
	}
	pane.SetPTY(pty)

	// Test sending text without enter
	err := pane.SendInput("hello", false)
	if err != nil {
		t.Errorf("SendInput() unexpected error: %v", err)
	}

	// Verify the output: should just be the text, no Enter key
	expected := "hello"
	if string(mockStdin.Bytes()) != expected {
		t.Errorf("Written data = %q, want %q", string(mockStdin.Bytes()), expected)
	}
}

func TestPaneModel_SendInput_EmptyWithEnter(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	mockStdin := &mockWriteCloser{}
	pty := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
		stdin:        mockStdin,
	}
	pane.SetPTY(pty)

	// Test sending just enter (empty text)
	err := pane.SendInput("", true)
	if err != nil {
		t.Errorf("SendInput() unexpected error: %v", err)
	}

	// Verify the output: should just be Kitty Enter key
	expected := "\x1b[13u"
	if string(mockStdin.Bytes()) != expected {
		t.Errorf("Written data = %q, want %q", string(mockStdin.Bytes()), expected)
	}
}

// TestPaneModel_SendToPTYWithEnter tests the deprecated wrapper method
func TestPaneModel_SendToPTYWithEnter(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	mockStdin := &mockWriteCloser{}
	pty := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
		stdin:        mockStdin,
	}
	pane.SetPTY(pty)

	// Test that deprecated method still works
	err := pane.SendToPTYWithEnter("test")
	if err != nil {
		t.Errorf("SendToPTYWithEnter() unexpected error: %v", err)
	}

	expected := "test\x1b[13u"
	if string(mockStdin.Bytes()) != expected {
		t.Errorf("Written data = %q, want %q", string(mockStdin.Bytes()), expected)
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

func TestPaneModel_ScrollHalfPageUp(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)

	// Add some content so we have something to scroll
	for i := 0; i < 100; i++ {
		pane.AppendOutput("Line content\n")
	}

	// ScrollHalfPageUp should enter scroll mode
	pane.ScrollHalfPageUp()
	if !pane.IsScrollMode() {
		t.Error("ScrollHalfPageUp() should enter scroll mode")
	}
}

func TestPaneModel_ScrollHalfPageDown(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)

	// Add content and enter scroll mode
	for i := 0; i < 100; i++ {
		pane.AppendOutput("Line content\n")
	}
	pane.ScrollPageUp() // Enter scroll mode and scroll up

	if !pane.IsScrollMode() {
		t.Error("Should be in scroll mode after ScrollPageUp()")
	}

	// ScrollHalfPageDown should stay in scroll mode when not at bottom
	pane.ScrollHalfPageDown()
	// We're still not at bottom after one half-page, so should still be in scroll mode
	if !pane.IsScrollMode() {
		t.Error("ScrollHalfPageDown() should stay in scroll mode when not at bottom")
	}

	// Scroll all the way to the bottom
	for i := 0; i < 50; i++ {
		pane.ScrollHalfPageDown()
	}
	// Should exit scroll mode when at bottom
	if pane.IsScrollMode() {
		t.Error("ScrollHalfPageDown() should exit scroll mode when at bottom")
	}
}

func TestPaneModel_renderVTermLine_PreservesColors(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)

	// Write colored content to vterm
	pane.WritePTYOutput([]byte("\x1b[31mRed text\x1b[0m\r\n"))

	// renderVTermLine should preserve colors
	line := pane.renderVTermLine(0)

	// Should contain ANSI color codes
	if !strings.Contains(line, "\x1b[") {
		t.Errorf("renderVTermLine() should preserve ANSI colors, got: %q", line)
	}

	// Should contain the text
	if !strings.Contains(stripANSI(line), "Red text") {
		t.Errorf("renderVTermLine() should contain text 'Red text', got: %q", stripANSI(line))
	}
}

func TestPaneModel_renderVTermLine_EmptyRow(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)

	// renderVTermLine on empty vterm should return empty or reset-only
	line := pane.renderVTermLine(0)
	stripped := stripANSI(line)
	if stripped != "" && strings.TrimSpace(stripped) != "" {
		t.Errorf("renderVTermLine() on empty row should return empty/whitespace, got: %q", stripped)
	}
}

func TestPaneModel_renderVTermLine_OutOfBounds(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)

	// Out of bounds should return empty
	line := pane.renderVTermLine(-1)
	if line != "" {
		t.Errorf("renderVTermLine(-1) should return empty, got: %q", line)
	}

	line = pane.renderVTermLine(100)
	if line != "" {
		t.Errorf("renderVTermLine(100) should return empty, got: %q", line)
	}
}

func TestPaneModel_WritePTYOutput_ScrollbackPreservesColors(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	// SetSize with height 16 means innerHeight = max(16-6, 10) = 10
	// So we need more than 10 lines to scroll
	pane.SetSize(80, 16)

	// Write many colored lines to trigger scrollback - write more than vterm height
	// Each line needs unique content so the first line changes on scroll
	for i := 0; i < 30; i++ {
		pane.WritePTYOutput([]byte(fmt.Sprintf("\x1b[32mGreen line %d\x1b[0m\r\n", i)))
	}

	// Check that scrollback contains entries
	if len(pane.scrollback) == 0 {
		w, h := pane.vterm.Size()
		t.Fatalf("Expected scrollback to have entries after scrolling, vterm size is %dx%d", w, h)
	}

	// At least some scrollback entries should have color codes
	hasColors := false
	for _, line := range pane.scrollback {
		if strings.Contains(line, "\x1b[") {
			hasColors = true
			break
		}
	}
	if !hasColors {
		t.Errorf("Scrollback should preserve ANSI color codes, got: %v", pane.scrollback[:min(3, len(pane.scrollback))])
	}
}

// TestPaneModel_EnterKey_SendsKittyProtocol verifies that pressing Enter sends the Kitty
// keyboard protocol sequence (CSI 13 u) to the PTY. This is crucial for proper terminal
// input handling with Claude Code which uses bubbletea with Kitty protocol support.
func TestPaneModel_EnterKey_SendsKittyProtocol(t *testing.T) {
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

	// Verify that Kitty Enter sequence was sent (CSI 13 u = ESC [ 1 3 u)
	written := mockStdin.Bytes()
	expected := "\x1b[13u"
	if string(written) != expected {
		t.Errorf("Enter key should send Kitty protocol sequence %q, got %q", expected, string(written))
	}
}

// TestPaneModel_View_ConsistentHeight verifies that the pane view has consistent
// height regardless of vterm content. This tests the fix for the issue where
// extra newlines in Claude Code's prompt caused scrollback display issues.
// The root cause was that renderVTerm() trims trailing empty lines, and without
// FillHeight=true on the viewport, the output height would vary.
func TestPaneModel_View_ConsistentHeight(t *testing.T) {
	ws := workstream.New("test prompt")
	ws.SetState(workstream.StateRunning)
	pane := NewPaneModel(ws)

	// Set a fixed size
	paneWidth := 80
	paneHeight := 30
	pane.SetSize(paneWidth, paneHeight)
	pane.SetFocused(true)

	// Helper to count lines in view
	countLines := func(view string) int {
		return strings.Count(view, "\n") + 1
	}

	// Test 1: Empty vterm - should have consistent height
	view1 := pane.View()
	lines1 := countLines(view1)

	// Test 2: Vterm with some content at top only (simulating Claude Code
	// with a large prompt area, leaving empty space)
	pane.WritePTYOutput([]byte("Line 1\r\n"))
	pane.WritePTYOutput([]byte("Line 2\r\n"))
	view2 := pane.View()
	lines2 := countLines(view2)

	// Test 3: Vterm with more content (simulating Claude Code with smaller prompt)
	for i := 3; i <= 10; i++ {
		pane.WritePTYOutput([]byte(fmt.Sprintf("Line %d\r\n", i)))
	}
	view3 := pane.View()
	lines3 := countLines(view3)

	// All views should have the same height (paneHeight)
	// The view includes the border, so it should be exactly paneHeight lines
	if lines1 != lines2 {
		t.Errorf("View height changed with content: empty=%d, few lines=%d", lines1, lines2)
	}
	if lines2 != lines3 {
		t.Errorf("View height changed with more content: few lines=%d, many lines=%d", lines2, lines3)
	}
	if lines1 != paneHeight {
		t.Errorf("View height should match pane height: got %d, want %d", lines1, paneHeight)
	}
}

// TestPaneModel_ViewportFillHeight verifies that the viewport has FillHeight enabled
func TestPaneModel_ViewportFillHeight(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	if !pane.viewport.FillHeight {
		t.Error("Viewport should have FillHeight=true to ensure consistent output height")
	}
}

// NOTE: Tests for GetCursorPosition() and hardware cursor positioning have been removed.
// We no longer render our own cursor - we rely on Claude Code's virtual cursor
// (styled text in its output). The vterm.Cursor() position doesn't accurately
// reflect where Claude Code renders its visual cursor, so attempting to position
// a hardware cursor or render inverse video was causing cursor misplacement.

// TestPaneModel_InverseVideoPreserved verifies that inverse video styling from
// the inner app (like Claude Code's cursor) is properly preserved in renderVTerm output.
func TestPaneModel_InverseVideoPreserved(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)

	// Write text with inverse video (simulating Claude Code's cursor)
	pane.WritePTYOutput([]byte("Input: "))
	pane.WritePTYOutput([]byte("\x1b[7m"))  // Start inverse video
	pane.WritePTYOutput([]byte(" "))        // Cursor block
	pane.WritePTYOutput([]byte("\x1b[0m"))  // Reset
	pane.WritePTYOutput([]byte(" more"))

	// Render the vterm content
	vtermContent := pane.renderVTerm()

	// Should contain inverse video escape sequence from the inner app
	if !strings.Contains(vtermContent, "\x1b[7m") {
		t.Error("renderVTerm should preserve inverse video (\\x1b[7m) from inner app output")
	}
}

// TestPaneModel_PRStatusMethods tests GetPRStatus, SetPRStatus, SetPRStatusLoading, IsPRStatusLoading
func TestPaneModel_PRStatusMethods(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	// Initially, PR status should be nil and not loading
	if pane.GetPRStatus() != nil {
		t.Error("Initial PR status should be nil")
	}
	if pane.IsPRStatusLoading() {
		t.Error("Initially should not be loading")
	}

	// Set loading state
	pane.SetPRStatusLoading(true)
	if !pane.IsPRStatusLoading() {
		t.Error("Should be loading after SetPRStatusLoading(true)")
	}

	// SetPRStatus should clear loading state
	status := &git.PRStatusInfo{
		Number:        123,
		URL:           "https://github.com/test/repo/pull/123",
		HeadSHA:       "abc123",
		CheckStatus:   git.PRCheckStatusSuccess,
		ChecksSummary: "3/3 passed",
		UnpushedCount: 2,
		DivergedCount: 0,
		IsDiverged:    false,
	}
	pane.SetPRStatus(status)

	// Verify status was set and loading was cleared
	if pane.IsPRStatusLoading() {
		t.Error("Loading should be false after SetPRStatus")
	}

	got := pane.GetPRStatus()
	if got == nil {
		t.Fatal("GetPRStatus should return the set status")
	}
	if got.Number != 123 {
		t.Errorf("Expected PR number 123, got %d", got.Number)
	}
	if got.CheckStatus != git.PRCheckStatusSuccess {
		t.Errorf("Expected success status, got %v", got.CheckStatus)
	}
	if got.UnpushedCount != 2 {
		t.Errorf("Expected 2 unpushed, got %d", got.UnpushedCount)
	}

	// Test SetPRStatus with nil
	pane.SetPRStatus(nil)
	if pane.GetPRStatus() != nil {
		t.Error("GetPRStatus should return nil after setting nil")
	}
}

// TestPaneModel_RenderPRFooter tests the renderPRFooter method
func TestPaneModel_RenderPRFooter(t *testing.T) {
	tests := []struct {
		name     string
		status   *git.PRStatusInfo
		contains []string
		excludes []string
	}{
		{
			name:     "nil status returns empty",
			status:   nil,
			contains: nil,
			excludes: []string{"PR #"},
		},
		{
			name: "success status with all checks passed",
			status: &git.PRStatusInfo{
				Number:        42,
				CheckStatus:   git.PRCheckStatusSuccess,
				ChecksSummary: "5/5 passed",
			},
			contains: []string{"PR #42:", "✓", "5/5 passed"},
			excludes: []string{"unpushed", "diverged"},
		},
		{
			name: "pending status",
			status: &git.PRStatusInfo{
				Number:        99,
				CheckStatus:   git.PRCheckStatusPending,
				ChecksSummary: "2/4 passed",
			},
			contains: []string{"PR #99:", "⏳", "2/4 passed"},
			excludes: []string{"unpushed", "diverged"},
		},
		{
			name: "failure status",
			status: &git.PRStatusInfo{
				Number:        55,
				CheckStatus:   git.PRCheckStatusFailure,
				ChecksSummary: "1/3 passed",
			},
			contains: []string{"PR #55:", "✗", "1/3 passed"},
			excludes: []string{"unpushed", "diverged"},
		},
		{
			name: "unknown status",
			status: &git.PRStatusInfo{
				Number:        10,
				CheckStatus:   git.PRCheckStatusUnknown,
				ChecksSummary: "No checks",
			},
			contains: []string{"PR #10:", "?", "No checks"},
			excludes: []string{"unpushed", "diverged"},
		},
		{
			name: "with unpushed commits",
			status: &git.PRStatusInfo{
				Number:        77,
				CheckStatus:   git.PRCheckStatusSuccess,
				ChecksSummary: "3/3 passed",
				UnpushedCount: 5,
			},
			contains: []string{"PR #77:", "✓", "↑5 unpushed"},
			excludes: []string{"diverged"},
		},
		{
			name: "with divergence",
			status: &git.PRStatusInfo{
				Number:        88,
				CheckStatus:   git.PRCheckStatusPending,
				ChecksSummary: "1/2 passed",
				IsDiverged:    true,
				DivergedCount: 3,
			},
			contains: []string{"PR #88:", "⚠ diverged"},
			excludes: nil,
		},
		{
			name: "with unpushed and diverged",
			status: &git.PRStatusInfo{
				Number:        100,
				CheckStatus:   git.PRCheckStatusFailure,
				ChecksSummary: "0/2 passed",
				UnpushedCount: 2,
				IsDiverged:    true,
				DivergedCount: 1,
			},
			contains: []string{"PR #100:", "✗", "↑2 unpushed", "⚠ diverged"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := workstream.New("test")
			pane := NewPaneModel(ws)
			pane.SetPRStatus(tt.status)

			result := pane.renderPRFooter()

			// Check for expected content
			for _, want := range tt.contains {
				if !strings.Contains(result, want) {
					t.Errorf("renderPRFooter() should contain %q, got %q", want, result)
				}
			}

			// Check that excluded content is not present (strip ANSI for accurate check)
			stripped := stripANSI(result)
			for _, exclude := range tt.excludes {
				if strings.Contains(stripped, exclude) {
					t.Errorf("renderPRFooter() should not contain %q, got %q", exclude, stripped)
				}
			}
		})
	}
}

func TestPaneModel_View_PRStatusIndicator(t *testing.T) {
	tests := []struct {
		name     string
		prURL    string
		status   *git.PRStatusInfo
		contains []string
		excludes []string
	}{
		{
			name:  "no PR - no indicator",
			prURL: "",
			status: &git.PRStatusInfo{
				UnpushedCount: 5,
			},
			contains: []string{},
			excludes: []string{"↑5"},
		},
		{
			name:  "PR with unpushed commits shows indicator",
			prURL: "https://github.com/foo/bar/pull/1",
			status: &git.PRStatusInfo{
				UnpushedCount: 3,
				CheckStatus:   git.PRCheckStatusSuccess,
			},
			contains: []string{"↑3"},
			excludes: []string{},
		},
		{
			name:  "PR with failing checks shows failure indicator",
			prURL: "https://github.com/foo/bar/pull/1",
			status: &git.PRStatusInfo{
				CheckStatus: git.PRCheckStatusFailure,
			},
			contains: []string{"✗"},
			excludes: []string{},
		},
		{
			name:  "PR with pending checks shows pending indicator",
			prURL: "https://github.com/foo/bar/pull/1",
			status: &git.PRStatusInfo{
				CheckStatus: git.PRCheckStatusPending,
			},
			contains: []string{"⏳"},
			excludes: []string{},
		},
		{
			name:  "PR with diverged branch shows warning",
			prURL: "https://github.com/foo/bar/pull/1",
			status: &git.PRStatusInfo{
				IsDiverged:    true,
				DivergedCount: 2,
			},
			contains: []string{"⚠", "↓2"},
			excludes: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := workstream.New("test")
			ws.PRURL = tt.prURL
			pane := NewPaneModel(ws)
			pane.SetSize(80, 24)
			pane.SetPRStatus(tt.status)

			view := pane.View()
			stripped := stripANSI(view)

			for _, want := range tt.contains {
				if !strings.Contains(stripped, want) {
					t.Errorf("View() should contain %q, got %q", want, stripped)
				}
			}

			for _, exclude := range tt.excludes {
				if strings.Contains(stripped, exclude) {
					t.Errorf("View() should not contain %q, got %q", exclude, stripped)
				}
			}
		})
	}
}

func TestPaneModel_InPaneDialog_FocusedUnfocusedBorders(t *testing.T) {
	// Test that in-pane dialogs have different visual styles when focused vs unfocused
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)

	// Set up an in-pane dialog (e.g., a merge dialog)
	dialog := NewMergeDialog("test-branch", ws.ID, "", "", false, "", nil)
	pane.SetInPaneDialog(&dialog)

	// Test focused state
	pane.SetFocused(true)
	focusedView := pane.View()

	// The focused view should use thick border (which has doubled corners ╔╗╚╝)
	if !strings.Contains(focusedView, "╔") && !strings.Contains(focusedView, "┏") {
		t.Error("Focused in-pane dialog should use thick border style")
	}

	// Test unfocused state
	pane.SetFocused(false)
	unfocusedView := pane.View()

	// The unfocused view should use normal border (which has single corners ┌┐└┘)
	if !strings.Contains(unfocusedView, "┌") && !strings.Contains(unfocusedView, "─") {
		t.Error("Unfocused in-pane dialog should use normal border style")
	}

	// The two views should be different (different border styles)
	if focusedView == unfocusedView {
		t.Error("Focused and unfocused in-pane dialog views should be different")
	}
}

func TestPaneModel_InPaneDialog_BothHaveDialogLabel(t *testing.T) {
	// Test that both focused and unfocused dialogs show the DIALOG mode indicator
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)

	dialog := NewProgressDialog("Testing", "Please wait", ws.ID)
	pane.SetInPaneDialog(&dialog)

	// Both focused and unfocused should show "DIALOG" indicator
	pane.SetFocused(true)
	focusedView := pane.View()
	if !strings.Contains(focusedView, "DIALOG") {
		t.Error("Focused in-pane dialog should show DIALOG mode indicator")
	}

	pane.SetFocused(false)
	unfocusedView := pane.View()
	if !strings.Contains(unfocusedView, "DIALOG") {
		t.Error("Unfocused in-pane dialog should show DIALOG mode indicator")
	}
}
