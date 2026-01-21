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

// TestPaneModel_SetSize_SendsCtrlLOnResize verifies that Ctrl+L is sent on any
// vterm size change to trigger a screen redraw. This ensures the process redraws
// for the new size, preventing corruption from output generated for the old size.
func TestPaneModel_SetSize_SendsCtrlLOnResize(t *testing.T) {
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

	// Resize smaller - SHOULD send Ctrl+L (any size change triggers redraw)
	pane.SetSize(80, 50)
	if !containsCtrlL(mockStdin.buf.Bytes()) {
		t.Error("Ctrl+L should be sent when resizing smaller")
	}
	mockStdin.buf.Reset()

	// Resize larger - SHOULD send Ctrl+L
	pane.SetSize(120, 50)
	if !containsCtrlL(mockStdin.buf.Bytes()) {
		t.Error("Ctrl+L should be sent when resizing larger")
	}
	mockStdin.buf.Reset()

	// Resize to same vterm dimensions (note: inner width = pane width - 4)
	// pane width 120 -> inner width 116
	// pane width 120 -> inner width 116 (same inner width, different height)
	// But inner height also changes: 50-6=44 -> 60-6=54
	pane.SetSize(120, 60) // Height changes, so vterm size changes
	if !containsCtrlL(mockStdin.buf.Bytes()) {
		t.Error("Ctrl+L should be sent when height changes")
	}
	mockStdin.buf.Reset()

	// Same exact size - should NOT send Ctrl+L
	pane.SetSize(120, 60)
	if containsCtrlL(mockStdin.buf.Bytes()) {
		t.Error("Ctrl+L should NOT be sent when size doesn't change")
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

func countCtrlO(data []byte) int {
	count := 0
	for _, b := range data {
		if b == 0x0F { // Ctrl+O
			count++
		}
	}
	return count
}

// TestPaneModel_SetSize_SendsCtrlOTwiceOnResize verifies that Ctrl+O is sent twice
// after resize to fix Claude Code display corruption. Empirically, this fixes issues
// where the text input floats to the top or other visual corruption occurs.
func TestPaneModel_SetSize_SendsCtrlOTwiceOnResize(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	// Create a PTYSession with a mock stdin to capture writes
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

	// Resize - should send Ctrl+O twice
	pane.SetSize(80, 50)
	ctrlOCount := countCtrlO(mockStdin.buf.Bytes())
	if ctrlOCount != 2 {
		t.Errorf("Expected 2 Ctrl+O on resize, got %d", ctrlOCount)
	}
	mockStdin.buf.Reset()

	// Same exact size - should NOT send Ctrl+O
	pane.SetSize(80, 50)
	ctrlOCount = countCtrlO(mockStdin.buf.Bytes())
	if ctrlOCount != 0 {
		t.Errorf("Expected 0 Ctrl+O when size unchanged, got %d", ctrlOCount)
	}
}

// TestPaneModel_ResizeCreatesNewVterm verifies that resizing creates a fresh vterm
// instead of trying to resize in-place. This prevents corruption from output that
// was generated for the old terminal size being written to the new-sized vterm.
func TestPaneModel_ResizeCreatesNewVterm(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	// Initial size with some content
	pane.SetSize(100, 50)
	pane.WritePTYOutput([]byte("Initial content line 1\r\n"))
	pane.WritePTYOutput([]byte("Initial content line 2\r\n"))

	// Verify content is present
	render1 := pane.renderVTerm()
	if !strings.Contains(render1, "Initial content") {
		t.Error("Content should be present before resize")
	}

	// Get a reference to the old vterm
	oldVterm := pane.vterm

	// Resize - this should create a new vterm
	pane.SetSize(80, 40)

	// Verify a new vterm was created
	if pane.vterm == oldVterm {
		t.Error("Resize should create a new vterm instance")
	}

	// The new vterm should be empty (fresh)
	render2 := pane.renderVTerm()
	if strings.Contains(render2, "Initial content") {
		t.Error("New vterm should not contain old content")
	}

	// Writing new content should work at the new size
	newWidth := 80 - 4 // inner width
	newLine := strings.Repeat("N", newWidth)
	pane.WritePTYOutput([]byte(newLine + "\r\n"))

	render3 := pane.renderVTerm()
	stripped := stripANSI(render3)
	if !strings.Contains(stripped, strings.Repeat("N", newWidth)) {
		t.Error("New content should be present at new width")
	}
}

// TestPaneModel_ResizeNoChange verifies that setting the same size doesn't
// create a new vterm unnecessarily.
func TestPaneModel_ResizeNoChange(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	// Initial size with content
	pane.SetSize(100, 50)
	pane.WritePTYOutput([]byte("Important content\r\n"))

	// Get vterm reference
	oldVterm := pane.vterm

	// Set same size again
	pane.SetSize(100, 50)

	// Should keep the same vterm
	if pane.vterm != oldVterm {
		t.Error("Same size should not create new vterm")
	}

	// Content should still be there
	render := pane.renderVTerm()
	if !strings.Contains(render, "Important content") {
		t.Error("Content should be preserved when size doesn't change")
	}
}
