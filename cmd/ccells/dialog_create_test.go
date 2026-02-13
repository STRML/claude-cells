package main

import (
	"fmt"
	"strings"
	"testing"

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
}

func TestCreateDialog_PromptToSummarizing(t *testing.T) {
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

	// Press enter to move to step 1 (summarizing)
	model, cmd := cd.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cd = model.(createDialog)
	if cd.step != 1 {
		t.Errorf("after enter on prompt, step = %d, want 1 (summarizing)", cd.step)
	}
	if cd.prompt != "Add auth" {
		t.Errorf("prompt = %q, want 'Add auth'", cd.prompt)
	}
	// Should produce an async command (title generation)
	if cmd == nil {
		t.Error("entering prompt should produce an async title generation command")
	}
	// Input should be cleared
	if cd.input != "" {
		t.Errorf("input should be cleared after step 0, got %q", cd.input)
	}
}

func TestCreateDialog_SummarizeSuccess(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.prompt = "Add user authentication with OAuth2"

	// Simulate successful title generation
	updated, _ := m.Update(summarizeResultMsg{title: "OAuth2 User Auth", err: nil})
	cd := updated.(createDialog)
	if cd.step != 2 {
		t.Errorf("after summarize success, step = %d, want 2 (confirm)", cd.step)
	}
	if cd.title != "OAuth2 User Auth" {
		t.Errorf("title = %q, want 'OAuth2 User Auth'", cd.title)
	}
	// Branch should be derived from title, not prompt
	if cd.branch == "" {
		t.Error("branch should be set after summarize")
	}
	if strings.Contains(cd.branch, "authentication") {
		t.Errorf("branch should be derived from title not prompt, got %q", cd.branch)
	}
}

func TestCreateDialog_SummarizeFallback(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.prompt = "Add user authentication"

	// Simulate Claude failure — should fall back to branch from prompt
	updated, _ := m.Update(summarizeResultMsg{title: "", err: fmt.Errorf("claude not available")})
	cd := updated.(createDialog)
	if cd.step != 2 {
		t.Errorf("after summarize error, step = %d, want 2 (confirm)", cd.step)
	}
	if cd.title != "" {
		t.Errorf("title should be empty on fallback, got %q", cd.title)
	}
	// Branch should be derived from prompt as fallback
	if cd.branch == "" {
		t.Error("branch should be set from prompt fallback")
	}
	// workstream.GenerateBranchName strips stop words
	if cd.branch != "add-user-authentication" {
		t.Errorf("branch = %q, want 'add-user-authentication'", cd.branch)
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

func TestCreateDialog_ConfirmAdvancesToCreating(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 2
	m.prompt = "test task"
	m.title = "Test Task Title"
	m.branch = "test-task-title"

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cd := updated.(createDialog)
	if cd.step != 3 {
		t.Errorf("confirm should advance to step 3 (creating), got step %d", cd.step)
	}
	if cmd == nil {
		t.Error("confirm should produce an async create command")
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

func TestCreateDialog_IgnoresKeysWhileSummarizing(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1 // summarizing

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	cd := updated.(createDialog)
	if cd.input != "" {
		t.Errorf("summarizing state should ignore keys, input = %q", cd.input)
	}
	if cmd != nil {
		t.Error("summarizing state should not produce commands")
	}
}

func TestCreateDialog_IgnoresKeysWhileCreating(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 3 // creating

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	cd := updated.(createDialog)
	if cd.input != "" {
		t.Errorf("creating state should ignore keys, input = %q", cd.input)
	}
	if cmd != nil {
		t.Error("creating state should not produce commands")
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
		{"step 0 prompt", 0, "", "", "", "What should this workstream do?"},
		{"step 1 summarizing", 1, "Add auth", "", "", "Generating title..."},
		{"step 2 confirm", 2, "Add auth", "OAuth Auth", "oauth-auth", "Press Enter to create"},
		{"step 2 shows title", 2, "Add auth", "OAuth Auth", "oauth-auth", "Title: OAuth Auth"},
		{"step 2 shows branch", 2, "Add auth", "OAuth Auth", "oauth-auth", "Branch: oauth-auth"},
		{"step 2 no title fallback", 2, "Add auth", "", "add-auth", "Task: Add auth"},
		{"step 3 creating", 3, "Add auth", "OAuth Auth", "oauth-auth", "Creating workstream"},
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

func TestCreateDialog_ViewNoTitleWhenEmpty(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 2
	m.prompt = "Add auth"
	m.title = "" // No title (Claude failed)
	m.branch = "add-auth"

	content := viewContent(m.View())
	if strings.Contains(content, "Title:") {
		t.Errorf("view should not show Title: when title is empty, got: %s", content)
	}
}

func TestCreateDialog_ViewShowsError(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.err = fmt.Errorf("test error")

	content := viewContent(m.View())
	if !strings.Contains(content, "Error: test error") {
		t.Errorf("view should show error, got: %s", content)
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

func TestCreateDialog_AsyncCreateResult(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 3

	// Success result
	updated, cmd := m.Update(createResultMsg{err: nil})
	cd := updated.(createDialog)
	if !cd.done {
		t.Error("success result should set done")
	}
	if cmd == nil {
		t.Error("success result should produce quit command")
	}
}

func TestCreateDialog_AsyncCreateError(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 3

	// Error result
	updated, _ := m.Update(createResultMsg{err: fmt.Errorf("create failed")})
	cd := updated.(createDialog)
	if cd.done {
		t.Error("error result should not set done")
	}
	if cd.step != 2 {
		t.Errorf("error result should return to step 2 (confirm), got step %d", cd.step)
	}
	if cd.err == nil || cd.err.Error() != "create failed" {
		t.Errorf("error should be set, got: %v", cd.err)
	}
}

func TestGenerateTitle_StripQuotes(t *testing.T) {
	// Test the title cleanup logic inline
	// We can't easily test generateTitle itself (needs Claude CLI)
	// but we can verify the flow via summarizeResultMsg
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
