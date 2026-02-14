package main

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// viewContent extracts the text content from a tea.View for test assertions.
func viewContent(v tea.View) string {
	return fmt.Sprintf("%v", v.Content)
}

func TestRmDialog_Init(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1", "ws2"})
	cmd := m.Init()
	if cmd != nil {
		t.Error("Init() should return nil")
	}
}

func TestRmDialog_EmptyState(t *testing.T) {
	m := newRmDialog("/tmp/state", nil)

	content := viewContent(m.View())
	if !strings.Contains(content, "No workstreams to destroy") {
		t.Errorf("empty state should show 'No workstreams to destroy', got: %s", content)
	}
	if !strings.Contains(content, "Esc to close") {
		t.Error("empty state should show dismiss hint")
	}
}

func TestRmDialog_EmptyState_EnterIgnored(t *testing.T) {
	m := newRmDialog("/tmp/state", nil)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	rm := updated.(rmDialog)
	if cmd != nil {
		t.Error("Enter on empty list should not produce a command")
	}
	if rm.done {
		t.Error("Enter on empty list should not quit")
	}
}

func TestRmDialog_Navigation(t *testing.T) {
	items := []string{"ws1", "ws2", "ws3"}
	m := newRmDialog("/tmp/state", items)

	if m.selected != 0 {
		t.Errorf("initial selected = %d, want 0", m.selected)
	}

	// Move down
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	rm := updated.(rmDialog)
	if rm.selected != 1 {
		t.Errorf("after down, selected = %d, want 1", rm.selected)
	}

	// Move down again
	updated, _ = rm.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	rm = updated.(rmDialog)
	if rm.selected != 2 {
		t.Errorf("after second down, selected = %d, want 2", rm.selected)
	}

	// Move down at bottom — should stay
	updated, _ = rm.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	rm = updated.(rmDialog)
	if rm.selected != 2 {
		t.Errorf("at bottom, selected = %d, want 2", rm.selected)
	}

	// Move up
	updated, _ = rm.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	rm = updated.(rmDialog)
	if rm.selected != 1 {
		t.Errorf("after up, selected = %d, want 1", rm.selected)
	}
}

func TestRmDialog_VimNavigation(t *testing.T) {
	items := []string{"ws1", "ws2"}
	m := newRmDialog("/tmp/state", items)

	// j = down
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	rm := updated.(rmDialog)
	if rm.selected != 1 {
		t.Errorf("after j, selected = %d, want 1", rm.selected)
	}

	// k = up
	updated, _ = rm.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	rm = updated.(rmDialog)
	if rm.selected != 0 {
		t.Errorf("after k, selected = %d, want 0", rm.selected)
	}
}

func TestRmDialog_UpAtTop(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1"})

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	rm := updated.(rmDialog)
	if rm.selected != 0 {
		t.Errorf("up at top: selected = %d, want 0", rm.selected)
	}
}

func TestRmDialog_SelectAndConfirm(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1", "ws2"})

	// Press enter to select
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	rm := updated.(rmDialog)
	if !rm.confirmed {
		t.Error("after first enter, should be in confirmed state")
	}

	// View should show confirmation
	content := viewContent(rm.View())
	if !strings.Contains(content, "Destroy 'ws1'?") {
		t.Errorf("confirmation view should show selected workstream, got: %s", content)
	}
	if !strings.Contains(content, "Press Enter to confirm") {
		t.Error("confirmation view should show enter prompt")
	}
}

func TestRmDialog_ViewSelection(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1", "ws2"})

	content := viewContent(m.View())
	if !strings.Contains(content, "> ws1") {
		t.Errorf("initial view should show cursor on first item, got: %s", content)
	}
	if !strings.Contains(content, "Destroy Workstream") {
		t.Error("view should show title")
	}
}

func TestRmDialog_EscQuits(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1"})

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	rm := updated.(rmDialog)
	if !rm.done {
		t.Error("esc should set done")
	}
	if cmd == nil {
		t.Error("esc should produce a tea.Quit command")
	}
}

func TestRmDialog_CtrlCQuits(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1"})

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	rm := updated.(rmDialog)
	if !rm.done {
		t.Error("ctrl+c should set done")
	}
	if cmd == nil {
		t.Error("ctrl+c should produce a tea.Quit command")
	}
}

func TestRmDialog_QKeyQuits(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1"})

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	rm := updated.(rmDialog)
	if !rm.done {
		t.Error("q should set done")
	}
	if cmd == nil {
		t.Error("q should produce a tea.Quit command")
	}
}

func TestRmDialog_DoneViewEmpty(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1"})
	m.done = true
	content := viewContent(m.View())
	if content != "" {
		t.Errorf("done view should be empty, got: %q", content)
	}
}
