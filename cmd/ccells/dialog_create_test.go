package main

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestGenerateBranchName(t *testing.T) {
	tests := []struct {
		prompt string
		want   string
	}{
		{"Add user authentication", "add-user-authentication"},
		{"Fix the login bug in auth module", "fix-the-login-bug"},
		{"", "workstream"},
		{"Simple", "simple"},
		{"Use special chars! @#$", "use-special-chars"},
		{"UPPERCASE words", "uppercase-words"},
	}
	for _, tt := range tests {
		t.Run(tt.prompt, func(t *testing.T) {
			got := generateBranchName(tt.prompt)
			if got != tt.want {
				t.Errorf("generateBranchName(%q) = %q, want %q", tt.prompt, got, tt.want)
			}
		})
	}
}

func TestValidateBranchName(t *testing.T) {
	tests := []struct {
		name    string
		branch  string
		wantErr bool
		errMsg  string
	}{
		{"valid simple", "my-branch", false, ""},
		{"valid with slash", "feat/auth", false, ""},
		{"valid with dots", "release.1.0", false, ""},
		{"valid with underscore", "my_branch", false, ""},
		{"valid mixed", "feat/my-branch_v2.0", false, ""},
		{"empty", "", true, "cannot be empty"},
		{"too long", strings.Repeat("a", 201), true, "too long"},
		{"shell metachar semicolon", "my;branch", true, "invalid character"},
		{"shell metachar pipe", "my|branch", true, "invalid character"},
		{"shell metachar ampersand", "my&branch", true, "invalid character"},
		{"shell metachar backtick", "my`branch", true, "invalid character"},
		{"shell metachar dollar", "my$branch", true, "invalid character"},
		{"space", "my branch", true, "invalid character"},
		{"double dots", "my..branch", true, "invalid sequence"},
		{"double slashes", "my//branch", true, "invalid sequence"},
		{"starts with slash", "/my-branch", true, "start or end"},
		{"ends with slash", "my-branch/", true, "start or end"},
		{"starts with dash", "-my-branch", true, "start with '-'"},
		{"ends with .lock", "my-branch.lock", true, ".lock"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBranchName(tt.branch)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateBranchName(%q) error = %v, wantErr %v", tt.branch, err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("validateBranchName(%q) error = %q, want to contain %q", tt.branch, err, tt.errMsg)
			}
		})
	}
}

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

func TestCreateDialog_StepTransitions(t *testing.T) {
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

	// Press enter to move to step 1 (branch)
	model, _ = cd.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cd = model.(createDialog)
	if cd.step != 1 {
		t.Errorf("after enter on prompt, step = %d, want 1", cd.step)
	}
	if cd.prompt != "Add auth" {
		t.Errorf("prompt = %q, want 'Add auth'", cd.prompt)
	}
	// Branch should be auto-generated
	if cd.branch != "add-auth" {
		t.Errorf("branch = %q, want 'add-auth'", cd.branch)
	}
	if cd.input != "add-auth" {
		t.Errorf("input should be branch name, got %q", cd.input)
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

func TestCreateDialog_EmptyBranchRejected(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	// Set up at step 1 with empty branch input
	m.step = 1
	m.prompt = "test"
	m.input = ""

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cd := updated.(createDialog)
	if cd.step != 1 {
		t.Errorf("empty branch should stay at step 1, got step %d", cd.step)
	}
	if cd.err == nil {
		t.Error("empty branch should set error")
	}
}

func TestCreateDialog_InvalidBranchRejected(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.prompt = "test"
	m.input = "my;branch"

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cd := updated.(createDialog)
	if cd.step != 1 {
		t.Errorf("invalid branch should stay at step 1, got step %d", cd.step)
	}
	if cd.err == nil {
		t.Error("invalid branch should set error")
	}
}

func TestCreateDialog_ValidBranchAdvances(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.prompt = "test"
	m.input = "valid-branch"

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cd := updated.(createDialog)
	if cd.step != 2 {
		t.Errorf("valid branch should advance to step 2, got step %d", cd.step)
	}
	if cd.err != nil {
		t.Errorf("valid branch should not set error: %v", cd.err)
	}
	if cd.branch != "valid-branch" {
		t.Errorf("branch = %q, want 'valid-branch'", cd.branch)
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

	// Space key in Bubble Tea v2 returns "space" from msg.String()
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	cd := updated.(createDialog)
	if cd.input != "hello " {
		t.Errorf("after space, input = %q, want 'hello '", cd.input)
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

func TestCreateDialog_IgnoresKeysWhileCreating(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 3 // creating state

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	cd := updated.(createDialog)
	if cd.input != "" {
		t.Errorf("creating state should ignore keys, input = %q", cd.input)
	}
	if cmd != nil {
		t.Error("creating state should not produce commands")
	}
}

func TestCreateDialog_ViewSteps(t *testing.T) {
	tests := []struct {
		name    string
		step    int
		prompt  string
		branch  string
		wantStr string
	}{
		{"step 0", 0, "", "", "What should this workstream do?"},
		{"step 1", 1, "Add auth", "", "Branch name:"},
		{"step 2", 2, "Add auth", "add-auth", "Press Enter to create"},
		{"step 3", 3, "Add auth", "add-auth", "Creating workstream"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newCreateDialog("/tmp/state", "claude")
			m.step = tt.step
			m.prompt = tt.prompt
			m.branch = tt.branch

			content := viewContent(m.View())
			if !strings.Contains(content, tt.wantStr) {
				t.Errorf("step %d view should contain %q, got: %s", tt.step, tt.wantStr, content)
			}
		})
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
		t.Errorf("error result should return to step 2, got step %d", cd.step)
	}
	if cd.err == nil || cd.err.Error() != "create failed" {
		t.Errorf("error should be set, got: %v", cd.err)
	}
}
