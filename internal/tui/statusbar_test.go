package tui

import (
	"strings"
	"testing"
)

func TestNewStatusBarModel(t *testing.T) {
	sb := NewStatusBarModel()
	if sb.workstreamCount != 0 {
		t.Error("Initial workstream count should be 0")
	}
	if sb.pairingBranch != "" {
		t.Error("Initial pairing branch should be empty")
	}
}

func TestStatusBarModel_SetWorkstreamCount(t *testing.T) {
	sb := NewStatusBarModel()
	sb.SetWorkstreamCount(5)
	if sb.workstreamCount != 5 {
		t.Errorf("workstreamCount = %d, want 5", sb.workstreamCount)
	}
}

func TestStatusBarModel_SetPairingBranch(t *testing.T) {
	sb := NewStatusBarModel()
	sb.SetPairingBranch("feature-auth")
	if sb.pairingBranch != "feature-auth" {
		t.Errorf("pairingBranch = %q, want 'feature-auth'", sb.pairingBranch)
	}
}

func TestStatusBarModel_SetWidth(t *testing.T) {
	sb := NewStatusBarModel()
	sb.SetWidth(120)
	if sb.width != 120 {
		t.Errorf("width = %d, want 120", sb.width)
	}
}

func TestStatusBarModel_SetShowHelp(t *testing.T) {
	sb := NewStatusBarModel()
	if sb.showHelp {
		t.Error("Initial showHelp should be false")
	}
	sb.SetShowHelp(true)
	if !sb.showHelp {
		t.Error("showHelp should be true after SetShowHelp(true)")
	}
}

func TestStatusBarModel_View(t *testing.T) {
	sb := NewStatusBarModel()
	sb.SetWidth(100)
	sb.SetWorkstreamCount(3)

	view := sb.View()

	// Should contain app name
	if !strings.Contains(view, "docker-tui") {
		t.Error("View should contain 'docker-tui'")
	}
	// Should contain workstream count
	if !strings.Contains(view, "3 workstreams") {
		t.Error("View should contain '3 workstreams'")
	}
	// Should contain key hints
	if !strings.Contains(view, "n") {
		t.Error("View should contain key hints")
	}
}

func TestStatusBarModel_View_WithPairing(t *testing.T) {
	sb := NewStatusBarModel()
	sb.SetWidth(120)
	sb.SetWorkstreamCount(2)
	sb.SetPairingBranch("add-auth")

	view := sb.View()

	// Should contain pairing indicator
	if !strings.Contains(view, "Pairing") {
		t.Error("View should contain 'Pairing' when branch is set")
	}
	if !strings.Contains(view, "add-auth") {
		t.Error("View should contain pairing branch name")
	}
}

func TestStatusBarModel_View_NoPairing(t *testing.T) {
	sb := NewStatusBarModel()
	sb.SetWidth(100)
	sb.SetWorkstreamCount(1)
	// No pairing branch set

	view := sb.View()

	// Should NOT contain pairing indicator when no branch
	if strings.Contains(view, "Pairing") {
		t.Error("View should NOT contain 'Pairing' when branch is empty")
	}
}

func TestStatusBarModel_View_ZeroWorkstreams(t *testing.T) {
	sb := NewStatusBarModel()
	sb.SetWidth(100)
	// workstreamCount is 0 by default

	view := sb.View()

	// Should show "0 workstreams"
	if !strings.Contains(view, "0 workstreams") {
		t.Error("View should contain '0 workstreams' when count is 0")
	}
}

func TestStatusBarModel_View_KeyHints(t *testing.T) {
	sb := NewStatusBarModel()
	sb.SetWidth(150)
	sb.SetWorkstreamCount(1)

	view := sb.View()

	// Check for all expected key hints
	expectedHints := []string{"n", "p", "m", "d", "i", "?"}
	for _, hint := range expectedHints {
		if !strings.Contains(view, hint) {
			t.Errorf("View should contain key hint %q", hint)
		}
	}
}

func TestStatusBarModel_Init(t *testing.T) {
	sb := NewStatusBarModel()
	cmd := sb.Init()
	if cmd != nil {
		t.Error("Init should return nil")
	}
}

func TestStatusBarModel_Update(t *testing.T) {
	sb := NewStatusBarModel()
	sb.SetWorkstreamCount(3)

	updated, cmd := sb.Update(nil)
	if cmd != nil {
		t.Error("Update should return nil cmd")
	}
	if updated.workstreamCount != 3 {
		t.Error("Update should preserve state")
	}
}

func TestStatusBarModel_SetInputMode(t *testing.T) {
	sb := NewStatusBarModel()

	// Initially false
	if sb.inputMode {
		t.Error("Initial inputMode should be false")
	}

	// Set to true
	sb.SetInputMode(true)
	if !sb.inputMode {
		t.Error("inputMode should be true after SetInputMode(true)")
	}

	// Set back to false
	sb.SetInputMode(false)
	if sb.inputMode {
		t.Error("inputMode should be false after SetInputMode(false)")
	}
}

func TestStatusBarModel_View_NavMode(t *testing.T) {
	sb := NewStatusBarModel()
	sb.SetWidth(100)
	sb.SetInputMode(false)

	view := sb.View()

	if !strings.Contains(view, "NAV") {
		t.Error("View should contain 'NAV' when not in input mode")
	}
}

func TestStatusBarModel_View_InputMode(t *testing.T) {
	sb := NewStatusBarModel()
	sb.SetWidth(100)
	sb.SetInputMode(true)

	view := sb.View()

	if !strings.Contains(view, "INPUT") {
		t.Error("View should contain 'INPUT' when in input mode")
	}
}
