package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/samuelreed/docker-tui/internal/workstream"
)

// AppModel is the main application model
type AppModel struct {
	manager     *workstream.Manager
	panes       []PaneModel
	focusedPane int
	statusBar   StatusBarModel
	dialog      *DialogModel
	width       int
	height      int
	quitting    bool
}

// NewAppModel creates a new application model
func NewAppModel() AppModel {
	return AppModel{
		manager:   workstream.NewManager(),
		statusBar: NewStatusBarModel(),
	}
}

// Init initializes the application
func (m AppModel) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()
		return m, nil

	case tea.KeyMsg:
		// If dialog is active, handle dialog input
		if m.dialog != nil {
			newDialog, cmd := m.dialog.Update(msg)
			m.dialog = &newDialog
			return m, cmd
		}

		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "n":
			// New workstream dialog
			dialog := NewWorkstreamDialog()
			dialog.SetSize(50, 15)
			m.dialog = &dialog
			return m, nil

		case "d":
			// Destroy focused workstream
			if len(m.panes) > 0 && m.focusedPane < len(m.panes) {
				ws := m.panes[m.focusedPane].Workstream()
				dialog := NewDestroyDialog(ws.BranchName, ws.ID)
				dialog.SetSize(50, 15)
				m.dialog = &dialog
			}
			return m, nil

		case "p":
			// Toggle pairing mode (placeholder)
			return m, nil

		case "m":
			// Merge/PR menu (placeholder)
			return m, nil

		case "tab":
			// Cycle focus
			if len(m.panes) > 0 {
				m.panes[m.focusedPane].SetFocused(false)
				m.focusedPane = (m.focusedPane + 1) % len(m.panes)
				m.panes[m.focusedPane].SetFocused(true)
			}
			return m, nil

		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			// Direct focus by number
			idx := int(msg.String()[0] - '1')
			if idx < len(m.panes) {
				if m.focusedPane < len(m.panes) {
					m.panes[m.focusedPane].SetFocused(false)
				}
				m.focusedPane = idx
				m.panes[m.focusedPane].SetFocused(true)
			}
			return m, nil

		case "?":
			// Help (placeholder)
			return m, nil
		}

	case DialogConfirmMsg:
		m.dialog = nil
		switch msg.Type {
		case DialogNewWorkstream:
			// Create new workstream
			ws := workstream.New(msg.Value)
			m.manager.Add(ws)
			pane := NewPaneModel(ws)
			m.panes = append(m.panes, pane)
			m.updateLayout()
			// Focus the new pane
			if m.focusedPane < len(m.panes)-1 && m.focusedPane < len(m.panes) {
				m.panes[m.focusedPane].SetFocused(false)
			}
			m.focusedPane = len(m.panes) - 1
			m.panes[m.focusedPane].SetFocused(true)

		case DialogDestroy:
			// Destroy workstream
			for i, pane := range m.panes {
				if pane.Workstream().ID == msg.WorkstreamID {
					m.manager.Remove(msg.WorkstreamID)
					m.panes = append(m.panes[:i], m.panes[i+1:]...)
					if m.focusedPane >= len(m.panes) && len(m.panes) > 0 {
						m.focusedPane = len(m.panes) - 1
					}
					if len(m.panes) > 0 {
						m.panes[m.focusedPane].SetFocused(true)
					}
					m.updateLayout()
					break
				}
			}
		}
		return m, nil

	case DialogCancelMsg:
		m.dialog = nil
		return m, nil

	case PromptMsg:
		// Handle prompt from pane
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				m.panes[i].AppendOutput(fmt.Sprintf("> %s\n\n", msg.Prompt))
				m.panes[i].AppendOutput("Claude: Processing...\n")
				// In real implementation, this would send to Claude
			}
		}
		return m, nil
	}

	// Update focused pane
	if len(m.panes) > 0 && m.focusedPane < len(m.panes) {
		var cmd tea.Cmd
		m.panes[m.focusedPane], cmd = m.panes[m.focusedPane].Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// View renders the application
func (m AppModel) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	var sections []string

	// Panes section
	if len(m.panes) > 0 {
		paneViews := m.renderPanes()
		sections = append(sections, paneViews)
	} else {
		empty := lipgloss.NewStyle().
			Width(m.width).
			Height(m.height - 3).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color("#666666")).
			Render("No workstreams. Press [n] to create one.")
		sections = append(sections, empty)
	}

	// Status bar
	m.statusBar.SetWidth(m.width)
	m.statusBar.SetWorkstreamCount(m.manager.Count())
	if pairing := m.manager.GetPairing(); pairing != nil {
		m.statusBar.SetPairingBranch(pairing.BranchName)
	} else {
		m.statusBar.SetPairingBranch("")
	}
	sections = append(sections, m.statusBar.View())

	view := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Overlay dialog if active
	if m.dialog != nil {
		view = m.overlayDialog(view)
	}

	return view
}

// renderPanes renders all panes in a grid layout
func (m AppModel) renderPanes() string {
	if len(m.panes) == 0 {
		return ""
	}

	// Simple horizontal split for up to 2 panes
	// More complex layouts can be added later
	if len(m.panes) == 1 {
		return m.panes[0].View()
	}

	// Two panes side by side
	if len(m.panes) == 2 {
		left := m.panes[0].View()
		right := m.panes[1].View()
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}

	// More than 2: first two side by side, rest below
	left := m.panes[0].View()
	right := m.panes[1].View()
	top := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	var bottomPanes []string
	for i := 2; i < len(m.panes); i++ {
		bottomPanes = append(bottomPanes, m.panes[i].View())
	}
	bottom := lipgloss.JoinVertical(lipgloss.Left, bottomPanes...)

	return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
}

// overlayDialog overlays the dialog on top of the view
func (m AppModel) overlayDialog(background string) string {
	dialog := m.dialog.View()

	// Center the dialog
	dialogWidth := lipgloss.Width(dialog)
	dialogHeight := lipgloss.Height(dialog)

	x := (m.width - dialogWidth) / 2
	y := (m.height - dialogHeight) / 2

	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	// Simple overlay by placing dialog
	lines := strings.Split(background, "\n")
	dialogLines := strings.Split(dialog, "\n")

	for i, dLine := range dialogLines {
		lineIdx := y + i
		if lineIdx >= 0 && lineIdx < len(lines) {
			line := lines[lineIdx]
			// Pad line if needed
			for len(line) < x+dialogWidth {
				line += " "
			}
			// Insert dialog line
			runes := []rune(line)
			dRunes := []rune(dLine)
			for j, r := range dRunes {
				if x+j < len(runes) {
					runes[x+j] = r
				}
			}
			lines[lineIdx] = string(runes)
		}
	}

	return strings.Join(lines, "\n")
}

// updateLayout recalculates pane sizes
func (m *AppModel) updateLayout() {
	statusBarHeight := 1
	availableHeight := m.height - statusBarHeight

	if len(m.panes) == 0 {
		return
	}

	if len(m.panes) == 1 {
		m.panes[0].SetSize(m.width, availableHeight)
		return
	}

	// Two or more: split horizontally
	paneWidth := m.width / 2
	paneHeight := availableHeight

	if len(m.panes) > 2 {
		// Reserve space for additional panes below
		paneHeight = availableHeight * 2 / 3
	}

	for i := range m.panes {
		if i < 2 {
			m.panes[i].SetSize(paneWidth, paneHeight)
		} else {
			m.panes[i].SetSize(m.width, (availableHeight-paneHeight)/(len(m.panes)-2))
		}
	}
}

// Manager returns the workstream manager
func (m *AppModel) Manager() *workstream.Manager {
	return m.manager
}
