package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

type rmStep int

const (
	rmStepSelect  rmStep = iota // Select workstream (skipped if only 1)
	rmStepConfirm               // Type "destroy" to confirm
)

const rmInputBoxWidth = 38 // inner width for the confirmation input box

// rmDialog is a Bubble Tea model for the interactive rm dialog.
// Invoked via: ccells rm --interactive
//
// Matches the old TUI layout:
//
//	Destroy workstream "name"?
//	This will:
//	  • Stop and remove the container
//	  • End any active Mutagen sync
//	  • Delete local branch (remote untouched)
//	Type "destroy" to confirm:
//	╭──────────────────────────────────────╮
//	│ ›                                    │
//	╰──────────────────────────────────────╯
type rmDialog struct {
	step         rmStep
	workstreams  []string
	selected     int
	confirmInput string // text typed in confirmation box
	err          error
	done         bool
	stateDir     string
}

func newRmDialog(stateDir string, workstreams []string) rmDialog {
	return rmDialog{
		stateDir:    stateDir,
		workstreams: workstreams,
	}
}

func (m rmDialog) Init() tea.Cmd {
	// Auto-select if only one workstream
	if len(m.workstreams) == 1 {
		m.selected = 0
		m.step = rmStepConfirm
	}
	return nil
}

func (m rmDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.done = true
			return m, tea.Quit
		case "esc":
			if m.step == rmStepConfirm && len(m.workstreams) > 1 {
				// Go back to selection
				m.step = rmStepSelect
				m.confirmInput = ""
				m.err = nil
				return m, nil
			}
			m.done = true
			return m, tea.Quit
		}

		switch m.step {
		case rmStepSelect:
			return m.updateSelect(msg)
		case rmStepConfirm:
			return m.updateConfirm(msg)
		}
	}
	return m, nil
}

func (m rmDialog) updateSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
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
		m.step = rmStepConfirm
		m.confirmInput = ""
		m.err = nil
	}
	return m, nil
}

func (m rmDialog) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.confirmInput == "destroy" {
			name := m.workstreams[m.selected]
			if err := runRemove(m.stateDir, name); err != nil {
				m.err = err
				return m, nil
			}
			m.done = true
			return m, tea.Quit
		}
		m.err = fmt.Errorf("type 'destroy' to confirm")
		return m, nil
	case "backspace":
		if len(m.confirmInput) > 0 {
			_, size := utf8.DecodeLastRuneInString(m.confirmInput)
			m.confirmInput = m.confirmInput[:len(m.confirmInput)-size]
		}
	case "space":
		// Ignore spaces — "destroy" has no spaces
	default:
		if len(msg.String()) == 1 {
			m.confirmInput += msg.String()
		}
	}
	return m, nil
}

func (m rmDialog) View() tea.View {
	if m.done {
		return tea.NewView("")
	}

	var b strings.Builder

	if len(m.workstreams) == 0 {
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  %sDestroy Workstream%s\n", cCyanBold, cReset))
		b.WriteString(fmt.Sprintf("  %s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n\n", cDim, cReset))
		b.WriteString("  No workstreams to destroy.\n")
		b.WriteString(fmt.Sprintf("\n  %s(Esc to close)%s", cDim, cReset))
		return tea.NewView(b.String())
	}

	switch m.step {
	case rmStepSelect:
		m.viewSelect(&b)
	case rmStepConfirm:
		m.viewConfirm(&b)
	}

	return tea.NewView(b.String())
}

func (m rmDialog) viewSelect(b *strings.Builder) {
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %sDestroy Workstream%s\n", cCyanBold, cReset))
	b.WriteString(fmt.Sprintf("  %s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n\n", cDim, cReset))
	b.WriteString(fmt.Sprintf("  %sSelect workstream to destroy:%s\n\n", cDim, cReset))

	for i, ws := range m.workstreams {
		cursor := "  "
		if i == m.selected {
			cursor = fmt.Sprintf("%s→%s ", cCyan, cReset)
		}
		b.WriteString(fmt.Sprintf("  %s%s\n", cursor, ws))
	}

	b.WriteString(fmt.Sprintf("\n  %s[↑/↓] navigate  [Enter] select  [Esc] Cancel%s", cDim, cReset))
}

func (m rmDialog) viewConfirm(b *strings.Builder) {
	name := m.workstreams[m.selected]

	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %sDestroy workstream \"%s\"?%s\n", cCyanBold, name, cReset))
	b.WriteString(fmt.Sprintf("  %s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n\n", cDim, cReset))

	b.WriteString("  This will:\n")
	b.WriteString("    • Stop and remove the container\n")
	b.WriteString("    • End any active Mutagen sync\n")
	b.WriteString("    • Delete local branch (remote untouched)\n\n")

	b.WriteString(fmt.Sprintf("  Type %s\"destroy\"%s to confirm:\n\n", cBold, cReset))

	// Bordered input box
	contentWidth := rmInputBoxWidth - 2 // minus leading+trailing space
	input := "› " + m.confirmInput + "█"
	runes := []rune(input)
	if len(runes) > contentWidth {
		runes = runes[len(runes)-contentWidth:]
	}
	pad := contentWidth - len(runes)
	if pad < 0 {
		pad = 0
	}

	b.WriteString(fmt.Sprintf("  ╭%s╮\n", strings.Repeat("─", rmInputBoxWidth)))
	b.WriteString(fmt.Sprintf("  │ %s%s │\n", string(runes), strings.Repeat(" ", pad)))
	b.WriteString(fmt.Sprintf("  ╰%s╯\n", strings.Repeat("─", rmInputBoxWidth)))

	if m.err != nil {
		b.WriteString(fmt.Sprintf("\n  %s✗ %v%s\n", cRed, m.err, cReset))
	}

	b.WriteString(fmt.Sprintf("\n  %s[Esc] Cancel%s", cDim, cReset))
}
