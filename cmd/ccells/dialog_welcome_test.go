package main

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestWelcomeDialog_View(t *testing.T) {
	m := newWelcomeDialog("/tmp/state", "claude")
	v := m.View()
	content := fmt.Sprintf("%v", v.Content)

	// Should contain branding
	if !containsStr(content, "Claude Cells") {
		t.Error("expected 'Claude Cells' in welcome view")
	}
	// Should contain keybinding hints
	for _, key := range []string{"Create workstream", "Destroy workstream", "Merge PR", "Help"} {
		if !containsStr(content, key) {
			t.Errorf("expected %q in welcome view", key)
		}
	}
	// Should contain call to action
	if !containsStr(content, "Enter") {
		t.Error("expected 'Enter' prompt in welcome view")
	}
	if !containsStr(content, "Esc") {
		t.Error("expected 'Esc' prompt in welcome view")
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

// containsStr checks if s contains substr (helper for view content testing)
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
