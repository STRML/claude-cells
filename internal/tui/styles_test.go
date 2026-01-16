package tui

import (
	"strings"
	"testing"
)

func TestStatusStyle(t *testing.T) {
	tests := []struct {
		state    string
		contains string
	}{
		{"running", IndicatorRunning},
		{"idle", IndicatorIdle},
		{"pairing", IndicatorPairing},
		{"stopped", IndicatorStopped},
		{"unknown", IndicatorStopped}, // defaults to stopped
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			result := StatusStyle(tt.state)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("StatusStyle(%q) = %q, want to contain %q", tt.state, result, tt.contains)
			}
		})
	}
}

func TestKeyHint(t *testing.T) {
	result := KeyHint("n", "ew")
	if !strings.Contains(result, "n") {
		t.Error("KeyHint should contain the key")
	}
	if !strings.Contains(result, "ew") {
		t.Error("KeyHint should contain the action")
	}
}

func TestStatusIndicatorConstants(t *testing.T) {
	// Verify the indicators match the design doc
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"running", IndicatorRunning, "●"},
		{"idle", IndicatorIdle, "○"},
		{"stopped", IndicatorStopped, "◌"},
		{"pairing", IndicatorPairing, "⟳"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("Indicator%s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestPaneStyles(t *testing.T) {
	// Verify pane styles are defined and render without panic
	t.Run("active pane", func(t *testing.T) {
		result := PaneBorderActive.Render("test content")
		if result == "" {
			t.Error("PaneBorderActive should render content")
		}
	})

	t.Run("inactive pane", func(t *testing.T) {
		result := PaneBorderInactive.Render("test content")
		if result == "" {
			t.Error("PaneBorderInactive should render content")
		}
	})

	t.Run("pane title", func(t *testing.T) {
		result := PaneTitle.Render("branch-name")
		if !strings.Contains(result, "branch-name") {
			t.Error("PaneTitle should render the branch name")
		}
	})
}

func TestStatusBarStyles(t *testing.T) {
	t.Run("status bar", func(t *testing.T) {
		result := StatusBarStyle.Render("status text")
		if !strings.Contains(result, "status text") {
			t.Error("StatusBarStyle should render the text")
		}
	})

	t.Run("key hint style", func(t *testing.T) {
		result := KeyHintStyle.Render("hint")
		if !strings.Contains(result, "hint") {
			t.Error("KeyHintStyle should render the hint")
		}
	})

	t.Run("key style", func(t *testing.T) {
		result := KeyStyle.Render("k")
		if !strings.Contains(result, "k") {
			t.Error("KeyStyle should render the key")
		}
	})
}

func TestDialogStyles(t *testing.T) {
	t.Run("dialog box", func(t *testing.T) {
		result := DialogBox.Render("dialog content")
		if result == "" {
			t.Error("DialogBox should render content")
		}
	})

	t.Run("dialog title", func(t *testing.T) {
		result := DialogTitle.Render("Title")
		if !strings.Contains(result, "Title") {
			t.Error("DialogTitle should render the title")
		}
	})

	t.Run("dialog input", func(t *testing.T) {
		result := DialogInput.Render("input text")
		if result == "" {
			t.Error("DialogInput should render content")
		}
	})
}
