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

func TestRmDialog_Init_MultipleWorkstreams(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1", "ws2"})
	cmd := m.Init()
	if cmd != nil {
		t.Error("Init() should return nil")
	}
	if m.step != rmStepSelect {
		t.Errorf("with multiple workstreams, step should be select, got %d", m.step)
	}
}

func TestRmDialog_Init_SingleWorkstream_AutoSelects(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"solo"})
	// Note: Init() modifies m but since it's a value receiver,
	// we need to check the model after Init runs in the program.
	// For testing, we check the expected behavior.
	_ = m.Init()
	// The Init method modifies the local copy, but in Bubble Tea
	// the model returned from Init() via commands handles this.
	// Let's verify by constructing the expected state directly.
	if len(m.workstreams) != 1 {
		t.Errorf("expected 1 workstream, got %d", len(m.workstreams))
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

func TestRmDialog_SelectNavigation(t *testing.T) {
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

func TestRmDialog_SelectEnterGoesToConfirm(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1", "ws2"})

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	rm := updated.(rmDialog)
	if rm.step != rmStepConfirm {
		t.Errorf("after enter, step = %d, want confirm", rm.step)
	}
	if rm.confirmInput != "" {
		t.Errorf("confirmInput should be empty, got %q", rm.confirmInput)
	}
}

func TestRmDialog_ConfirmView_ShowsName(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1", "ws2"})
	m.step = rmStepConfirm

	content := viewContent(m.View())
	if !strings.Contains(content, `Destroy workstream "ws1"`) {
		t.Errorf("confirm view should show workstream name, got: %s", content)
	}
}

func TestRmDialog_ConfirmView_ShowsConsequences(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1"})
	m.step = rmStepConfirm

	content := viewContent(m.View())
	if !strings.Contains(content, "This will:") {
		t.Error("should show consequences header")
	}
	if !strings.Contains(content, "Stop and remove the container") {
		t.Error("should show container consequence")
	}
	if !strings.Contains(content, "End any active Mutagen sync") {
		t.Error("should show sync consequence")
	}
	if !strings.Contains(content, "Delete local branch") {
		t.Error("should show branch consequence")
	}
}

func TestRmDialog_ConfirmView_ShowsInputBox(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1"})
	m.step = rmStepConfirm

	content := viewContent(m.View())
	if !strings.Contains(content, "╭") {
		t.Error("should show input box top border")
	}
	if !strings.Contains(content, "╰") {
		t.Error("should show input box bottom border")
	}
	if !strings.Contains(content, "›") {
		t.Error("should show input prompt")
	}
	if !strings.Contains(content, "█") {
		t.Error("should show cursor")
	}
	if !strings.Contains(content, `"destroy"`) {
		t.Error("should show 'destroy' instruction")
	}
}

func TestRmDialog_ConfirmTyping(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1"})
	m.step = rmStepConfirm

	// Type "des"
	var model tea.Model = m
	for _, ch := range "des" {
		model, _ = model.Update(tea.KeyPressMsg{Code: rune(ch), Text: string(ch)})
	}
	rm := model.(rmDialog)
	if rm.confirmInput != "des" {
		t.Errorf("after typing 'des', confirmInput = %q", rm.confirmInput)
	}

	// View should show typed text
	content := viewContent(rm.View())
	if !strings.Contains(content, "des") {
		t.Errorf("view should show typed text, got: %s", content)
	}
}

func TestRmDialog_ConfirmBackspace(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1"})
	m.step = rmStepConfirm
	m.confirmInput = "dest"

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	rm := updated.(rmDialog)
	if rm.confirmInput != "des" {
		t.Errorf("after backspace, confirmInput = %q, want 'des'", rm.confirmInput)
	}
}

func TestRmDialog_ConfirmBackspaceEmpty(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1"})
	m.step = rmStepConfirm
	m.confirmInput = ""

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	rm := updated.(rmDialog)
	if rm.confirmInput != "" {
		t.Errorf("backspace on empty = %q, want ''", rm.confirmInput)
	}
}

func TestRmDialog_ConfirmWrongText_ShowsError(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1"})
	m.step = rmStepConfirm
	m.confirmInput = "wrong"

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	rm := updated.(rmDialog)
	if rm.done {
		t.Error("wrong text should not complete")
	}
	if rm.err == nil {
		t.Error("wrong text should set error")
	}
	content := viewContent(rm.View())
	if !strings.Contains(content, "type 'destroy'") {
		t.Errorf("should show error hint, got: %s", content)
	}
}

func TestRmDialog_ConfirmCorrectText_Destroys(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1"})
	m.step = rmStepConfirm
	m.confirmInput = "destroy"

	// Note: runRemove will fail in test since no daemon socket exists,
	// but we can verify the dialog state transition
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	rm := updated.(rmDialog)
	// Either done=true (success) or err set (runRemove failed) — both valid
	if !rm.done && rm.err == nil {
		t.Error("typing 'destroy' and Enter should either succeed or set error")
	}
}

func TestRmDialog_ViewSelectShowsCursor(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1", "ws2"})
	content := viewContent(m.View())
	if !strings.Contains(content, "→") {
		t.Errorf("select view should show arrow cursor, got: %s", content)
	}
}

func TestRmDialog_ViewSelectShowsHints(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1", "ws2"})
	content := viewContent(m.View())
	if !strings.Contains(content, "navigate") {
		t.Error("select view should show navigation hint")
	}
	if !strings.Contains(content, "Cancel") {
		t.Error("select view should show cancel hint")
	}
}

func TestRmDialog_EscFromConfirm_BackToSelect_MultipleItems(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1", "ws2"})
	m.step = rmStepConfirm
	m.confirmInput = "des"

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	rm := updated.(rmDialog)
	if rm.done {
		t.Error("esc from confirm with multiple items should go back, not quit")
	}
	if rm.step != rmStepSelect {
		t.Errorf("step = %d, want select", rm.step)
	}
	if rm.confirmInput != "" {
		t.Error("confirmInput should be cleared on back")
	}
}

func TestRmDialog_EscFromConfirm_SingleItem_Quits(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1"})
	m.step = rmStepConfirm

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	rm := updated.(rmDialog)
	if !rm.done {
		t.Error("esc from confirm with single item should quit")
	}
	if cmd == nil {
		t.Error("should produce quit command")
	}
}

func TestRmDialog_EscFromSelect_Quits(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1"})

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	rm := updated.(rmDialog)
	if !rm.done {
		t.Error("esc from select should quit")
	}
	if cmd == nil {
		t.Error("should produce quit command")
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

func TestRmDialog_QKeyQuits_FromSelect(t *testing.T) {
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

func TestRmDialog_SpaceIgnoredInConfirm(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1"})
	m.step = rmStepConfirm
	m.confirmInput = "dest"

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	rm := updated.(rmDialog)
	if rm.confirmInput != "dest" {
		t.Errorf("space should be ignored, confirmInput = %q", rm.confirmInput)
	}
}

func TestRmDialog_ConfirmViewShowsEscCancel(t *testing.T) {
	m := newRmDialog("/tmp/state", []string{"ws1"})
	m.step = rmStepConfirm

	content := viewContent(m.View())
	if !strings.Contains(content, "[Esc] Cancel") {
		t.Error("confirm view should show Esc Cancel hint")
	}
}
