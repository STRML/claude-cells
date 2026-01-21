package tui

import (
	"strings"
	"testing"

	"github.com/STRML/claude-cells/internal/workstream"
)

func TestPaneModel_VTermSize_FullHalfFullResize(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	// Simulate full screen (200 chars wide pane)
	fullWidth := 200
	fullHeight := 50
	pane.SetSize(fullWidth, fullHeight)

	// Check vterm dimensions (should be width - 4 for border/padding)
	expectedVtermWidth := fullWidth - 4
	cols, rows := pane.vterm.Size()
	t.Logf("After full size (%d): vterm cols=%d, rows=%d", fullWidth, cols, rows)
	if cols != expectedVtermWidth {
		t.Errorf("Full size: vterm cols = %d, want %d", cols, expectedVtermWidth)
	}

	// Write some wide content
	wideLine := strings.Repeat("X", expectedVtermWidth)
	pane.WritePTYOutput([]byte(wideLine + "\r\n"))

	// Render and check
	output1 := pane.renderVTerm()
	line1Width := len(strings.TrimRight(strings.Split(output1, "\n")[0], "\x1b[0m "))
	t.Logf("After full render: content width=%d", line1Width)

	// Resize to half width
	halfWidth := 100
	pane.SetSize(halfWidth, fullHeight)

	cols, rows = pane.vterm.Size()
	expectedHalfVtermWidth := halfWidth - 4
	t.Logf("After half size (%d): vterm cols=%d, rows=%d", halfWidth, cols, rows)
	if cols != expectedHalfVtermWidth {
		t.Errorf("Half size: vterm cols = %d, want %d", cols, expectedHalfVtermWidth)
	}

	// Resize back to full
	pane.SetSize(fullWidth, fullHeight)

	cols, rows = pane.vterm.Size()
	t.Logf("After full size again (%d): vterm cols=%d, rows=%d", fullWidth, cols, rows)
	if cols != expectedVtermWidth {
		t.Errorf("Full size again: vterm cols = %d, want %d", cols, expectedVtermWidth)
	}

	// Write new content at full width to verify vterm accepts it
	newWideLine := strings.Repeat("Y", expectedVtermWidth)
	pane.WritePTYOutput([]byte(newWideLine + "\r\n"))

	// Render and verify the new content is at full width
	output2 := pane.renderVTerm()
	lines := strings.Split(output2, "\n")

	// Find the line with Y's
	var yLine string
	for _, line := range lines {
		if strings.Contains(line, "Y") {
			yLine = line
			break
		}
	}

	// Strip ANSI codes to count actual characters
	stripped := stripANSI(yLine)
	actualWidth := len(strings.TrimRight(stripped, " "))
	t.Logf("After full restore: Y line content width=%d (expected=%d)", actualWidth, expectedVtermWidth)

	if actualWidth < expectedVtermWidth-10 {
		t.Errorf("Content width after restore = %d, want approximately %d (40%% would be %d)",
			actualWidth, expectedVtermWidth, expectedVtermWidth*40/100)
	}
}

// TestPaneModel_SetSize_SendsCtrlLOnGrow verifies that when the terminal grows wider,
// a Ctrl+L is sent to trigger a screen redraw. This fixes the issue where content
// truncated during a smaller resize stays truncated when resizing back to a larger width.
func TestPaneModel_SetSize_SendsCtrlLOnGrow(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	// Create a PTYSession with a mock stdin to capture writes
	// mockWriteCloser is defined in pty_test.go
	mockStdin := &mockWriteCloser{}
	pty := &PTYSession{
		stdin:        mockStdin,
		closed:       false,
		workstreamID: "test",
	}
	pane.SetPTY(pty)

	// Initial size
	pane.SetSize(100, 50)
	mockStdin.buf.Reset() // Clear any initial writes

	// Resize smaller - should NOT send Ctrl+L
	pane.SetSize(80, 50)
	if containsCtrlL(mockStdin.buf.Bytes()) {
		t.Error("Ctrl+L should NOT be sent when resizing smaller")
	}
	mockStdin.buf.Reset()

	// Resize larger - SHOULD send Ctrl+L
	pane.SetSize(120, 50)
	if !containsCtrlL(mockStdin.buf.Bytes()) {
		t.Error("Ctrl+L should be sent when resizing larger")
	}
	mockStdin.buf.Reset()

	// Resize to same width - should NOT send Ctrl+L
	pane.SetSize(120, 60) // Only height changes
	if containsCtrlL(mockStdin.buf.Bytes()) {
		t.Error("Ctrl+L should NOT be sent when only height changes")
	}
}

func containsCtrlL(data []byte) bool {
	for _, b := range data {
		if b == 12 { // Ctrl+L
			return true
		}
	}
	return false
}
