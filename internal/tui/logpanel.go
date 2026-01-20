package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
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

	// Build bottom border with scroll indicator if needed
	var bottomBorder string
	if len(filtered) > contentHeight {
		scrollInfo := fmt.Sprintf(" %d/%d ", len(filtered)-m.scrollOffset, len(filtered))
		scrollVisualWidth := lipgloss.Width(scrollInfo)
		bottomInnerWidth := m.width - 2
		// Position scroll info near right side
		rightMargin := 2
		leftWidth := bottomInnerWidth - scrollVisualWidth - rightMargin
		if leftWidth < 0 {
			leftWidth = 0
		}
		bottomBorder = borderStyle.Render(bottomLeft+strings.Repeat(horizontal, leftWidth)) +
			scrollInfo +
			borderStyle.Render(strings.Repeat(horizontal, rightMargin)+bottomRight)
	} else {
		bottomBorder = borderStyle.Render(bottomLeft + strings.Repeat(horizontal, m.width-2) + bottomRight)
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
