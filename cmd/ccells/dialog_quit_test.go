package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestQuitDialog_Init(t *testing.T) {
	m := newQuitDialog()
	cmd := m.Init()
	if cmd != nil {
		t.Error("Init() should return nil")
	}
	if m.selected != 0 {
		t.Errorf("default selected = %d, want 0 (Yes)", m.selected)
	}
}

func TestQuitDialog_View(t *testing.T) {
	m := newQuitDialog()
	content := viewContent(m.View())

	if !strings.Contains(content, "Quit ccells?") {
		t.Error("should show 'Quit ccells?' header")
	}
	if !strings.Contains(content, "paused") {
		t.Error("should mention pausing workstreams")
	}
	if !strings.Contains(content, "resumed") {
		t.Error("should mention resuming")
	}
	if !strings.Contains(content, "Yes, quit") {
		t.Error("should show Yes option")
	}
	if !strings.Contains(content, "No, cancel") {
		t.Error("should show No option")
	}
	if !strings.Contains(content, "→") {
		t.Error("should show arrow cursor")
	}
	if !strings.Contains(content, "navigate") {
		t.Error("should show navigation hint")
	}
}

func TestQuitDialog_Navigation(t *testing.T) {
	m := newQuitDialog()

	// Default is 0 (Yes)
	if m.selected != 0 {
		t.Errorf("initial selected = %d, want 0", m.selected)
	}

	// Move down
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	qd := updated.(quitDialog)
	if qd.selected != 1 {
		t.Errorf("after down, selected = %d, want 1", qd.selected)
	}

	// Move down at bottom — stay
	updated, _ = qd.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	qd = updated.(quitDialog)
	if qd.selected != 1 {
		t.Errorf("at bottom, selected = %d, want 1", qd.selected)
	}

	// Move up
	updated, _ = qd.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	qd = updated.(quitDialog)
	if qd.selected != 0 {
		t.Errorf("after up, selected = %d, want 0", qd.selected)
	}

	// Move up at top — stay
	updated, _ = qd.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	qd = updated.(quitDialog)
	if qd.selected != 0 {
		t.Errorf("at top, selected = %d, want 0", qd.selected)
	}
}

func TestQuitDialog_VimNavigation(t *testing.T) {
	m := newQuitDialog()

	// j = down
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	qd := updated.(quitDialog)
	if qd.selected != 1 {
		t.Errorf("after j, selected = %d, want 1", qd.selected)
	}

	// k = up
	updated, _ = qd.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	qd = updated.(quitDialog)
	if qd.selected != 0 {
		t.Errorf("after k, selected = %d, want 0", qd.selected)
	}
}

func TestQuitDialog_EnterYes(t *testing.T) {
	m := newQuitDialog()
	// selected=0 → Yes
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	qd := updated.(quitDialog)
	if !qd.done {
		t.Error("should be done")
	}
	if !qd.quit {
		t.Error("should signal quit when Yes is selected")
	}
	if cmd == nil {
		t.Error("should produce tea.Quit command")
	}
}

func TestQuitDialog_EnterNo(t *testing.T) {
	m := newQuitDialog()
	// Move to No first
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	qd := updated.(quitDialog)

	updated, cmd := qd.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	qd = updated.(quitDialog)
	if !qd.done {
		t.Error("should be done")
	}
	if qd.quit {
		t.Error("should NOT signal quit when No is selected")
	}
	if cmd == nil {
		t.Error("should produce tea.Quit command")
	}
}

func TestQuitDialog_YShortcut(t *testing.T) {
	m := newQuitDialog()
	// Even if on "No", pressing y should quit
	m.selected = 1

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	qd := updated.(quitDialog)
	if !qd.done {
		t.Error("should be done")
	}
	if !qd.quit {
		t.Error("y shortcut should signal quit")
	}
	if cmd == nil {
		t.Error("should produce tea.Quit command")
	}
}

func TestQuitDialog_NShortcut(t *testing.T) {
	m := newQuitDialog()
	// Even if on "Yes", pressing n should cancel
	m.selected = 0

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	qd := updated.(quitDialog)
	if !qd.done {
		t.Error("should be done")
	}
	if qd.quit {
		t.Error("n shortcut should NOT signal quit")
	}
	if cmd == nil {
		t.Error("should produce tea.Quit command")
	}
}

func TestQuitDialog_EscCancels(t *testing.T) {
	m := newQuitDialog()
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	qd := updated.(quitDialog)
	if !qd.done {
		t.Error("esc should set done")
	}
	if qd.quit {
		t.Error("esc should NOT signal quit")
	}
	if cmd == nil {
		t.Error("should produce tea.Quit command")
	}
}

func TestQuitDialog_CtrlCCancels(t *testing.T) {
	m := newQuitDialog()
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	qd := updated.(quitDialog)
	if !qd.done {
		t.Error("ctrl+c should set done")
	}
	if qd.quit {
		t.Error("ctrl+c should NOT signal quit")
	}
	if cmd == nil {
		t.Error("should produce tea.Quit command")
	}
}

func TestQuitDialog_QKeyCancels(t *testing.T) {
	m := newQuitDialog()
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	qd := updated.(quitDialog)
	if !qd.done {
		t.Error("q should set done")
	}
	if qd.quit {
		t.Error("q should NOT signal quit")
	}
	if cmd == nil {
		t.Error("should produce tea.Quit command")
	}
}

func TestQuitDialog_DoneViewEmpty(t *testing.T) {
	m := newQuitDialog()
	m.done = true
	content := viewContent(m.View())
	if content != "" {
		t.Errorf("done view should be empty, got: %q", content)
	}
}

func TestQuitDialog_CursorOnYes(t *testing.T) {
	m := newQuitDialog()
	m.selected = 0
	content := viewContent(m.View())
	// The arrow should be before "Yes"
	lines := strings.Split(content, "\n")
	foundArrow := false
	for _, line := range lines {
		if strings.Contains(line, "→") && strings.Contains(line, "Yes") {
			foundArrow = true
		}
	}
	if !foundArrow {
		t.Error("arrow cursor should be on Yes option")
	}
}

func TestQuitDialog_CursorOnNo(t *testing.T) {
	m := newQuitDialog()
	m.selected = 1
	content := viewContent(m.View())
	lines := strings.Split(content, "\n")
	foundArrow := false
	for _, line := range lines {
		if strings.Contains(line, "→") && strings.Contains(line, "No") {
			foundArrow = true
		}
	}
	if !foundArrow {
		t.Error("arrow cursor should be on No option")
	}
}
