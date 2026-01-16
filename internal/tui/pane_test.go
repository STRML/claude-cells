package tui

import (
	"strings"
	"testing"

	"github.com/samuelreed/docker-tui/internal/workstream"
)

func TestNewPaneModel(t *testing.T) {
	ws := workstream.New("test prompt")
	pane := NewPaneModel(ws)

	if pane.workstream != ws {
		t.Error("Pane should reference the workstream")
	}
	if pane.focused {
		t.Error("Pane should not be focused initially")
	}
}

func TestPaneModel_SetFocused(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	pane.SetFocused(true)
	if !pane.focused {
		t.Error("Pane should be focused after SetFocused(true)")
	}

	pane.SetFocused(false)
	if pane.focused {
		t.Error("Pane should not be focused after SetFocused(false)")
	}
}

func TestPaneModel_SetSize(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	pane.SetSize(80, 24)
	if pane.width != 80 {
		t.Errorf("width = %d, want 80", pane.width)
	}
	if pane.height != 24 {
		t.Errorf("height = %d, want 24", pane.height)
	}
}

func TestPaneModel_AppendOutput(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	pane.AppendOutput("Hello")
	pane.AppendOutput(" World")

	if !strings.Contains(pane.output.String(), "Hello World") {
		t.Errorf("output = %q, want to contain 'Hello World'", pane.output.String())
	}
}

func TestPaneModel_View(t *testing.T) {
	ws := workstream.New("test prompt")
	ws.SetState(workstream.StateRunning)
	pane := NewPaneModel(ws)
	pane.SetSize(40, 10)

	view := pane.View()

	// Should contain the branch name
	if !strings.Contains(view, ws.BranchName) {
		t.Error("View should contain branch name")
	}
	// Should contain status indicator
	if !strings.Contains(view, IndicatorRunning) {
		t.Error("View should contain running indicator")
	}
}

func TestPaneModel_Workstream(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	if pane.Workstream() != ws {
		t.Error("Workstream() should return the workstream")
	}
}
