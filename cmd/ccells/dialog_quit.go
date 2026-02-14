package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// quitDialog is a Bubble Tea model for the interactive quit confirmation dialog.
// Invoked via: ccells down --interactive (or prefix+Q keybinding)
//
// Layout:
//
//	Quit ccells?
//	━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
//	All workstreams will be paused and can
//	be resumed by running ccells again.
//
//	→ Yes, quit
//	  No, cancel
//
//	[↑/↓] navigate  [Enter] select  [Esc] Cancel
type quitDialog struct {
	selected int  // 0=Yes, 1=No
	done     bool // dialog completed
	quit     bool // user chose to quit
}

func newQuitDialog() quitDialog {
	return quitDialog{
		selected: 0, // default to "Yes"
	}
}

func (m quitDialog) Init() tea.Cmd {
	return nil
}

func (m quitDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.done = true
			return m, tea.Quit
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < 1 {
				m.selected++
			}
		case "enter":
			m.done = true
			m.quit = m.selected == 0
			return m, tea.Quit
		case "y", "Y":
			m.done = true
			m.quit = true
			return m, tea.Quit
		case "n", "N":
			m.done = true
			m.quit = false
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m quitDialog) View() tea.View {
	if m.done {
		return tea.NewView("")
	}

	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %sQuit ccells?%s\n", cCyanBold, cReset))
	b.WriteString(fmt.Sprintf("  %s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n\n", cDim, cReset))

	b.WriteString("  All workstreams will be paused and can\n")
	b.WriteString("  be resumed by running ccells again.\n\n")

	options := []string{"Yes, quit", "No, cancel"}
	for i, opt := range options {
		cursor := "  "
		if i == m.selected {
			cursor = fmt.Sprintf("%s→%s ", cCyan, cReset)
		}
		b.WriteString(fmt.Sprintf("  %s%s\n", cursor, opt))
	}

	b.WriteString(fmt.Sprintf("\n  %s[↑/↓] navigate  [Enter] select  [Esc] Cancel%s", cDim, cReset))

	return tea.NewView(b.String())
}
