package main

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Pearlescent blue shimmer palette (256-color ANSI).
// Smooth gradient from medium blue through cyan to white and back.
var shimmerPalette = []int{
	33, 33, 39, 39, 45, 51, 87, 123, 159, 195, 231,
	195, 159, 123, 87, 51, 45, 39, 39, 33, 33, 33,
}

const shimmerInterval = 120 * time.Millisecond

// welcomeTickMsg drives the shimmer animation (separate from create dialog's tickMsg).
type welcomeTickMsg time.Time

// welcomeDialog is a Bubble Tea model for the first-run welcome screen.
// Shows intro, keybindings, then chains to create dialog on Enter.
type welcomeDialog struct {
	stateDir string
	runtime  string
	done     bool
	create   bool // whether user chose to create a workstream
	width    int
	height   int
	frame    int
}

func newWelcomeDialog(stateDir, runtime string) welcomeDialog {
	return welcomeDialog{
		stateDir: stateDir,
		runtime:  runtime,
	}
}

func shimmerTickCmd() tea.Cmd {
	return tea.Tick(shimmerInterval, func(t time.Time) tea.Msg {
		return welcomeTickMsg(t)
	})
}

func (m welcomeDialog) Init() tea.Cmd {
	return shimmerTickCmd()
}

func (m welcomeDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case welcomeTickMsg:
		m.frame++
		return m, shimmerTickCmd()
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

// renderShimmerTitle renders "CLAUDE CELLS" with per-character pearlescent animation.
func renderShimmerTitle(frame int) string {
	title := "CLAUDE CELLS"
	var b strings.Builder
	b.WriteString("\033[1m") // bold
	for i, ch := range title {
		if ch == ' ' {
			b.WriteRune(' ')
			continue
		}
		idx := (i + frame) % len(shimmerPalette)
		b.WriteString(fmt.Sprintf("\033[38;5;%dm%c", shimmerPalette[idx], ch))
	}
	b.WriteString("\033[0m")
	return b.String()
}

// visibleLen returns the number of visible characters in a string,
// excluding ANSI escape sequences.
func visibleLen(s string) int {
	n := 0
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		n++
	}
	return n
}

func (m welcomeDialog) View() tea.View {
	if m.done {
		return tea.NewView("")
	}

	title := renderShimmerTitle(m.frame)
	lines := []string{
		title,
		"\033[2m══════════════════════════════════════\033[0m",
		"",
		"Run parallel Claude Code instances",
		"in isolated Docker containers.",
		"",
		"Each workstream gets its own branch,",
		"container, and Claude session.",
		"",
		"\033[2mKeybindings (prefix + key):\033[0m",
		"\033[35mn\033[0m  Create workstream",
		"\033[35mx\033[0m  Destroy workstream",
		"\033[35mm\033[0m  Merge PR",
		"\033[35m?\033[0m  Help",
		"",
		"\033[2m──────────────────────────────────────\033[0m",
		"",
		"Press \033[1mEnter\033[0m to create your first workstream",
	}

	// Find max visible width for centering
	maxWidth := 0
	for _, line := range lines {
		w := visibleLen(line)
		if w > maxWidth {
			maxWidth = w
		}
	}

	contentHeight := len(lines)

	// Calculate padding for centering
	leftPad := 0
	topPad := 0
	if m.width > maxWidth+4 {
		leftPad = (m.width - maxWidth) / 2
	}
	if m.height > contentHeight+2 {
		topPad = (m.height - contentHeight) / 2
	}

	var b strings.Builder
	pad := strings.Repeat(" ", leftPad)

	// Vertical centering
	for i := 0; i < topPad; i++ {
		b.WriteString("\n")
	}

	// Content with horizontal centering
	for _, line := range lines {
		b.WriteString(pad)
		b.WriteString(line)
		b.WriteString("\n")
	}

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
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
