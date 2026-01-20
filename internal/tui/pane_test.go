package tui

import (
	"strings"
	"testing"

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
			result := muteANSI(tt.input, 0.25, 0.6)
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
			result := muteANSI(tt.input, 0.25, 0.6)
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
