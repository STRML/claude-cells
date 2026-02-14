package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// welcomeDialog is a Bubble Tea model for the first-run welcome screen.
// Shows intro, keybindings, then chains to create dialog on Enter.
type welcomeDialog struct {
	stateDir string
	runtime  string
	done     bool
	create   bool // whether user chose to create a workstream
}

func newWelcomeDialog(stateDir, runtime string) welcomeDialog {
	return welcomeDialog{
		stateDir: stateDir,
		runtime:  runtime,
	}
}

func (m welcomeDialog) Init() tea.Cmd {
	return nil
}

func (m welcomeDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "n":
			m.create = true
			m.done = true
			return m, tea.Quit
		case "esc", "q", "ctrl+c":
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m welcomeDialog) View() tea.View {
	if m.done {
		return tea.NewView("")
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  \033[1;32mClaude Cells\033[0m\n")
	b.WriteString("  \033[2m══════════════════════════════════════\033[0m\n\n")
	b.WriteString("  Run parallel Claude Code instances\n")
	b.WriteString("  in isolated Docker containers.\n\n")
	b.WriteString("  Each workstream gets its own branch,\n")
	b.WriteString("  container, and Claude session.\n\n")
	b.WriteString("  \033[2mKeybindings (prefix + key):\033[0m\n")
	b.WriteString("  \033[35mn\033[0m  Create workstream\n")
	b.WriteString("  \033[35mx\033[0m  Destroy workstream\n")
	b.WriteString("  \033[35mm\033[0m  Merge PR\n")
	b.WriteString("  \033[35mp\033[0m  Pause    \033[35mr\033[0m  Resume\n")
	b.WriteString("  \033[35m?\033[0m  Help\n\n")
	b.WriteString("  \033[2m──────────────────────────────────────\033[0m\n\n")
	b.WriteString("  Press \033[1mEnter\033[0m to create your first workstream\n")
	b.WriteString("  Press \033[2mEsc\033[0m for a shell\n")

	return tea.NewView(b.String())
}

// runWelcome shows the welcome screen, then optionally chains to create dialog.
func runWelcome(stateDir, runtime string) error {
	m := newWelcomeDialog(stateDir, runtime)
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return err
	}

	// If user chose to create, launch create dialog inline
	if result.(welcomeDialog).create {
		return runCreateInteractive(stateDir, runtime)
	}
	return nil
}
