package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// rmDialog is a Bubble Tea model for the interactive rm dialog.
// Invoked via: ccells rm --interactive
type rmDialog struct {
	workstreams []string
	selected    int
	confirmed   bool
	err         error
	done        bool
	stateDir    string
}

func newRmDialog(stateDir string, workstreams []string) rmDialog {
	return rmDialog{
		stateDir:    stateDir,
		workstreams: workstreams,
	}
}

func (m rmDialog) Init() tea.Cmd {
	return nil
}

func (m rmDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.done = true
			return m, tea.Quit
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.workstreams)-1 {
				m.selected++
			}
		case "enter":
			if len(m.workstreams) == 0 {
				return m, nil
			}
			if !m.confirmed {
				m.confirmed = true
			} else {
				name := m.workstreams[m.selected]
				if err := runRemove(m.stateDir, name); err != nil {
					m.err = err
					return m, nil
				}
				m.done = true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m rmDialog) View() tea.View {
	if m.done {
		return tea.NewView("")
	}

	var b strings.Builder
	b.WriteString("  Destroy Workstream\n")
	b.WriteString("  ──────────────────\n\n")

	if len(m.workstreams) == 0 {
		b.WriteString("  No workstreams to destroy.\n")
		return tea.NewView(b.String())
	}

	if !m.confirmed {
		b.WriteString("  Select workstream to destroy:\n\n")
		for i, ws := range m.workstreams {
			cursor := "  "
			if i == m.selected {
				cursor = "> "
			}
			b.WriteString(fmt.Sprintf("  %s%s\n", cursor, ws))
		}
	} else {
		b.WriteString(fmt.Sprintf("  Destroy '%s'?\n\n", m.workstreams[m.selected]))
		b.WriteString("  This will stop the container and delete the worktree.\n\n")
		b.WriteString("  Press Enter to confirm, Esc to cancel\n")
	}

	if m.err != nil {
		b.WriteString(fmt.Sprintf("\n  Error: %v\n", m.err))
	}

	b.WriteString("\n  (Esc to cancel)")
	return tea.NewView(b.String())
}
