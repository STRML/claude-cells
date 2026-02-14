package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestMergeDialog_Init(t *testing.T) {
	m := newMergeDialog([]string{"ws1"})
	cmd := m.Init()
	if cmd != nil {
		t.Error("Init() should return nil")
	}
}

func TestMergeDialog_EmptyState(t *testing.T) {
	m := newMergeDialog(nil)

	content := viewContent(m.View())
	if !strings.Contains(content, "No workstreams available") {
		t.Errorf("empty state should show 'No workstreams available', got: %s", content)
	}
	if !strings.Contains(content, "Esc to close") {
		t.Error("empty state should show dismiss hint")
	}
}

func TestMergeDialog_EmptyState_EnterIgnored(t *testing.T) {
	m := newMergeDialog(nil)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	md := updated.(mergeDialog)
	if cmd != nil {
		t.Error("Enter on empty list should not produce a command")
	}
	if md.done {
		t.Error("Enter on empty list should not quit")
	}
}

func TestMergeDialog_Navigation(t *testing.T) {
	m := newMergeDialog([]string{"ws1", "ws2", "ws3"})

	if m.selected != 0 {
		t.Errorf("initial selected = %d, want 0", m.selected)
	}

	// Move down
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	md := updated.(mergeDialog)
	if md.selected != 1 {
		t.Errorf("after down, selected = %d, want 1", md.selected)
	}

	// Move down to end
	updated, _ = md.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	md = updated.(mergeDialog)
	if md.selected != 2 {
		t.Errorf("after second down, selected = %d, want 2", md.selected)
	}

	// At bottom, stays
	updated, _ = md.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	md = updated.(mergeDialog)
	if md.selected != 2 {
		t.Errorf("at bottom, selected = %d, want 2", md.selected)
	}

	// Move up
	updated, _ = md.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	md = updated.(mergeDialog)
	if md.selected != 1 {
		t.Errorf("after up, selected = %d, want 1", md.selected)
	}
}

func TestMergeDialog_VimNavigation(t *testing.T) {
	m := newMergeDialog([]string{"ws1", "ws2"})

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	md := updated.(mergeDialog)
	if md.selected != 1 {
		t.Errorf("after j, selected = %d, want 1", md.selected)
	}

	updated, _ = md.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	md = updated.(mergeDialog)
	if md.selected != 0 {
		t.Errorf("after k, selected = %d, want 0", md.selected)
	}
}

func TestMergeDialog_SelectShowsConfirmation(t *testing.T) {
	m := newMergeDialog([]string{"ws1", "ws2"})

	// Press enter to select
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	md := updated.(mergeDialog)
	if md.step != 1 {
		t.Errorf("after enter, step = %d, want 1", md.step)
	}

	content := viewContent(md.View())
	if !strings.Contains(content, "Create PR for 'ws1'?") {
		t.Errorf("confirmation should show selected workstream name, got: %s", content)
	}
	if !strings.Contains(content, "Press Enter to create PR") {
		t.Error("confirmation should show enter prompt")
	}
}

func TestMergeDialog_ViewSelection(t *testing.T) {
	m := newMergeDialog([]string{"ws1", "ws2"})

	content := viewContent(m.View())
	if !strings.Contains(content, "> ws1") {
		t.Errorf("initial view should show cursor on first item, got: %s", content)
	}
	if !strings.Contains(content, "Create Pull Request") {
		t.Error("view should show title")
	}
}

func TestMergeDialog_EscQuits(t *testing.T) {
	m := newMergeDialog([]string{"ws1"})

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	md := updated.(mergeDialog)
	if !md.done {
		t.Error("esc should set done")
	}
	if cmd == nil {
		t.Error("esc should produce a quit command")
	}
}

func TestMergeDialog_CtrlCQuits(t *testing.T) {
	m := newMergeDialog([]string{"ws1"})

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	md := updated.(mergeDialog)
	if !md.done {
		t.Error("ctrl+c should set done")
	}
	if cmd == nil {
		t.Error("ctrl+c should produce a quit command")
	}
}

func TestMergeDialog_QKeyQuits(t *testing.T) {
	m := newMergeDialog([]string{"ws1"})

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	md := updated.(mergeDialog)
	if !md.done {
		t.Error("q should set done")
	}
	if cmd == nil {
		t.Error("q should produce a quit command")
	}
}

func TestMergeDialog_DoneViewEmpty(t *testing.T) {
	m := newMergeDialog([]string{"ws1"})
	m.done = true
	content := viewContent(m.View())
	if content != "" {
		t.Errorf("done view should be empty, got: %q", content)
	}
}

func TestMergeDialog_UpAtTop(t *testing.T) {
	m := newMergeDialog([]string{"ws1"})

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	md := updated.(mergeDialog)
	if md.selected != 0 {
		t.Errorf("up at top: selected = %d, want 0", md.selected)
	}
}
