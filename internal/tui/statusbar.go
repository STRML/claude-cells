package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// StatusBarModel represents the bottom status bar
type StatusBarModel struct {
	width           int
	workstreamCount int
	pairingBranch   string
	showHelp        bool
	inputMode       bool
	layoutName      string
	repoPath        string
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
	// Mode indicator
	var modeIndicator string
	if s.inputMode {
		inputStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("#00AA00")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1).
			Bold(true)
		modeIndicator = inputStyle.Render("INPUT")
	} else {
		navStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("#0066CC")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1).
			Bold(true)
		modeIndicator = navStyle.Render("NAV")
	}

	// Left section: mode + repo path + app name and count
	repoPathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA"))
	left := fmt.Sprintf("%s %s ccells: %d workstreams", modeIndicator, repoPathStyle.Render(s.repoPath), s.workstreamCount)

	// Layout indicator (only show if more than 1 workstream)
	if s.layoutName != "" && s.workstreamCount > 1 {
		layoutStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
		left += layoutStyle.Render(fmt.Sprintf(" [%s]", s.layoutName))
	}

	// Center section: pairing indicator
	var center string
	if s.pairingBranch != "" {
		pairingStyle := lipgloss.NewStyle().Foreground(ColorPairing).Bold(true)
		center = fmt.Sprintf(" | %s Pairing: %s", IndicatorPairing, pairingStyle.Render(s.pairingBranch))
	}

	// Right section: action key hints
	// Navigation hints are in the top title bar, so focus on actions here
	hints := []string{
		KeyHint("n", "ew"),
		KeyHint("d", "estroy"),
		KeyHint("m", "erge/PR"),
		KeyHint("p", "air"),
		KeyHint("l", "ogs"),
		KeyHint("`", "panel"),
		KeyHint("r", "esources"),
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

// SetInputMode sets whether the app is in input mode
func (s *StatusBarModel) SetInputMode(inputMode bool) {
	s.inputMode = inputMode
}

// SetLayoutName sets the current layout name for display
func (s *StatusBarModel) SetLayoutName(name string) {
	s.layoutName = name
}

// SetRepoPath sets the repository path to display
func (s *StatusBarModel) SetRepoPath(path string) {
	s.repoPath = path
}
