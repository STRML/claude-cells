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
