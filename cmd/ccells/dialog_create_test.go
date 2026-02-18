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

// noUntrackedFiles is a test helper that returns no untracked files.
func noUntrackedFiles() ([]string, error) {
	return nil, nil
}

func TestCreateDialog_SummarizeDispatchesUntrackedCheck(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.prompt = "Add user authentication with OAuth2"
	m.checkUntrackedFn = noUntrackedFiles

	// Simulate successful title generation — should dispatch untracked files check (not auto-create)
	updated, cmd := m.Update(summarizeResultMsg{title: "OAuth2 User Auth", err: nil})
	cd := updated.(createDialog)
	if cd.title != "OAuth2 User Auth" {
		t.Errorf("title = %q, want 'OAuth2 User Auth'", cd.title)
	}
	if cd.branch == "" {
		t.Error("branch should be set after summarize")
	}
	// Step should still be 1 (waiting for untracked check result)
	if cd.step != 1 {
		t.Errorf("after summarize, step = %d, want 1 (still generating, waiting for untracked check)", cd.step)
	}
	// Should produce an async command (untracked files check)
	if cmd == nil {
		t.Error("summarize success should produce async untracked check command")
	}
}

func TestCreateDialog_NoUntrackedAutoCreates(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.prompt = "Add user authentication"
	m.title = "OAuth2 User Auth"
	m.branch = "oauth2-user-auth"

	// No untracked files → should auto-advance to step 2 (creating)
	updated, cmd := m.Update(untrackedFilesMsg{files: nil, err: nil})
	cd := updated.(createDialog)
	if cd.step != 2 {
		t.Errorf("after no untracked files, step = %d, want 2 (creating)", cd.step)
	}
	if cmd == nil {
		t.Error("no untracked files should trigger create command")
	}
}

func TestCreateDialog_UntrackedFilesShowSelection(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.prompt = "Add auth"
	m.title = "OAuth Auth"
	m.branch = "oauth-auth"

	// Untracked files found → should show selection
	updated, cmd := m.Update(untrackedFilesMsg{files: []string{"foo.txt", "bar.go"}, err: nil})
	cd := updated.(createDialog)
	if !cd.showUntracked {
		t.Error("expected showUntracked to be true")
	}
	if cd.step != 1 {
		t.Errorf("step should remain 1 while showing untracked prompt, got %d", cd.step)
	}
	if len(cd.untrackedFiles) != 2 {
		t.Errorf("expected 2 untracked files, got %d", len(cd.untrackedFiles))
	}
	if cd.untrackedIdx != 0 {
		t.Errorf("untrackedIdx should default to 0 (Yes), got %d", cd.untrackedIdx)
	}
	if cmd != nil {
		t.Error("showing untracked selection should not produce a command (waits for user input)")
	}
}

func TestCreateDialog_UntrackedNavigation(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.showUntracked = true
	m.untrackedFiles = []string{"foo.txt"}
	m.untrackedIdx = 0

	// Navigate down to "No"
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	cd := updated.(createDialog)
	if cd.untrackedIdx != 1 {
		t.Errorf("after down, untrackedIdx = %d, want 1", cd.untrackedIdx)
	}

	// At bottom, stays
	updated, _ = cd.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	cd = updated.(createDialog)
	if cd.untrackedIdx != 1 {
		t.Errorf("at bottom, untrackedIdx = %d, want 1", cd.untrackedIdx)
	}

	// Navigate back up to "Yes"
	updated, _ = cd.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	cd = updated.(createDialog)
	if cd.untrackedIdx != 0 {
		t.Errorf("after up, untrackedIdx = %d, want 0", cd.untrackedIdx)
	}

	// At top, stays
	updated, _ = cd.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	cd = updated.(createDialog)
	if cd.untrackedIdx != 0 {
		t.Errorf("at top, untrackedIdx = %d, want 0", cd.untrackedIdx)
	}
}

func TestCreateDialog_UntrackedVimNavigation(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.showUntracked = true
	m.untrackedFiles = []string{"foo.txt"}
	m.untrackedIdx = 0

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	cd := updated.(createDialog)
	if cd.untrackedIdx != 1 {
		t.Errorf("after j, untrackedIdx = %d, want 1", cd.untrackedIdx)
	}

	updated, _ = cd.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	cd = updated.(createDialog)
	if cd.untrackedIdx != 0 {
		t.Errorf("after k, untrackedIdx = %d, want 0", cd.untrackedIdx)
	}
}

func TestCreateDialog_UntrackedEnterSelectsYes(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.prompt = "Add auth"
	m.branch = "add-auth"
	m.showUntracked = true
	m.untrackedFiles = []string{"foo.txt"}
	m.untrackedIdx = 0 // Yes selected

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cd := updated.(createDialog)
	if cd.showUntracked {
		t.Error("showUntracked should be false after Enter")
	}
	if !cd.copyUntracked {
		t.Error("copyUntracked should be true when Yes selected")
	}
	if cd.step != 2 {
		t.Errorf("step = %d, want 2 (creating)", cd.step)
	}
	if cmd == nil {
		t.Error("Enter should produce create command")
	}
}

func TestCreateDialog_UntrackedEnterSelectsNo(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.prompt = "Add auth"
	m.branch = "add-auth"
	m.showUntracked = true
	m.untrackedFiles = []string{"foo.txt"}
	m.untrackedIdx = 1 // No selected

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cd := updated.(createDialog)
	if cd.showUntracked {
		t.Error("showUntracked should be false after Enter")
	}
	if cd.copyUntracked {
		t.Error("copyUntracked should be false when No selected")
	}
	if cd.step != 2 {
		t.Errorf("step = %d, want 2 (creating)", cd.step)
	}
	if cmd == nil {
		t.Error("Enter should produce create command")
	}
}

func TestCreateDialog_UntrackedYShortcut(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.prompt = "Add auth"
	m.branch = "add-auth"
	m.showUntracked = true
	m.untrackedFiles = []string{"foo.txt"}
	m.untrackedIdx = 1 // Even if No is selected, y shortcut picks Yes

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	cd := updated.(createDialog)
	if !cd.copyUntracked {
		t.Error("y shortcut should set copyUntracked=true")
	}
	if cd.step != 2 {
		t.Errorf("step = %d, want 2", cd.step)
	}
	if cmd == nil {
		t.Error("y should produce create command")
	}
}

func TestCreateDialog_UntrackedNShortcut(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.prompt = "Add auth"
	m.branch = "add-auth"
	m.showUntracked = true
	m.untrackedFiles = []string{"foo.txt"}
	m.untrackedIdx = 0 // Even if Yes is selected, n shortcut picks No

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	cd := updated.(createDialog)
	if cd.copyUntracked {
		t.Error("n shortcut should set copyUntracked=false")
	}
	if cd.step != 2 {
		t.Errorf("step = %d, want 2", cd.step)
	}
	if cmd == nil {
		t.Error("n should produce create command")
	}
}

func TestCreateDialog_UntrackedEscQuits(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.showUntracked = true
	m.untrackedFiles = []string{"foo.txt"}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	cd := updated.(createDialog)
	if !cd.done {
		t.Error("esc during untracked selection should set done")
	}
	if cmd == nil {
		t.Error("esc during untracked selection should produce quit command")
	}
}

func TestCreateDialog_UntrackedErrorSkipsPrompt(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.prompt = "Add auth"
	m.title = "Auth"
	m.branch = "auth"

	// Error checking untracked files → should skip prompt and auto-create
	updated, cmd := m.Update(untrackedFilesMsg{files: nil, err: fmt.Errorf("git error")})
	cd := updated.(createDialog)
	if cd.showUntracked {
		t.Error("error should skip the untracked prompt")
	}
	if cd.step != 2 {
		t.Errorf("step = %d, want 2 (creating)", cd.step)
	}
	if cmd == nil {
		t.Error("should produce create command even on error")
	}
}

func TestCreateDialog_ViewUntrackedSelection(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.prompt = "Add auth"
	m.title = "OAuth Auth"
	m.branch = "oauth-auth"
	m.showUntracked = true
	m.untrackedFiles = []string{"file1.txt", "file2.go", "dir/file3.rs"}

	content := viewContent(m.View())
	if !strings.Contains(content, "Copy Untracked Files?") {
		t.Errorf("should show title, got: %s", content)
	}
	if !strings.Contains(content, "Found 3 untracked file(s) in the repository") {
		t.Errorf("should show file count, got: %s", content)
	}
	if !strings.Contains(content, "• file1.txt") {
		t.Errorf("should show files with bullet points, got: %s", content)
	}
	if !strings.Contains(content, "• file2.go") {
		t.Error("should show all files")
	}
	if !strings.Contains(content, "Copy these files to the new worktree?") {
		t.Errorf("should show question, got: %s", content)
	}
	if !strings.Contains(content, "Yes, copy untracked files") {
		t.Errorf("should show Yes option, got: %s", content)
	}
	if !strings.Contains(content, "No, start with clean worktree") {
		t.Errorf("should show No option, got: %s", content)
	}
	if !strings.Contains(content, "→") {
		t.Error("should show arrow cursor on selected item")
	}
	if !strings.Contains(content, "navigate") {
		t.Error("should show navigation hints")
	}
}

func TestCreateDialog_ViewUntrackedTruncatesLongList(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.prompt = "Add auth"
	m.showUntracked = true
	m.branch = "auth"
	m.untrackedFiles = []string{"f1", "f2", "f3", "f4", "f5", "f6", "f7"}

	content := viewContent(m.View())
	// Should show "... and 2 more" (7 - 5 = 2)
	if !strings.Contains(content, "... and 2 more") {
		t.Errorf("view should truncate long file list, got: %s", content)
	}
	// Should NOT show f6 or f7
	if strings.Contains(content, "f6") {
		t.Errorf("view should not show files beyond first 5, got: %s", content)
	}
}

func TestCreateDialog_SummarizeFallback(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.step = 1
	m.prompt = "Add user authentication"
	m.checkUntrackedFn = noUntrackedFiles

	// Simulate Claude failure — should fall back to branch from prompt
	updated, cmd := m.Update(summarizeResultMsg{title: "", err: fmt.Errorf("claude not available")})
	cd := updated.(createDialog)
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
	// Should produce untracked check command (not directly create)
	if cmd == nil {
		t.Error("summarize fallback should produce async untracked check command")
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

func TestCreateDialog_ShiftEnterNewline(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	m.input = "line one"

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	cd := updated.(createDialog)
	if cd.input != "line one\n" {
		t.Errorf("after shift+enter, input = %q, want 'line one\\n'", cd.input)
	}
	// Should still be at step 0 (not submitted)
	if cd.step != 0 {
		t.Errorf("shift+enter should not submit, step = %d", cd.step)
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
		{"step 0 prompt", 0, "", "", "", "Enter a prompt for Claude"},
		{"step 0 input box", 0, "", "", "", "╭"},
		{"step 0 cursor", 0, "", "", "", "█"},
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
	if !strings.Contains(content, "New Workstream") {
		t.Errorf("view should contain 'New Workstream', got: %s", content)
	}
	if !strings.Contains(content, "━") {
		t.Errorf("view should contain heavy divider, got: %s", content)
	}
}

func TestCreateDialog_ViewStep0Hints(t *testing.T) {
	m := newCreateDialog("/tmp/state", "claude")
	content := viewContent(m.View())
	if !strings.Contains(content, "Shift+Enter") {
		t.Error("step 0 should show Shift+Enter hint")
	}
	if !strings.Contains(content, "[Enter] create") {
		t.Error("step 0 should show Enter create hint")
	}
	if !strings.Contains(content, "Cancel") {
		t.Error("step 0 should show Cancel hint")
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
	m.checkUntrackedFn = noUntrackedFiles

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

func TestRenderInputBox_Empty(t *testing.T) {
	box := renderInputBox("")
	if !strings.Contains(box, "╭") {
		t.Error("should contain top border")
	}
	if !strings.Contains(box, "╰") {
		t.Error("should contain bottom border")
	}
	if !strings.Contains(box, "█") {
		t.Error("should contain cursor")
	}
	// Should have exactly 4 content lines (inputBoxRows)
	lines := strings.Split(strings.TrimRight(box, "\n"), "\n")
	// 1 top border + 4 content + 1 bottom border = 6
	if len(lines) != 6 {
		t.Errorf("expected 6 lines (borders + %d rows), got %d", inputBoxRows, len(lines))
	}
}

func TestRenderInputBox_WithText(t *testing.T) {
	box := renderInputBox("hello world")
	if !strings.Contains(box, "hello world") {
		t.Errorf("should contain text, got: %s", box)
	}
	if !strings.Contains(box, "█") {
		t.Error("should contain cursor after text")
	}
}

func TestRenderInputBox_MultiLine(t *testing.T) {
	box := renderInputBox("line one\nline two")
	if !strings.Contains(box, "line one") {
		t.Errorf("should contain first line, got: %s", box)
	}
	if !strings.Contains(box, "line two") {
		t.Errorf("should contain second line, got: %s", box)
	}
}

func TestRenderInputBox_ScrollsWhenFull(t *testing.T) {
	// 5 lines of input with only 4 visible rows
	box := renderInputBox("line1\nline2\nline3\nline4\nline5")
	// Should show lines 2-5 (scrolled, line1 off screen)
	if strings.Contains(box, "line1") {
		t.Errorf("first line should be scrolled off, got: %s", box)
	}
	if !strings.Contains(box, "line5") {
		t.Errorf("last line should be visible, got: %s", box)
	}
}

func TestRenderInputBox_WrapsLongLine(t *testing.T) {
	// Create a string longer than contentWidth (56 chars)
	long := strings.Repeat("a", 70)
	box := renderInputBox(long)
	// The full text should be visible (wrapped across lines)
	// First 56 chars on line 1, remaining on line 2
	if !strings.Contains(box, strings.Repeat("a", 56)) {
		t.Errorf("should contain full-width segment, got: %s", box)
	}
	// Count content lines: should have wrapped into 2 lines + cursor on 2nd
	// Box has 4 rows minimum, so all content fits
	lines := strings.Split(strings.TrimRight(box, "\n"), "\n")
	// 1 top border + 4 content + 1 bottom = 6
	if len(lines) != 6 {
		t.Errorf("expected 6 lines, got %d", len(lines))
	}
}

func TestRenderInputBox_WrapScrollsCombined(t *testing.T) {
	// A long line that wraps beyond inputBoxRows should scroll
	// contentWidth = 56, inputBoxRows = 4
	// 280 chars = 5 wrapped lines, only last 4 visible
	long := strings.Repeat("x", 280)
	box := renderInputBox(long)
	if !strings.Contains(box, "█") {
		t.Error("cursor should be visible")
	}
	// Should have exactly 6 lines (border + 4 rows + border)
	lines := strings.Split(strings.TrimRight(box, "\n"), "\n")
	if len(lines) != 6 {
		t.Errorf("expected 6 lines, got %d", len(lines))
	}
}

func TestWrapLine(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		width int
		want  int // expected number of wrapped lines
	}{
		{"short", "hello", 10, 1},
		{"exact", "1234567890", 10, 1},
		{"wraps once", "12345678901", 10, 2},
		{"wraps twice", "123456789012345678901", 10, 3},
		{"empty", "", 10, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapLine(tt.line, tt.width)
			if len(got) != tt.want {
				t.Errorf("wrapLine(%q, %d) = %d lines, want %d", tt.line, tt.width, len(got), tt.want)
			}
		})
	}
}

func TestWrapLine_WordBreak(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		width int
		want  []string
	}{
		{
			name:  "breaks at space",
			line:  "hello world foo",
			width: 12,
			want:  []string{"hello world", "foo"},
		},
		{
			name:  "space at break point not moved to next line",
			line:  "hello world bar",
			width: 11,
			want:  []string{"hello world", "bar"},
		},
		{
			name:  "no space falls back to hard break",
			line:  "abcdefghijklmno",
			width: 10,
			want:  []string{"abcdefghij", "klmno"},
		},
		{
			name:  "multiple spaces at break consumed from next line",
			line:  "hello   world",
			width: 8,
			want:  []string{"hello  ", "world"},
		},
		{
			name:  "space exactly at width boundary",
			line:  "1234567890 abc",
			width: 10,
			want:  []string{"1234567890", "abc"},
		},
		{
			name:  "no leading space on wrapped line",
			line:  "hello world",
			width: 6,
			want:  []string{"hello", "world"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapLine(tt.line, tt.width)
			if len(got) != len(tt.want) {
				t.Fatalf("wrapLine(%q, %d) = %v (%d lines), want %v (%d lines)",
					tt.line, tt.width, got, len(got), tt.want, len(tt.want))
			}
			for i, line := range got {
				if line != tt.want[i] {
					t.Errorf("wrapLine(%q, %d)[%d] = %q, want %q",
						tt.line, tt.width, i, line, tt.want[i])
				}
			}
		})
	}
}
