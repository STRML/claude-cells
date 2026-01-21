package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/STRML/claude-cells/internal/workstream"
)

// Test helpers for creating key messages in bubbletea v2

// viewString extracts string content from tea.View for testing
// In v2, View() returns tea.View which contains a Layer
func viewString(v interface{}) string {
	switch val := v.(type) {
	case tea.View:
		return fmt.Sprintf("%v", val.Content)
	case string:
		return val
	default:
		return fmt.Sprintf("%v", v)
	}
}

// keyPress creates a KeyPressMsg for a single character
func keyPress(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

// keyPressStr creates a KeyPressMsg for a string (first char)
func keyPressStr(s string) tea.KeyPressMsg {
	if len(s) == 0 {
		return tea.KeyPressMsg{}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

// specialKey creates a KeyPressMsg for special keys (Enter, Esc, etc.)
func specialKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

// ctrlKey creates a KeyPressMsg for Ctrl+key combinations
func ctrlKey(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
}

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

func TestAppModel_Update_QuitNoWorkstreams(t *testing.T) {
	// When no workstreams exist, quit immediately without confirmation
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
			var msg tea.KeyPressMsg
			if tt.key == "ctrl+c" {
				msg = ctrlKey('c')
			} else {
				msg = keyPressStr(tt.key)
			}
			model, cmd := app.Update(msg)

			appModel := model.(AppModel)
			// Should quit immediately without dialog when no workstreams
			if !appModel.quitting {
				t.Errorf("Should be quitting immediately after '%s' with no workstreams", tt.key)
			}
			if appModel.dialog != nil {
				t.Errorf("Dialog should not be open after '%s' with no workstreams", tt.key)
			}
			if cmd == nil {
				t.Errorf("Should return quit command after '%s' with no workstreams", tt.key)
			}
		})
	}
}

func TestAppModel_Update_QuitWithWorkstreams(t *testing.T) {
	// When workstreams exist, show confirmation dialog
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
			app.width = 100
			app.height = 40

			// Create a workstream first
			model, _ := app.Update(DialogConfirmMsg{
				Type:  DialogNewWorkstream,
				Value: "test feature",
			})
			app = model.(AppModel)

			if len(app.panes) == 0 {
				t.Fatal("Should have created a workstream")
			}

			var msg tea.KeyPressMsg
			if tt.key == "ctrl+c" {
				msg = ctrlKey('c')
			} else {
				msg = keyPressStr(tt.key)
			}
			model, _ = app.Update(msg)

			appModel := model.(AppModel)
			// Should show quit confirmation dialog, not quit immediately
			if appModel.quitting {
				t.Errorf("Should not be quitting immediately after '%s' with workstreams", tt.key)
			}
			if appModel.dialog == nil {
				t.Errorf("Dialog should be open after '%s' with workstreams", tt.key)
			}
			if appModel.dialog.Type != DialogQuitConfirm {
				t.Errorf("Dialog should be DialogQuitConfirm type after '%s'", tt.key)
			}
		})
	}
}

func TestAppModel_Update_DoubleEscapeQuitNoWorkstreams(t *testing.T) {
	// Double-escape in nav mode with no workstreams should quit immediately
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// First escape - sets lastEscapeTime
	model, _ := app.Update(specialKey(tea.KeyEscape))
	app = model.(AppModel)

	if app.quitting {
		t.Error("Should not be quitting after first escape")
	}

	// Second escape (within timeout) - should quit immediately without dialog
	model, cmd := app.Update(specialKey(tea.KeyEscape))
	app = model.(AppModel)

	if !app.quitting {
		t.Error("Should be quitting after double-escape with no workstreams")
	}
	if app.dialog != nil {
		t.Error("Dialog should not be open after double-escape with no workstreams")
	}
	if cmd == nil {
		t.Error("Should return quit command after double-escape with no workstreams")
	}
}

func TestAppModel_Update_DoubleEscapeQuitWithWorkstreams(t *testing.T) {
	// Double-escape in nav mode with workstreams should show confirmation dialog
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream first
	model, _ := app.Update(DialogConfirmMsg{
		Type:  DialogNewWorkstream,
		Value: "test feature",
	})
	app = model.(AppModel)

	if len(app.panes) == 0 {
		t.Fatal("Should have created a workstream")
	}

	// First escape - sets lastEscapeTime
	model, _ = app.Update(specialKey(tea.KeyEscape))
	app = model.(AppModel)

	if app.quitting {
		t.Error("Should not be quitting after first escape")
	}

	// Second escape (within timeout) - should show dialog
	model, _ = app.Update(specialKey(tea.KeyEscape))
	app = model.(AppModel)

	if app.quitting {
		t.Error("Should not be quitting immediately after double-escape with workstreams")
	}
	if app.dialog == nil {
		t.Error("Dialog should be open after double-escape with workstreams")
	}
	if app.dialog.Type != DialogQuitConfirm {
		t.Error("Dialog should be DialogQuitConfirm type")
	}
}

func TestAppModel_Update_QuitDialogConfirm(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Confirm quit via DialogConfirmMsg
	model, cmd := app.Update(DialogConfirmMsg{
		Type: DialogQuitConfirm,
	})
	appModel := model.(AppModel)

	if !appModel.quitting {
		t.Error("Should be quitting after DialogQuitConfirm")
	}
	if cmd == nil {
		t.Error("Should return quit command")
	}
}

func TestAppModel_Update_NewWorkstreamDialog(t *testing.T) {
	app := NewAppModel(context.Background())

	// Press 'n' to open dialog
	model, _ := app.Update(keyPress('n'))
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
	model, _ = app.Update(keyPress('d'))
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
	model, _ := app.Update(keyPress('d'))
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
	model, _ := app.Update(keyPress('n'))
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
	model, _ = app.Update(keyPress('n'))
	app = model.(AppModel)

	if app.dialog == nil {
		t.Fatal("Dialog should be open")
	}

	// Try pressing 'd' while dialog is open - should not open destroy dialog
	model, _ = app.Update(keyPress('d'))
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
	model, _ = app.Update(specialKey(tea.KeyTab))
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
	model, _ = app.Update(specialKey(tea.KeyTab))
	app = model.(AppModel)

	if app.focusedPane != 1 {
		t.Errorf("Focus should be on pane 1 after second tab, got %d", app.focusedPane)
	}
}

func TestAppModel_Update_TabWithNoPanes(t *testing.T) {
	app := NewAppModel(context.Background())

	// Tab with no panes should not panic
	model, _ := app.Update(specialKey(tea.KeyTab))
	app = model.(AppModel)

	if app.focusedPane != 0 {
		t.Error("Focus should remain 0")
	}
}

func TestAppModel_Update_NumberKeyFocus(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create three workstreams (they get permanent indices 1, 2, 3)
	for _, name := range []string{"first", "second", "third"} {
		model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: name})
		app = model.(AppModel)
	}

	// Number keys focus by permanent pane index, which maps to slice position
	// when panes haven't been destroyed
	tests := []struct {
		key              string
		expectedPosition int // slice position
	}{
		{"1", 0}, // index 1 -> slice position 0
		{"2", 1}, // index 2 -> slice position 1
		{"3", 2}, // index 3 -> slice position 2
		{"1", 0}, // back to index 1
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			model, _ := app.Update(keyPressStr(tt.key))
			app = model.(AppModel)

			if app.focusedPane != tt.expectedPosition {
				t.Errorf("After pressing '%s', focusedPane (slice position) should be %d, got %d", tt.key, tt.expectedPosition, app.focusedPane)
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
	model, _ = app.Update(keyPress('5'))
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

	if !strings.Contains(viewString(view), "No workstreams") {
		t.Error("Empty view should show 'No workstreams' message")
	}
	if !strings.Contains(viewString(view), "[n]") {
		t.Error("Empty view should show key hint to create")
	}
}

func TestAppModel_View_Quitting(t *testing.T) {
	app := NewAppModel(context.Background())
	app.quitting = true

	view := app.View()

	if !strings.Contains(viewString(view), "Goodbye") {
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

	if strings.Contains(viewString(view), "No workstreams") {
		t.Error("View with panes should not show 'No workstreams'")
	}
}

func TestAppModel_View_WithDialog(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Open dialog
	model, _ := app.Update(keyPress('n'))
	app = model.(AppModel)

	view := app.View()

	if !strings.Contains(viewString(view), "New Workstream") {
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
	msg := keyPress('i')
	model, _ = app.Update(msg)
	app = model.(AppModel)

	if !app.inputMode {
		t.Error("Should be in input mode after pressing 'i'")
	}
	if !app.InputMode() {
		t.Error("InputMode() should return true")
	}

	// Press Escape twice to exit input mode (single escape sends to pane, double exits)
	msg = specialKey(tea.KeyEscape)
	model, _ = app.Update(msg)
	app = model.(AppModel)

	if !app.inputMode {
		t.Error("Single escape should NOT exit input mode (it sends to pane)")
	}

	// Second escape within timeout should exit input mode
	msg = specialKey(tea.KeyEscape)
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
	msg := specialKey(tea.KeyEnter)
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
	msg := keyPress('n')
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
		checkFn  func(app AppModel) bool
		errorMsg string
	}{
		{
			name:     "n opens new workstream dialog",
			key:      "n",
			checkFn:  func(app AppModel) bool { return app.dialog != nil },
			errorMsg: "n should open dialog",
		},
		{
			name:     "l does nothing without panes",
			key:      "l",
			checkFn:  func(app AppModel) bool { return app.dialog == nil },
			errorMsg: "l without panes should do nothing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := NewAppModel(context.Background())
			app.width = 100
			app.height = 40

			msg := keyPressStr(tt.key)
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
	msg := ctrlKey('b')
	model, _ = app.Update(msg)
	app = model.(AppModel)

	if !app.tmuxPrefix {
		t.Error("ctrl+b should set tmux prefix")
	}

	// Press left arrow
	msg = specialKey(tea.KeyLeft)
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

	// Use LayoutColumns which wraps on left/right
	app.layout = LayoutColumns

	// Enter input mode
	app.inputMode = true
	app.focusedPane = 1

	// Press ctrl+b (tmux prefix) - should work in input mode
	msg := ctrlKey('b')
	model, _ = app.Update(msg)
	app = model.(AppModel)

	if !app.tmuxPrefix {
		t.Error("ctrl+b should set tmux prefix in input mode")
	}

	// Press right arrow
	msg = specialKey(tea.KeyRight)
	model, _ = app.Update(msg)
	app = model.(AppModel)

	// Focus should wrap to pane 0 (LayoutColumns wraps on right)
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
	msg := specialKey(tea.KeyLeft)
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

	if !strings.Contains(viewString(view), "NAV") {
		t.Error("Focused pane in nav mode should show NAV indicator")
	}

	// Test input mode (focused, in input mode)
	pane.SetInputMode(true)
	view = pane.View()

	if !strings.Contains(viewString(view), "INPUT") {
		t.Error("Focused pane in input mode should show INPUT indicator")
	}

	// Test unfocused pane (should not show mode indicator)
	pane.SetFocused(false)
	pane.SetInputMode(false)
	view = pane.View()

	if strings.Contains(viewString(view), "NAV") || strings.Contains(viewString(view), "INPUT") {
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

	// Use LayoutRows which stacks panes vertically and wraps on up/down
	app.layout = LayoutRows

	// Focus is on pane 2 (last created)
	if app.focusedPane != 2 {
		t.Fatalf("Expected focusedPane=2, got %d", app.focusedPane)
	}

	// Press ctrl+b then up
	model, _ := app.Update(ctrlKey('b'))
	app = model.(AppModel)
	model, _ = app.Update(specialKey(tea.KeyUp))
	app = model.(AppModel)

	// Should move to pane 1 (spatially above in LayoutRows)
	if app.focusedPane != 1 {
		t.Errorf("ctrl+b + up should move focus to pane 1, got %d", app.focusedPane)
	}

	// Press ctrl+b then down
	model, _ = app.Update(ctrlKey('b'))
	app = model.(AppModel)
	model, _ = app.Update(specialKey(tea.KeyDown))
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

	// Use LayoutColumns which wraps on left/right
	app.layout = LayoutColumns

	// Focus is on pane 1
	app.focusedPane = 1
	app.panes[0].SetFocused(false)
	app.panes[1].SetFocused(true)

	// Press ctrl+b then right - should wrap to pane 0 (LayoutColumns wraps)
	model, _ = app.Update(ctrlKey('b'))
	app = model.(AppModel)
	model, _ = app.Update(specialKey(tea.KeyRight))
	app = model.(AppModel)

	if app.focusedPane != 0 {
		t.Errorf("ctrl+b + right should wrap to pane 0, got %d", app.focusedPane)
	}

	// Press ctrl+b then left - should wrap to pane 1
	model, _ = app.Update(ctrlKey('b'))
	app = model.(AppModel)
	model, _ = app.Update(specialKey(tea.KeyLeft))
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
	model, _ = app.Update(ctrlKey('b'))
	app = model.(AppModel)
	model, _ = app.Update(specialKey(tea.KeyLeft))
	app = model.(AppModel)

	// Focus should stay on pane 0 (only pane)
	if app.focusedPane != 0 {
		t.Errorf("With single pane, focus should stay at 0, got %d", app.focusedPane)
	}
}

func TestAppModel_TmuxPrefix_NoPanes(t *testing.T) {
	app := NewAppModel(context.Background())

	// Press ctrl+b then left with no panes - should not panic
	model, _ := app.Update(ctrlKey('b'))
	app = model.(AppModel)
	model, _ = app.Update(specialKey(tea.KeyLeft))
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
	model, _ = app.Update(ctrlKey('b'))
	app = model.(AppModel)

	if !app.tmuxPrefix {
		t.Error("tmux prefix should be set")
	}

	// Press arrow key - should reset prefix after use
	model, _ = app.Update(specialKey(tea.KeyLeft))
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
	model, _ = app.Update(ctrlKey('b'))
	app = model.(AppModel)

	// Press 'n' (not an arrow key) - should still open dialog
	model, _ = app.Update(keyPress('n'))
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
	model, _ = app.Update(ctrlKey('c'))
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
	model, _ = app.Update(specialKey(tea.KeyEscape))
	app = model.(AppModel)

	if !app.inputMode {
		t.Error("Single escape should NOT exit input mode (it sends to pane)")
	}

	// Double escape should exit to nav mode
	model, _ = app.Update(specialKey(tea.KeyEscape))
	app = model.(AppModel)

	if app.inputMode {
		t.Error("Double escape should exit input mode")
	}
}

func TestAppModel_InputMode_DeferredEscape(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Enter input mode
	app.inputMode = true

	// Single escape should set pendingEscape but NOT immediately send to pane
	model, cmd := app.Update(specialKey(tea.KeyEscape))
	app = model.(AppModel)

	if !app.inputMode {
		t.Error("Single escape should NOT exit input mode")
	}
	if !app.pendingEscape {
		t.Error("Single escape should set pendingEscape")
	}
	if cmd == nil {
		t.Error("Single escape should return a timer command")
	}

	// Double escape should exit WITHOUT triggering the deferred escape
	model, _ = app.Update(specialKey(tea.KeyEscape))
	app = model.(AppModel)

	if app.inputMode {
		t.Error("Double escape should exit input mode")
	}
	if app.pendingEscape {
		t.Error("Double escape should clear pendingEscape")
	}
}

func TestAppModel_InputMode_EscapeTimeoutSendsToPane(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Enter input mode
	app.inputMode = true

	// Single escape - sets pending
	escTime := time.Now()
	app.lastEscapeTime = escTime
	app.pendingEscape = true

	// Simulate timeout firing with matching timestamp
	model, _ = app.Update(escapeTimeoutMsg{timestamp: escTime})
	app = model.(AppModel)

	// Should still be in input mode (Esc was forwarded to pane, not used for exit)
	if !app.inputMode {
		t.Error("Escape timeout should NOT exit input mode")
	}
	if app.pendingEscape {
		t.Error("Escape timeout should clear pendingEscape")
	}
}

func TestAppModel_InputMode_EscapeTimeoutIgnoredAfterDoubleTap(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Enter input mode
	app.inputMode = true

	// Press Esc twice to exit (double tap)
	model, _ = app.Update(specialKey(tea.KeyEscape))
	app = model.(AppModel)
	escTime := app.lastEscapeTime

	model, _ = app.Update(specialKey(tea.KeyEscape))
	app = model.(AppModel)

	if app.inputMode {
		t.Error("Double escape should exit input mode")
	}

	// Now simulate the stale timeout arriving (should be ignored)
	model, _ = app.Update(escapeTimeoutMsg{timestamp: escTime})
	app = model.(AppModel)

	// Should still be in nav mode (timeout was ignored)
	if app.inputMode {
		t.Error("Stale escape timeout should not re-enter input mode")
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
	model, _ = app.Update(specialKey(tea.KeyLeft))
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
	model, _ = app.Update(specialKey(tea.KeyLeft))
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
	// Error message is printed to stderr, not shown as toast
	_, cmd := app.Update(StateSavedMsg{Error: fmt.Errorf("save error")})

	// Should still quit even with save error
	if cmd == nil {
		t.Error("Should still quit even with save error")
	}
}

func TestAppModel_StateSavedMsg_WithRepairMessage(t *testing.T) {
	app := NewAppModel(context.Background())
	app.quitting = true

	// Simulate state saved with repair message
	// Repair message is printed to stderr
	_, cmd := app.Update(StateSavedMsg{RepairMessage: "2 session ID(s) repaired"})

	// Should quit after saving
	if cmd == nil {
		t.Error("Should quit after saving state")
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
	model, _ = app.Update(keyPress('x'))
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

	if !strings.Contains(viewString(view), "Test toast message") {
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

	if strings.Contains(viewString(view), "Expired toast") {
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
	if !strings.Contains(viewString(view), "NAV") {
		t.Error("Title bar should show NAV in nav mode")
	}

	// Input mode
	app.inputMode = true
	view = app.View()
	if !strings.Contains(viewString(view), "INPUT") {
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
	model, _ = app.Update(specialKey(tea.KeyEscape))
	app = model.(AppModel)
	model, _ = app.Update(specialKey(tea.KeyEscape)) // Second escape to exit
	app = model.(AppModel)
	model, _ = app.Update(keyPress('n'))
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

	model, _ := app.Update(specialKey(tea.KeyTab))
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

	if !strings.Contains(viewString(view), "3") {
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

func TestAppModel_PermanentPaneIndices(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create three workstreams - they should get indices 1, 2, 3
	for _, name := range []string{"first", "second", "third"} {
		model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: name})
		app = model.(AppModel)
	}

	// Verify permanent indices
	if app.panes[0].Index() != 1 {
		t.Errorf("First pane should have index 1, got %d", app.panes[0].Index())
	}
	if app.panes[1].Index() != 2 {
		t.Errorf("Second pane should have index 2, got %d", app.panes[1].Index())
	}
	if app.panes[2].Index() != 3 {
		t.Errorf("Third pane should have index 3, got %d", app.panes[2].Index())
	}
}

func TestAppModel_NumberKeyFocusByPermanentIndex(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create three workstreams (indices 1, 2, 3)
	for _, name := range []string{"first", "second", "third"} {
		model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: name})
		app = model.(AppModel)
	}

	// Destroy the first workstream (index 1)
	wsID := app.panes[0].Workstream().ID
	model, _ := app.Update(DialogConfirmMsg{Type: DialogDestroy, WorkstreamID: wsID})
	app = model.(AppModel)

	// Now we have 2 panes renumbered to indices 1 and 2 (at slice positions 0 and 1)
	if len(app.panes) != 2 {
		t.Fatalf("Expected 2 panes, got %d", len(app.panes))
	}
	if app.panes[0].Index() != 1 {
		t.Errorf("First remaining pane should be renumbered to index 1, got %d", app.panes[0].Index())
	}
	if app.panes[1].Index() != 2 {
		t.Errorf("Second remaining pane should be renumbered to index 2, got %d", app.panes[1].Index())
	}

	// Press '1' - should focus the pane with index 1 (slice position 0)
	model, _ = app.Update(keyPress('1'))
	app = model.(AppModel)

	if app.focusedPane != 0 {
		t.Errorf("Pressing '1' should focus slice position 0 (pane index 1), got focusedPane=%d", app.focusedPane)
	}

	// Press '2' - should focus the pane with index 2 (slice position 1)
	model, _ = app.Update(keyPress('2'))
	app = model.(AppModel)

	if app.focusedPane != 1 {
		t.Errorf("Pressing '2' should focus slice position 1 (pane index 2), got focusedPane=%d", app.focusedPane)
	}

	// Press '3' - should do nothing (no pane with index 3 exists after renumbering)
	model, _ = app.Update(keyPress('3'))
	app = model.(AppModel)

	if app.focusedPane != 1 {
		t.Errorf("Pressing '3' (non-existent index) should not change focus, got focusedPane=%d", app.focusedPane)
	}
}

func TestAppModel_PaneIndexGetter(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	// Default index is 0
	if pane.Index() != 0 {
		t.Errorf("Default Index() should return 0, got %d", pane.Index())
	}

	// Set index and verify getter
	pane.SetIndex(5)
	if pane.Index() != 5 {
		t.Errorf("Index() should return 5, got %d", pane.Index())
	}
}

func TestAppModel_SpaceSwapPreservesIndices(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create two workstreams (indices 1, 2)
	for _, name := range []string{"first", "second"} {
		model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: name})
		app = model.(AppModel)
	}

	// Focus pane at slice position 1 (index 2)
	app.panes[0].SetFocused(false)
	app.panes[1].SetFocused(true)
	app.focusedPane = 1

	// Verify indices before swap
	if app.panes[0].Index() != 1 {
		t.Errorf("Before swap: pane at position 0 should have index 1, got %d", app.panes[0].Index())
	}
	if app.panes[1].Index() != 2 {
		t.Errorf("Before swap: pane at position 1 should have index 2, got %d", app.panes[1].Index())
	}

	// Press Space to swap focused pane (index 2) to main position
	model, _ := app.Update(keyPress(' '))
	app = model.(AppModel)

	// After swap, pane indices should be preserved but positions swapped
	// Pane at position 0 should now have index 2, position 1 should have index 1
	if app.panes[0].Index() != 2 {
		t.Errorf("After swap: pane at position 0 should have index 2, got %d", app.panes[0].Index())
	}
	if app.panes[1].Index() != 1 {
		t.Errorf("After swap: pane at position 1 should have index 1, got %d", app.panes[1].Index())
	}

	// Focus should be at position 0 now
	if app.focusedPane != 0 {
		t.Errorf("After swap, focus should be at position 0, got %d", app.focusedPane)
	}
}

// ============================================================================
// Mouse Click-to-Focus Tests
// ============================================================================

func TestAppModel_MouseClick_FocusesPane(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create two workstreams
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "first"})
	app = model.(AppModel)
	model, _ = app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "second"})
	app = model.(AppModel)

	// Focus is on pane 1 (second created)
	if app.focusedPane != 1 {
		t.Fatalf("Expected focusedPane=1, got %d", app.focusedPane)
	}

	// Calculate bounds for the first pane (pane 0)
	titleBarHeight := 1
	statusBarHeight := 1
	availableHeight := app.height - titleBarHeight - statusBarHeight
	bounds := CalculatePaneBounds(app.layout, len(app.panes), app.width, availableHeight, titleBarHeight)

	// Click in the center of the first pane
	centerX := bounds[0].X + bounds[0].Width/2
	centerY := bounds[0].Y + bounds[0].Height/2

	mouseMsg := tea.MouseClickMsg{
		X:      centerX,
		Y:      centerY,
		Button: tea.MouseLeft,
	}

	model, _ = app.Update(mouseMsg)
	app = model.(AppModel)

	// Focus should have moved to pane 0
	if app.focusedPane != 0 {
		t.Errorf("After clicking pane 0, focusedPane should be 0, got %d", app.focusedPane)
	}

	// Pane 0 should be focused, pane 1 should not
	if !app.panes[0].focused {
		t.Error("Pane 0 should have focus flag set")
	}
	if app.panes[1].focused {
		t.Error("Pane 1 should not have focus flag set")
	}
}

func TestAppModel_MouseClick_EntersInputMode(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Start in nav mode
	app.inputMode = false

	// Calculate bounds
	titleBarHeight := 1
	statusBarHeight := 1
	availableHeight := app.height - titleBarHeight - statusBarHeight
	bounds := CalculatePaneBounds(app.layout, len(app.panes), app.width, availableHeight, titleBarHeight)

	// Click in the pane
	mouseMsg := tea.MouseClickMsg{
		X:      bounds[0].X + 10,
		Y:      bounds[0].Y + 5,
		Button: tea.MouseLeft,
	}

	model, cmd := app.Update(mouseMsg)
	app = model.(AppModel)

	// Should enter input mode
	if !app.inputMode {
		t.Error("Clicking on pane should enter input mode")
	}

	// Note: In bubble-tea v2, cursor visibility is controlled via View().Cursor
	// rather than returning a ShowCursor command. The command being nil is fine.
	_ = cmd
}

func TestAppModel_MouseClick_OutsidePanes(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	originalFocus := app.focusedPane

	// Click on the title bar (Y=0)
	mouseMsg := tea.MouseClickMsg{
		X:      50,
		Y:      0,
		Button: tea.MouseLeft,
	}

	model, _ = app.Update(mouseMsg)
	app = model.(AppModel)

	// Focus should not change
	if app.focusedPane != originalFocus {
		t.Errorf("Clicking outside panes should not change focus, expected %d, got %d", originalFocus, app.focusedPane)
	}
}

func TestAppModel_MouseClick_OnStatusBar(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	originalFocus := app.focusedPane

	// Click on the status bar (bottom line)
	mouseMsg := tea.MouseClickMsg{
		X:      50,
		Y:      app.height - 1,
		Button: tea.MouseLeft,
	}

	model, _ = app.Update(mouseMsg)
	app = model.(AppModel)

	// Focus should not change
	if app.focusedPane != originalFocus {
		t.Errorf("Clicking on status bar should not change focus, expected %d, got %d", originalFocus, app.focusedPane)
	}
}

func TestAppModel_MouseClick_WithDialog(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create two workstreams
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "first"})
	app = model.(AppModel)
	model, _ = app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "second"})
	app = model.(AppModel)

	// Open a dialog
	model, _ = app.Update(keyPress('n'))
	app = model.(AppModel)

	if app.dialog == nil {
		t.Fatal("Dialog should be open")
	}

	originalFocus := app.focusedPane

	// Try to click on a pane
	titleBarHeight := 1
	statusBarHeight := 1
	availableHeight := app.height - titleBarHeight - statusBarHeight
	bounds := CalculatePaneBounds(app.layout, len(app.panes), app.width, availableHeight, titleBarHeight)

	mouseMsg := tea.MouseClickMsg{
		X:      bounds[0].X + 10,
		Y:      bounds[0].Y + 5,
		Button: tea.MouseLeft,
	}

	model, _ = app.Update(mouseMsg)
	app = model.(AppModel)

	// Focus should not change (dialog blocks mouse)
	if app.focusedPane != originalFocus {
		t.Errorf("Clicking with dialog open should not change focus, expected %d, got %d", originalFocus, app.focusedPane)
	}

	// Dialog should still be open
	if app.dialog == nil {
		t.Error("Dialog should still be open after click")
	}
}

func TestAppModel_MouseClick_RightButton(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create two workstreams
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "first"})
	app = model.(AppModel)
	model, _ = app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "second"})
	app = model.(AppModel)

	originalFocus := app.focusedPane

	// Right click should be ignored
	titleBarHeight := 1
	statusBarHeight := 1
	availableHeight := app.height - titleBarHeight - statusBarHeight
	bounds := CalculatePaneBounds(app.layout, len(app.panes), app.width, availableHeight, titleBarHeight)

	mouseMsg := tea.MouseClickMsg{
		X:      bounds[0].X + 10,
		Y:      bounds[0].Y + 5,
		Button: tea.MouseRight,
	}

	model, _ = app.Update(mouseMsg)
	app = model.(AppModel)

	// Focus should not change (only left click focuses)
	if app.focusedPane != originalFocus {
		t.Errorf("Right click should not change focus, expected %d, got %d", originalFocus, app.focusedPane)
	}
}

func TestAppModel_MouseClick_Release(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create two workstreams
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "first"})
	app = model.(AppModel)
	model, _ = app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "second"})
	app = model.(AppModel)

	originalFocus := app.focusedPane

	// Mouse release should be ignored (only press focuses)
	titleBarHeight := 1
	statusBarHeight := 1
	availableHeight := app.height - titleBarHeight - statusBarHeight
	bounds := CalculatePaneBounds(app.layout, len(app.panes), app.width, availableHeight, titleBarHeight)

	// In v2, mouse release is a different message type
	mouseMsg := tea.MouseReleaseMsg{
		X:      bounds[0].X + 10,
		Y:      bounds[0].Y + 5,
		Button: tea.MouseLeft,
	}

	model, _ = app.Update(mouseMsg)
	app = model.(AppModel)

	// Focus should not change (only click focuses, not release)
	if app.focusedPane != originalFocus {
		t.Errorf("Mouse release should not change focus, expected %d, got %d", originalFocus, app.focusedPane)
	}
}

func TestAppModel_MouseClick_NoPanes(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// No panes - click should not panic
	mouseMsg := tea.MouseClickMsg{
		X:      50,
		Y:      20,
		Button: tea.MouseLeft,
	}

	model, _ := app.Update(mouseMsg)
	app = model.(AppModel)

	// Should not panic and focus should remain 0
	if app.focusedPane != 0 {
		t.Errorf("Focus should remain 0 with no panes, got %d", app.focusedPane)
	}
}

func TestAppModel_MouseClick_AllLayouts(t *testing.T) {
	layouts := []LayoutType{LayoutGrid, LayoutMainLeft, LayoutMainTop, LayoutRows, LayoutColumns}

	for _, layout := range layouts {
		t.Run(layout.String(), func(t *testing.T) {
			app := NewAppModel(context.Background())
			app.width = 120
			app.height = 60
			app.layout = layout

			// Create three workstreams
			for _, name := range []string{"first", "second", "third"} {
				model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: name})
				app = model.(AppModel)
			}

			// Calculate bounds
			titleBarHeight := 1
			statusBarHeight := 1
			availableHeight := app.height - titleBarHeight - statusBarHeight
			bounds := CalculatePaneBounds(app.layout, len(app.panes), app.width, availableHeight, titleBarHeight)

			// Click on each pane and verify focus changes
			for targetPane := 0; targetPane < len(app.panes); targetPane++ {
				// Reset focus to pane 0
				for i := range app.panes {
					app.panes[i].SetFocused(i == 0)
				}
				app.focusedPane = 0

				// Click on target pane center
				centerX := bounds[targetPane].X + bounds[targetPane].Width/2
				centerY := bounds[targetPane].Y + bounds[targetPane].Height/2

				mouseMsg := tea.MouseClickMsg{
					X:      centerX,
					Y:      centerY,
					Button: tea.MouseLeft,
				}

				model, _ := app.Update(mouseMsg)
				app = model.(AppModel)

				if app.focusedPane != targetPane {
					t.Errorf("Layout %s: clicking pane %d at (%d,%d) should focus it, got focusedPane=%d",
						layout, targetPane, centerX, centerY, app.focusedPane)
				}
			}
		})
	}
}

func TestAppModel_MouseClick_EdgeCases(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create two workstreams
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "first"})
	app = model.(AppModel)
	model, _ = app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "second"})
	app = model.(AppModel)

	titleBarHeight := 1
	statusBarHeight := 1
	availableHeight := app.height - titleBarHeight - statusBarHeight
	bounds := CalculatePaneBounds(app.layout, len(app.panes), app.width, availableHeight, titleBarHeight)

	tests := []struct {
		name         string
		x, y         int
		expectedPane int // -1 means no change
	}{
		{"pane 0 top-left corner", bounds[0].X, bounds[0].Y, 0},
		{"pane 0 just inside right edge", bounds[0].X + bounds[0].Width - 1, bounds[0].Y + 5, 0},
		{"pane 1 left edge (boundary)", bounds[1].X, bounds[1].Y + 5, 1},
		{"pane 1 top-right corner", bounds[1].X + bounds[1].Width - 1, bounds[1].Y, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset focus to pane 1 (so we can detect change to pane 0)
			for i := range app.panes {
				app.panes[i].SetFocused(i == 1)
			}
			app.focusedPane = 1

			mouseMsg := tea.MouseClickMsg{
				X:      tt.x,
				Y:      tt.y,
				Button: tea.MouseLeft,
			}

			model, _ := app.Update(mouseMsg)
			app = model.(AppModel)

			if app.focusedPane != tt.expectedPane {
				t.Errorf("Click at (%d,%d) expected pane %d, got %d", tt.x, tt.y, tt.expectedPane, app.focusedPane)
			}
		})
	}
}

func TestAppModel_MouseClick_WithInPaneDialog(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create two workstreams
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "first"})
	app = model.(AppModel)
	model, _ = app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "second"})
	app = model.(AppModel)

	// Focus first pane
	app.focusedPane = 0

	// Set an in-pane dialog on the first pane (simulating a progress dialog)
	inPaneDialog := NewProgressDialog("Processing...", "Please wait", "test-id")
	app.panes[0].SetInPaneDialog(&inPaneDialog)

	if !app.panes[0].HasInPaneDialog() {
		t.Fatal("Pane 0 should have an in-pane dialog")
	}

	// Click on the second pane - should still switch focus (in-pane dialogs are non-modal)
	titleBarHeight := 1
	statusBarHeight := 1
	availableHeight := app.height - titleBarHeight - statusBarHeight
	bounds := CalculatePaneBounds(app.layout, len(app.panes), app.width, availableHeight, titleBarHeight)

	// Click on second pane
	mouseMsg := tea.MouseClickMsg{
		X:      bounds[1].X + 10,
		Y:      bounds[1].Y + 5,
		Button: tea.MouseLeft,
	}

	model, _ = app.Update(mouseMsg)
	app = model.(AppModel)

	// Focus should change to second pane (in-pane dialogs don't block mouse clicks)
	if app.focusedPane != 1 {
		t.Errorf("Clicking on second pane with in-pane dialog on first should switch focus, expected 1, got %d", app.focusedPane)
	}

	// The in-pane dialog should still be on pane 0
	if !app.panes[0].HasInPaneDialog() {
		t.Error("In-pane dialog should still be on pane 0")
	}
}

func TestAppModel_View_MouseModeEnabled(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Get the view
	view := app.View()

	// The view should have mouse mode enabled (MouseModeCellMotion) by default
	if view.MouseMode != tea.MouseModeCellMotion {
		t.Errorf("View should have MouseModeCellMotion enabled, got %v", view.MouseMode)
	}
}

func TestAppModel_MouseModeToggle_CtrlBM_NavMode(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Mouse should be enabled by default
	if !app.mouseEnabled {
		t.Error("Mouse should be enabled by default")
	}

	// Press Ctrl+B to set prefix
	model, _ = app.Update(ctrlKey('b'))
	app = model.(AppModel)

	if !app.tmuxPrefix {
		t.Error("tmuxPrefix should be set after Ctrl+B")
	}

	// Press 'm' to toggle mouse mode off
	model, _ = app.Update(keyPress('m'))
	app = model.(AppModel)

	if app.mouseEnabled {
		t.Error("Mouse should be disabled after Ctrl+B m")
	}
	if app.toast == "" {
		t.Error("Toast message should be shown")
	}

	// View should return MouseModeNone when disabled
	view := app.View()
	if view.MouseMode != tea.MouseModeNone {
		t.Errorf("View should have MouseModeNone when disabled, got %v", view.MouseMode)
	}

	// Toggle back on
	model, _ = app.Update(ctrlKey('b'))
	app = model.(AppModel)
	model, _ = app.Update(keyPress('m'))
	app = model.(AppModel)

	if !app.mouseEnabled {
		t.Error("Mouse should be enabled after toggling back on")
	}

	view = app.View()
	if view.MouseMode != tea.MouseModeCellMotion {
		t.Errorf("View should have MouseModeCellMotion when enabled, got %v", view.MouseMode)
	}
}

func TestAppModel_MouseModeToggle_CtrlBM_InputMode(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Enter input mode
	app.inputMode = true

	// Press Ctrl+B then m
	model, _ = app.Update(ctrlKey('b'))
	app = model.(AppModel)
	model, _ = app.Update(keyPress('m'))
	app = model.(AppModel)

	if app.mouseEnabled {
		t.Error("Mouse should be disabled after Ctrl+B m in input mode")
	}
}

func TestAppModel_MouseClick_DisabledWhenMouseModeOff(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create two workstreams
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "first"})
	app = model.(AppModel)
	model, _ = app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "second"})
	app = model.(AppModel)

	// Disable mouse mode
	app.mouseEnabled = false
	originalFocus := app.focusedPane

	// Try to click on a pane
	titleBarHeight := 1
	statusBarHeight := 1
	availableHeight := app.height - titleBarHeight - statusBarHeight
	bounds := CalculatePaneBounds(app.layout, len(app.panes), app.width, availableHeight, titleBarHeight)

	mouseMsg := tea.MouseClickMsg{
		X:      bounds[0].X + 10,
		Y:      bounds[0].Y + 5,
		Button: tea.MouseLeft,
	}

	model, _ = app.Update(mouseMsg)
	app = model.(AppModel)

	// Focus should NOT change when mouse mode is disabled
	if app.focusedPane != originalFocus {
		t.Errorf("Clicking should not change focus when mouse mode disabled, expected %d, got %d", originalFocus, app.focusedPane)
	}
}

func TestAppModel_MouseDragHint_ShownOnce(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Hint should not be shown yet
	if app.dragHintShown {
		t.Error("Drag hint should not be shown initially")
	}

	// Simulate drag motion
	motionMsg := tea.MouseMotionMsg{
		X:      50,
		Y:      20,
		Button: tea.MouseLeft,
	}

	model, _ = app.Update(motionMsg)
	app = model.(AppModel)

	// Hint should now be shown
	if !app.dragHintShown {
		t.Error("Drag hint should be shown after drag")
	}
	if app.toast == "" {
		t.Error("Toast should contain drag hint")
	}

	// Clear toast and simulate another drag
	app.toast = ""
	model, _ = app.Update(motionMsg)
	app = model.(AppModel)

	// Toast should NOT be shown again
	if app.toast != "" {
		t.Error("Drag hint should only be shown once per session")
	}
}

// ============================================================================
// Scroll/Copy Mode Tests
// ============================================================================

func TestAppModel_ScrollMode_EnterWithBracket_NavMode(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Should not be in scroll mode initially
	if app.panes[0].IsScrollMode() {
		t.Error("Pane should not be in scroll mode initially")
	}

	// Press '[' in nav mode to enter scroll mode
	model, _ = app.Update(keyPress('['))
	app = model.(AppModel)

	if !app.panes[0].IsScrollMode() {
		t.Error("Pressing '[' in nav mode should enter scroll mode")
	}
}

func TestAppModel_ScrollMode_EnterWithCtrlBBracket_InputMode(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Enter input mode
	app.inputMode = true

	// Press ctrl+b then '[' to enter scroll mode
	model, _ = app.Update(ctrlKey('b'))
	app = model.(AppModel)

	if !app.tmuxPrefix {
		t.Error("ctrl+b should set tmux prefix")
	}

	model, _ = app.Update(keyPress('['))
	app = model.(AppModel)

	if !app.panes[0].IsScrollMode() {
		t.Error("Pressing ctrl+b [ in input mode should enter scroll mode")
	}
	if app.tmuxPrefix {
		t.Error("tmux prefix should be reset after use")
	}
}

func TestAppModel_ScrollMode_ArrowKeysScroll_NavMode(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Enter scroll mode
	app.panes[0].EnterScrollMode()

	if !app.panes[0].IsScrollMode() {
		t.Fatal("Pane should be in scroll mode")
	}

	// Press up arrow - should scroll, not navigate (there's only one pane anyway)
	model, _ = app.Update(specialKey(tea.KeyUp))
	app = model.(AppModel)

	// Should still be in scroll mode
	if !app.panes[0].IsScrollMode() {
		t.Error("Up arrow in scroll mode should keep scroll mode active")
	}

	// Press down arrow - should scroll
	model, _ = app.Update(specialKey(tea.KeyDown))
	app = model.(AppModel)

	// Might exit scroll mode if at bottom, but should not panic
}

func TestAppModel_ScrollMode_ArrowKeysScroll_InputMode(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Enter input mode and scroll mode
	app.inputMode = true
	app.panes[0].EnterScrollMode()

	// Press up arrow in input mode while in scroll mode
	model, _ = app.Update(specialKey(tea.KeyUp))
	app = model.(AppModel)

	// Should still be in scroll mode (arrow scrolled instead of going to pane)
	if !app.panes[0].IsScrollMode() {
		t.Error("Up arrow in scroll mode (input mode) should keep scroll mode active")
	}
}

func TestAppModel_ScrollMode_ExitWithEscape_NavMode(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Enter scroll mode
	app.panes[0].EnterScrollMode()

	if !app.panes[0].IsScrollMode() {
		t.Fatal("Pane should be in scroll mode")
	}

	// Press escape in nav mode - should exit scroll mode
	model, _ = app.Update(specialKey(tea.KeyEscape))
	app = model.(AppModel)

	if app.panes[0].IsScrollMode() {
		t.Error("Escape in nav mode should exit scroll mode")
	}
}

func TestAppModel_ScrollMode_TitleBarIndicator(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Not in scroll mode - just call View() to verify it doesn't crash
	_ = app.View()
	// The title bar SCROLL indicator should not be present
	// (Note: we can't easily test this without parsing the view, but we can at least
	// verify it doesn't crash)

	// Enter scroll mode
	app.panes[0].EnterScrollMode()
	view := app.View()

	// Should contain SCROLL indicator
	if !strings.Contains(viewString(view), "SCROLL") {
		t.Error("View should contain SCROLL indicator when in scroll mode")
	}
}

func TestAppModel_ScrollMode_BracketWithoutPrefix_InputMode(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Enter input mode
	app.inputMode = true

	// Press '[' without ctrl+b prefix - should pass through to pane, not enter scroll mode
	model, _ = app.Update(keyPress('['))
	app = model.(AppModel)

	if app.panes[0].IsScrollMode() {
		t.Error("Pressing '[' without ctrl+b prefix in input mode should not enter scroll mode")
	}
}

func TestAppModel_ScrollMode_ArrowsDontNavigate_InScrollMode(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create two workstreams
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "first"})
	app = model.(AppModel)
	model, _ = app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "second"})
	app = model.(AppModel)

	// Focus is on pane 1
	if app.focusedPane != 1 {
		t.Fatalf("Expected focusedPane=1, got %d", app.focusedPane)
	}

	// Enter scroll mode on focused pane
	app.panes[app.focusedPane].EnterScrollMode()

	// Press left arrow in nav mode while in scroll mode
	// Left/right arrows should still navigate panes (only up/down scroll)
	model, _ = app.Update(specialKey(tea.KeyLeft))
	app = model.(AppModel)

	// Focus should change to pane 0 (left/right still navigate)
	if app.focusedPane != 0 {
		t.Errorf("Left arrow should still navigate panes, expected focusedPane=0, got %d", app.focusedPane)
	}
}

func TestAppModel_ScrollMode_EnterWithPgUp_NavMode(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Ensure in nav mode (default)
	if app.inputMode {
		t.Fatal("Should start in nav mode")
	}

	// Should not be in scroll mode initially
	if app.panes[0].IsScrollMode() {
		t.Error("Pane should not be in scroll mode initially")
	}

	// Press pgup in nav mode - should enter scroll mode
	model, _ = app.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	app = model.(AppModel)

	if !app.panes[0].IsScrollMode() {
		t.Error("Pressing pgup in nav mode should enter scroll mode")
	}
}

func TestAppModel_ScrollMode_EnterWithPgUp_NavMode_KittyCode(t *testing.T) {
	// Test with the actual Kitty keyboard protocol code (1114122)
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Ensure in nav mode (default)
	if app.inputMode {
		t.Fatal("Should start in nav mode")
	}

	// Should not be in scroll mode initially
	if app.panes[0].IsScrollMode() {
		t.Error("Pane should not be in scroll mode initially")
	}

	// Press pgup with Kitty code 1114122 - should enter scroll mode
	kittyPgUp := tea.KeyPressMsg{Code: 1114122}
	t.Logf("Kitty pgup key string: %q", kittyPgUp.String())

	model, _ = app.Update(kittyPgUp)
	app = model.(AppModel)

	if !app.panes[0].IsScrollMode() {
		t.Errorf("Pressing pgup (Kitty code 1114122, string=%q) in nav mode should enter scroll mode", kittyPgUp.String())
	}
}

func TestAppModel_ScrollMode_EnterWithPgDown_NavMode(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Ensure in nav mode (default)
	if app.inputMode {
		t.Fatal("Should start in nav mode")
	}

	// First enter scroll mode via pgup (so we're not at bottom)
	model, _ = app.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	app = model.(AppModel)

	if !app.panes[0].IsScrollMode() {
		t.Fatal("Should be in scroll mode after pgup")
	}

	// Press pgdown - should stay in scroll mode (if not at bottom)
	// Note: With empty pane, pgdown may exit scroll mode since we're at bottom
	// This test verifies the flow works without crashing
	model, _ = app.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	app = model.(AppModel)
	// At bottom, scroll mode should be exited
}

func TestAppModel_ScrollMode_CtrlBPgUp_InputMode(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Enter input mode
	app.inputMode = true

	// Should not be in scroll mode initially
	if app.panes[0].IsScrollMode() {
		t.Error("Pane should not be in scroll mode initially")
	}

	// Press ctrl+b to set tmux prefix
	model, _ = app.Update(ctrlKey('b'))
	app = model.(AppModel)

	if !app.tmuxPrefix {
		t.Fatal("ctrl+b should set tmux prefix")
	}

	// Press pgup with tmux prefix - should enter scroll mode
	model, _ = app.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	app = model.(AppModel)

	if !app.panes[0].IsScrollMode() {
		t.Error("Pressing ctrl+b pgup in input mode should enter scroll mode")
	}
	if app.tmuxPrefix {
		t.Error("tmux prefix should be reset after use")
	}
}

func TestAppModel_ScrollMode_CtrlBPgDown_InputMode(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Enter input mode and scroll mode
	app.inputMode = true
	app.panes[0].EnterScrollMode()

	// Press ctrl+b to set tmux prefix
	model, _ = app.Update(ctrlKey('b'))
	app = model.(AppModel)

	if !app.tmuxPrefix {
		t.Fatal("ctrl+b should set tmux prefix")
	}

	// Press pgdown with tmux prefix
	model, _ = app.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	app = model.(AppModel)

	if app.tmuxPrefix {
		t.Error("tmux prefix should be reset after use")
	}
	// Note: With empty pane, we're likely at bottom, so scroll mode may be exited
}

func TestKeyStringRepresentations(t *testing.T) {
	// Verify that key string representations match what the code expects
	tests := []struct {
		name     string
		key      tea.KeyPressMsg
		expected string
	}{
		{"pgup via Code", tea.KeyPressMsg{Code: tea.KeyPgUp}, "pgup"},
		{"pgdown via Code", tea.KeyPressMsg{Code: tea.KeyPgDown}, "pgdown"},
		{"pgup via Kitty code 1114122", tea.KeyPressMsg{Code: 1114122}, "pgup"},
		{"pgdown via Kitty code 1114123", tea.KeyPressMsg{Code: 1114123}, "pgdown"},
		{"up via Code", tea.KeyPressMsg{Code: tea.KeyUp}, "up"},
		{"down via Code", tea.KeyPressMsg{Code: tea.KeyDown}, "down"},
		{"ctrl+b", tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl}, "ctrl+b"},
		{"ctrl+u", tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}, "ctrl+u"},
		{"ctrl+d", tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}, "ctrl+d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.key.String()
			if got != tt.expected {
				t.Errorf("Key %s: expected String() = %q, got %q", tt.name, tt.expected, got)
			}
		})
	}
}

// TestAppModel_ScrollMode_NavMode_AllScrollKeys tests all scroll-related keys in nav mode
func TestAppModel_ScrollMode_NavMode_AllScrollKeys(t *testing.T) {
	tests := []struct {
		name        string
		key         tea.KeyPressMsg
		shouldEnter bool // Should this key enter scroll mode?
		description string
	}{
		{"pgup", tea.KeyPressMsg{Code: tea.KeyPgUp}, true, "Page up should enter scroll mode"},
		{"pgdown", tea.KeyPressMsg{Code: tea.KeyPgDown}, false, "Page down doesn't enter scroll mode (it's for scrolling while in scroll mode)"},
		{"ctrl+u", tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}, true, "Ctrl+U (half-page up) should enter scroll mode"},
		{"ctrl+d", tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}, false, "Ctrl+D (half-page down) doesn't enter scroll mode"},
		{"bracket [", keyPress('['), true, "Bracket '[' should enter scroll mode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := NewAppModel(context.Background())
			app.width = 100
			app.height = 40

			// Create a workstream
			model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
			app = model.(AppModel)

			// Verify prerequisites
			if app.inputMode {
				t.Fatal("Should start in nav mode (inputMode=false)")
			}
			if len(app.panes) != 1 {
				t.Fatalf("Expected 1 pane, got %d", len(app.panes))
			}
			if app.focusedPane != 0 {
				t.Fatalf("Expected focusedPane=0, got %d", app.focusedPane)
			}
			if app.panes[0].IsScrollMode() {
				t.Fatal("Pane should not be in scroll mode initially")
			}

			// Send the key
			model, _ = app.Update(tt.key)
			app = model.(AppModel)

			// Check result
			if tt.shouldEnter {
				if !app.panes[0].IsScrollMode() {
					t.Errorf("%s: %s - expected scroll mode to be entered, but it wasn't", tt.name, tt.description)
				}
			} else {
				// For keys that don't enter scroll mode, verify they at least don't crash
				// (scroll mode state depends on whether at bottom, etc.)
			}
		})
	}
}

// ============================================================================
// Workstream Renumbering After Destruction Tests
// ============================================================================

func TestAppModel_RenumberPanesAfterDestroy(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create three workstreams (indices 1, 2, 3)
	for _, name := range []string{"first", "second", "third"} {
		model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: name})
		app = model.(AppModel)
	}

	// Verify initial indices
	if app.panes[0].Index() != 1 || app.panes[1].Index() != 2 || app.panes[2].Index() != 3 {
		t.Fatalf("Initial indices should be 1, 2, 3, got %d, %d, %d",
			app.panes[0].Index(), app.panes[1].Index(), app.panes[2].Index())
	}

	// Destroy the middle workstream (index 2)
	wsID := app.panes[1].Workstream().ID
	model, _ := app.Update(DialogConfirmMsg{Type: DialogDestroy, WorkstreamID: wsID})
	app = model.(AppModel)

	// Should have 2 panes remaining
	if len(app.panes) != 2 {
		t.Fatalf("Expected 2 panes after destroy, got %d", len(app.panes))
	}

	// Panes should be renumbered to 1 and 2
	if app.panes[0].Index() != 1 {
		t.Errorf("First pane should be renumbered to index 1, got %d", app.panes[0].Index())
	}
	if app.panes[1].Index() != 2 {
		t.Errorf("Second pane should be renumbered to index 2, got %d", app.panes[1].Index())
	}

	// nextPaneIndex should be updated for next creation
	if app.nextPaneIndex != 3 {
		t.Errorf("nextPaneIndex should be 3 after renumbering, got %d", app.nextPaneIndex)
	}
}

func TestAppModel_RenumberPanesAfterDestroy_FirstPane(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create three workstreams (indices 1, 2, 3)
	for _, name := range []string{"first", "second", "third"} {
		model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: name})
		app = model.(AppModel)
	}

	// Destroy the first workstream (index 1)
	wsID := app.panes[0].Workstream().ID
	model, _ := app.Update(DialogConfirmMsg{Type: DialogDestroy, WorkstreamID: wsID})
	app = model.(AppModel)

	// Should have 2 panes remaining, renumbered to 1 and 2
	if len(app.panes) != 2 {
		t.Fatalf("Expected 2 panes after destroy, got %d", len(app.panes))
	}

	if app.panes[0].Index() != 1 {
		t.Errorf("First pane should be renumbered to index 1, got %d", app.panes[0].Index())
	}
	if app.panes[1].Index() != 2 {
		t.Errorf("Second pane should be renumbered to index 2, got %d", app.panes[1].Index())
	}
}

func TestAppModel_RenumberPanesAfterDestroy_QuickDestroy(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create two workstreams
	for _, name := range []string{"first", "second"} {
		model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: name})
		app = model.(AppModel)
	}

	// Set the first workstream to error state (for quick destroy without dialog)
	app.panes[0].Workstream().SetState(workstream.StateError)

	// Focus the first pane
	app.focusedPane = 0
	app.panes[0].SetFocused(true)
	app.panes[1].SetFocused(false)

	// Press 'd' to quick-destroy the errored workstream
	model, _ := app.Update(keyPress('d'))
	app = model.(AppModel)

	// Should have 1 pane remaining, renumbered to index 1
	if len(app.panes) != 1 {
		t.Fatalf("Expected 1 pane after quick destroy, got %d", len(app.panes))
	}

	if app.panes[0].Index() != 1 {
		t.Errorf("Remaining pane should be renumbered to index 1, got %d", app.panes[0].Index())
	}
}

func TestAppModel_RenumberPanes_NumberKeysFocusCorrectly(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create three workstreams (indices 1, 2, 3)
	for _, name := range []string{"first", "second", "third"} {
		model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: name})
		app = model.(AppModel)
	}

	// Destroy the first workstream (original index 1)
	wsID := app.panes[0].Workstream().ID
	model, _ := app.Update(DialogConfirmMsg{Type: DialogDestroy, WorkstreamID: wsID})
	app = model.(AppModel)

	// After renumbering, panes should have indices 1 and 2

	// Focus on pane 0 initially
	app.focusedPane = 0
	app.panes[0].SetFocused(true)
	app.panes[1].SetFocused(false)

	// Press '2' - should focus the pane with index 2 (slice position 1)
	model, _ = app.Update(keyPress('2'))
	app = model.(AppModel)

	if app.focusedPane != 1 {
		t.Errorf("Pressing '2' should focus slice position 1 (pane index 2), got focusedPane=%d", app.focusedPane)
	}

	// Press '1' - should focus the pane with index 1 (slice position 0)
	model, _ = app.Update(keyPress('1'))
	app = model.(AppModel)

	if app.focusedPane != 0 {
		t.Errorf("Pressing '1' should focus slice position 0 (pane index 1), got focusedPane=%d", app.focusedPane)
	}

	// Press '3' - should do nothing (no pane with index 3 exists after renumbering)
	model, _ = app.Update(keyPress('3'))
	app = model.(AppModel)

	if app.focusedPane != 0 {
		t.Errorf("Pressing '3' (non-existent index) should not change focus, got focusedPane=%d", app.focusedPane)
	}
}

func TestAppModel_RenumberPanes_LastSwapPositionReset(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create three workstreams
	for _, name := range []string{"first", "second", "third"} {
		model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: name})
		app = model.(AppModel)
	}

	// Swap pane 2 to main position
	app.focusedPane = 2
	app.panes[0].SetFocused(false)
	app.panes[2].SetFocused(true)
	model, _ := app.Update(keyPress(' '))
	app = model.(AppModel)

	// Should have a lastSwapPosition set
	if app.lastSwapPosition == 0 {
		t.Fatal("lastSwapPosition should be set after swap")
	}

	// Destroy a pane - lastSwapPosition should be reset to avoid invalid indices
	wsID := app.panes[1].Workstream().ID
	model, _ = app.Update(DialogConfirmMsg{Type: DialogDestroy, WorkstreamID: wsID})
	app = model.(AppModel)

	// lastSwapPosition should be reset to 0
	if app.lastSwapPosition != 0 {
		t.Errorf("lastSwapPosition should be reset to 0 after destruction, got %d", app.lastSwapPosition)
	}
}

// Paste Tests

func TestAppModel_Paste_InInputMode(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Enter input mode
	app.inputMode = true

	// Create a mock PTY session
	mockStdin := &mockWriteCloser{}
	mockPTY := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
		stdin:        mockStdin,
	}
	app.panes[0].SetPTY(mockPTY)

	// Send paste message
	pasteMsg := tea.PasteMsg{Content: "pasted text content"}
	model, _ = app.Update(pasteMsg)
	app = model.(AppModel)

	// Verify paste content was written to PTY
	if string(mockStdin.Bytes()) != "pasted text content" {
		t.Errorf("Paste content not forwarded to PTY, got %q", string(mockStdin.Bytes()))
	}
}

func TestAppModel_Paste_InNavMode_Ignored(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Stay in nav mode (inputMode = false by default)
	if app.inputMode {
		t.Fatal("App should start in nav mode")
	}

	// Create a mock PTY session
	mockStdin := &mockWriteCloser{}
	mockPTY := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
		stdin:        mockStdin,
	}
	app.panes[0].SetPTY(mockPTY)

	// Send paste message
	pasteMsg := tea.PasteMsg{Content: "pasted text"}
	model, _ = app.Update(pasteMsg)
	app = model.(AppModel)

	// Verify paste was NOT forwarded (nav mode)
	if string(mockStdin.Bytes()) != "" {
		t.Errorf("Paste should not be forwarded in nav mode, got %q", string(mockStdin.Bytes()))
	}
}

func TestAppModel_Paste_NoPanes_Ignored(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// No panes created, just enable input mode
	app.inputMode = true

	// Send paste message - should not panic
	pasteMsg := tea.PasteMsg{Content: "pasted text"}
	model, _ := app.Update(pasteMsg)
	_ = model.(AppModel)

	// No crash = success
}

func TestAppModel_Paste_NoPTY_Ignored(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream but don't set up PTY
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Enter input mode
	app.inputMode = true

	// Don't set a PTY - pane has no PTY

	// Send paste message - should not panic
	pasteMsg := tea.PasteMsg{Content: "pasted text"}
	model, _ = app.Update(pasteMsg)
	_ = model.(AppModel)

	// No crash = success
}

func TestAppModel_CtrlV_ShowsHintInInputMode(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Enter input mode
	app.inputMode = true

	// Create a mock PTY session
	mockStdin := &mockWriteCloser{}
	mockPTY := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
		stdin:        mockStdin,
	}
	app.panes[0].SetPTY(mockPTY)

	// Verify hint not shown initially
	if app.ctrlVHintShown {
		t.Fatal("Ctrl+V hint should not be shown initially")
	}

	// Press ctrl+v
	ctrlVMsg := tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl}
	model, _ = app.Update(ctrlVMsg)
	app = model.(AppModel)

	// Verify hint was shown
	if !app.ctrlVHintShown {
		t.Error("Ctrl+V hint should be shown after pressing ctrl+v")
	}

	// Verify toast contains paste hint
	if !strings.Contains(app.toast, "Ctrl+Shift+V") {
		t.Errorf("Toast should mention Ctrl+Shift+V, got %q", app.toast)
	}

	// Verify byte 22 (literal next) was still sent to PTY
	data := mockStdin.Bytes()
	if len(data) != 1 || data[0] != 22 {
		t.Errorf("Expected byte 22 to be sent to PTY, got %v", data)
	}
}

func TestAppModel_CtrlV_HintOnlyShownOnce(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Enter input mode
	app.inputMode = true

	// Create a mock PTY session
	mockStdin := &mockWriteCloser{}
	mockPTY := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
		stdin:        mockStdin,
	}
	app.panes[0].SetPTY(mockPTY)

	// Press ctrl+v first time
	ctrlVMsg := tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl}
	model, _ = app.Update(ctrlVMsg)
	app = model.(AppModel)

	// Verify hint was shown
	if !app.ctrlVHintShown {
		t.Error("Ctrl+V hint should be shown after first press")
	}
	firstToast := app.toast

	// Clear toast manually
	app.toast = ""

	// Press ctrl+v second time
	model, _ = app.Update(ctrlVMsg)
	app = model.(AppModel)

	// Verify toast was NOT set again (hint only shows once)
	if app.toast != "" {
		t.Errorf("Toast should not be shown on second ctrl+v press, got %q", app.toast)
	}

	// Verify first toast contained the hint
	if !strings.Contains(firstToast, "Ctrl+Shift+V") {
		t.Errorf("First toast should mention Ctrl+Shift+V, got %q", firstToast)
	}
}

func TestAutoPressEnterCmd(t *testing.T) {
	// Test that autoPressEnterCmd creates a command that returns autoPressEnterMsg
	cmd := autoPressEnterCmd("test-ws-123")
	if cmd == nil {
		t.Fatal("autoPressEnterCmd should return a non-nil command")
	}

	// The command is a tea.Tick, so we can't easily test the message it produces
	// without waiting for the tick. This test just verifies the function doesn't panic.
}

func TestAutoPressEnterMsg(t *testing.T) {
	// Test the autoPressEnterMsg struct
	msg := autoPressEnterMsg{WorkstreamID: "test-ws-456"}
	if msg.WorkstreamID != "test-ws-456" {
		t.Errorf("WorkstreamID = %q, want %q", msg.WorkstreamID, "test-ws-456")
	}
}

func TestAppModel_Update_AutoPressEnterMsg(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	wsID := app.panes[0].Workstream().ID

	// Send autoPressEnterMsg - should not panic (no PTY attached)
	model, _ = app.Update(autoPressEnterMsg{WorkstreamID: wsID})
	_ = model.(AppModel)

	// No crash = success
}

func TestAppModel_Update_AutoPressEnterMsg_WrongWorkstream(t *testing.T) {
	app := NewAppModel(context.Background())
	app.width = 100
	app.height = 40

	// Create a workstream
	model, _ := app.Update(DialogConfirmMsg{Type: DialogNewWorkstream, Value: "test"})
	app = model.(AppModel)

	// Send autoPressEnterMsg with wrong workstream ID - should be ignored
	model, _ = app.Update(autoPressEnterMsg{WorkstreamID: "nonexistent-ws"})
	_ = model.(AppModel)

	// No crash = success
}

func TestPromptStartsWithContinue(t *testing.T) {
	// Test various prompts that should/shouldn't trigger auto-press-enter
	tests := []struct {
		name        string
		prompt      string
		shouldMatch bool
	}{
		{"exact continue", "continue", true},
		{"continue with space", "continue ", true},
		{"continue with text", "continue working on the feature", true},
		{"Continue uppercase", "Continue", true},
		{"CONTINUE all caps", "CONTINUE", true},
		{"continue with leading space", "  continue", true},
		{"continued is different", "continued", true}, // starts with "continue"
		{"other text", "something else", false},
		{"empty prompt", "", false},
		{"contains continue", "please continue", false}, // doesn't start with
		{"cont prefix", "cont", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This mirrors the logic in app.go
			matches := strings.HasPrefix(strings.ToLower(strings.TrimSpace(tt.prompt)), "continue")
			if matches != tt.shouldMatch {
				t.Errorf("prompt %q: got %v, want %v", tt.prompt, matches, tt.shouldMatch)
			}
		})
	}
}
