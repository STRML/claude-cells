package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelErr
)

// String returns the display string for a log level
func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelErr:
		return "ERR"
	default:
		return "???"
	}
}

// LogEntry represents a single log message
type LogEntry struct {
	Time    time.Time
	Level   LogLevel
	Message string
}

// LogPanelModel manages the collapsible log panel
type LogPanelModel struct {
	entries      []LogEntry
	maxEntries   int
	mu           sync.RWMutex
	width        int
	height       int
	visible      bool
	fullscreen   bool     // Whether panel is in fullscreen mode
	filterLevel  LogLevel // Show this level and above
	scrollOffset int      // Lines scrolled from bottom (0 = at bottom)
}

// DefaultLogPanelHeight is the default height when expanded
const DefaultLogPanelHeight = 8

// MaxLogEntries is the maximum number of log entries to keep
const MaxLogEntries = 1000

// NewLogPanelModel creates a new log panel
func NewLogPanelModel() *LogPanelModel {
	return &LogPanelModel{
		entries:     make([]LogEntry, 0, MaxLogEntries),
		maxEntries:  MaxLogEntries,
		filterLevel: LevelDebug, // Show all by default
		visible:     false,
	}
}

// AddEntry adds a log entry to the panel
func (m *LogPanelModel) AddEntry(level LogLevel, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry := LogEntry{
		Time:    time.Now(),
		Level:   level,
		Message: message,
	}

	// Circular buffer behavior - remove oldest if at capacity
	if len(m.entries) >= m.maxEntries {
		m.entries = m.entries[1:]
	}
	m.entries = append(m.entries, entry)
}

// SetSize sets the panel dimensions
func (m *LogPanelModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Toggle toggles panel visibility
func (m *LogPanelModel) Toggle() {
	m.visible = !m.visible
	m.scrollOffset = 0 // Reset scroll when toggling
}

// IsVisible returns whether the panel is visible
func (m *LogPanelModel) IsVisible() bool {
	return m.visible
}

// IsFullscreen returns whether the panel is in fullscreen mode
func (m *LogPanelModel) IsFullscreen() bool {
	return m.fullscreen
}

// ToggleFullscreen toggles fullscreen mode
func (m *LogPanelModel) ToggleFullscreen() {
	m.fullscreen = !m.fullscreen
	m.scrollOffset = 0 // Reset scroll when toggling
}

// CycleFilter cycles through filter levels
func (m *LogPanelModel) CycleFilter() {
	m.filterLevel = (m.filterLevel + 1) % 4
	m.scrollOffset = 0 // Reset scroll when changing filter
}

// FilterLevel returns the current filter level
func (m *LogPanelModel) FilterLevel() LogLevel {
	return m.filterLevel
}

// ScrollUp scrolls up one line
func (m *LogPanelModel) ScrollUp() {
	filtered := m.filteredEntries()
	maxScroll := len(filtered) - (m.height - 2) // -2 for border
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scrollOffset < maxScroll {
		m.scrollOffset++
	}
}

// ScrollDown scrolls down one line
func (m *LogPanelModel) ScrollDown() {
	if m.scrollOffset > 0 {
		m.scrollOffset--
	}
}

// PageUp scrolls up one page
func (m *LogPanelModel) PageUp() {
	pageSize := m.height - 2
	if pageSize < 1 {
		pageSize = 1
	}
	filtered := m.filteredEntries()
	maxScroll := len(filtered) - (m.height - 2)
	if maxScroll < 0 {
		maxScroll = 0
	}
	m.scrollOffset += pageSize
	if m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}
}

// PageDown scrolls down one page
func (m *LogPanelModel) PageDown() {
	pageSize := m.height - 2
	if pageSize < 1 {
		pageSize = 1
	}
	m.scrollOffset -= pageSize
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

// filteredEntries returns entries matching the current filter
func (m *LogPanelModel) filteredEntries() []LogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var filtered []LogEntry
	for _, e := range m.entries {
		if e.Level >= m.filterLevel {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// View renders the log panel
func (m *LogPanelModel) View() string {
	if !m.visible || m.height < 3 || m.width < 20 {
		return ""
	}

	filtered := m.filteredEntries()

	// Build header with filter indicator
	filterStr := m.filterLevel.String() + "+"
	header := fmt.Sprintf(" Logs [%s] ", filterStr)

	// Calculate content area
	contentHeight := m.height - 2 // -2 for top and bottom border
	contentWidth := m.width - 2   // -2 for left and right border

	// Get visible lines based on scroll
	var visibleLines []string
	startIdx := len(filtered) - contentHeight - m.scrollOffset
	if startIdx < 0 {
		startIdx = 0
	}
	endIdx := startIdx + contentHeight
	if endIdx > len(filtered) {
		endIdx = len(filtered)
	}

	for i := startIdx; i < endIdx; i++ {
		entry := filtered[i]
		line := m.formatEntry(entry, contentWidth)
		visibleLines = append(visibleLines, line)
	}

	// Pad with empty lines if needed
	for len(visibleLines) < contentHeight {
		visibleLines = append(visibleLines, strings.Repeat(" ", contentWidth))
	}

	// Style definitions
	borderColor := lipgloss.Color("#444444")
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#888888"))
	borderStyle := lipgloss.NewStyle().
		Foreground(borderColor)

	// Build border lines manually to avoid ANSI splice corruption
	// Using rounded border characters: ╭ ╮ ╰ ╯ ─ │
	topLeft := "╭"
	topRight := "╮"
	bottomLeft := "╰"
	bottomRight := "╯"
	horizontal := "─"
	vertical := "│"

	// Build top border with header centered
	headerVisualWidth := lipgloss.Width(header)
	topInnerWidth := m.width - 2 // excluding corners
	leftPadding := (topInnerWidth - headerVisualWidth) / 2
	rightPadding := topInnerWidth - headerVisualWidth - leftPadding

	topBorder := borderStyle.Render(topLeft+strings.Repeat(horizontal, leftPadding)) +
		headerStyle.Render(header) +
		borderStyle.Render(strings.Repeat(horizontal, rightPadding)+topRight)

	// Build bottom border with hints and scroll indicator
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Bold(true)

	// Build hints section
	hints := keyStyle.Render("~") + hintStyle.Render(":filter ") +
		keyStyle.Render("f") + hintStyle.Render(":fullscreen ") +
		keyStyle.Render("E") + hintStyle.Render(":export")
	hintsWidth := lipgloss.Width(hints)

	var bottomBorder string
	bottomInnerWidth := m.width - 2

	if len(filtered) > contentHeight {
		// Show scroll indicator
		scrollInfo := fmt.Sprintf(" %d/%d ", len(filtered)-m.scrollOffset, len(filtered))
		scrollVisualWidth := lipgloss.Width(scrollInfo)

		// Calculate spacing: hints on left, scroll on right
		availableMiddle := bottomInnerWidth - hintsWidth - scrollVisualWidth - 4 // 4 for padding
		if availableMiddle < 0 {
			availableMiddle = 0
		}
		bottomBorder = borderStyle.Render(bottomLeft+" ") +
			hints +
			borderStyle.Render(strings.Repeat(horizontal, availableMiddle)) +
			scrollInfo +
			borderStyle.Render(" "+bottomRight)
	} else {
		// No scroll indicator, just hints
		availableRight := bottomInnerWidth - hintsWidth - 2 // 2 for padding
		if availableRight < 0 {
			availableRight = 0
		}
		bottomBorder = borderStyle.Render(bottomLeft+" ") +
			hints +
			borderStyle.Render(strings.Repeat(horizontal, availableRight)+" "+bottomRight)
	}

	// Build content lines with side borders
	var lines []string
	lines = append(lines, topBorder)
	for _, line := range visibleLines {
		// Pad or truncate line to fit content width
		lineWidth := lipgloss.Width(line)
		if lineWidth < contentWidth {
			line = line + strings.Repeat(" ", contentWidth-lineWidth)
		}
		lines = append(lines, borderStyle.Render(vertical)+line+borderStyle.Render(vertical))
	}
	lines = append(lines, bottomBorder)

	return strings.Join(lines, "\n")
}

// formatEntry formats a single log entry for display
func (m *LogPanelModel) formatEntry(entry LogEntry, maxWidth int) string {
	// Format: HH:MM:SS.mmm [LEVEL] message
	timestamp := entry.Time.Format("15:04:05.000")

	var levelStyle lipgloss.Style
	switch entry.Level {
	case LevelDebug:
		levelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	case LevelInfo:
		levelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA"))
	case LevelWarn:
		levelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFAA00"))
	case LevelErr:
		levelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4444")).Bold(true)
	}

	levelStr := levelStyle.Render(fmt.Sprintf("[%-5s]", entry.Level.String()))
	timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	timeStr := timeStyle.Render(timestamp)

	prefix := timeStr + " " + levelStr + " "
	prefixLen := len(timestamp) + 1 + 7 + 1 // timestamp + space + [LEVEL] + space

	// Truncate message if needed
	msg := entry.Message
	availableWidth := maxWidth - prefixLen
	if availableWidth > 0 && len(msg) > availableWidth {
		msg = msg[:availableWidth-3] + "..."
	}

	line := prefix + msg

	// Pad to full width
	if len(line) < maxWidth {
		line += strings.Repeat(" ", maxWidth-len(line))
	}

	return line
}

// Export writes all log entries to a file in the specified directory
// Returns the path to the exported file
func (m *LogPanelModel) Export(dir string) (string, error) {
	m.mu.RLock()
	entries := make([]LogEntry, len(m.entries))
	copy(entries, m.entries)
	m.mu.RUnlock()

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}

	// Generate filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("system-logs-%s.txt", timestamp)
	path := filepath.Join(dir, filename)

	// Create and write to file
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	// Write header
	fmt.Fprintf(f, "Claude Cells System Logs\n")
	fmt.Fprintf(f, "Exported: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(f, "Entries: %d\n", len(entries))
	fmt.Fprintf(f, "================================================================================\n\n")

	// Write entries
	for _, entry := range entries {
		timestamp := entry.Time.Format("2006-01-02 15:04:05.000")
		fmt.Fprintf(f, "%s [%-5s] %s\n", timestamp, entry.Level.String(), entry.Message)
	}

	return path, nil
}

// Clear removes all log entries
func (m *LogPanelModel) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = m.entries[:0]
	m.scrollOffset = 0
}

// EntryCount returns the total number of entries
func (m *LogPanelModel) EntryCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}
