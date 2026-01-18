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
	// Verify input starts empty
	if d.Input.Value() != "" {
		t.Errorf("Input should start empty, got %q", d.Input.Value())
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
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEsc})

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
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})

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
	d.Input.SetValue("implement login feature")
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
	d := NewSettingsDialog(5)

	if d.Type != DialogSettings {
		t.Error("Type should be DialogSettings")
	}
	if !strings.Contains(d.Title, "Settings") {
		t.Error("Title should contain 'Settings'")
	}
	if !strings.Contains(d.Body, "5") {
		t.Error("Body should contain container count")
	}
	if len(d.MenuItems) != 3 {
		t.Errorf("Should have 3 menu items, got %d", len(d.MenuItems))
	}
	if d.MenuSelection != 0 {
		t.Error("Initial selection should be 0")
	}
}

func TestSettingsDialog_Navigation(t *testing.T) {
	d := NewSettingsDialog(3)

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

	// Navigate down at bottom - should stay at 2
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.MenuSelection != 2 {
		t.Errorf("Selection should stay at 2 at bottom, got %d", d.MenuSelection)
	}

	// Navigate up
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
	d := NewSettingsDialog(3)

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
	d := NewSettingsDialog(3)
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

func TestSettingsDialog_PruneAll(t *testing.T) {
	d := NewSettingsDialog(3)
	// Selection 1 = Prune ALL containers
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
	if settingsMsg.Action != SettingsActionPruneAll {
		t.Errorf("Action should be PruneAll, got %v", settingsMsg.Action)
	}
}

func TestSettingsDialog_Cancel(t *testing.T) {
	d := NewSettingsDialog(3)
	// Selection 2 = Cancel
	d.MenuSelection = 2
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
	d := NewSettingsDialog(7)
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
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEnter})

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
	d, cmd := d.Update(tea.KeyMsg{Type: tea.KeyEsc})

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
	d, cmd = d.Update(tea.KeyMsg{Type: tea.KeyEsc})
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
