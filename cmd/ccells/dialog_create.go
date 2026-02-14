package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/STRML/claude-cells/internal/claude"
	"github.com/STRML/claude-cells/internal/git"
	"github.com/STRML/claude-cells/internal/workstream"
)

// summarizeResultMsg is the result of an async title generation via Claude CLI.
type summarizeResultMsg struct {
	title string
	err   error
}

// untrackedFilesMsg is the result of checking for untracked files.
type untrackedFilesMsg struct {
	files []string
	err   error
}

// createResultMsg is the result of an async create operation.
type createResultMsg struct {
	err           error
	containerName string
}

// tickMsg drives the spinner animation.
type tickMsg time.Time

// Braille spinner frames.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const spinnerInterval = 80 * time.Millisecond

// ANSI color helpers for the create dialog.
const (
	cBold      = "\033[1m"
	cDim       = "\033[2m"
	cReset     = "\033[0m"
	cCyan      = "\033[36m"
	cCyanBold  = "\033[1;36m"
	cMagenta   = "\033[35m"
	cGreen     = "\033[32m"
	cGreenBold = "\033[1;32m"
	cYellow    = "\033[33m"
	cRed       = "\033[31m"
	cGray      = "\033[90m"
	cWhite     = "\033[97m"
)

const inputBoxInnerWidth = 58 // width between │ chars
const inputBoxRows = 4        // visible rows in the input box

// createDialog is a Bubble Tea model for the interactive create dialog.
// Invoked via: ccells create --interactive
// Runs inside tmux display-popup or the initial pane.
//
// Flow: 0=prompt → 1=generating (animated) → [untracked files prompt] → 2=creating (animated)
// Title is generated via Claude CLI, branch name is derived from the title.
// If untracked files are found, user is prompted before creation starts.
type createDialog struct {
	step          int // 0=prompt, 1=generating, 2=creating
	prompt        string
	title         string // AI-generated short title
	branch        string
	input         string
	err           error
	done          bool
	stateDir      string
	runtime       string
	frame         int    // spinner frame index
	containerName string // set after successful creation (for exec)

	// Untracked files flow
	untrackedFiles []string // populated after title gen
	copyUntracked  bool     // user's choice
	showUntracked  bool     // true when showing the untracked files selection
	untrackedIdx   int      // 0=Yes, 1=No for untracked selection

	// For testability: override the function that checks for untracked files
	checkUntrackedFn func() ([]string, error)
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

func spinnerTick() tea.Cmd {
	return tea.Tick(spinnerInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m createDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		// Advance spinner frame during animated steps
		if m.step == 1 || m.step == 2 {
			m.frame = (m.frame + 1) % len(spinnerFrames)
			return m, spinnerTick()
		}
		return m, nil
	case summarizeResultMsg:
		if msg.err != nil {
			// Claude failed — fall back to generating branch from prompt directly
			m.title = ""
			m.branch = workstream.GenerateBranchName(m.prompt)
		} else {
			m.title = msg.title
			m.branch = workstream.GenerateBranchName(msg.title)
		}
		// Check for untracked files before creating
		checkFn := m.checkUntrackedFn
		if checkFn == nil {
			checkFn = defaultCheckUntracked
		}
		return m, func() tea.Msg {
			files, err := checkFn()
			return untrackedFilesMsg{files: files, err: err}
		}
	case untrackedFilesMsg:
		// Ignore errors — just skip the prompt
		if msg.err == nil && len(msg.files) > 0 {
			m.untrackedFiles = msg.files
			m.copyUntracked = true // default selection is Yes
			m.showUntracked = true
			m.untrackedIdx = 0 // default to "Yes"
			return m, nil      // show selection, no spinner
		}
		// No untracked files — proceed directly to create
		return m.startCreate()
	case createResultMsg:
		if msg.err != nil {
			m.err = msg.err
			m.step = 0 // back to prompt for retry
			m.input = m.prompt
			return m, nil
		}
		m.containerName = msg.containerName
		m.done = true
		return m, tea.Quit
	case tea.KeyMsg:
		// Handle untracked files selection
		if m.showUntracked {
			return m.handleUntrackedKey(msg)
		}
		if m.step == 1 || m.step == 2 {
			// Generating or creating — only allow quit
			if msg.String() == "ctrl+c" || msg.String() == "esc" {
				m.done = true
				return m, tea.Quit
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c", "esc":
			m.done = true
			return m, tea.Quit
		case "shift+enter":
			if m.step == 0 {
				m.input += "\n"
			}
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

// handleUntrackedKey processes key input during the untracked files selection.
func (m createDialog) handleUntrackedKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.done = true
		return m, tea.Quit
	case "up", "k":
		if m.untrackedIdx > 0 {
			m.untrackedIdx--
		}
	case "down", "j":
		if m.untrackedIdx < 1 {
			m.untrackedIdx++
		}
	case "enter":
		m.copyUntracked = m.untrackedIdx == 0
		m.showUntracked = false
		return m.startCreate()
	case "y", "Y":
		m.copyUntracked = true
		m.showUntracked = false
		return m.startCreate()
	case "n", "N":
		m.copyUntracked = false
		m.showUntracked = false
		return m.startCreate()
	}
	return m, nil
}

// startCreate advances to step 2 and dispatches the async create operation.
func (m createDialog) startCreate() (tea.Model, tea.Cmd) {
	m.step = 2
	m.frame = 0
	stateDir := m.stateDir
	branch := m.branch
	prompt := m.prompt
	runtime := m.runtime
	copyUntracked := m.copyUntracked
	untrackedFiles := m.untrackedFiles
	return m, tea.Batch(spinnerTick(), func() tea.Msg {
		var opts []createOpts
		if copyUntracked && len(untrackedFiles) > 0 {
			opts = append(opts, createOpts{
				CopyUntracked:  true,
				UntrackedFiles: untrackedFiles,
			})
		}
		// skipPane=true: the dialog pane will exec into the container
		result, err := runCreate(stateDir, branch, prompt, runtime, true, opts...)
		if err != nil {
			return createResultMsg{err: err}
		}
		return createResultMsg{containerName: result.ContainerName}
	})
}

func (m createDialog) handleEnter() (tea.Model, tea.Cmd) {
	if m.step != 0 {
		return m, nil
	}
	// prompt entered → generate title via Claude
	m.prompt = strings.TrimSpace(m.input)
	if m.prompt == "" {
		m.err = fmt.Errorf("prompt cannot be empty")
		return m, nil
	}
	m.err = nil
	m.input = ""
	m.step = 1
	m.frame = 0
	prompt := m.prompt
	return m, tea.Batch(spinnerTick(), func() tea.Msg {
		return generateTitle(prompt)
	})
}

func (m createDialog) View() tea.View {
	if m.done {
		return tea.NewView("")
	}

	var b strings.Builder

	// Untracked files gets its own full view
	if m.showUntracked {
		m.viewUntracked(&b)
		return tea.NewView(b.String())
	}

	b.WriteString("\n")

	// Header
	b.WriteString(fmt.Sprintf("  %sNew Workstream%s\n", cCyanBold, cReset))
	b.WriteString(fmt.Sprintf("  %s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", cDim, cReset))

	spinner := spinnerFrames[m.frame%len(spinnerFrames)]

	switch m.step {
	case 0:
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  %sEnter a prompt for Claude:%s\n\n", cWhite, cReset))
		b.WriteString(renderInputBox(m.input))
		if m.err != nil {
			b.WriteString(fmt.Sprintf("\n  %s✗ %v%s\n", cRed, m.err, cReset))
		}
		b.WriteString(fmt.Sprintf("\n  %s[Shift+Enter] newline  [Enter] create  [Esc] Cancel%s", cDim, cReset))
	case 1:
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  %sTask%s  %s\n\n", cGray, cReset, m.prompt))
		b.WriteString(fmt.Sprintf("  %s%s%s %sGenerating title...%s\n", cCyan, spinner, cReset, cDim, cReset))
		b.WriteString(fmt.Sprintf("\n  %s(Esc to cancel)%s", cDim, cReset))
	case 2:
		b.WriteString("\n")
		if m.title != "" {
			b.WriteString(fmt.Sprintf("  %sTitle%s   %s%s%s\n", cGray, cReset, cWhite, m.title, cReset))
		}
		b.WriteString(fmt.Sprintf("  %sTask%s    %s\n", cGray, cReset, m.prompt))
		b.WriteString(fmt.Sprintf("  %sBranch%s  %s%s%s\n\n", cGray, cReset, cGreen, m.branch, cReset))
		b.WriteString(fmt.Sprintf("  %s%s%s %sCreating workstream...%s\n", cCyan, spinner, cReset, cDim, cReset))
		if m.err != nil {
			b.WriteString(fmt.Sprintf("\n  %s✗ %v%s\n", cRed, m.err, cReset))
		}
		b.WriteString(fmt.Sprintf("\n  %s(Esc to cancel)%s", cDim, cReset))
	}

	return tea.NewView(b.String())
}

func (m createDialog) viewUntracked(b *strings.Builder) {
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %sCopy Untracked Files?%s\n", cCyanBold, cReset))
	b.WriteString(fmt.Sprintf("  %s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n\n", cDim, cReset))

	count := len(m.untrackedFiles)
	b.WriteString(fmt.Sprintf("  Found %d untracked file(s) in the repository:\n\n", count))

	// Show up to 5 files with bullet points
	shown := m.untrackedFiles
	if len(shown) > 5 {
		shown = shown[:5]
	}
	for _, f := range shown {
		b.WriteString(fmt.Sprintf("    • %s\n", f))
	}
	if count > 5 {
		b.WriteString(fmt.Sprintf("    %s... and %d more%s\n", cDim, count-5, cReset))
	}

	b.WriteString("\n  Copy these files to the new worktree?\n\n")

	// Selection items with arrow cursor
	options := []string{"Yes, copy untracked files", "No, start with clean worktree"}
	for i, opt := range options {
		cursor := "  "
		if i == m.untrackedIdx {
			cursor = fmt.Sprintf("%s→%s ", cCyan, cReset)
		}
		b.WriteString(fmt.Sprintf("  %s%s\n", cursor, opt))
	}

	b.WriteString(fmt.Sprintf("\n  %s[↑/↓] navigate  [Enter] select  [Esc] Cancel%s", cDim, cReset))
}

// wrapLine wraps a single line into multiple lines at the given width.
func wrapLine(line string, width int) []string {
	runes := []rune(line)
	if len(runes) <= width {
		return []string{line}
	}
	var wrapped []string
	for len(runes) > width {
		wrapped = append(wrapped, string(runes[:width]))
		runes = runes[width:]
	}
	wrapped = append(wrapped, string(runes))
	return wrapped
}

// renderInputBox draws a bordered text input box with cursor and word wrapping.
func renderInputBox(text string) string {
	contentWidth := inputBoxInnerWidth - 2 // minus leading+trailing space

	// Split into lines (Shift+Enter creates explicit newlines), add cursor to last line
	rawLines := strings.Split(text+"█", "\n")

	// Wrap each line at contentWidth
	var lines []string
	for _, raw := range rawLines {
		lines = append(lines, wrapLine(raw, contentWidth)...)
	}

	// Ensure minimum visible rows
	for len(lines) < inputBoxRows {
		lines = append(lines, "")
	}

	// If more lines than visible, show last N (scroll)
	if len(lines) > inputBoxRows {
		lines = lines[len(lines)-inputBoxRows:]
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("  ╭%s╮\n", strings.Repeat("─", inputBoxInnerWidth)))

	for _, line := range lines {
		runes := []rune(line)
		pad := contentWidth - len(runes)
		if pad < 0 {
			pad = 0
		}
		b.WriteString(fmt.Sprintf("  │ %s%s │\n", string(runes), strings.Repeat(" ", pad)))
	}

	b.WriteString(fmt.Sprintf("  ╰%s╯\n", strings.Repeat("─", inputBoxInnerWidth)))
	return b.String()
}

// defaultCheckUntracked checks for untracked files in the current working directory.
func defaultCheckUntracked() ([]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	gitOps := git.New(cwd)
	return gitOps.GetUntrackedFiles(ctx)
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
