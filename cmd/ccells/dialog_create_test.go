package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestCreateDialog_Init(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	cmd := m.Init()
	if cmd != nil {
		t.Error("Init() should return nil")
	}
	if m.step != 0 {
		t.Errorf("initial step = %d, want 0", m.step)
	}
	if m.frame != 0 {
		t.Errorf("initial frame = %d, want 0", m.frame)
	}
}

func TestCreateDialog_PromptToGenerating(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")

	// Type a prompt
	var model tea.Model = m
	for _, ch := range "Add auth" {
		s := string(ch)
		if s == " " {
			model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeySpace})
		} else {
			model, _ = model.Update(tea.KeyPressMsg{Code: rune(ch), Text: s})
		}
	}
	cd := model.(createDialog)
	if cd.input != "Add auth" {
		t.Errorf("after typing, input = %q, want 'Add auth'", cd.input)
	}

	// Press enter to move to step 1 (generating)
	model, cmd := cd.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cd = model.(createDialog)
	if cd.step != 1 {
		t.Errorf("after enter on prompt, step = %d, want 1 (generating)", cd.step)
	}
	if cd.prompt != "Add auth" {
		t.Errorf("prompt = %q, want 'Add auth'", cd.prompt)
	}
	// Should produce an async command (title generation + spinner tick)
	if cmd == nil {
		t.Error("entering prompt should produce a batch command")
	}
	// Input should be cleared
	if cd.input != "" {
		t.Errorf("input should be cleared after step 0, got %q", cd.input)
	}
	// Frame should be reset
	if cd.frame != 0 {
		t.Errorf("frame should be 0 at start of generating, got %d", cd.frame)
	}
}

func TestCreateDialog_SummarizeAutoCreates(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.prompt = "Add user authentication with OAuth2"

	// Simulate successful title generation — should auto-advance to step 2 (creating)
	updated, cmd := m.Update(summarizeResultMsg{title: "OAuth2 User Auth", err: nil})
	cd := updated.(createDialog)
	if cd.step != 2 {
		t.Errorf("after summarize success, step = %d, want 2 (creating)", cd.step)
	}
	if cd.title != "OAuth2 User Auth" {
		t.Errorf("title = %q, want 'OAuth2 User Auth'", cd.title)
	}
	if cd.branch == "" {
		t.Error("branch should be set after summarize")
	}
	// Should produce a batch command (create + spinner tick)
	if cmd == nil {
		t.Error("summarize success should produce async create command")
	}
}

func TestCreateDialog_SummarizeFallback(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.prompt = "Add user authentication"

	// Simulate Claude failure — should fall back to branch from prompt and auto-create
	updated, cmd := m.Update(summarizeResultMsg{title: "", err: fmt.Errorf("claude not available")})
	cd := updated.(createDialog)
	if cd.step != 2 {
		t.Errorf("after summarize error, step = %d, want 2 (creating)", cd.step)
	}
	if cd.title != "" {
		t.Errorf("title should be empty on fallback, got %q", cd.title)
	}
	if cd.branch == "" {
		t.Error("branch should be set from prompt fallback")
	}
	// workstream.GenerateBranchName strips stop words
	if cd.branch != "add-user-authentication" {
		t.Errorf("branch = %q, want 'add-user-authentication'", cd.branch)
	}
	// Should still produce create command
	if cmd == nil {
		t.Error("summarize fallback should produce async create command")
	}
}

func TestCreateDialog_EmptyPromptRejected(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")

	// Press enter with empty prompt
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cd := updated.(createDialog)
	if cd.step != 0 {
		t.Errorf("empty prompt should stay at step 0, got step %d", cd.step)
	}
	if cd.err == nil {
		t.Error("empty prompt should set error")
	}
}

func TestCreateDialog_CreateSuccess(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 2

	updated, cmd := m.Update(createResultMsg{err: nil})
	cd := updated.(createDialog)
	if !cd.done {
		t.Error("success result should set done")
	}
	if cmd == nil {
		t.Error("success result should produce quit command")
	}
}

func TestCreateDialog_CreateError(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 2
	m.prompt = "test task"

	updated, _ := m.Update(createResultMsg{err: fmt.Errorf("create failed")})
	cd := updated.(createDialog)
	if cd.done {
		t.Error("error result should not set done")
	}
	if cd.step != 0 {
		t.Errorf("error result should return to step 0 (prompt), got step %d", cd.step)
	}
	if cd.err == nil || cd.err.Error() != "create failed" {
		t.Errorf("error should be set, got: %v", cd.err)
	}
	// Input should be restored from prompt for retry
	if cd.input != "test task" {
		t.Errorf("input should be restored from prompt for retry, got %q", cd.input)
	}
}

func TestCreateDialog_SpinnerTick(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1 // generating
	m.frame = 0

	updated, cmd := m.Update(tickMsg(time.Now()))
	cd := updated.(createDialog)
	if cd.frame != 1 {
		t.Errorf("after tick, frame = %d, want 1", cd.frame)
	}
	// Should produce another tick
	if cmd == nil {
		t.Error("tick at step 1 should produce another tick command")
	}
}

func TestCreateDialog_SpinnerTickIgnoredAtStep0(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 0 // prompt
	m.frame = 0

	updated, cmd := m.Update(tickMsg(time.Now()))
	cd := updated.(createDialog)
	if cd.frame != 0 {
		t.Errorf("tick at step 0 should not advance frame, got %d", cd.frame)
	}
	if cmd != nil {
		t.Error("tick at step 0 should not produce another tick")
	}
}

func TestCreateDialog_Backspace(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.input = "hello"

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	cd := updated.(createDialog)
	if cd.input != "hell" {
		t.Errorf("after backspace, input = %q, want 'hell'", cd.input)
	}
}

func TestCreateDialog_BackspaceEmpty(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.input = ""

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	cd := updated.(createDialog)
	if cd.input != "" {
		t.Errorf("backspace on empty input = %q, want ''", cd.input)
	}
}

func TestCreateDialog_SpaceKey(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.input = "hello"

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	cd := updated.(createDialog)
	if cd.input != "hello " {
		t.Errorf("after space, input = %q, want 'hello '", cd.input)
	}
}

func TestCreateDialog_IgnoresKeysWhileGenerating(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1 // generating

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	cd := updated.(createDialog)
	if cd.input != "" {
		t.Errorf("generating state should ignore keys, input = %q", cd.input)
	}
	if cmd != nil {
		t.Error("generating state should not produce commands for regular keys")
	}
}

func TestCreateDialog_IgnoresKeysWhileCreating(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 2 // creating

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	cd := updated.(createDialog)
	if cd.input != "" {
		t.Errorf("creating state should ignore keys, input = %q", cd.input)
	}
	if cmd != nil {
		t.Error("creating state should not produce commands for regular keys")
	}
}

func TestCreateDialog_EscQuitsDuringGenerating(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1 // generating

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	cd := updated.(createDialog)
	if !cd.done {
		t.Error("esc during generating should set done")
	}
	if cmd == nil {
		t.Error("esc during generating should produce quit command")
	}
}

func TestCreateDialog_EscQuits(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	cd := updated.(createDialog)
	if !cd.done {
		t.Error("esc should set done")
	}
	if cmd == nil {
		t.Error("esc should produce a quit command")
	}
}

func TestCreateDialog_CtrlCQuits(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	cd := updated.(createDialog)
	if !cd.done {
		t.Error("ctrl+c should set done")
	}
	if cmd == nil {
		t.Error("ctrl+c should produce a quit command")
	}
}

func TestCreateDialog_ViewSteps(t *testing.T) {
	tests := []struct {
		name    string
		step    int
		prompt  string
		title   string
		branch  string
		wantStr string
	}{
		{"step 0 prompt", 0, "", "", "", "Describe your task"},
		{"step 0 cursor", 0, "", "", "", "›"},
		{"step 1 generating", 1, "Add auth", "", "", "Generating title"},
		{"step 1 shows task", 1, "Add auth", "", "", "Add auth"},
		{"step 2 creating", 2, "Add auth", "OAuth Auth", "oauth-auth", "Creating workstream"},
		{"step 2 shows title", 2, "Add auth", "OAuth Auth", "oauth-auth", "OAuth Auth"},
		{"step 2 shows branch", 2, "Add auth", "OAuth Auth", "oauth-auth", "oauth-auth"},
		{"step 2 no title fallback", 2, "Add auth", "", "add-auth", "Add auth"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newCreateDialog("/tmp/state", "claude")
			m.step = tt.step
			m.prompt = tt.prompt
			m.title = tt.title
			m.branch = tt.branch

			content := viewContent(m.View())
			if !strings.Contains(content, tt.wantStr) {
				t.Errorf("step %d view should contain %q, got: %s", tt.step, tt.wantStr, content)
			}
		})
	}
}

func TestCreateDialog_ViewHeader(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	content := viewContent(m.View())
	if !strings.Contains(content, "N E W   W O R K S T R E A M") {
		t.Errorf("view should contain spaced header, got: %s", content)
	}
	if !strings.Contains(content, "━") {
		t.Errorf("view should contain heavy divider, got: %s", content)
	}
}

func TestCreateDialog_ViewSpinner(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.prompt = "test"
	m.frame = 0

	content := viewContent(m.View())
	if !strings.Contains(content, spinnerFrames[0]) {
		t.Errorf("view should contain spinner frame, got: %s", content)
	}

	// Different frame
	m.frame = 3
	content = viewContent(m.View())
	if !strings.Contains(content, spinnerFrames[3]) {
		t.Errorf("view should contain spinner frame 3, got: %s", content)
	}
}

func TestCreateDialog_ViewNoTitleWhenEmpty(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 2
	m.prompt = "Add auth"
	m.title = "" // No title (Claude failed)
	m.branch = "add-auth"

	content := viewContent(m.View())
	if strings.Contains(content, "Title") {
		t.Errorf("view should not show Title when title is empty, got: %s", content)
	}
}

func TestCreateDialog_ViewShowsError(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.err = fmt.Errorf("test error")

	content := viewContent(m.View())
	if !strings.Contains(content, "test error") {
		t.Errorf("view should show error, got: %s", content)
	}
	// Error should have ✗ marker
	if !strings.Contains(content, "✗") {
		t.Errorf("view should show error marker ✗, got: %s", content)
	}
}

func TestCreateDialog_DoneViewEmpty(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.done = true
	content := viewContent(m.View())
	if content != "" {
		t.Errorf("done view should be empty, got: %q", content)
	}
}

func TestGenerateTitle_StripQuotes(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.prompt = "test"

	// Title with quotes should still work
	updated, _ := m.Update(summarizeResultMsg{title: "Add OAuth Login", err: nil})
	cd := updated.(createDialog)
	if cd.title != "Add OAuth Login" {
		t.Errorf("title = %q, want 'Add OAuth Login'", cd.title)
	}
}

func TestCreateDialog_SpinnerFrameWraps(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.frame = len(spinnerFrames) - 1 // last frame

	updated, _ := m.Update(tickMsg(time.Now()))
	cd := updated.(createDialog)
	if cd.frame != 0 {
		t.Errorf("spinner should wrap to frame 0, got %d", cd.frame)
	}
}
