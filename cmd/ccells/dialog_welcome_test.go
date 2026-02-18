package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestWelcomeDialog_View(t *testing.T) {
	m := newWelcomeDialog("/tmp/state", "claude")
	// Simulate window size for centering
	m.width = 80
	m.height = 40
	v := m.View()
	content := fmt.Sprintf("%v", v.Content)

	// Should contain content text (not the title which is per-char colored)
	for _, want := range []string{
		"Run parallel Claude Code instances",
		"Create workstream",
		"Destroy workstream",
		"Merge PR",
		"Help",
		"Enter",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected %q in welcome view", want)
		}
	}

	// Should NOT contain pause/resume
	if strings.Contains(content, "Pause") || strings.Contains(content, "Resume") {
		t.Error("should not contain Pause/Resume in welcome view")
	}
}

func TestWelcomeDialog_ViewCentered(t *testing.T) {
	m := newWelcomeDialog("/tmp/state", "claude")
	m.width = 120
	m.height = 50
	v := m.View()
	content := fmt.Sprintf("%v", v.Content)

	lines := strings.Split(content, "\n")
	// With height 50 and ~18 content lines, top padding should be > 0
	// First several lines should be empty (vertical padding)
	emptyCount := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			emptyCount++
		} else {
			break
		}
	}
	if emptyCount < 5 {
		t.Errorf("expected vertical centering padding, got %d empty lines at top", emptyCount)
	}

	// Content lines should have leading spaces (horizontal padding)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(line, "  ") {
			t.Errorf("expected horizontal padding, line starts at column 0: %q", line[:min(40, len(line))])
			break
		}
	}
}

func TestWelcomeDialog_ViewNoCenteringSmallTerminal(t *testing.T) {
	m := newWelcomeDialog("/tmp/state", "claude")
	m.width = 40 // narrower than content
	m.height = 15
	v := m.View()
	content := fmt.Sprintf("%v", v.Content)

	// Should still render without panicking
	if content == "" {
		t.Error("expected non-empty view even in small terminal")
	}
}

func TestWelcomeDialog_ShimmerTitle(t *testing.T) {
	title := renderShimmerTitle(0)

	// Should contain CLAUDE CELLS characters
	for _, ch := range "CLAUDECELLS" {
		if !strings.ContainsRune(title, ch) {
			t.Errorf("expected character %c in shimmer title", ch)
		}
	}

	// Should contain ANSI 256-color sequences
	if !strings.Contains(title, "\033[38;5;") {
		t.Error("expected ANSI 256-color sequences in shimmer title")
	}

	// Should be bold
	if !strings.Contains(title, "\033[1m") {
		t.Error("expected bold in shimmer title")
	}
}

func TestWelcomeDialog_ShimmerChangesPerFrame(t *testing.T) {
	frame0 := renderShimmerTitle(0)
	frame5 := renderShimmerTitle(5)

	if frame0 == frame5 {
		t.Error("shimmer should produce different output at different frames")
	}
}

func TestWelcomeDialog_EnterCreates(t *testing.T) {
	m := newWelcomeDialog("/tmp/state", "claude")
	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	result, cmd := m.Update(msg)
	wm := result.(welcomeDialog)

	if !wm.create {
		t.Error("expected create=true after Enter")
	}
	if !wm.done {
		t.Error("expected done=true after Enter")
	}
	if cmd == nil {
		t.Error("expected tea.Quit command")
	}
}

func TestWelcomeDialog_NCreates(t *testing.T) {
	m := newWelcomeDialog("/tmp/state", "claude")
	msg := tea.KeyPressMsg{Code: rune('n'), Text: "n"}
	result, cmd := m.Update(msg)
	wm := result.(welcomeDialog)

	if !wm.create {
		t.Error("expected create=true after 'n'")
	}
	if !wm.done {
		t.Error("expected done=true after 'n'")
	}
	if cmd == nil {
		t.Error("expected tea.Quit command")
	}
}

func TestWelcomeDialog_EscExits(t *testing.T) {
	m := newWelcomeDialog("/tmp/state", "claude")
	msg := tea.KeyPressMsg{Code: tea.KeyEscape}
	result, cmd := m.Update(msg)
	wm := result.(welcomeDialog)

	if wm.create {
		t.Error("expected create=false after Esc")
	}
	if !wm.done {
		t.Error("expected done=true after Esc")
	}
	if cmd == nil {
		t.Error("expected tea.Quit command")
	}
}

func TestWelcomeDialog_QExits(t *testing.T) {
	m := newWelcomeDialog("/tmp/state", "claude")
	msg := tea.KeyPressMsg{Code: rune('q'), Text: "q"}
	result, cmd := m.Update(msg)
	wm := result.(welcomeDialog)

	if wm.create {
		t.Error("expected create=false after 'q'")
	}
	if !wm.done {
		t.Error("expected done=true after 'q'")
	}
	if cmd == nil {
		t.Error("expected tea.Quit command")
	}
}

func TestWelcomeDialog_CtrlCExits(t *testing.T) {
	m := newWelcomeDialog("/tmp/state", "claude")
	msg := tea.KeyPressMsg{Code: rune('c'), Mod: tea.ModCtrl}
	result, _ := m.Update(msg)
	wm := result.(welcomeDialog)

	if wm.create {
		t.Error("expected create=false after Ctrl+C")
	}
	if !wm.done {
		t.Error("expected done=true after Ctrl+C")
	}
}

func TestWelcomeDialog_DoneViewEmpty(t *testing.T) {
	m := newWelcomeDialog("/tmp/state", "claude")
	m.done = true
	v := m.View()
	content := fmt.Sprintf("%v", v.Content)
	if content != "" {
		t.Errorf("expected empty view when done, got %q", content)
	}
}

func TestWelcomeDialog_IgnoresOtherKeys(t *testing.T) {
	m := newWelcomeDialog("/tmp/state", "claude")
	msg := tea.KeyPressMsg{Code: rune('x'), Text: "x"}
	result, cmd := m.Update(msg)
	wm := result.(welcomeDialog)

	if wm.done {
		t.Error("expected done=false after unrelated key")
	}
	if wm.create {
		t.Error("expected create=false after unrelated key")
	}
	if cmd != nil {
		t.Error("expected no command after unrelated key")
	}
}

func TestWelcomeDialog_WindowSizeMsg(t *testing.T) {
	m := newWelcomeDialog("/tmp/state", "claude")
	result, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	wm := result.(welcomeDialog)

	if wm.width != 120 {
		t.Errorf("width = %d, want 120", wm.width)
	}
	if wm.height != 40 {
		t.Errorf("height = %d, want 40", wm.height)
	}
	if cmd != nil {
		t.Error("expected no command from WindowSizeMsg")
	}
}

func TestWelcomeDialog_TickAdvancesFrame(t *testing.T) {
	m := newWelcomeDialog("/tmp/state", "claude")
	result, cmd := m.Update(welcomeTickMsg(time.Now()))
	wm := result.(welcomeDialog)

	if wm.frame != 1 {
		t.Errorf("frame = %d, want 1 after tick", wm.frame)
	}
	if cmd == nil {
		t.Error("expected tick command to continue animation")
	}
}

func TestWelcomeDialog_InitReturnsTick(t *testing.T) {
	m := newWelcomeDialog("/tmp/state", "claude")
	cmd := m.Init()
	if cmd == nil {
		t.Error("Init should return shimmer tick command")
	}
}

func TestVisibleLen(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"plain text", "hello", 5},
		{"with ANSI", "\033[1mhello\033[0m", 5},
		{"multiple ANSI", "\033[35mn\033[0m  Create", 9},
		{"empty", "", 0},
		{"only ANSI", "\033[2m\033[0m", 0},
		{"256 color", "\033[38;5;33mX\033[0m", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := visibleLen(tt.input)
			if got != tt.want {
				t.Errorf("visibleLen(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
