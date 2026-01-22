//go:build integration

package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/STRML/claude-cells/internal/workstream"
)

// TestViewportFillHeight_MultiLinePromptScenario tests the exact scenario
// that caused the original bug: Claude Code with multi-line prompts creating
// variable amounts of trailing empty lines in the vterm.
//
// The bug: renderVTerm() trims trailing empty lines, and without FillHeight=true
// on the viewport, the output height would vary based on content, causing
// visual issues in the scrollback area above the prompt.
func TestViewportFillHeight_MultiLinePromptScenario(t *testing.T) {
	testCases := []struct {
		name         string
		contentLines int // Lines of actual content to write
	}{
		{"empty_vterm", 0},
		{"minimal_content_large_prompt", 2},
		{"half_content", 10},
		{"mostly_content_small_prompt", 18},
		{"full_content", 20},
	}

	paneWidth := 80
	paneHeight := 30

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ws := workstream.New("test")
			ws.SetState(workstream.StateRunning)
			pane := NewPaneModel(ws)
			pane.SetSize(paneWidth, paneHeight)
			pane.SetFocused(true)

			// Write content lines to simulate Claude Code output
			for i := 0; i < tc.contentLines; i++ {
				pane.WritePTYOutput([]byte(fmt.Sprintf("Content line %d\r\n", i+1)))
			}

			view := pane.View()
			lineCount := strings.Count(view, "\n") + 1

			if lineCount != paneHeight {
				t.Errorf("View height = %d, want %d (content=%d)",
					lineCount, paneHeight, tc.contentLines)
			}
		})
	}
}

// TestViewportFillHeight_AfterScrollback verifies the fix works correctly
// even after scrollback has accumulated. This tests the scenario where
// a user has been working for a while (lots of scrollback) and then
// Claude Code clears its output or has a large prompt area.
func TestViewportFillHeight_AfterScrollback(t *testing.T) {
	ws := workstream.New("test")
	ws.SetState(workstream.StateRunning)
	pane := NewPaneModel(ws)

	paneWidth := 80
	paneHeight := 30
	pane.SetSize(paneWidth, paneHeight)
	pane.SetFocused(true)

	// Write enough content to trigger scrollback
	for i := 0; i < 100; i++ {
		pane.WritePTYOutput([]byte(fmt.Sprintf("Scrollback line %d\r\n", i)))
	}

	// Simulate content being cleared (sparse content like multi-line prompt scenario)
	// This is similar to when Claude Code has a large input prompt area
	for i := 0; i < 5; i++ {
		pane.WritePTYOutput([]byte(fmt.Sprintf("New content %d\r\n", i)))
	}

	view := pane.View()
	lineCount := strings.Count(view, "\n") + 1

	if lineCount != paneHeight {
		t.Errorf("View height after scrollback = %d, want %d", lineCount, paneHeight)
	}
}

// TestViewportFillHeight_ResizePreservesConsistency verifies that
// resize operations maintain consistent height. This tests the scenario
// where the terminal window is resized while content is sparse.
func TestViewportFillHeight_ResizePreservesConsistency(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetFocused(true)

	// Write sparse content (simulates large prompt area)
	for i := 0; i < 5; i++ {
		pane.WritePTYOutput([]byte(fmt.Sprintf("Line %d\r\n", i)))
	}

	sizes := []struct{ w, h int }{
		{80, 30},
		{120, 40},
		{60, 20},
		{100, 35},
	}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.w, size.h), func(t *testing.T) {
			pane.SetSize(size.w, size.h)
			view := pane.View()
			lineCount := strings.Count(view, "\n") + 1

			if lineCount != size.h {
				t.Errorf("View height at %dx%d = %d, want %d",
					size.w, size.h, lineCount, size.h)
			}
		})
	}
}

// TestViewportFillHeight_TrailingEmptyLines specifically tests the scenario
// where the vterm has content followed by many empty lines (the exact bug scenario).
// renderVTerm() trims these empty lines, and without FillHeight=true, the
// viewport would not pad them back.
func TestViewportFillHeight_TrailingEmptyLines(t *testing.T) {
	ws := workstream.New("test")
	ws.SetState(workstream.StateRunning)
	pane := NewPaneModel(ws)

	paneWidth := 80
	paneHeight := 30
	pane.SetSize(paneWidth, paneHeight)
	pane.SetFocused(true)

	// Write content only at the top of the vterm
	// This simulates Claude Code with a very large multi-line prompt
	pane.WritePTYOutput([]byte("First line\r\n"))
	pane.WritePTYOutput([]byte("Second line\r\n"))

	// The vterm now has content at lines 0-1, and lines 2+ are empty
	// renderVTerm() will trim these trailing empty lines
	// FillHeight=true ensures the viewport pads the output

	view := pane.View()
	lineCount := strings.Count(view, "\n") + 1

	if lineCount != paneHeight {
		t.Errorf("View height with trailing empty lines = %d, want %d", lineCount, paneHeight)
	}

	// Also verify that the content is actually at the expected position
	// (not pushed to bottom or otherwise misaligned)
	// Note: The content is rendered through the vterm which may transform it,
	// but the key assertion is the consistent height
	vtermContent := pane.renderVTerm()
	if !strings.Contains(vtermContent, "First line") {
		t.Logf("vterm content: %q", vtermContent)
		t.Error("vterm should contain 'First line'")
	}
	if !strings.Contains(vtermContent, "Second line") {
		t.Error("vterm should contain 'Second line'")
	}
}

// TestViewportFillHeight_Enabled is a simple verification that the
// FillHeight property is actually set. This catches if the fix is
// accidentally removed.
func TestViewportFillHeight_Enabled(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	if !pane.viewport.FillHeight {
		t.Error("Viewport should have FillHeight=true to ensure consistent output height")
	}
}
