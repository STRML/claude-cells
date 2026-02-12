package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// createDialog is a Bubble Tea model for the interactive create dialog.
// Invoked via: ccells create --interactive
// Runs inside tmux display-popup.
type createDialog struct {
	step     int // 0=prompt, 1=branch, 2=confirm
	prompt   string
	branch   string
	cursor   int
	input    string
	err      error
	done     bool
	stateDir string
	runtime  string
}

func newCreateDialog(stateDir, runtime string) createDialog {
	return createDialog{
		step:     0,
		stateDir: stateDir,
		runtime:  runtime,
	}
}

func (m createDialog) Init() tea.Cmd {
	return nil
}

func (m createDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.done = true
			return m, tea.Quit
		case "enter":
			return m.handleEnter()
		case "backspace":
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
		default:
			if len(msg.String()) == 1 {
				m.input += msg.String()
			}
		}
	}
	return m, nil
}

func (m createDialog) handleEnter() (tea.Model, tea.Cmd) {
	switch m.step {
	case 0: // prompt entered
		m.prompt = strings.TrimSpace(m.input)
		if m.prompt == "" {
			m.err = fmt.Errorf("prompt cannot be empty")
			return m, nil
		}
		m.err = nil
		// Auto-generate branch name from prompt
		m.branch = generateBranchName(m.prompt)
		m.input = m.branch
		m.step = 1
	case 1: // branch confirmed/edited
		m.branch = strings.TrimSpace(m.input)
		if m.branch == "" {
			m.err = fmt.Errorf("branch name cannot be empty")
			return m, nil
		}
		m.err = nil
		m.input = ""
		m.step = 2
	case 2: // confirmed
		// Send create request
		if err := runCreate(m.stateDir, m.branch, m.prompt, m.runtime); err != nil {
			m.err = err
			return m, nil
		}
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m createDialog) View() tea.View {
	if m.done {
		return tea.NewView("")
	}

	var b strings.Builder
	b.WriteString("  Create New Workstream\n")
	b.WriteString("  ─────────────────────\n\n")

	switch m.step {
	case 0:
		b.WriteString("  What should this workstream do?\n\n")
		b.WriteString(fmt.Sprintf("  > %s█\n", m.input))
	case 1:
		b.WriteString(fmt.Sprintf("  Task: %s\n\n", m.prompt))
		b.WriteString("  Branch name:\n\n")
		b.WriteString(fmt.Sprintf("  > %s█\n", m.input))
	case 2:
		b.WriteString(fmt.Sprintf("  Task: %s\n", m.prompt))
		b.WriteString(fmt.Sprintf("  Branch: %s\n\n", m.branch))
		b.WriteString("  Press Enter to create, Esc to cancel\n")
	}

	if m.err != nil {
		b.WriteString(fmt.Sprintf("\n  Error: %v\n", m.err))
	}

	b.WriteString("\n  (Esc to cancel)")
	return tea.NewView(b.String())
}

// generateBranchName creates a branch name from a prompt.
func generateBranchName(prompt string) string {
	// Take first 4 words, lowercase, hyphenate
	words := strings.Fields(strings.ToLower(prompt))
	if len(words) > 4 {
		words = words[:4]
	}
	name := strings.Join(words, "-")
	// Remove non-alphanumeric chars except hyphens
	var clean strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			clean.WriteRune(r)
		}
	}
	result := strings.Trim(clean.String(), "-")
	if result == "" {
		return "workstream"
	}
	return result
}
