package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogPanelModel_Basic(t *testing.T) {
	panel := NewLogPanelModel()

	// Initially not visible
	if panel.IsVisible() {
		t.Error("Panel should not be visible initially")
	}

	// Toggle visibility
	panel.Toggle()
	if !panel.IsVisible() {
		t.Error("Panel should be visible after toggle")
	}

	panel.Toggle()
	if panel.IsVisible() {
		t.Error("Panel should not be visible after second toggle")
	}
}

func TestLogPanelModel_AddEntry(t *testing.T) {
	panel := NewLogPanelModel()

	panel.AddEntry(LevelDebug, "debug message")
	panel.AddEntry(LevelInfo, "info message")
	panel.AddEntry(LevelWarn, "warn message")
	panel.AddEntry(LevelErr, "error message")

	if panel.EntryCount() != 4 {
		t.Errorf("Expected 4 entries, got %d", panel.EntryCount())
	}
}

func TestLogPanelModel_Filter(t *testing.T) {
	panel := NewLogPanelModel()

	// Default filter is DEBUG (show all)
	if panel.FilterLevel() != LevelDebug {
		t.Errorf("Expected default filter level DEBUG, got %s", panel.FilterLevel().String())
	}

	// Add entries at each level
	panel.AddEntry(LevelDebug, "debug")
	panel.AddEntry(LevelInfo, "info")
	panel.AddEntry(LevelWarn, "warn")
	panel.AddEntry(LevelErr, "error")

	// All entries should be visible with DEBUG filter
	filtered := panel.filteredEntries()
	if len(filtered) != 4 {
		t.Errorf("With DEBUG filter, expected 4 entries, got %d", len(filtered))
	}

	// Cycle to INFO filter
	panel.CycleFilter()
	if panel.FilterLevel() != LevelInfo {
		t.Errorf("Expected INFO filter, got %s", panel.FilterLevel().String())
	}
	filtered = panel.filteredEntries()
	if len(filtered) != 3 {
		t.Errorf("With INFO filter, expected 3 entries, got %d", len(filtered))
	}

	// Cycle to WARN filter
	panel.CycleFilter()
	if panel.FilterLevel() != LevelWarn {
		t.Errorf("Expected WARN filter, got %s", panel.FilterLevel().String())
	}
	filtered = panel.filteredEntries()
	if len(filtered) != 2 {
		t.Errorf("With WARN filter, expected 2 entries, got %d", len(filtered))
	}

	// Cycle to ERR filter
	panel.CycleFilter()
	if panel.FilterLevel() != LevelErr {
		t.Errorf("Expected ERR filter, got %s", panel.FilterLevel().String())
	}
	filtered = panel.filteredEntries()
	if len(filtered) != 1 {
		t.Errorf("With ERR filter, expected 1 entry, got %d", len(filtered))
	}

	// Cycle back to DEBUG
	panel.CycleFilter()
	if panel.FilterLevel() != LevelDebug {
		t.Errorf("Expected DEBUG filter after cycling, got %s", panel.FilterLevel().String())
	}
}

func TestLogPanelModel_CircularBuffer(t *testing.T) {
	panel := NewLogPanelModel()

	// Fill beyond capacity
	for i := 0; i < MaxLogEntries+100; i++ {
		panel.AddEntry(LevelInfo, "message")
	}

	if panel.EntryCount() != MaxLogEntries {
		t.Errorf("Expected %d entries (max), got %d", MaxLogEntries, panel.EntryCount())
	}
}

func TestLogPanelModel_Clear(t *testing.T) {
	panel := NewLogPanelModel()

	panel.AddEntry(LevelInfo, "message 1")
	panel.AddEntry(LevelInfo, "message 2")

	if panel.EntryCount() != 2 {
		t.Errorf("Expected 2 entries, got %d", panel.EntryCount())
	}

	panel.Clear()

	if panel.EntryCount() != 0 {
		t.Errorf("Expected 0 entries after clear, got %d", panel.EntryCount())
	}
}

func TestLogPanelModel_View(t *testing.T) {
	panel := NewLogPanelModel()
	panel.SetSize(80, 10)

	// View should be empty when not visible
	view := panel.View()
	if view != "" {
		t.Error("View should be empty when panel is not visible")
	}

	// Make visible and add entries
	panel.Toggle()
	panel.AddEntry(LevelInfo, "test message")

	view = panel.View()
	if view == "" {
		t.Error("View should not be empty when panel is visible with entries")
	}

	// Should contain the header with filter level
	if !strings.Contains(view, "Logs") {
		t.Error("View should contain 'Logs' header")
	}
}

func TestLogPanelModel_Scroll(t *testing.T) {
	panel := NewLogPanelModel()
	panel.SetSize(80, 10)
	panel.Toggle()

	// Add more entries than can fit
	for i := 0; i < 20; i++ {
		panel.AddEntry(LevelInfo, "message")
	}

	// Initial scroll should be at bottom (offset 0)
	if panel.scrollOffset != 0 {
		t.Errorf("Initial scroll offset should be 0, got %d", panel.scrollOffset)
	}

	// Scroll up
	panel.ScrollUp()
	if panel.scrollOffset != 1 {
		t.Errorf("After ScrollUp, offset should be 1, got %d", panel.scrollOffset)
	}

	// Scroll down back to bottom
	panel.ScrollDown()
	if panel.scrollOffset != 0 {
		t.Errorf("After ScrollDown, offset should be 0, got %d", panel.scrollOffset)
	}

	// Scroll down when at bottom should stay at 0
	panel.ScrollDown()
	if panel.scrollOffset != 0 {
		t.Errorf("Scroll down at bottom should stay at 0, got %d", panel.scrollOffset)
	}
}

func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelErr, "ERR"},
	}

	for _, tc := range tests {
		if tc.level.String() != tc.expected {
			t.Errorf("Level %d String() = %s, expected %s", tc.level, tc.level.String(), tc.expected)
		}
	}
}

func TestLoggerFunctions(t *testing.T) {
	// Create a panel and set it as global
	panel := NewLogPanelModel()
	SetLogPanel(panel)

	// Use the logging functions
	LogDebug("debug %s", "message")
	LogInfo("info %s", "message")
	LogWarn("warn %s", "message")
	LogErr("error %s", "message")

	if panel.EntryCount() != 4 {
		t.Errorf("Expected 4 entries from logger functions, got %d", panel.EntryCount())
	}

	// Verify entries are at correct levels
	entries := panel.filteredEntries()
	if entries[0].Level != LevelDebug {
		t.Errorf("First entry should be DEBUG, got %s", entries[0].Level.String())
	}
	if entries[1].Level != LevelInfo {
		t.Errorf("Second entry should be INFO, got %s", entries[1].Level.String())
	}
	if entries[2].Level != LevelWarn {
		t.Errorf("Third entry should be WARN, got %s", entries[2].Level.String())
	}
	if entries[3].Level != LevelErr {
		t.Errorf("Fourth entry should be ERR, got %s", entries[3].Level.String())
	}
}

func TestLogPanelModel_Fullscreen(t *testing.T) {
	panel := NewLogPanelModel()

	// Initially not fullscreen
	if panel.IsFullscreen() {
		t.Error("Panel should not be fullscreen initially")
	}

	// Toggle fullscreen
	panel.ToggleFullscreen()
	if !panel.IsFullscreen() {
		t.Error("Panel should be fullscreen after toggle")
	}

	// Toggle back
	panel.ToggleFullscreen()
	if panel.IsFullscreen() {
		t.Error("Panel should not be fullscreen after second toggle")
	}
}

func TestLogPanelModel_FullscreenResetsScroll(t *testing.T) {
	panel := NewLogPanelModel()
	panel.SetSize(80, 10)
	panel.Toggle()

	// Add entries and scroll
	for i := 0; i < 20; i++ {
		panel.AddEntry(LevelInfo, "message")
	}
	panel.ScrollUp()
	panel.ScrollUp()

	if panel.scrollOffset != 2 {
		t.Errorf("Expected scroll offset 2, got %d", panel.scrollOffset)
	}

	// Toggle fullscreen should reset scroll
	panel.ToggleFullscreen()
	if panel.scrollOffset != 0 {
		t.Errorf("Fullscreen toggle should reset scroll, got %d", panel.scrollOffset)
	}
}

func TestLogPanelModel_Export(t *testing.T) {
	panel := NewLogPanelModel()

	// Add some entries
	panel.AddEntry(LevelDebug, "debug message")
	panel.AddEntry(LevelInfo, "info message")
	panel.AddEntry(LevelWarn, "warning message")
	panel.AddEntry(LevelErr, "error message")

	// Create temp directory
	tmpDir := t.TempDir()

	// Export
	path, err := panel.Export(tmpDir)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("Export file not created: %s", path)
	}

	// Verify file contents
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read export file: %v", err)
	}

	contentStr := string(content)

	// Check header
	if !strings.Contains(contentStr, "Claude Cells System Logs") {
		t.Error("Export should contain header")
	}
	if !strings.Contains(contentStr, "Entries: 4") {
		t.Error("Export should show entry count")
	}

	// Check entries
	if !strings.Contains(contentStr, "[DEBUG] debug message") {
		t.Error("Export should contain debug entry")
	}
	if !strings.Contains(contentStr, "[INFO ] info message") {
		t.Error("Export should contain info entry")
	}
	if !strings.Contains(contentStr, "[WARN ] warning message") {
		t.Error("Export should contain warn entry")
	}
	if !strings.Contains(contentStr, "[ERR  ] error message") {
		t.Error("Export should contain error entry")
	}
}

func TestLogPanelModel_ExportCreatesDirectory(t *testing.T) {
	panel := NewLogPanelModel()
	panel.AddEntry(LevelInfo, "test message")

	// Export to a non-existent nested directory
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "nested", "logs")

	path, err := panel.Export(nestedDir)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
		t.Errorf("Export should create directory: %s", nestedDir)
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("Export file not created: %s", path)
	}
}

func TestLogPanelModel_ViewShowsHints(t *testing.T) {
	panel := NewLogPanelModel()
	panel.SetSize(100, 10)
	panel.Toggle()

	// Add an entry
	panel.AddEntry(LevelInfo, "test message")

	view := panel.View()

	// Should show hints in bottom border
	if !strings.Contains(view, "filter") {
		t.Error("View should contain 'filter' hint")
	}
	if !strings.Contains(view, "fullscreen") {
		t.Error("View should contain 'fullscreen' hint")
	}
	if !strings.Contains(view, "export") {
		t.Error("View should contain 'export' hint")
	}
}
