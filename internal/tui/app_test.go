package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/STRML/claude-cells/internal/workstream"
	tea "github.com/charmbracelet/bubbletea"
)

func TestNewAppModel(t *testing.T) {
	app := NewAppModel(context.Background())

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
			app := NewAppModel(context.Background())
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
	app := NewAppModel(context.Background())

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
	app := NewAppModel(context.Background())
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
	app := NewAppModel(context.Background())

	// Press 'd' with no panes - should not crash
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	app = model.(AppModel)

	if app.dialog != nil {
		t.Error("Dialog should not open with no panes")
	}
}

func TestAppModel_Update_DialogConfirm(t *testing.T) {
	app := NewAppModel(context.Background())
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
	app := NewAppModel(context.Background())

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
	app := NewAppModel(context.Background())
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
	app := NewAppModel(context.Background())
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
	app := NewAppModel(context.Background())
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
	app := NewAppModel(context.Background())

	// Tab with no panes should not panic
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = model.(AppModel)

	if app.focusedPane != 0 {
		t.Error("Focus should remain 0")
	}
}

func TestAppModel_Update_NumberKeyFocus(t *testing.T) {
	app := NewAppModel(context.Background())
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
	app := NewAppModel(context.Background())
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
	app := NewAppModel(context.Background())

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
	app := NewAppModel(context.Background())
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
	app := NewAppModel(context.Background())
	app.quitting = true

	view := app.View()

	if !strings.Contains(view, "Goodbye") {
		t.Error("Quitting view should show goodbye message")
	}
}

func TestAppModel_View_WithPanes(t *testing.T) {
	app := NewAppModel(context.Background())
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
	app := NewAppModel(context.Background())
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
	app := NewAppModel(context.Background())
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
	app := NewAppModel(context.Background())
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
	app := NewAppModel(context.Background())

	if app.Manager() == nil {
		t.Error("Manager() should return the manager")
	}
	if app.Manager() != app.manager {
		t.Error("Manager() should return the same manager instance")
	}
}

func TestAppModel_Update_PromptMsg(t *testing.T) {
	app := NewAppModel(context.Background())
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
	app := NewAppModel(context.Background())
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

func TestAppModel_InputMode(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream first
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Should start in nav mode
	if app.inputMode {
		t.Error("Should start in nav mode")
	}
	if app.InputMode() {
		t.Error("InputMode() should return false initially")
	}

	// Press 'i' to enter input mode
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")}
	model, _ = app.Update(msg)
	app = model.(AppModel)

	if !app.inputMode {
		t.Error("Should be in input mode after pressing 'i'")
	}
	if !app.InputMode() {
		t.Error("InputMode() should return true")
	}

	// Press Escape twice to exit input mode (single escape sends to pane, double exits)
	msg = tea.KeyMsg{Type: tea.KeyEscape}
	model, _ = app.Update(msg)
	app = model.(AppModel)

	if !app.inputMode {
		t.Error("Single escape should NOT exit input mode (it sends to pane)")
	}

	// Second escape within timeout should exit input mode
	msg = tea.KeyMsg{Type: tea.KeyEscape}
	model, _ = app.Update(msg)
	app = model.(AppModel)

	if app.inputMode {
		t.Error("Double escape should exit input mode")
	}
}

func TestAppModel_InputMode_Enter(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Press 'enter' to enter input mode
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	model, _ = app.Update(msg)
	app = model.(AppModel)

	if !app.inputMode {
		t.Error("Should be in input mode after pressing Enter")
	}
}

func TestAppModel_InputMode_RoutesKeysToPane(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Enter input mode
	app.inputMode = true

	// Press 'n' - should NOT create a new workstream dialog
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}
	model, _ = app.Update(msg)
	app = model.(AppModel)

	if app.dialog != nil {
		t.Error("In input mode, 'n' should not open dialog")
	}
}

func TestAppModel_NavMode_Keybinds(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		keyType  tea.KeyType
		checkFn  func(app AppModel) bool
		errorMsg string
	}{
		{
			name:     "n opens new workstream dialog",
			key:      "n",
			keyType:  tea.KeyRunes,
			checkFn:  func(app AppModel) bool { return app.dialog != nil },
			errorMsg: "n should open dialog",
		},
		{
			name:     "l does nothing without panes",
			key:      "l",
			keyType:  tea.KeyRunes,
			checkFn:  func(app AppModel) bool { return app.dialog == nil },
			errorMsg: "l without panes should do nothing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := NewAppModel(context.Background())
			app.width = 100
			app.height = 40

			var msg tea.KeyMsg
			if tt.keyType == tea.KeyRunes {
				msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)}
			} else {
				msg = tea.KeyMsg{Type: tt.keyType}
			}

			model, _ := app.Update(msg)
			result := model.(AppModel)

			if !tt.checkFn(result) {
				t.Error(tt.errorMsg)
			}
		})
	}
}

func TestAppModel_TmuxPrefix_NavMode(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create two workstreams
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test1"})
	app = model.(AppModel)
	model, _ = app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test2"})
	app = model.(AppModel)

	// Focus should be on pane 1 (index 1, second pane)
	if app.focusedPane != 1 {
		t.Fatalf("Expected focusedPane=1, got %d", app.focusedPane)
	}

	// Press ctrl+b (tmux prefix)
	msg := tea.KeyMsg{Type: tea.KeyCtrlB}
	model, _ = app.Update(msg)
	app = model.(AppModel)

	if !app.tmuxPrefix {
		t.Error("ctrl+b should set tmux prefix")
	}

	// Press left arrow
	msg = tea.KeyMsg{Type: tea.KeyLeft}
	model, _ = app.Update(msg)
	app = model.(AppModel)

	// Focus should move to pane 0
	if app.focusedPane != 0 {
		t.Errorf("ctrl+b + left should move focus to pane 0, got %d", app.focusedPane)
	}
	if app.tmuxPrefix {
		t.Error("tmux prefix should be reset after use")
	}
}

func TestAppModel_TmuxPrefix_InputMode(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create two workstreams
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test1"})
	app = model.(AppModel)
	model, _ = app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test2"})
	app = model.(AppModel)

	// Enter input mode
	app.inputMode = true
	app.focusedPane = 1

	// Press ctrl+b (tmux prefix) - should work in input mode
	msg := tea.KeyMsg{Type: tea.KeyCtrlB}
	model, _ = app.Update(msg)
	app = model.(AppModel)

	if !app.tmuxPrefix {
		t.Error("ctrl+b should set tmux prefix in input mode")
	}

	// Press right arrow
	msg = tea.KeyMsg{Type: tea.KeyRight}
	model, _ = app.Update(msg)
	app = model.(AppModel)

	// Focus should wrap to pane 0
	if app.focusedPane != 0 {
		t.Errorf("ctrl+b + right should move focus to pane 0 (wrapped), got %d", app.focusedPane)
	}
}

func TestAppModel_TmuxPrefix_Timeout(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create two workstreams
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test1"})
	app = model.(AppModel)
	model, _ = app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test2"})
	app = model.(AppModel)

	// Set prefix with old timestamp (simulating timeout)
	app.tmuxPrefix = true
	app.tmuxPrefixTime = time.Now().Add(-3 * time.Second) // Expired

	originalFocus := app.focusedPane

	// Press left arrow - in nav mode, arrow keys ALWAYS switch panes
	// (the tmux prefix is only required in input mode)
	msg := tea.KeyMsg{Type: tea.KeyLeft}
	model, _ = app.Update(msg)
	app = model.(AppModel)

	// Prefix should be reset
	if app.tmuxPrefix {
		t.Error("Expired tmux prefix should be reset")
	}
	// Focus SHOULD change in nav mode (arrow keys work without prefix)
	expectedFocus := originalFocus - 1
	if expectedFocus < 0 {
		expectedFocus = len(app.panes) - 1
	}
	if app.focusedPane != expectedFocus {
		t.Errorf("Focus should change with arrow key in nav mode, expected %d, got %d", expectedFocus, app.focusedPane)
	}
}

func TestPaneModel_InputModeVisual(t *testing.T) {
	ws := workstream.New("test prompt")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)
	pane.SetIndex(1)

	// Test nav mode (focused, not in input mode)
	pane.SetFocused(true)
	pane.SetInputMode(false)
	view := pane.View()

	if !strings.Contains(view, "NAV") {
		t.Error("Focused pane in nav mode should show NAV indicator")
	}

	// Test input mode (focused, in input mode)
	pane.SetInputMode(true)
	view = pane.View()

	if !strings.Contains(view, "INPUT") {
		t.Error("Focused pane in input mode should show INPUT indicator")
	}

	// Test unfocused pane (should not show mode indicator)
	pane.SetFocused(false)
	pane.SetInputMode(false)
	view = pane.View()

	if strings.Contains(view, "NAV") || strings.Contains(view, "INPUT") {
		t.Error("Unfocused pane should not show mode indicator")
	}
}

func TestAppModel_TmuxPrefix_UpDown(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create three workstreams
	for _, name := range []string{"test1", "test2", "test3"} {
		model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: name})
		app = model.(AppModel)
	}

	// Focus is on pane 2 (last created)
	if app.focusedPane != 2 {
		t.Fatalf("Expected focusedPane=2, got %d", app.focusedPane)
	}

	// Press ctrl+b then up
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	app = model.(AppModel)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyUp})
	app = model.(AppModel)

	// Should move to pane 1
	if app.focusedPane != 1 {
		t.Errorf("ctrl+b + up should move focus to pane 1, got %d", app.focusedPane)
	}

	// Press ctrl+b then down
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	app = model.(AppModel)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = model.(AppModel)

	// Should move to pane 2
	if app.focusedPane != 2 {
		t.Errorf("ctrl+b + down should move focus to pane 2, got %d", app.focusedPane)
	}
}

func TestAppModel_TmuxPrefix_Wrapping(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create two workstreams
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test1"})
	app = model.(AppModel)
	model, _ = app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test2"})
	app = model.(AppModel)

	// Focus is on pane 1
	app.focusedPane = 1
	app.panes[0].SetFocused(false)
	app.panes[1].SetFocused(true)

	// Press ctrl+b then right - should wrap to pane 0
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	app = model.(AppModel)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRight})
	app = model.(AppModel)

	if app.focusedPane != 0 {
		t.Errorf("ctrl+b + right should wrap to pane 0, got %d", app.focusedPane)
	}

	// Press ctrl+b then left - should wrap to pane 1
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	app = model.(AppModel)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyLeft})
	app = model.(AppModel)

	if app.focusedPane != 1 {
		t.Errorf("ctrl+b + left should wrap to pane 1, got %d", app.focusedPane)
	}
}

func TestAppModel_TmuxPrefix_SinglePane(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create only one workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test1"})
	app = model.(AppModel)

	// Press ctrl+b then left
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	app = model.(AppModel)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyLeft})
	app = model.(AppModel)

	// Focus should stay on pane 0 (only pane)
	if app.focusedPane != 0 {
		t.Errorf("With single pane, focus should stay at 0, got %d", app.focusedPane)
	}
}

func TestAppModel_TmuxPrefix_NoPanes(t *testing.T) {
	app := NewAppModel(context.Background())

	// Press ctrl+b then left with no panes - should not panic
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	app = model.(AppModel)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyLeft})
	app = model.(AppModel)

	if app.focusedPane != 0 {
		t.Errorf("With no panes, focus should stay at 0, got %d", app.focusedPane)
	}
}

func TestAppModel_TmuxPrefix_ResetOnArrowKey(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create two workstreams
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test1"})
	app = model.(AppModel)
	model, _ = app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test2"})
	app = model.(AppModel)

	// Press ctrl+b
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	app = model.(AppModel)

	if !app.tmuxPrefix {
		t.Error("tmux prefix should be set")
	}

	// Press arrow key - should reset prefix after use
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyLeft})
	app = model.(AppModel)

	if app.tmuxPrefix {
		t.Error("tmux prefix should be reset after arrow key")
	}
}

func TestAppModel_TmuxPrefix_OtherKeyStillWorks(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create two workstreams
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test1"})
	app = model.(AppModel)
	model, _ = app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test2"})
	app = model.(AppModel)

	originalFocus := app.focusedPane

	// Press ctrl+b
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	app = model.(AppModel)

	// Press 'n' (not an arrow key) - should still open dialog
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	app = model.(AppModel)

	// Focus should not change
	if app.focusedPane != originalFocus {
		t.Error("Focus should not change on non-arrow key after prefix")
	}
	// Dialog should open (n key works normally)
	if app.dialog == nil {
		t.Error("Dialog should open when pressing 'n'")
	}
}

func TestAppModel_InputMode_CtrlC(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Enter input mode
	app.inputMode = true

	// Press ctrl+c - should be sent to pane, not quit app
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	app = model.(AppModel)

	// App should not be quitting (ctrl+c was sent to pane)
	if app.quitting {
		t.Error("In input mode, ctrl+c should go to pane, not quit app")
	}
}

func TestAppModel_InputMode_EscapeToNavMode(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Enter input mode
	app.inputMode = true

	// Single escape should NOT exit (sends to pane)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	app = model.(AppModel)

	if !app.inputMode {
		t.Error("Single escape should NOT exit input mode (it sends to pane)")
	}

	// Double escape should exit to nav mode
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	app = model.(AppModel)

	if app.inputMode {
		t.Error("Double escape should exit input mode")
	}
}

func TestAppModel_InputMode_ArrowWithoutTmuxPrefix(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create two workstreams
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test1"})
	app = model.(AppModel)
	model, _ = app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test2"})
	app = model.(AppModel)

	// Enter input mode
	app.inputMode = true
	originalFocus := app.focusedPane

	// Press left arrow without tmux prefix - should not change focus
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyLeft})
	app = model.(AppModel)

	// Focus should not change (arrow goes to pane in input mode)
	if app.focusedPane != originalFocus {
		t.Error("Arrow without tmux prefix in input mode should not change focus")
	}
}

func TestAppModel_NavMode_ArrowWithoutTmuxPrefix(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create two workstreams
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test1"})
	app = model.(AppModel)
	model, _ = app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test2"})
	app = model.(AppModel)

	originalFocus := app.focusedPane

	// Press left arrow without tmux prefix in nav mode
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyLeft})
	app = model.(AppModel)

	// In nav mode, arrow keys SHOULD change focus (no prefix needed)
	expectedFocus := originalFocus - 1
	if expectedFocus < 0 {
		expectedFocus = len(app.panes) - 1
	}
	if app.focusedPane != expectedFocus {
		t.Errorf("Arrow in nav mode should change focus, expected %d, got %d", expectedFocus, app.focusedPane)
	}
}

func TestAppModel_StateLoadedMsg_NoState(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Simulate state loaded with no saved state
	model, _ := app.Update(StateLoadedMsg{State: nil, Error: nil})
	app = model.(AppModel)

	// Should have no panes
	if len(app.panes) != 0 {
		t.Errorf("Expected 0 panes with no state, got %d", len(app.panes))
	}
}

func TestAppModel_StateLoadedMsg_WithError(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Simulate state loaded with error
	model, _ := app.Update(StateLoadedMsg{State: nil, Error: fmt.Errorf("test error")})
	app = model.(AppModel)

	// Should show toast with error
	if app.toast == "" {
		t.Error("Should show toast on state load error")
	}
	if !strings.Contains(app.toast, "Failed to load") {
		t.Errorf("Toast should mention load failure, got: %s", app.toast)
	}
}

func TestAppModel_StateSavedMsg_Success(t *testing.T) {
	app := NewAppModel(context.Background())
	app.quitting = true

	// Simulate state saved successfully
	model, cmd := app.Update(StateSavedMsg{Error: nil})
	app = model.(AppModel)

	// Should return quit command
	if cmd == nil {
		t.Error("Should return quit command after state saved")
	}
}

func TestAppModel_StateSavedMsg_WithError(t *testing.T) {
	app := NewAppModel(context.Background())
	app.quitting = true

	// Simulate state saved with error
	model, cmd := app.Update(StateSavedMsg{Error: fmt.Errorf("save error")})
	app = model.(AppModel)

	// Should show toast with error but still quit
	if app.toast == "" {
		t.Error("Should show toast on save error")
	}
	if cmd == nil {
		t.Error("Should still quit even with save error")
	}
}

func TestAppModel_ToastHint_UnhandledKey(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Press an unhandled key in nav mode
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	app = model.(AppModel)

	// Should show toast hint
	if app.toast == "" {
		t.Error("Should show toast hint for unhandled key")
	}
	if !strings.Contains(app.toast, "input mode") {
		t.Errorf("Toast should mention input mode, got: %s", app.toast)
	}
}

func TestAppModel_View_ToastOverlay(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 80
	app.height = 24
	app.toast = "Test toast message"
	app.toastExpiry = time.Now().Add(5 * time.Second)

	view := app.View()

	if !strings.Contains(view, "Test toast message") {
		t.Error("View should contain active toast message")
	}
}

func TestAppModel_View_ExpiredToast(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 80
	app.height = 24
	app.toast = "Expired toast"
	app.toastExpiry = time.Now().Add(-1 * time.Second) // Already expired

	view := app.View()

	if strings.Contains(view, "Expired toast") {
		t.Error("View should not contain expired toast message")
	}
}

func TestAppModel_TitleBar_InputModeIndicator(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Nav mode
	app.inputMode = false
	view := app.View()
	if !strings.Contains(view, "NAV") {
		t.Error("Title bar should show NAV in nav mode")
	}

	// Input mode
	app.inputMode = true
	view = app.View()
	if !strings.Contains(view, "INPUT") {
		t.Error("Title bar should show INPUT in input mode")
	}
}

func TestAppModel_InputMode_WithDialog(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Enter input mode
	app.inputMode = true

	// Open dialog (press escape twice to exit input mode, then 'n')
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	app = model.(AppModel)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyEscape}) // Second escape to exit
	app = model.(AppModel)
	model, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	app = model.(AppModel)

	// Dialog should be open
	if app.dialog == nil {
		t.Error("Dialog should be open")
	}

	// Input mode should be false when dialog is open
	// (Keys go to dialog, not to pane)
	if app.inputMode {
		t.Error("Input mode should be disabled when exiting to nav mode")
	}
}

func TestAppModel_ShiftTab(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create three workstreams
	for _, name := range []string{"first", "second", "third"} {
		model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: name})
		app = model.(AppModel)
	}

	// Tab cycles forward
	app.focusedPane = 0
	for i := range app.panes {
		app.panes[i].SetFocused(i == 0)
	}

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = model.(AppModel)

	if app.focusedPane != 1 {
		t.Errorf("Tab should move to pane 1, got %d", app.focusedPane)
	}
}

func TestPaneModel_Index(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	// Initially index is 0
	if pane.index != 0 {
		t.Errorf("Initial index should be 0, got %d", pane.index)
	}

	// Set index
	pane.SetIndex(3)
	if pane.index != 3 {
		t.Errorf("Index should be 3, got %d", pane.index)
	}

	// Verify index shows in view
	pane.SetSize(80, 24)
	pane.SetFocused(true)
	view := pane.View()

	if !strings.Contains(view, "3") {
		t.Error("View should contain pane index")
	}
}

func TestPaneModel_Dimensions(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	// Set size
	pane.SetSize(120, 50)

	if pane.Width() != 120 {
		t.Errorf("Width should be 120, got %d", pane.Width())
	}
	if pane.Height() != 50 {
		t.Errorf("Height should be 50, got %d", pane.Height())
	}
}
