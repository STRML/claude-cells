package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func testWorkstreams() []mergeWorkstream {
	return []mergeWorkstream{
		{BranchName: "feature-auth", Prompt: "Add authentication", PRNumber: 0, HasPR: false},
		{BranchName: "fix-bug-123", Prompt: "Fix the login bug", PRNumber: 42, PRURL: "https://github.com/org/repo/pull/42", HasPR: true},
		{BranchName: "add-tests", Prompt: "Add unit tests", PRNumber: 0, HasPR: false},
	}
}

func testMergeDialog(items []mergeWorkstream) mergeDialog {
	m := newMergeDialog(items, "/tmp/test-repo")
	// Override with test stubs
	m.createPRFn = func(ctx context.Context, branch, prompt string) (string, error) {
		return "https://github.com/org/repo/pull/99", nil
	}
	m.mergePRFn = func(ctx context.Context, method string) error {
		return nil
	}
	m.pushFn = func(ctx context.Context, branch string) error {
		return nil
	}
	return m
}

func TestMergeDialog_Init(t *testing.T) {
	m := testMergeDialog(testWorkstreams())
	cmd := m.Init()
	if cmd != nil {
		t.Error("Init() should return nil")
	}
}

func TestMergeDialog_EmptyState(t *testing.T) {
	m := testMergeDialog(nil)
	content := viewContent(m.View())
	if !strings.Contains(content, "No workstreams available") {
		t.Errorf("empty state should show 'No workstreams available', got: %s", content)
	}
	if !strings.Contains(content, "Esc to close") {
		t.Error("empty state should show dismiss hint")
	}
}

func TestMergeDialog_EmptyState_EnterIgnored(t *testing.T) {
	m := testMergeDialog(nil)
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
	m := testMergeDialog(testWorkstreams())

	if m.selected != 0 {
		t.Errorf("initial selected = %d, want 0", m.selected)
	}

	// Move down
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	md := updated.(mergeDialog)
	if md.selected != 1 {
		t.Errorf("after down, selected = %d, want 1", md.selected)
	}

	// Move down again
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
	m := testMergeDialog(testWorkstreams())

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

func TestMergeDialog_SelectNoPR_ShowsCreateAction(t *testing.T) {
	m := testMergeDialog(testWorkstreams())
	// Select first item (no PR)
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	md := updated.(mergeDialog)

	if md.step != mergeStepAction {
		t.Errorf("after enter, step = %d, want %d (action)", md.step, mergeStepAction)
	}
	if len(md.actions) != 1 || md.actions[0] != "Create PR" {
		t.Errorf("no-PR actions = %v, want [Create PR]", md.actions)
	}

	content := viewContent(md.View())
	if !strings.Contains(content, "Create PR") {
		t.Errorf("action view should show 'Create PR', got: %s", content)
	}
	if !strings.Contains(content, "feature-auth") {
		t.Errorf("action view should show branch name, got: %s", content)
	}
}

func TestMergeDialog_SelectWithPR_ShowsMergeActions(t *testing.T) {
	m := testMergeDialog(testWorkstreams())
	// Navigate to second item (has PR)
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	md := updated.(mergeDialog)
	updated, _ = md.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	md = updated.(mergeDialog)

	if md.step != mergeStepAction {
		t.Errorf("step = %d, want %d (action)", md.step, mergeStepAction)
	}
	if len(md.actions) != 2 {
		t.Fatalf("PR actions = %v, want 2 items", md.actions)
	}
	if md.actions[0] != "Merge PR" {
		t.Errorf("first action = %q, want 'Merge PR'", md.actions[0])
	}
	if md.actions[1] != "View in browser" {
		t.Errorf("second action = %q, want 'View in browser'", md.actions[1])
	}

	content := viewContent(md.View())
	if !strings.Contains(content, "PR #42") {
		t.Errorf("action view should show PR number, got: %s", content)
	}
}

func TestMergeDialog_MergeAction_ShowsMethodSelection(t *testing.T) {
	m := testMergeDialog(testWorkstreams())
	// Navigate to PR workstream and select
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	md := updated.(mergeDialog)
	updated, _ = md.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	md = updated.(mergeDialog)
	// Select "Merge PR"
	updated, _ = md.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	md = updated.(mergeDialog)

	if md.step != mergeStepMethod {
		t.Errorf("step = %d, want %d (method)", md.step, mergeStepMethod)
	}

	content := viewContent(md.View())
	if !strings.Contains(content, "Squash merge") {
		t.Errorf("method view should show 'Squash merge', got: %s", content)
	}
	if !strings.Contains(content, "Merge commit") {
		t.Errorf("method view should show 'Merge commit', got: %s", content)
	}
	if !strings.Contains(content, "Rebase") {
		t.Errorf("method view should show 'Rebase', got: %s", content)
	}
}

func TestMergeDialog_MethodNavigation(t *testing.T) {
	m := testMergeDialog(testWorkstreams())
	// Navigate to method step
	m.step = mergeStepMethod
	m.items = testWorkstreams()
	m.selected = 1 // PR workstream
	m.methods = []string{"Squash merge", "Merge commit", "Rebase"}

	// Move down
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	md := updated.(mergeDialog)
	if md.methodIdx != 1 {
		t.Errorf("methodIdx = %d, want 1", md.methodIdx)
	}

	// Move down again
	updated, _ = md.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	md = updated.(mergeDialog)
	if md.methodIdx != 2 {
		t.Errorf("methodIdx = %d, want 2", md.methodIdx)
	}

	// At bottom, stays
	updated, _ = md.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	md = updated.(mergeDialog)
	if md.methodIdx != 2 {
		t.Errorf("at bottom, methodIdx = %d, want 2", md.methodIdx)
	}
}

func TestMergeDialog_CreatePR_StartesWorking(t *testing.T) {
	m := testMergeDialog(testWorkstreams())
	// Select no-PR workstream
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	md := updated.(mergeDialog)
	// Select "Create PR"
	updated, cmd := md.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	md = updated.(mergeDialog)

	if md.step != mergeStepWorking {
		t.Errorf("step = %d, want %d (working)", md.step, mergeStepWorking)
	}
	if cmd == nil {
		t.Error("expected batch command for spinner + async work")
	}
	if !strings.Contains(md.workingMsg, "creating PR") {
		t.Errorf("workingMsg = %q, should mention creating PR", md.workingMsg)
	}
}

func TestMergeDialog_MergeResult_Success(t *testing.T) {
	m := testMergeDialog(testWorkstreams())
	m.step = mergeStepWorking

	updated, _ := m.Update(mergeResultMsg{message: "PR merged via squash"})
	md := updated.(mergeDialog)

	if md.step != mergeStepResult {
		t.Errorf("step = %d, want %d (result)", md.step, mergeStepResult)
	}
	if md.result != "PR merged via squash" {
		t.Errorf("result = %q, want 'PR merged via squash'", md.result)
	}

	content := viewContent(md.View())
	if !strings.Contains(content, "PR merged via squash") {
		t.Errorf("result view should show success, got: %s", content)
	}
}

func TestMergeDialog_MergeResult_Error(t *testing.T) {
	m := testMergeDialog(testWorkstreams())
	m.step = mergeStepWorking

	updated, _ := m.Update(mergeResultMsg{err: fmt.Errorf("merge conflict")})
	md := updated.(mergeDialog)

	if md.step != mergeStepSelect {
		t.Errorf("on error, step = %d, want %d (select)", md.step, mergeStepSelect)
	}
	if md.err == nil {
		t.Error("expected error to be set")
	}

	content := viewContent(md.View())
	if !strings.Contains(content, "merge conflict") {
		t.Errorf("should show error, got: %s", content)
	}
}

func TestMergeDialog_EscBack_FromAction(t *testing.T) {
	m := testMergeDialog(testWorkstreams())
	m.step = mergeStepAction
	m.actions = []string{"Create PR"}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	md := updated.(mergeDialog)

	if md.step != mergeStepSelect {
		t.Errorf("esc from action: step = %d, want %d (select)", md.step, mergeStepSelect)
	}
	if md.done {
		t.Error("esc from action should not quit")
	}
}

func TestMergeDialog_EscBack_FromMethod(t *testing.T) {
	m := testMergeDialog(testWorkstreams())
	m.step = mergeStepMethod
	m.actions = []string{"Merge PR"}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	md := updated.(mergeDialog)

	if md.step != mergeStepAction {
		t.Errorf("esc from method: step = %d, want %d (action)", md.step, mergeStepAction)
	}
}

func TestMergeDialog_EscQuits_FromSelect(t *testing.T) {
	m := testMergeDialog(testWorkstreams())

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	md := updated.(mergeDialog)
	if !md.done {
		t.Error("esc from select should set done")
	}
	if cmd == nil {
		t.Error("esc from select should produce quit command")
	}
}

func TestMergeDialog_CtrlCQuits(t *testing.T) {
	m := testMergeDialog(testWorkstreams())
	m.step = mergeStepAction

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	md := updated.(mergeDialog)
	if !md.done {
		t.Error("ctrl+c should set done")
	}
	if cmd == nil {
		t.Error("ctrl+c should produce quit command")
	}
}

func TestMergeDialog_QKeyQuits_FromSelect(t *testing.T) {
	m := testMergeDialog(testWorkstreams())

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	md := updated.(mergeDialog)
	if !md.done {
		t.Error("q from select should set done")
	}
	if cmd == nil {
		t.Error("q from select should produce quit command")
	}
}

func TestMergeDialog_QKeyBack_FromAction(t *testing.T) {
	m := testMergeDialog(testWorkstreams())
	m.step = mergeStepAction
	m.actions = []string{"Create PR"}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	md := updated.(mergeDialog)
	if md.done {
		t.Error("q from action should not quit, should go back")
	}
	if md.step != mergeStepSelect {
		t.Errorf("q from action: step = %d, want %d (select)", md.step, mergeStepSelect)
	}
}

func TestMergeDialog_DoneViewEmpty(t *testing.T) {
	m := testMergeDialog(testWorkstreams())
	m.done = true
	content := viewContent(m.View())
	if content != "" {
		t.Errorf("done view should be empty, got: %q", content)
	}
}

func TestMergeDialog_UpAtTop(t *testing.T) {
	m := testMergeDialog(testWorkstreams()[:1])

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	md := updated.(mergeDialog)
	if md.selected != 0 {
		t.Errorf("up at top: selected = %d, want 0", md.selected)
	}
}

func TestMergeDialog_ViewHeader(t *testing.T) {
	m := testMergeDialog(testWorkstreams())
	content := viewContent(m.View())
	if !strings.Contains(content, "Pull Request") {
		t.Error("view should show title")
	}
}

func TestMergeDialog_ViewSelectShowsPRStatus(t *testing.T) {
	m := testMergeDialog(testWorkstreams())
	content := viewContent(m.View())
	if !strings.Contains(content, "no PR") {
		t.Errorf("should show 'no PR' for first item, got: %s", content)
	}

	// Navigate to PR item
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	md := updated.(mergeDialog)
	content = viewContent(md.View())
	if !strings.Contains(content, "PR#42") {
		t.Errorf("should show PR number for second item, got: %s", content)
	}
}

func TestMergeDialog_SpinnerTick(t *testing.T) {
	m := testMergeDialog(testWorkstreams())
	m.step = mergeStepWorking
	m.workingMsg = "Testing"

	updated, cmd := m.Update(mergeTickMsg(time.Now()))
	md := updated.(mergeDialog)
	if md.frame != 1 {
		t.Errorf("frame = %d, want 1", md.frame)
	}
	if cmd == nil {
		t.Error("tick during working should return tick command")
	}

	content := viewContent(md.View())
	if !strings.Contains(content, "Testing") {
		t.Errorf("working view should show message, got: %s", content)
	}
}

func TestMergeDialog_SpinnerTick_IgnoredOutsideWorking(t *testing.T) {
	m := testMergeDialog(testWorkstreams())
	m.step = mergeStepSelect

	_, cmd := m.Update(mergeTickMsg(time.Now()))
	if cmd != nil {
		t.Error("tick outside working should not return command")
	}
}

func TestMergeDialog_ResultDismiss(t *testing.T) {
	m := testMergeDialog(testWorkstreams())
	m.step = mergeStepResult
	m.result = "Done"

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	md := updated.(mergeDialog)
	if !md.done {
		t.Error("any key at result should set done")
	}
	if cmd == nil {
		t.Error("any key at result should produce quit")
	}
}

func TestMergeDialog_CreatePR_PushError(t *testing.T) {
	m := testMergeDialog(testWorkstreams())
	m.pushFn = func(ctx context.Context, branch string) error {
		return fmt.Errorf("auth required")
	}

	// Select no-PR workstream → action → Create PR
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	md := updated.(mergeDialog)
	updated, _ = md.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	md = updated.(mergeDialog)

	if md.step != mergeStepWorking {
		t.Fatalf("step = %d, want working", md.step)
	}

	// Simulate the async result
	updated, _ = md.Update(mergeResultMsg{err: fmt.Errorf("push failed: auth required")})
	md = updated.(mergeDialog)

	if md.step != mergeStepSelect {
		t.Errorf("on push error, step = %d, want select", md.step)
	}
	if md.err == nil || !strings.Contains(md.err.Error(), "push failed") {
		t.Errorf("expected push error, got: %v", md.err)
	}
}

func TestMergeDialog_ViewBrowser_PrintsURL(t *testing.T) {
	items := []mergeWorkstream{
		{BranchName: "fix-bug", PRNumber: 42, PRURL: "https://github.com/org/repo/pull/42", HasPR: true},
	}
	m := testMergeDialog(items)

	// Select → action → "View in browser"
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	md := updated.(mergeDialog)
	// Navigate to "View in browser" (second action)
	updated, _ = md.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	md = updated.(mergeDialog)
	updated, cmd := md.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	md = updated.(mergeDialog)

	if !md.done {
		t.Error("view browser should set done")
	}
	if cmd == nil {
		t.Error("view browser should produce command")
	}
}

func TestMergeDialog_ErrorClearedOnReselect(t *testing.T) {
	m := testMergeDialog(testWorkstreams())
	m.err = fmt.Errorf("previous error")

	// Select a workstream — should clear error
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	md := updated.(mergeDialog)

	if md.err != nil {
		t.Error("selecting workstream should clear previous error")
	}
}
