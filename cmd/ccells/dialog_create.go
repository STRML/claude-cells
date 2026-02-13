package main

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/STRML/claude-cells/internal/claude"
	"github.com/STRML/claude-cells/internal/workstream"
)

// summarizeResultMsg is the result of an async title generation via Claude CLI.
type summarizeResultMsg struct {
	title string
	err   error
}

// createResultMsg is the result of an async create operation.
type createResultMsg struct {
	err error
}

// createDialog is a Bubble Tea model for the interactive create dialog.
// Invoked via: ccells create --interactive
// Runs inside tmux display-popup or the initial pane.
//
// Flow: 0=prompt → 1=summarizing → 2=confirm → 3=creating
// Title is generated via Claude CLI, branch name is derived from the title.
type createDialog struct {
	step     int // 0=prompt, 1=summarizing, 2=confirm, 3=creating
	prompt   string
	title    string // AI-generated short title
	branch   string
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
	case summarizeResultMsg:
		if msg.err != nil {
			// Claude failed — fall back to generating branch from prompt directly
			m.title = ""
			m.branch = workstream.GenerateBranchName(m.prompt)
		} else {
			m.title = msg.title
			m.branch = workstream.GenerateBranchName(msg.title)
		}
		m.step = 2
		return m, nil
	case createResultMsg:
		if msg.err != nil {
			m.err = msg.err
			m.step = 2 // back to confirm
			return m, nil
		}
		m.done = true
		return m, tea.Quit
	case tea.KeyMsg:
		if m.step == 1 || m.step == 3 {
			// Summarizing or creating — ignore keys
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c", "esc":
			m.done = true
			return m, tea.Quit
		case "enter":
			return m.handleEnter()
		case "backspace":
			if m.step == 0 && len(m.input) > 0 {
				_, size := utf8.DecodeLastRuneInString(m.input)
				m.input = m.input[:len(m.input)-size]
			}
		case "space":
			if m.step == 0 {
				m.input += " "
			}
		default:
			if m.step == 0 && len(msg.String()) == 1 {
				m.input += msg.String()
			}
		}
	}
	return m, nil
}

func (m createDialog) handleEnter() (tea.Model, tea.Cmd) {
	switch m.step {
	case 0: // prompt entered → generate title via Claude
		m.prompt = strings.TrimSpace(m.input)
		if m.prompt == "" {
			m.err = fmt.Errorf("prompt cannot be empty")
			return m, nil
		}
		m.err = nil
		m.input = ""
		m.step = 1
		prompt := m.prompt
		return m, func() tea.Msg {
			return generateTitle(prompt)
		}
	case 2: // confirmed — launch async create
		m.step = 3
		stateDir := m.stateDir
		branch := m.branch
		prompt := m.prompt
		runtime := m.runtime
		return m, func() tea.Msg {
			return createResultMsg{err: runCreate(stateDir, branch, prompt, runtime)}
		}
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
		b.WriteString("  Generating title...\n")
	case 2:
		if m.title != "" {
			b.WriteString(fmt.Sprintf("  Title: %s\n", m.title))
		}
		b.WriteString(fmt.Sprintf("  Task: %s\n", m.prompt))
		b.WriteString(fmt.Sprintf("  Branch: %s\n\n", m.branch))
		b.WriteString("  Press Enter to create, Esc to cancel\n")
	case 3:
		if m.title != "" {
			b.WriteString(fmt.Sprintf("  Title: %s\n", m.title))
		}
		b.WriteString(fmt.Sprintf("  Task: %s\n", m.prompt))
		b.WriteString(fmt.Sprintf("  Branch: %s\n\n", m.branch))
		b.WriteString("  Creating workstream...\n")
	}

	if m.err != nil {
		b.WriteString(fmt.Sprintf("\n  Error: %v\n", m.err))
	}

	b.WriteString("\n  (Esc to cancel)")
	return tea.NewView(b.String())
}

// generateTitle calls Claude CLI to generate a short title from a task prompt.
// Returns a summarizeResultMsg with the title or an error.
func generateTitle(taskPrompt string) summarizeResultMsg {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	prompt := fmt.Sprintf(`Generate a 3-5 word title summarizing this task. Output ONLY the title text, no quotes or explanation.

Task: %s`, taskPrompt)

	title, err := claude.Query(ctx, prompt, nil)
	if err != nil {
		return summarizeResultMsg{title: "", err: err}
	}

	title = strings.TrimSpace(title)
	// Strip quotes if Claude added them
	title = strings.Trim(title, `"'`)
	// Strip markdown code blocks if wrapped
	if idx := strings.Index(title, "```"); idx >= 0 {
		// Simple strip: remove lines starting with ```
		var lines []string
		for _, line := range strings.Split(title, "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "```") {
				lines = append(lines, line)
			}
		}
		title = strings.TrimSpace(strings.Join(lines, "\n"))
	}

	if title == "" {
		return summarizeResultMsg{title: "", err: fmt.Errorf("empty title generated")}
	}

	return summarizeResultMsg{title: title, err: nil}
}
