package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// mergeDialog is a Bubble Tea model for the interactive merge/PR dialog.
// Invoked via: ccells merge --interactive
type mergeDialog struct {
	step     int // 0=select workstream, 1=confirm
	items    []string
	selected int
	err      error
	done     bool
	stateDir string
}

func newMergeDialog(stateDir string, workstreams []string) mergeDialog {
	return mergeDialog{
		stateDir: stateDir,
		items:    workstreams,
	}
}

func (m mergeDialog) Init() tea.Cmd {
	return nil
}

func (m mergeDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if m.selected < len(m.items)-1 {
				m.selected++
			}
		case "enter":
			if m.step == 0 {
				m.step = 1
			} else {
				// TODO(task-14): Send merge/PR request to daemon
				fmt.Printf("Creating PR for %s...\n", m.items[m.selected])
				m.done = true
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m mergeDialog) View() tea.View {
	if m.done {
		return tea.NewView("")
	}

	var b strings.Builder
	b.WriteString("  Create Pull Request\n")
	b.WriteString("  ───────────────────\n\n")

	if len(m.items) == 0 {
		b.WriteString("  No workstreams available.\n")
		return tea.NewView(b.String())
	}

	if m.step == 0 {
		b.WriteString("  Select workstream:\n\n")
		for i, ws := range m.items {
			cursor := "  "
			if i == m.selected {
				cursor = "> "
			}
			b.WriteString(fmt.Sprintf("  %s%s\n", cursor, ws))
		}
	} else {
		b.WriteString(fmt.Sprintf("  Create PR for '%s'?\n\n", m.items[m.selected]))
		b.WriteString("  Press Enter to create PR, Esc to cancel\n")
	}

	if m.err != nil {
		b.WriteString(fmt.Sprintf("\n  Error: %v\n", m.err))
	}

	b.WriteString("\n  (Esc to cancel)")
	return tea.NewView(b.String())
}
