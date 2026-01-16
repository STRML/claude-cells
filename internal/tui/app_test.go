package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewAppModel(t *testing.T) {
	app := NewAppModel()

	if app.manager == nil {
		t.Error("Manager should not be nil")
	}
	if len(app.panes) != 0 {
		t.Error("Should start with no panes")
	}
	if app.focusedPane != 0 {
		t.Error("Focused pane should start at 0")
	}
	if app.quitting {
		t.Error("Should not be quitting initially")
	}
}

func TestAppModel_Update_Quit(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"q key", "q"},
		{"ctrl+c", "ctrl+c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := NewAppModel()
			var msg tea.KeyMsg
			if tt.key == "ctrl+c" {
				msg = tea.KeyMsg{Type: tea.KeyCtrlC}
			} else {
				msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)}
			}
			model, cmd := app.Update(msg)

			appModel := model.(AppModel)
			if !appModel.quitting {
				t.Errorf("Should be quitting after '%s'", tt.key)
			}
			if cmd == nil {
				t.Error("Should return quit command")
			}
		})
	}
}

func TestAppModel_Update_NewWorkstreamDialog(t *testing.T) {
	app := NewAppModel()

	// Press 'n' to open dialog
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	appModel := model.(AppModel)

	if appModel.dialog == nil {
		t.Error("Dialog should be open after 'n'")
	}
	if appModel.dialog.Type != DialogNewWorkstream {
		t.Error("Dialog should be NewWorkstream type")
	}
}

func TestAppModel_Update_DestroyDialog(t *testing.T) {
	app := NewAppModel()
	app.width = 100
	app.height = 40

	// First create a workstream
	model, _ := app.Update(DialogConfirmMsg{
		Type:  DialogNewWorkstream,
		Value: "test feature",
	})
	app = model.(AppModel)

	// Press 'd' to open destroy dialog
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	app = model.(AppModel)

	if app.dialog == nil {
		t.Error("Dialog should be open after 'd'")
	}
	if app.dialog.Type != DialogDestroy {
		t.Error("Dialog should be Destroy type")
	}
}

func TestAppModel_Update_DestroyWithNoPanes(t *testing.T) {
	app := NewAppModel()

	// Press 'd' with no panes - should not crash
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	app = model.(AppModel)

	if app.dialog != nil {
		t.Error("Dialog should not open with no panes")
	}
}

func TestAppModel_Update_DialogConfirm(t *testing.T) {
	app := NewAppModel()
	app.width = 100
	app.height = 40

	// Simulate creating a workstream
	model, _ := app.Update(DialogConfirmMsg{
		Type:  DialogNewWorkstream,
		Value: "add user auth",
	})
	appModel := model.(AppModel)

	if len(appModel.panes) != 1 {
		t.Errorf("Should have 1 pane, got %d", len(appModel.panes))
	}
	if appModel.manager.Count() != 1 {
		t.Error("Manager should have 1 workstream")
	}
	if appModel.focusedPane != 0 {
		t.Error("New pane should be focused")
	}
	if !appModel.panes[0].focused {
		t.Error("New pane should have focus flag set")
	}
}

func TestAppModel_Update_DialogCancel(t *testing.T) {
	app := NewAppModel()

	// Open dialog
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	app = model.(AppModel)

	if app.dialog == nil {
		t.Fatal("Dialog should be open")
	}

	// Cancel dialog
	model, _ = app.Update(DialogCancelMsg{})
	app = model.(AppModel)

	if app.dialog != nil {
		t.Error("Dialog should be closed after cancel")
	}
}

func TestAppModel_Update_DialogBlocksInput(t *testing.T) {
	app := NewAppModel()
	app.width = 100
	app.height = 40

	// Create a workstream first
	model, _ := app.Update(DialogConfirmMsg{
		Type:  DialogNewWorkstream,
		Value: "test",
	})
	app = model.(AppModel)

	// Open dialog
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	app = model.(AppModel)

	if app.dialog == nil {
		t.Fatal("Dialog should be open")
	}

	// Try pressing 'd' while dialog is open - should not open destroy dialog
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	app = model.(AppModel)

	// Dialog should still be the new workstream dialog
	if app.dialog.Type != DialogNewWorkstream {
		t.Error("Dialog should still be NewWorkstream type")
	}
}

func TestAppModel_Update_DestroyWorkstream(t *testing.T) {
	app := NewAppModel()
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{
		Type:  DialogNewWorkstream,
		Value: "test",
	})
	app = model.(AppModel)
	wsID := app.panes[0].Workstream().ID

	// Destroy it
	model, _ = app.Update(DialogConfirmMsg{
		Type:         DialogDestroy,
		WorkstreamID: wsID,
	})
	app = model.(AppModel)

	if len(app.panes) != 0 {
		t.Errorf("Should have 0 panes, got %d", len(app.panes))
	}
	if app.manager.Count() != 0 {
		t.Error("Manager should have 0 workstreams")
	}
}

func TestAppModel_Update_TabCycle(t *testing.T) {
	app := NewAppModel()
	app.width = 100
	app.height = 40

	// Create two workstreams
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "first"})
	app = model.(AppModel)
	model, _ = app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "second"})
	app = model.(AppModel)

	// Focus should be on second (most recently created)
	if app.focusedPane != 1 {
		t.Errorf("Focus should be on pane 1, got %d", app.focusedPane)
	}

	// Tab should cycle focus back to 0
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = model.(AppModel)

	if app.focusedPane != 0 {
		t.Errorf("Focus should be on pane 0 after tab, got %d", app.focusedPane)
	}
	if !app.panes[0].focused {
		t.Error("Pane 0 should have focus flag set")
	}
	if app.panes[1].focused {
		t.Error("Pane 1 should not have focus flag set")
	}

	// Tab again should go to 1
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = model.(AppModel)

	if app.focusedPane != 1 {
		t.Errorf("Focus should be on pane 1 after second tab, got %d", app.focusedPane)
	}
}

func TestAppModel_Update_TabWithNoPanes(t *testing.T) {
	app := NewAppModel()

	// Tab with no panes should not panic
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = model.(AppModel)

	if app.focusedPane != 0 {
		t.Error("Focus should remain 0")
	}
}

func TestAppModel_Update_NumberKeyFocus(t *testing.T) {
	app := NewAppModel()
	app.width = 100
	app.height = 40

	// Create three workstreams
	for _, name := range []string{"first", "second", "third"} {
		model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: name})
		app = model.(AppModel)
	}

	tests := []struct {
		key      string
		expected int
	}{
		{"1", 0},
		{"2", 1},
		{"3", 2},
		{"1", 0},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
			app = model.(AppModel)

			if app.focusedPane != tt.expected {
				t.Errorf("After pressing '%s', focus should be %d, got %d", tt.key, tt.expected, app.focusedPane)
			}
		})
	}
}

func TestAppModel_Update_NumberKeyOutOfRange(t *testing.T) {
	app := NewAppModel()
	app.width = 100
	app.height = 40

	// Create one workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Press '5' which is out of range
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	app = model.(AppModel)

	// Focus should not change
	if app.focusedPane != 0 {
		t.Errorf("Focus should remain 0 when pressing out of range number, got %d", app.focusedPane)
	}
}

func TestAppModel_Update_WindowSize(t *testing.T) {
	app := NewAppModel()

	model, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app = model.(AppModel)

	if app.width != 120 {
		t.Errorf("Width should be 120, got %d", app.width)
	}
	if app.height != 40 {
		t.Errorf("Height should be 40, got %d", app.height)
	}
}

func TestAppModel_View_Empty(t *testing.T) {
	app := NewAppModel()
	app.width = 80
	app.height = 24

	view := app.View()

	if !strings.Contains(view, "No workstreams") {
		t.Error("Empty view should show 'No workstreams' message")
	}
	if !strings.Contains(view, "[n]") {
		t.Error("Empty view should show key hint to create")
	}
}

func TestAppModel_View_Quitting(t *testing.T) {
	app := NewAppModel()
	app.quitting = true

	view := app.View()

	if !strings.Contains(view, "Goodbye") {
		t.Error("Quitting view should show goodbye message")
	}
}

func TestAppModel_View_WithPanes(t *testing.T) {
	app := NewAppModel()
	app.width = 100
	app.height = 40

	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test feature"})
	app = model.(AppModel)

	view := app.View()

	if strings.Contains(view, "No workstreams") {
		t.Error("View with panes should not show 'No workstreams'")
	}
}

func TestAppModel_View_WithDialog(t *testing.T) {
	app := NewAppModel()
	app.width = 100
	app.height = 40

	// Open dialog
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	app = model.(AppModel)

	view := app.View()

	if !strings.Contains(view, "New Workstream") {
		t.Error("View should contain dialog title")
	}
}

func TestAppModel_UpdateLayout_SinglePane(t *testing.T) {
	app := NewAppModel()
	app.width = 100
	app.height = 40

	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Single pane should use full width
	if app.panes[0].width != 100 {
		t.Errorf("Single pane should have full width, got %d", app.panes[0].width)
	}
}

func TestAppModel_UpdateLayout_TwoPanes(t *testing.T) {
	app := NewAppModel()
	app.width = 100
	app.height = 40

	for _, name := range []string{"first", "second"} {
		model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: name})
		app = model.(AppModel)
	}

	// Two panes should split width
	if app.panes[0].width != 50 {
		t.Errorf("With two panes, each should have half width, got %d", app.panes[0].width)
	}
	if app.panes[1].width != 50 {
		t.Errorf("With two panes, each should have half width, got %d", app.panes[1].width)
	}
}

func TestAppModel_Manager(t *testing.T) {
	app := NewAppModel()

	if app.Manager() == nil {
		t.Error("Manager() should return the manager")
	}
	if app.Manager() != app.manager {
		t.Error("Manager() should return the same manager instance")
	}
}

func TestAppModel_Update_PromptMsg(t *testing.T) {
	app := NewAppModel()
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	wsID := app.panes[0].Workstream().ID

	// Send a prompt message
	model, _ = app.Update(PromptMsg{WorkstreamID: wsID, Prompt: "hello"})
	app = model.(AppModel)

	// The output should contain the prompt
	// Note: We can't easily check the output content from here, but the test
	// verifies the message handling doesn't panic
}

func TestAppModel_DestroyFocusUpdate(t *testing.T) {
	app := NewAppModel()
	app.width = 100
	app.height = 40

	// Create three workstreams
	for _, name := range []string{"first", "second", "third"} {
		model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: name})
		app = model.(AppModel)
	}

	// Focus is on third (index 2)
	if app.focusedPane != 2 {
		t.Errorf("Focus should be on pane 2, got %d", app.focusedPane)
	}

	// Destroy the third workstream
	wsID := app.panes[2].Workstream().ID
	model, _ := app.Update(DialogConfirmMsg{Type: DialogDestroy, WorkstreamID: wsID})
	app = model.(AppModel)

	// Focus should move to pane 1 (last remaining valid index)
	if app.focusedPane != 1 {
		t.Errorf("After destroying last pane, focus should be 1, got %d", app.focusedPane)
	}
	if !app.panes[1].focused {
		t.Error("Pane 1 should have focus flag set")
	}
}
