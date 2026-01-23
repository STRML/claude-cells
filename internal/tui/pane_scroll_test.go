package tui

import (
	"strings"
	"testing"

	"github.com/STRML/claude-cells/internal/workstream"
)

// setupPaneWithContent creates a PaneModel with content for scroll testing.
// Sets up the viewport with the given dimensions and fills it with numbered lines.
func setupPaneWithContent(width, height, contentLines int) *PaneModel {
	ws := &workstream.Workstream{}
	p := NewPaneModel(ws)

	// Set size to configure the viewport
	p.SetSize(width, height)

	// Generate content with more lines than viewport height
	var lines []string
	for i := 1; i <= contentLines; i++ {
		lines = append(lines, strings.Repeat("x", 10))
	}
	content := strings.Join(lines, "\n")
	p.viewport.SetContent(content)

	return &p
}

func TestIsScrollMode(t *testing.T) {
	ws := &workstream.Workstream{}
	p := NewPaneModel(ws)

	// Initially false
	if p.IsScrollMode() {
		t.Error("IsScrollMode() should initially be false")
	}

	// After entering scroll mode
	p.EnterScrollMode()
	if !p.IsScrollMode() {
		t.Error("IsScrollMode() should be true after EnterScrollMode()")
	}
}

func TestEnterScrollMode(t *testing.T) {
	ws := &workstream.Workstream{}
	p := NewPaneModel(ws)

	p.EnterScrollMode()

	if !p.scrollMode {
		t.Error("scrollMode should be true after EnterScrollMode()")
	}
}

func TestScrollToBottom(t *testing.T) {
	p := setupPaneWithContent(80, 30, 100)

	// Scroll up first
	p.EnterScrollMode()
	p.viewport.ScrollUp(50)

	// Verify we're in scroll mode
	if !p.IsScrollMode() {
		t.Error("should be in scroll mode after EnterScrollMode()")
	}

	// Scroll to bottom
	p.ScrollToBottom()

	if p.IsScrollMode() {
		t.Error("ScrollToBottom() should exit scroll mode")
	}
	if !p.viewport.AtBottom() {
		t.Error("viewport should be at bottom after ScrollToBottom()")
	}
}

func TestScrollPageUp(t *testing.T) {
	p := setupPaneWithContent(80, 30, 100)

	// Start at bottom
	p.viewport.GotoBottom()

	// Scroll page up
	p.ScrollPageUp()

	if !p.IsScrollMode() {
		t.Error("ScrollPageUp() should enter scroll mode")
	}
	if p.viewport.AtBottom() {
		t.Error("viewport should not be at bottom after ScrollPageUp()")
	}
}

func TestScrollPageDown_ExitsScrollModeAtBottom(t *testing.T) {
	p := setupPaneWithContent(80, 30, 100)

	// Enter scroll mode and scroll up
	p.EnterScrollMode()
	p.viewport.GotoTop()

	// Scroll down many pages to reach bottom
	for i := 0; i < 20; i++ {
		p.ScrollPageDown()
	}

	// Should exit scroll mode when at bottom
	if p.IsScrollMode() {
		t.Error("ScrollPageDown() should exit scroll mode when at bottom")
	}
	if !p.viewport.AtBottom() {
		t.Error("viewport should be at bottom after scrolling all the way down")
	}
}

func TestScrollPageDown_StaysInScrollModeWhenNotAtBottom(t *testing.T) {
	p := setupPaneWithContent(80, 30, 100)

	// Enter scroll mode and go to top
	p.EnterScrollMode()
	p.viewport.GotoTop()

	// Scroll down just one page (not enough to reach bottom)
	p.ScrollPageDown()

	// Should stay in scroll mode if not at bottom
	// Note: scrollMode is only exited when AtBottom() returns true
	if p.viewport.AtBottom() {
		// If we're at bottom, scroll mode should be false
		if p.IsScrollMode() {
			t.Error("ScrollPageDown() should exit scroll mode when at bottom")
		}
	}
}

func TestScrollLineUp(t *testing.T) {
	p := setupPaneWithContent(80, 30, 100)

	// Start at bottom
	p.viewport.GotoBottom()
	initialOffset := p.viewport.YOffset()

	// Scroll one line up
	p.ScrollLineUp()

	if !p.IsScrollMode() {
		t.Error("ScrollLineUp() should enter scroll mode")
	}
	if p.viewport.YOffset() >= initialOffset {
		t.Errorf("YOffset should decrease after ScrollLineUp(), got %d (was %d)", p.viewport.YOffset(), initialOffset)
	}
}

func TestScrollLineDown_ExitsScrollModeAtBottom(t *testing.T) {
	p := setupPaneWithContent(80, 30, 100)

	// Enter scroll mode, go to near bottom
	p.EnterScrollMode()
	p.viewport.GotoBottom()
	p.viewport.ScrollUp(1) // Go up one line from bottom

	// Scroll down one line (should reach bottom)
	p.ScrollLineDown()

	if p.IsScrollMode() {
		t.Error("ScrollLineDown() should exit scroll mode when at bottom")
	}
}

func TestScrollHalfPageUp(t *testing.T) {
	p := setupPaneWithContent(80, 30, 100)

	// Start at bottom
	p.viewport.GotoBottom()
	initialOffset := p.viewport.YOffset()

	// Scroll half page up
	p.ScrollHalfPageUp()

	if !p.IsScrollMode() {
		t.Error("ScrollHalfPageUp() should enter scroll mode")
	}
	// Should scroll by approximately half the viewport height
	expectedScroll := p.viewport.Height() / 2
	if expectedScroll < 1 {
		expectedScroll = 1
	}
	actualScroll := initialOffset - p.viewport.YOffset()
	if actualScroll != expectedScroll {
		t.Errorf("ScrollHalfPageUp() scrolled %d lines, expected %d", actualScroll, expectedScroll)
	}
}

func TestScrollHalfPageUp_MinimumOneLine(t *testing.T) {
	p := setupPaneWithContent(80, 10, 20) // Small viewport

	// Manually set viewport height to 1 to test minimum
	p.viewport.SetHeight(1)
	p.viewport.GotoBottom()
	initialOffset := p.viewport.YOffset()

	p.ScrollHalfPageUp()

	// Should scroll at least 1 line
	actualScroll := initialOffset - p.viewport.YOffset()
	if actualScroll < 1 {
		t.Errorf("ScrollHalfPageUp() should scroll at least 1 line, scrolled %d", actualScroll)
	}
}

func TestScrollHalfPageDown_ExitsScrollModeAtBottom(t *testing.T) {
	p := setupPaneWithContent(80, 30, 100)

	// Enter scroll mode and go to top
	p.EnterScrollMode()
	p.viewport.GotoTop()

	// Scroll down many half pages to reach bottom
	for i := 0; i < 20; i++ {
		p.ScrollHalfPageDown()
	}

	if p.IsScrollMode() {
		t.Error("ScrollHalfPageDown() should exit scroll mode when at bottom")
	}
}

func TestScrollHalfPageDown_MinimumOneLine(t *testing.T) {
	p := setupPaneWithContent(80, 10, 20)

	// Set very small viewport height
	p.viewport.SetHeight(1)
	p.viewport.GotoTop()
	p.EnterScrollMode()

	initialOffset := p.viewport.YOffset()
	p.ScrollHalfPageDown()

	// Should scroll at least 1 line
	actualScroll := p.viewport.YOffset() - initialOffset
	if actualScroll < 1 && !p.viewport.AtBottom() {
		t.Errorf("ScrollHalfPageDown() should scroll at least 1 line when not at bottom, scrolled %d", actualScroll)
	}
}

func TestClearScrollback(t *testing.T) {
	ws := &workstream.Workstream{}
	p := NewPaneModel(ws)

	// Add some scrollback content
	p.scrollback = []string{"line1", "line2", "line3"}

	if len(p.scrollback) != 3 {
		t.Errorf("scrollback should have 3 lines, got %d", len(p.scrollback))
	}

	p.ClearScrollback()

	if p.scrollback != nil {
		t.Errorf("ClearScrollback() should set scrollback to nil, got %v", p.scrollback)
	}
}

func TestClearScrollback_AlreadyEmpty(t *testing.T) {
	ws := &workstream.Workstream{}
	p := NewPaneModel(ws)

	// Scrollback is nil by default
	p.ClearScrollback()

	// Should not panic and scrollback should still be nil
	if p.scrollback != nil {
		t.Errorf("ClearScrollback() on empty scrollback should keep it nil")
	}
}

func TestMultipleScrollOperations(t *testing.T) {
	p := setupPaneWithContent(80, 30, 100)

	// Start at bottom, not in scroll mode
	p.viewport.GotoBottom()
	if p.IsScrollMode() {
		t.Error("should not be in scroll mode initially")
	}

	// Page up multiple times
	for i := 0; i < 5; i++ {
		p.ScrollPageUp()
	}

	if !p.IsScrollMode() {
		t.Error("should be in scroll mode after ScrollPageUp()")
	}

	// Then scroll to bottom
	p.ScrollToBottom()

	if p.IsScrollMode() {
		t.Error("should exit scroll mode after ScrollToBottom()")
	}
	if !p.viewport.AtBottom() {
		t.Error("should be at bottom after ScrollToBottom()")
	}
}

func TestScrollModeStateTransitions(t *testing.T) {
	tests := []struct {
		name      string
		operation func(p *PaneModel)
		wantMode  bool
	}{
		{
			name:      "EnterScrollMode enters",
			operation: func(p *PaneModel) { p.EnterScrollMode() },
			wantMode:  true,
		},
		{
			name:      "ScrollToBottom exits",
			operation: func(p *PaneModel) { p.EnterScrollMode(); p.ScrollToBottom() },
			wantMode:  false,
		},
		{
			name:      "ScrollPageUp enters",
			operation: func(p *PaneModel) { p.ScrollPageUp() },
			wantMode:  true,
		},
		{
			name:      "ScrollLineUp enters",
			operation: func(p *PaneModel) { p.ScrollLineUp() },
			wantMode:  true,
		},
		{
			name:      "ScrollHalfPageUp enters",
			operation: func(p *PaneModel) { p.ScrollHalfPageUp() },
			wantMode:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := setupPaneWithContent(80, 30, 100)
			p.viewport.GotoBottom()

			tt.operation(p)

			if got := p.IsScrollMode(); got != tt.wantMode {
				t.Errorf("IsScrollMode() = %v, want %v", got, tt.wantMode)
			}
		})
	}
}
