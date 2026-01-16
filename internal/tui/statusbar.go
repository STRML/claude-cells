package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// StatusBarModel represents the bottom status bar
type StatusBarModel struct {
	width           int
	workstreamCount int
	pairingBranch   string
	showHelp        bool
}

// NewStatusBarModel creates a new status bar
func NewStatusBarModel() StatusBarModel {
	return StatusBarModel{}
}

// Init initializes the status bar
func (s StatusBarModel) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (s StatusBarModel) Update(msg tea.Msg) (StatusBarModel, tea.Cmd) {
	return s, nil
}

// View renders the status bar
func (s StatusBarModel) View() string {
	// Left section: app name and count
	left := fmt.Sprintf("docker-tui: %d workstreams", s.workstreamCount)

	// Center section: pairing indicator
	var center string
	if s.pairingBranch != "" {
		pairingStyle := lipgloss.NewStyle().Foreground(ColorPairing).Bold(true)
		center = fmt.Sprintf(" | %s Pairing: %s", IndicatorPairing, pairingStyle.Render(s.pairingBranch))
	}

	// Right section: key hints
	hints := []string{
		KeyHint("n", "ew"),
		KeyHint("p", "air"),
		KeyHint("m", "erge/PR"),
		KeyHint("d", "estroy"),
		KeyHint("1-9", " focus"),
		KeyHint("?", "help"),
	}
	right := ""
	for i, hint := range hints {
		if i > 0 {
			right += " | "
		}
		right += hint
	}

	// Combine sections
	content := left + center

	// Calculate padding to right-align hints
	leftLen := lipgloss.Width(content)
	rightLen := lipgloss.Width(right)
	padding := s.width - leftLen - rightLen - 2 // 2 for padding
	if padding < 1 {
		padding = 1
	}

	fullContent := content + fmt.Sprintf("%*s", padding, "") + right

	return StatusBarStyle.Width(s.width).Render(fullContent)
}

// SetWidth sets the status bar width
func (s *StatusBarModel) SetWidth(width int) {
	s.width = width
}

// SetWorkstreamCount updates the workstream count
func (s *StatusBarModel) SetWorkstreamCount(count int) {
	s.workstreamCount = count
}

// SetPairingBranch sets the current pairing branch (empty string if none)
func (s *StatusBarModel) SetPairingBranch(branch string) {
	s.pairingBranch = branch
}

// SetShowHelp toggles help display
func (s *StatusBarModel) SetShowHelp(show bool) {
	s.showHelp = show
}
