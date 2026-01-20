package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewDestroyDialog(t *testing.T) {
	d := NewDestroyDialog("add-auth", "ws-123")

	if d.Type != DialogDestroy {
		t.Error("Type should be DialogDestroy")
	}
	if !strings.Contains(d.Title, "add-auth") {
		t.Error("Title should contain branch name")
	}
	if d.ConfirmWord != "destroy" {
		t.Error("ConfirmWord should be 'destroy'")
	}
	if d.WorkstreamID != "ws-123" {
		t.Error("WorkstreamID should be set")
	}
}

func TestNewWorkstreamDialog(t *testing.T) {
	d := NewWorkstreamDialog()

	if d.Type != DialogNewWorkstream {
		t.Error("Type should be DialogNewWorkstream")
	}
	if !strings.Contains(d.Title, "New Workstream") {
		t.Error("Title should contain 'New Workstream'")
	}
	// Verify useTextArea is set
	if !d.useTextArea {
		t.Error("useTextArea should be true for workstream dialog")
	}
	// Verify textarea starts empty
	if d.TextArea.Value() != "" {
		t.Errorf("TextArea should start empty, got %q", d.TextArea.Value())
	}
}

func TestNewPRDialog(t *testing.T) {
	d := NewPRDialog("feature-x", "Add feature X", "Description here")

	if d.Type != DialogPRPreview {
		t.Error("Type should be DialogPRPreview")
	}
	if !strings.Contains(d.Title, "feature-x") {
		t.Error("Title should contain branch name")
	}
}

func TestDialogModel_Update_Escape(t *testing.T) {
	d := NewWorkstreamDialog()
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if cmd == nil {
		t.Error("Should return a command on escape")
	}

	// Execute the command to get the message
	msg := cmd()
	if _, ok := msg.(DialogCancelMsg); !ok {
		t.Error("Should return DialogCancelMsg on escape")
	}
}

func TestDialogModel_Update_Enter_Empty(t *testing.T) {
	d := NewWorkstreamDialog()
	// Don't type anything, just press enter
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should not confirm with empty input
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(DialogConfirmMsg); ok {
			t.Error("Should not confirm with empty input")
		}
	}
}

func TestDialogModel_Update_Enter_WithValue(t *testing.T) {
	d := NewWorkstreamDialog()
	d.TextArea.SetValue("implement login feature")
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("Should return a command on enter with value")
	}

	msg := cmd()
	confirmMsg, ok := msg.(DialogConfirmMsg)
	if !ok {
		t.Fatal("Should return DialogConfirmMsg on enter with value")
	}
	if confirmMsg.Value != "implement login feature" {
		t.Errorf("Value should be 'implement login feature', got %q", confirmMsg.Value)
	}
	if confirmMsg.Type != DialogNewWorkstream {
		t.Error("Type should be DialogNewWorkstream")
	}
}

func TestDialogModel_Update_DestroyConfirm_WrongWord(t *testing.T) {
	d := NewDestroyDialog("test-branch", "ws-123")
	d.Input.SetValue("delete") // Wrong word
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should not confirm with wrong word
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(DialogConfirmMsg); ok {
			t.Error("Should not confirm with wrong word")
		}
	}
}

func TestDialogModel_Update_DestroyConfirm_CorrectWord(t *testing.T) {
	d := NewDestroyDialog("test-branch", "ws-123")
	d.Input.SetValue("destroy")
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("Should return a command on correct confirm word")
	}

	msg := cmd()
	confirmMsg, ok := msg.(DialogConfirmMsg)
	if !ok {
		t.Fatal("Should return DialogConfirmMsg on correct confirm word")
	}
	if confirmMsg.WorkstreamID != "ws-123" {
		t.Errorf("WorkstreamID should be 'ws-123', got %q", confirmMsg.WorkstreamID)
	}
	if confirmMsg.Type != DialogDestroy {
		t.Error("Type should be DialogDestroy")
	}
}

func TestDialogModel_Update_DestroyConfirm_CaseInsensitive(t *testing.T) {
	d := NewDestroyDialog("test-branch", "ws-123")
	d.Input.SetValue("DESTROY") // Uppercase should work
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("Should return a command on uppercase confirm word")
	}

	msg := cmd()
	if _, ok := msg.(DialogConfirmMsg); !ok {
		t.Error("Should return DialogConfirmMsg on uppercase confirm word")
	}
}

func TestDialogModel_View(t *testing.T) {
	d := NewDestroyDialog("test-branch", "id-123")
	d.SetSize(50, 20)

	view := d.View()

	if !strings.Contains(view, "Destroy") {
		t.Error("View should contain 'Destroy'")
	}
	if !strings.Contains(view, "test-branch") {
		t.Error("View should contain branch name")
	}
}

func TestDialogModel_SetSize(t *testing.T) {
	d := NewWorkstreamDialog()
	d.SetSize(60, 30)

	if d.width != 60 {
		t.Errorf("width should be 60, got %d", d.width)
	}
	if d.height != 30 {
		t.Errorf("height should be 30, got %d", d.height)
	}
}

func TestDialogModel_Init(t *testing.T) {
	d := NewWorkstreamDialog()
	cmd := d.Init()

	// Init should return textinput.Blink command
	if cmd == nil {
		t.Error("Init should return a command for cursor blink")
	}
}

func TestDialogType_Constants(t *testing.T) {
	// Ensure dialog types are distinct
	types := []DialogType{DialogNone, DialogDestroy, DialogNewWorkstream, DialogPRPreview}
	seen := make(map[DialogType]bool)

	for _, dt := range types {
		if seen[dt] {
			t.Errorf("DialogType %d is duplicated", dt)
		}
		seen[dt] = true
	}
}

func TestPRDialog_HasInputValue(t *testing.T) {
	d := NewPRDialog("feature-x", "Add feature X", "Description here")

	// PR dialog should pre-populate the input with the title
	if d.Input.Value() != "Add feature X" {
		t.Errorf("PR dialog should have pre-populated title, got %q", d.Input.Value())
	}
}

func TestNewSettingsDialog(t *testing.T) {
	d := NewSettingsDialog(5, "test-project")

	if d.Type != DialogSettings {
		t.Error("Type should be DialogSettings")
	}
	if !strings.Contains(d.Title, "Settings") {
		t.Error("Title should contain 'Settings'")
	}
	if !strings.Contains(d.Body, "5") {
		t.Error("Body should contain container count")
	}
	if len(d.MenuItems) != 4 {
		t.Errorf("Should have 4 menu items, got %d", len(d.MenuItems))
	}
	if d.MenuSelection != 0 {
		t.Error("Initial selection should be 0")
	}
}

func TestSettingsDialog_Navigation(t *testing.T) {
	d := NewSettingsDialog(3, "test-project")

	// Navigate down
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.MenuSelection != 1 {
		t.Errorf("Selection should be 1, got %d", d.MenuSelection)
	}

	// Navigate down again
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.MenuSelection != 2 {
		t.Errorf("Selection should be 2, got %d", d.MenuSelection)
	}

	// Navigate down again
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.MenuSelection != 3 {
		t.Errorf("Selection should be 3, got %d", d.MenuSelection)
	}

	// Navigate down at bottom - should stay at 3
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.MenuSelection != 3 {
		t.Errorf("Selection should stay at 3 at bottom, got %d", d.MenuSelection)
	}

	// Navigate up
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.MenuSelection != 2 {
		t.Errorf("Selection should be 2, got %d", d.MenuSelection)
	}

	// Navigate up again
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.MenuSelection != 1 {
		t.Errorf("Selection should be 1, got %d", d.MenuSelection)
	}

	// Navigate up to top
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.MenuSelection != 0 {
		t.Errorf("Selection should be 0, got %d", d.MenuSelection)
	}

	// Navigate up at top - should stay at 0
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.MenuSelection != 0 {
		t.Errorf("Selection should stay at 0 at top, got %d", d.MenuSelection)
	}
}

func TestSettingsDialog_VimNavigation(t *testing.T) {
	d := NewSettingsDialog(3, "test-project")

	// Navigate with j (down)
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if d.MenuSelection != 1 {
		t.Errorf("Selection should be 1 after 'j', got %d", d.MenuSelection)
	}

	// Navigate with k (up)
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if d.MenuSelection != 0 {
		t.Errorf("Selection should be 0 after 'k', got %d", d.MenuSelection)
	}
}

func TestSettingsDialog_PruneStopped(t *testing.T) {
	d := NewSettingsDialog(3, "test-project")
	// Selection 0 = Prune stopped containers
	d.MenuSelection = 0
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("Should return a command on enter")
	}

	msg := cmd()
	settingsMsg, ok := msg.(SettingsConfirmMsg)
	if !ok {
		t.Fatal("Should return SettingsConfirmMsg")
	}
	if settingsMsg.Action != SettingsActionPruneStopped {
		t.Errorf("Action should be PruneStopped, got %v", settingsMsg.Action)
	}
}

func TestSettingsDialog_PruneProject(t *testing.T) {
	d := NewSettingsDialog(3, "test-project")
	// Selection 1 = Destroy project containers
	d.MenuSelection = 1
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("Should return a command on enter")
	}

	msg := cmd()
	settingsMsg, ok := msg.(SettingsConfirmMsg)
	if !ok {
		t.Fatal("Should return SettingsConfirmMsg")
	}
	if settingsMsg.Action != SettingsActionPruneProject {
		t.Errorf("Action should be PruneProject, got %v", settingsMsg.Action)
	}
}

func TestSettingsDialog_PruneAll(t *testing.T) {
	d := NewSettingsDialog(3, "test-project")
	// Selection 2 = Destroy ALL containers (all projects)
	d.MenuSelection = 2
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("Should return a command on enter")
	}

	msg := cmd()
	settingsMsg, ok := msg.(SettingsConfirmMsg)
	if !ok {
		t.Fatal("Should return SettingsConfirmMsg")
	}
	if settingsMsg.Action != SettingsActionPruneAll {
		t.Errorf("Action should be PruneAll, got %v", settingsMsg.Action)
	}
}

func TestSettingsDialog_Cancel(t *testing.T) {
	d := NewSettingsDialog(3, "test-project")
	// Selection 3 = Cancel
	d.MenuSelection = 3
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("Should return a command on enter")
	}

	msg := cmd()
	if _, ok := msg.(DialogCancelMsg); !ok {
		t.Error("Should return DialogCancelMsg on cancel selection")
	}
}

func TestSettingsDialog_View(t *testing.T) {
	d := NewSettingsDialog(7, "test-project")
	d.SetSize(50, 20)

	view := d.View()

	if !strings.Contains(view, "Settings") {
		t.Error("View should contain 'Settings'")
	}
	if !strings.Contains(view, "7") {
		t.Error("View should contain container count")
	}
	if !strings.Contains(view, "Prune stopped ccells") {
		t.Error("View should contain menu item")
	}
	if !strings.Contains(view, "→") {
		t.Error("View should contain selection indicator")
	}
}

func TestDialogType_IncludesSettings(t *testing.T) {
	// Ensure DialogSettings is included
	types := []DialogType{DialogNone, DialogDestroy, DialogNewWorkstream, DialogPRPreview, DialogSettings}
	seen := make(map[DialogType]bool)

	for _, dt := range types {
		if seen[dt] {
			t.Errorf("DialogType %d is duplicated", dt)
		}
		seen[dt] = true
	}
}

func TestNewLogDialog(t *testing.T) {
	content := "Line 1\nLine 2\nLine 3\nLine 4"
	d := NewLogDialog("feature-branch", content)

	if d.Type != DialogLog {
		t.Error("Type should be DialogLog")
	}
	if !strings.Contains(d.Title, "feature-branch") {
		t.Error("Title should contain branch name")
	}
	if d.Body != content {
		t.Error("Body should contain log content")
	}
}

func TestLogDialog_Scrolling(t *testing.T) {
	// Create content with many lines
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, "Line "+string(rune('0'+i%10)))
	}
	content := strings.Join(lines, "\n")
	d := NewLogDialog("test-branch", content)
	d.SetSize(60, 30)

	// Initial scroll offset should be 0
	if d.scrollOffset != 0 {
		t.Errorf("Initial scrollOffset should be 0, got %d", d.scrollOffset)
	}

	// Scroll down
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.scrollOffset != 1 {
		t.Errorf("scrollOffset should be 1 after down, got %d", d.scrollOffset)
	}

	// Scroll down with j
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if d.scrollOffset != 2 {
		t.Errorf("scrollOffset should be 2 after 'j', got %d", d.scrollOffset)
	}

	// Scroll up
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.scrollOffset != 1 {
		t.Errorf("scrollOffset should be 1 after up, got %d", d.scrollOffset)
	}

	// Scroll up with k
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if d.scrollOffset != 0 {
		t.Errorf("scrollOffset should be 0 after 'k', got %d", d.scrollOffset)
	}

	// Can't scroll past top
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.scrollOffset != 0 {
		t.Errorf("scrollOffset should stay at 0 at top, got %d", d.scrollOffset)
	}
}

func TestLogDialog_DismissOnEnter(t *testing.T) {
	d := NewLogDialog("test-branch", "Some log content")
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("Should return a command on enter")
	}

	msg := cmd()
	if _, ok := msg.(DialogCancelMsg); !ok {
		t.Error("Should return DialogCancelMsg on enter")
	}
}

func TestLogDialog_DismissOnEscape(t *testing.T) {
	d := NewLogDialog("test-branch", "Some log content")
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if cmd == nil {
		t.Fatal("Should return a command on escape")
	}

	msg := cmd()
	if _, ok := msg.(DialogCancelMsg); !ok {
		t.Error("Should return DialogCancelMsg on escape")
	}
}

func TestLogDialog_View(t *testing.T) {
	d := NewLogDialog("test-branch", "Line 1\nLine 2\nLine 3")
	d.SetSize(60, 30)

	view := d.View()

	if !strings.Contains(view, "Logs:") {
		t.Error("View should contain 'Logs:'")
	}
	if !strings.Contains(view, "test-branch") {
		t.Error("View should contain branch name")
	}
	if !strings.Contains(view, "Line 1") {
		t.Error("View should contain log content")
	}
	if !strings.Contains(view, "scroll") {
		t.Error("View should contain scroll hint")
	}
}

func TestNewProgressDialog(t *testing.T) {
	d := NewProgressDialog("Pushing Branch", "Working on it...", "ws-123")

	if d.Type != DialogProgress {
		t.Error("Type should be DialogProgress")
	}
	if d.Title != "Pushing Branch" {
		t.Errorf("Title should be 'Pushing Branch', got %q", d.Title)
	}
	if d.Body != "Working on it..." {
		t.Errorf("Body should be 'Working on it...', got %q", d.Body)
	}
	if d.WorkstreamID != "ws-123" {
		t.Errorf("WorkstreamID should be 'ws-123', got %q", d.WorkstreamID)
	}
	if !d.inProgress {
		t.Error("inProgress should be true initially")
	}
}

func TestProgressDialog_CantDismissWhileInProgress(t *testing.T) {
	d := NewProgressDialog("Working", "Please wait...", "ws-1")

	// Try to dismiss with Enter - should not work
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(DialogCancelMsg); ok {
			t.Error("Should not be able to dismiss while in progress with Enter")
		}
	}

	// Try to dismiss with Escape - should not work
	_, cmd = d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(DialogCancelMsg); ok {
			t.Error("Should not be able to dismiss while in progress with Escape")
		}
	}
}

func TestProgressDialog_CanDismissWhenComplete(t *testing.T) {
	d := NewProgressDialog("Working", "Please wait...", "ws-1")
	d.SetComplete("Done!")

	if d.inProgress {
		t.Error("inProgress should be false after SetComplete")
	}
	if d.Body != "Done!" {
		t.Errorf("Body should be 'Done!', got %q", d.Body)
	}

	// Now can dismiss with Enter
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Should return a command on enter when complete")
	}
	msg := cmd()
	if _, ok := msg.(DialogCancelMsg); !ok {
		t.Error("Should return DialogCancelMsg on enter when complete")
	}
}

func TestProgressDialog_View_InProgress(t *testing.T) {
	d := NewProgressDialog("Working", "Please wait...", "ws-1")
	d.SetSize(50, 15)

	view := d.View()

	if !strings.Contains(view, "Working") {
		t.Error("View should contain title")
	}
	if !strings.Contains(view, "Please wait...") {
		t.Error("View should contain body message")
	}
	if !strings.Contains(view, "Working...") {
		t.Error("View should show 'Working...' hint when in progress")
	}
}

func TestProgressDialog_View_Complete(t *testing.T) {
	d := NewProgressDialog("Push Complete", "Branch pushed!", "ws-1")
	d.SetComplete("Done successfully!")
	d.SetSize(50, 15)

	view := d.View()

	if !strings.Contains(view, "Done successfully!") {
		t.Error("View should contain completed body")
	}
	if !strings.Contains(view, "Enter") {
		t.Error("View should show close hint when complete")
	}
}

func TestProgressDialog_AppendBody(t *testing.T) {
	d := NewProgressDialog("Working", "Line 1\n", "ws-1")
	d.AppendBody("Line 2\n")

	if d.Body != "Line 1\nLine 2\n" {
		t.Errorf("Body should be 'Line 1\\nLine 2\\n', got %q", d.Body)
	}
}

func TestWorkstreamDialog_TextAreaWidthUpdatesOnSetSize(t *testing.T) {
	d := NewWorkstreamDialog()

	// Initial width before SetSize - just verify it exists
	initialWidth := d.TextArea.Width()
	if initialWidth <= 0 {
		t.Errorf("Initial TextArea width should be > 0, got %d", initialWidth)
	}

	// Set dialog size to 70 (as used in app.go)
	d.SetSize(70, 15)

	// TextArea width should be dialog width - 12 (for borders and padding)
	expectedWidth := 70 - 12
	actualWidth := d.TextArea.Width()
	if actualWidth != expectedWidth {
		t.Errorf("TextArea width should be %d after SetSize(70, 15), got %d", expectedWidth, actualWidth)
	}
}

func TestWorkstreamDialog_TextAreaWidthMinimum(t *testing.T) {
	d := NewWorkstreamDialog()

	// Set a very small dialog size
	d.SetSize(20, 10)

	// TextArea width should be at least 20 (the minimum)
	actualWidth := d.TextArea.Width()
	if actualWidth < 20 {
		t.Errorf("TextArea width should be at least 20, got %d", actualWidth)
	}
}

func TestWorkstreamDialog_TextAreaWidthVarious(t *testing.T) {
	testCases := []struct {
		dialogWidth      int
		expectedMinWidth int
	}{
		{50, 38},  // 50 - 12 = 38
		{60, 48},  // 60 - 12 = 48
		{70, 58},  // 70 - 12 = 58
		{80, 68},  // 80 - 12 = 68
		{100, 88}, // 100 - 12 = 88
	}

	for _, tc := range testCases {
		d := NewWorkstreamDialog()
		d.SetSize(tc.dialogWidth, 15)

		actualWidth := d.TextArea.Width()
		if actualWidth != tc.expectedMinWidth {
			t.Errorf("With dialog width %d, TextArea width should be %d, got %d",
				tc.dialogWidth, tc.expectedMinWidth, actualWidth)
		}
	}
}

func TestWorkstreamDialog_NonTextAreaDialogUnaffectedBySetSize(t *testing.T) {
	// Destroy dialog uses textinput, not textarea
	d := NewDestroyDialog("test-branch", "ws-123")

	// This should not panic even though it's not a textarea dialog
	d.SetSize(70, 15)

	// Verify width is set
	if d.width != 70 {
		t.Errorf("width should be 70, got %d", d.width)
	}
}

func TestNewFirstRunIntroductionDialog(t *testing.T) {
	d := NewFirstRunIntroductionDialog()

	if d.Type != DialogFirstRunIntroduction {
		t.Error("Type should be DialogFirstRunIntroduction")
	}
	if d.Title != "Getting Started" {
		t.Errorf("Title should be 'Getting Started', got %q", d.Title)
	}
	if !strings.Contains(d.Body, "Welcome to Claude Cells") {
		t.Error("Body should contain welcome message")
	}
	if !strings.Contains(d.Body, "NAV mode") {
		t.Error("Body should contain NAV mode explanation")
	}
	if !strings.Contains(d.Body, "INPUT mode") {
		t.Error("Body should contain INPUT mode explanation")
	}
	if !strings.Contains(d.Body, "github.com/STRML/claude-cells") {
		t.Error("Body should contain GitHub link")
	}
}

func TestFirstRunIntroductionDialog_DismissOnEnter(t *testing.T) {
	d := NewFirstRunIntroductionDialog()
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("Should return a command on enter")
	}

	msg := cmd()
	confirmMsg, ok := msg.(DialogConfirmMsg)
	if !ok {
		t.Fatal("Should return DialogConfirmMsg on enter")
	}
	if confirmMsg.Type != DialogFirstRunIntroduction {
		t.Errorf("DialogConfirmMsg type should be DialogFirstRunIntroduction, got %d", confirmMsg.Type)
	}
}

func TestFirstRunIntroductionDialog_DismissOnEscape(t *testing.T) {
	d := NewFirstRunIntroductionDialog()
	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if cmd == nil {
		t.Fatal("Should return a command on escape")
	}

	msg := cmd()
	if _, ok := msg.(DialogCancelMsg); !ok {
		t.Error("Should return DialogCancelMsg on escape")
	}
}

func TestFirstRunIntroductionDialog_Scrolling(t *testing.T) {
	d := NewFirstRunIntroductionDialog()
	d.SetSize(60, 15) // Small height to ensure scrolling is needed

	// Initial scroll offset should be 0
	if d.scrollOffset != 0 {
		t.Errorf("Initial scrollOffset should be 0, got %d", d.scrollOffset)
	}

	// Scroll down
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.scrollOffset != 1 {
		t.Errorf("scrollOffset should be 1 after down, got %d", d.scrollOffset)
	}

	// Scroll down with j
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if d.scrollOffset != 2 {
		t.Errorf("scrollOffset should be 2 after 'j', got %d", d.scrollOffset)
	}

	// Scroll up
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.scrollOffset != 1 {
		t.Errorf("scrollOffset should be 1 after up, got %d", d.scrollOffset)
	}

	// Scroll up with k
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if d.scrollOffset != 0 {
		t.Errorf("scrollOffset should be 0 after 'k', got %d", d.scrollOffset)
	}
}

func TestFirstRunIntroductionDialog_View(t *testing.T) {
	d := NewFirstRunIntroductionDialog()
	d.SetSize(70, 30)

	view := d.View()

	if !strings.Contains(view, "Getting Started") {
		t.Error("View should contain title")
	}
	if !strings.Contains(view, "Welcome") {
		t.Error("View should contain welcome message")
	}
	if !strings.Contains(view, "Enter") {
		t.Error("View should contain continue hint")
	}
}

func TestNewMergeConflictDialog(t *testing.T) {
	conflictFiles := []string{"file1.go", "file2.go"}
	d := NewMergeConflictDialog("feature-branch", "ws-123", conflictFiles)

	if d.Type != DialogMergeConflict {
		t.Error("Type should be DialogMergeConflict")
	}
	if !strings.Contains(d.Title, "Merge Conflict") {
		t.Error("Title should contain 'Merge Conflict'")
	}
	if d.WorkstreamID != "ws-123" {
		t.Errorf("WorkstreamID should be 'ws-123', got %q", d.WorkstreamID)
	}
	if len(d.MenuItems) != 2 {
		t.Errorf("Should have 2 menu items, got %d", len(d.MenuItems))
	}
	if d.MenuSelection != 0 {
		t.Error("Initial selection should be 0")
	}
	if !strings.Contains(d.Body, "file1.go") {
		t.Error("Body should contain conflict file names")
	}
}

func TestMergeConflictDialog_Navigation(t *testing.T) {
	d := NewMergeConflictDialog("feature-branch", "ws-123", []string{"file1.go"})

	// Initial selection should be 0
	if d.MenuSelection != 0 {
		t.Errorf("Initial selection should be 0, got %d", d.MenuSelection)
	}

	// Navigate down
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.MenuSelection != 1 {
		t.Errorf("Selection should be 1 after down, got %d", d.MenuSelection)
	}

	// Navigate down at bottom - should stay at 1
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.MenuSelection != 1 {
		t.Errorf("Selection should stay at 1 at bottom, got %d", d.MenuSelection)
	}

	// Navigate up
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.MenuSelection != 0 {
		t.Errorf("Selection should be 0 after up, got %d", d.MenuSelection)
	}

	// Navigate up at top - should stay at 0
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.MenuSelection != 0 {
		t.Errorf("Selection should stay at 0 at top, got %d", d.MenuSelection)
	}
}

func TestMergeConflictDialog_VimNavigation(t *testing.T) {
	d := NewMergeConflictDialog("feature-branch", "ws-123", []string{"file1.go"})

	// Navigate with j (down)
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if d.MenuSelection != 1 {
		t.Errorf("Selection should be 1 after 'j', got %d", d.MenuSelection)
	}

	// Navigate with k (up)
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if d.MenuSelection != 0 {
		t.Errorf("Selection should be 0 after 'k', got %d", d.MenuSelection)
	}
}

func TestMergeConflictDialog_EnterRebase(t *testing.T) {
	d := NewMergeConflictDialog("feature-branch", "ws-123", []string{"file1.go"})
	// Selection 0 = Rebase onto main
	d.MenuSelection = 0
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("Should return a command on enter")
	}

	msg := cmd()
	confirmMsg, ok := msg.(DialogConfirmMsg)
	if !ok {
		t.Fatalf("Should return DialogConfirmMsg, got %T", msg)
	}
	if confirmMsg.Type != DialogMergeConflict {
		t.Error("Type should be DialogMergeConflict")
	}
	if confirmMsg.WorkstreamID != "ws-123" {
		t.Errorf("WorkstreamID should be 'ws-123', got %q", confirmMsg.WorkstreamID)
	}
	if confirmMsg.Value != "0" {
		t.Errorf("Value should be '0' for rebase, got %q", confirmMsg.Value)
	}
}

func TestMergeConflictDialog_EnterCancel(t *testing.T) {
	d := NewMergeConflictDialog("feature-branch", "ws-123", []string{"file1.go"})
	// Selection 1 = Cancel
	d.MenuSelection = 1
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("Should return a command on enter")
	}

	msg := cmd()
	if _, ok := msg.(DialogCancelMsg); !ok {
		t.Errorf("Should return DialogCancelMsg on cancel selection, got %T", msg)
	}
}

func TestMergeConflictDialog_View(t *testing.T) {
	d := NewMergeConflictDialog("feature-branch", "ws-123", []string{"file1.go", "file2.go"})
	d.SetSize(60, 20)

	view := d.View()

	if !strings.Contains(view, "Merge Conflict") {
		t.Error("View should contain 'Merge Conflict'")
	}
	if !strings.Contains(view, "Rebase") {
		t.Error("View should contain 'Rebase' menu item")
	}
	if !strings.Contains(view, "Cancel") {
		t.Error("View should contain 'Cancel' menu item")
	}
	if !strings.Contains(view, "→") {
		t.Error("View should contain selection indicator")
	}
	if !strings.Contains(view, "↑/↓") {
		t.Error("View should contain navigation hint")
	}
}

func TestNewPostMergeDestroyDialog(t *testing.T) {
	d := NewPostMergeDestroyDialog("feature-branch", "ws-123")

	if d.Type != DialogPostMergeDestroy {
		t.Error("Type should be DialogPostMergeDestroy")
	}
	if !strings.Contains(d.Title, "Merge Successful") {
		t.Error("Title should contain 'Merge Successful'")
	}
	if d.WorkstreamID != "ws-123" {
		t.Errorf("WorkstreamID should be 'ws-123', got %q", d.WorkstreamID)
	}
	if len(d.MenuItems) != 2 {
		t.Errorf("Should have 2 menu items, got %d", len(d.MenuItems))
	}
	if d.MenuSelection != 0 {
		t.Error("Initial selection should be 0")
	}
}

func TestPostMergeDestroyDialog_Navigation(t *testing.T) {
	d := NewPostMergeDestroyDialog("feature-branch", "ws-123")

	// Navigate down
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.MenuSelection != 1 {
		t.Errorf("Selection should be 1 after down, got %d", d.MenuSelection)
	}

	// Navigate down at bottom - should stay at 1
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.MenuSelection != 1 {
		t.Errorf("Selection should stay at 1 at bottom, got %d", d.MenuSelection)
	}

	// Navigate up
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.MenuSelection != 0 {
		t.Errorf("Selection should be 0 after up, got %d", d.MenuSelection)
	}
}

func TestPostMergeDestroyDialog_EnterDestroy(t *testing.T) {
	d := NewPostMergeDestroyDialog("feature-branch", "ws-123")
	// Selection 0 = Yes, destroy container
	d.MenuSelection = 0
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("Should return a command on enter")
	}

	msg := cmd()
	confirmMsg, ok := msg.(DialogConfirmMsg)
	if !ok {
		t.Fatalf("Should return DialogConfirmMsg, got %T", msg)
	}
	if confirmMsg.Type != DialogPostMergeDestroy {
		t.Error("Type should be DialogPostMergeDestroy")
	}
	if confirmMsg.WorkstreamID != "ws-123" {
		t.Errorf("WorkstreamID should be 'ws-123', got %q", confirmMsg.WorkstreamID)
	}
	if confirmMsg.Value != "0" {
		t.Errorf("Value should be '0' for destroy, got %q", confirmMsg.Value)
	}
}

func TestPostMergeDestroyDialog_EnterKeep(t *testing.T) {
	d := NewPostMergeDestroyDialog("feature-branch", "ws-123")
	// Selection 1 = No, keep container
	d.MenuSelection = 1
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("Should return a command on enter")
	}

	msg := cmd()
	if _, ok := msg.(DialogCancelMsg); !ok {
		t.Errorf("Should return DialogCancelMsg on keep selection, got %T", msg)
	}
}

func TestPostMergeDestroyDialog_View(t *testing.T) {
	d := NewPostMergeDestroyDialog("feature-branch", "ws-123")
	d.SetSize(50, 12)

	view := d.View()

	if !strings.Contains(view, "Merge Successful") {
		t.Error("View should contain 'Merge Successful'")
	}
	if !strings.Contains(view, "destroy") {
		t.Error("View should contain 'destroy' menu item")
	}
	if !strings.Contains(view, "keep") {
		t.Error("View should contain 'keep' menu item")
	}
	if !strings.Contains(view, "→") {
		t.Error("View should contain selection indicator")
	}
}
