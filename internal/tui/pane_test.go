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

func TestPaneModel_VTerm_Initialized(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	if pane.vterm == nil {
		t.Error("vterm should be initialized")
	}
}

func TestPaneModel_WritePTYOutput(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)

	// Write some data to vterm
	pane.WritePTYOutput([]byte("Hello from PTY\r\n"))

	// Render and check
	output := pane.renderVTerm()
	if !strings.Contains(output, "Hello from PTY") {
		t.Errorf("renderVTerm() = %q, want to contain 'Hello from PTY'", output)
	}
}

func TestPaneModel_WritePTYOutput_WithCarriageReturn(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)

	// Write with carriage return (should overwrite line)
	pane.WritePTYOutput([]byte("First line\r\n"))
	pane.WritePTYOutput([]byte("Overwrite\r"))
	pane.WritePTYOutput([]byte("NewText"))

	output := pane.renderVTerm()

	if !strings.Contains(output, "First line") {
		t.Errorf("renderVTerm() should contain 'First line', got: %q", output)
	}
	if !strings.Contains(output, "NewText") {
		t.Errorf("renderVTerm() should contain 'NewText', got: %q", output)
	}
}

func TestPaneModel_renderVTerm_AfterResize(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	// Start with one size
	pane.SetSize(80, 24)
	pane.WritePTYOutput([]byte("Test content\r\n"))

	output1 := pane.renderVTerm()
	if output1 == "" {
		t.Error("renderVTerm() should not be empty after write")
	}

	// Resize to different sizes - should not panic
	pane.SetSize(120, 40)
	output2 := pane.renderVTerm()
	_ = output2 // Just ensure no panic

	pane.SetSize(40, 10)
	output3 := pane.renderVTerm()
	_ = output3 // Just ensure no panic
}

func TestPaneModel_renderVTerm_EmptyVterm(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)

	// Render empty vterm - should not panic
	output := pane.renderVTerm()
	_ = output
}

func TestPaneModel_renderVTerm_CachesLastRender(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)
	pane.SetSize(80, 24)

	pane.WritePTYOutput([]byte("Cached content\r\n"))
	output := pane.renderVTerm()

	if pane.lastVtermRender == "" {
		t.Error("lastVtermRender should be set after successful render")
	}
	if pane.lastVtermRender != output {
		t.Error("lastVtermRender should match the returned output")
	}
}

func TestPaneModel_HasPTY(t *testing.T) {
	ws := workstream.New("test")
	pane := NewPaneModel(ws)

	if pane.HasPTY() {
		t.Error("HasPTY() should be false initially")
	}

	pane.SetPTY(nil)
	if pane.HasPTY() {
		t.Error("HasPTY() should be false with nil PTY")
	}
}
