package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/STRML/claude-cells/internal/git"
	"github.com/STRML/claude-cells/internal/workstream"
)

// mergeStep tracks the dialog's current phase.
type mergeStep int

const (
	mergeStepSelect  mergeStep = iota // Select workstream
	mergeStepAction                   // Choose action (create PR / merge / view status)
	mergeStepMethod                   // Choose merge method (squash/merge/rebase)
	mergeStepWorking                  // Async operation in progress
	mergeStepResult                   // Show result
)

// mergeWorkstream holds the display data for a workstream in the merge dialog.
type mergeWorkstream struct {
	BranchName string
	Prompt     string
	PRNumber   int
	PRURL      string
	HasPR      bool
}

// mergeResultMsg is the result of an async PR operation.
type mergeResultMsg struct {
	message string
	err     error
}

// mergeTickMsg drives the spinner animation for the merge dialog.
type mergeTickMsg time.Time

// mergeDialog is a Bubble Tea model for the interactive merge/PR dialog.
// Invoked via: ccells merge --interactive
//
// Flow:
//   - Select workstream → Choose action → Execute
//   - No PR: Push + Create PR (with Claude-generated content)
//   - Has PR: Merge (squash/merge/rebase) or view status
type mergeDialog struct {
	step     mergeStep
	items    []mergeWorkstream
	selected int
	err      error
	done     bool
	result   string // success message
	frame    int    // spinner frame

	// Action selection (step 1)
	actionIdx int
	actions   []string

	// Merge method (step 2)
	methodIdx int
	methods   []string

	// Working state
	workingMsg string // what we're doing

	// Injectable dependencies for testing
	repoPath   string
	createPRFn func(ctx context.Context, branch, prompt string) (string, error)
	mergePRFn  func(ctx context.Context, method string) error
	pushFn     func(ctx context.Context, branch string) error
}

func newMergeDialog(workstreams []mergeWorkstream, repoPath string) mergeDialog {
	return mergeDialog{
		items:    workstreams,
		repoPath: repoPath,
		methods:  []string{"Squash merge", "Merge commit", "Rebase"},
		createPRFn: func(ctx context.Context, branch, prompt string) (string, error) {
			return defaultCreatePR(ctx, repoPath, branch, prompt)
		},
		mergePRFn: func(ctx context.Context, method string) error {
			return defaultMergePR(ctx, repoPath, method)
		},
		pushFn: func(ctx context.Context, branch string) error {
			return defaultPush(ctx, repoPath, branch)
		},
	}
}

func mergeTickCmd() tea.Cmd {
	return tea.Tick(spinnerInterval, func(t time.Time) tea.Msg {
		return mergeTickMsg(t)
	})
}

func (m mergeDialog) Init() tea.Cmd {
	return nil
}

func (m mergeDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case mergeTickMsg:
		if m.step == mergeStepWorking {
			m.frame++
			return m, mergeTickCmd()
		}
		return m, nil

	case mergeResultMsg:
		if msg.err != nil {
			m.err = msg.err
			m.step = mergeStepSelect
			return m, nil
		}
		m.result = msg.message
		m.step = mergeStepResult
		return m, nil

	case tea.KeyMsg:
		// Global quit keys (except during working)
		if m.step != mergeStepWorking {
			switch msg.String() {
			case "ctrl+c":
				m.done = true
				return m, tea.Quit
			case "esc":
				return m.handleBack()
			case "q":
				if m.step == mergeStepSelect {
					m.done = true
					return m, tea.Quit
				}
				return m.handleBack()
			}
		}

		switch m.step {
		case mergeStepSelect:
			return m.updateSelect(msg)
		case mergeStepAction:
			return m.updateAction(msg)
		case mergeStepMethod:
			return m.updateMethod(msg)
		case mergeStepResult:
			// Any key dismisses
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m mergeDialog) handleBack() (tea.Model, tea.Cmd) {
	switch m.step {
	case mergeStepAction:
		m.step = mergeStepSelect
		m.err = nil
	case mergeStepMethod:
		m.step = mergeStepAction
	default:
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m mergeDialog) updateSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		if m.selected < len(m.items)-1 {
			m.selected++
		}
	case "enter":
		if len(m.items) == 0 {
			return m, nil
		}
		ws := m.items[m.selected]
		if ws.HasPR {
			m.actions = []string{"Merge PR", "View in browser"}
			m.actionIdx = 0
		} else {
			m.actions = []string{"Create PR"}
			m.actionIdx = 0
		}
		m.step = mergeStepAction
		m.err = nil
	}
	return m, nil
}

func (m mergeDialog) updateAction(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.actionIdx > 0 {
			m.actionIdx--
		}
	case "down", "j":
		if m.actionIdx < len(m.actions)-1 {
			m.actionIdx++
		}
	case "enter":
		ws := m.items[m.selected]
		action := m.actions[m.actionIdx]

		switch action {
		case "Create PR":
			m.step = mergeStepWorking
			m.workingMsg = "Pushing & creating PR"
			m.frame = 0
			branch := ws.BranchName
			prompt := ws.Prompt
			cmd := func() tea.Msg {
				ctx := context.Background()
				// Push first
				if err := m.pushFn(ctx, branch); err != nil {
					return mergeResultMsg{err: fmt.Errorf("push failed: %w", err)}
				}
				url, err := m.createPRFn(ctx, branch, prompt)
				if err != nil {
					return mergeResultMsg{err: err}
				}
				return mergeResultMsg{message: fmt.Sprintf("PR created: %s", url)}
			}
			return m, tea.Batch(mergeTickCmd(), cmd)

		case "Merge PR":
			m.step = mergeStepMethod
			m.methodIdx = 0

		case "View in browser":
			m.done = true
			m.result = ws.PRURL
			return m, tea.Sequence(
				tea.Printf("\033[36m%s\033[0m", ws.PRURL),
				tea.Quit,
			)
		}
	}
	return m, nil
}

func (m mergeDialog) updateMethod(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.methodIdx > 0 {
			m.methodIdx--
		}
	case "down", "j":
		if m.methodIdx < len(m.methods)-1 {
			m.methodIdx++
		}
	case "enter":
		methodMap := map[int]string{0: "squash", 1: "merge", 2: "rebase"}
		method := methodMap[m.methodIdx]
		m.step = mergeStepWorking
		m.workingMsg = fmt.Sprintf("Merging via %s", method)
		m.frame = 0
		cmd := func() tea.Msg {
			ctx := context.Background()
			if err := m.mergePRFn(ctx, method); err != nil {
				return mergeResultMsg{err: err}
			}
			return mergeResultMsg{message: fmt.Sprintf("PR merged via %s", method)}
		}
		return m, tea.Batch(mergeTickCmd(), cmd)
	}
	return m, nil
}

func (m mergeDialog) View() tea.View {
	if m.done {
		return tea.NewView("")
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("  %sPull Request%s\n", cCyanBold, cReset))
	b.WriteString(fmt.Sprintf("  %s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n\n", cDim, cReset))

	if len(m.items) == 0 {
		b.WriteString("  No workstreams available.\n")
		b.WriteString(fmt.Sprintf("\n  %s(Esc to close)%s", cDim, cReset))
		return tea.NewView(b.String())
	}

	switch m.step {
	case mergeStepSelect:
		m.viewSelect(&b)
	case mergeStepAction:
		m.viewAction(&b)
	case mergeStepMethod:
		m.viewMethod(&b)
	case mergeStepWorking:
		m.viewWorking(&b)
	case mergeStepResult:
		m.viewResult(&b)
	}

	if m.err != nil {
		b.WriteString(fmt.Sprintf("\n  %s✗ %v%s\n", cRed, m.err, cReset))
	}

	return tea.NewView(b.String())
}

func (m mergeDialog) viewSelect(b *strings.Builder) {
	b.WriteString(fmt.Sprintf("  %sSelect workstream:%s\n\n", cDim, cReset))
	for i, ws := range m.items {
		cursor := "  "
		if i == m.selected {
			cursor = fmt.Sprintf("%s> %s", cCyan, cReset)
		}

		// Status indicator
		var status string
		if ws.HasPR {
			status = fmt.Sprintf(" %sPR#%d%s", cGreen, ws.PRNumber, cReset)
		} else {
			status = fmt.Sprintf(" %sno PR%s", cGray, cReset)
		}

		b.WriteString(fmt.Sprintf("  %s%s%s%s\n", cursor, cWhite, ws.BranchName, cReset))
		if i == m.selected {
			b.WriteString(fmt.Sprintf("    %s", status))
			b.WriteString("\n")
		}
	}
	b.WriteString(fmt.Sprintf("\n  %s↑↓ navigate  Enter select  Esc cancel%s", cDim, cReset))
}

func (m mergeDialog) viewAction(b *strings.Builder) {
	ws := m.items[m.selected]
	b.WriteString(fmt.Sprintf("  %s%s%s%s\n\n", cMagenta, ws.BranchName, cReset, ""))
	if ws.HasPR {
		b.WriteString(fmt.Sprintf("  %sPR #%d%s\n\n", cGreen, ws.PRNumber, cReset))
	}
	for i, action := range m.actions {
		cursor := "  "
		if i == m.actionIdx {
			cursor = fmt.Sprintf("%s> %s", cCyan, cReset)
		}
		b.WriteString(fmt.Sprintf("  %s%s\n", cursor, action))
	}
	b.WriteString(fmt.Sprintf("\n  %s↑↓ navigate  Enter select  Esc back%s", cDim, cReset))
}

func (m mergeDialog) viewMethod(b *strings.Builder) {
	ws := m.items[m.selected]
	b.WriteString(fmt.Sprintf("  Merge %sPR #%d%s for %s%s%s\n\n", cGreen, ws.PRNumber, cReset, cMagenta, ws.BranchName, cReset))

	for i, method := range m.methods {
		cursor := "  "
		if i == m.methodIdx {
			cursor = fmt.Sprintf("%s> %s", cCyan, cReset)
		}
		b.WriteString(fmt.Sprintf("  %s%s\n", cursor, method))
	}
	b.WriteString(fmt.Sprintf("\n  %s↑↓ navigate  Enter merge  Esc back%s", cDim, cReset))
}

func (m mergeDialog) viewWorking(b *strings.Builder) {
	frame := spinnerFrames[m.frame%len(spinnerFrames)]
	b.WriteString(fmt.Sprintf("  %s%s%s %s...\n", cCyan, frame, cReset, m.workingMsg))
}

func (m mergeDialog) viewResult(b *strings.Builder) {
	b.WriteString(fmt.Sprintf("  %s✓%s %s\n", cGreenBold, cReset, m.result))
	b.WriteString(fmt.Sprintf("\n  %sPress any key to close%s", cDim, cReset))
}

// defaultCreatePR pushes the branch and creates a PR using Claude-generated content.
func defaultCreatePR(ctx context.Context, repoPath, branch, prompt string) (string, error) {
	gh := git.NewGH()

	// Check if PR already exists
	exists, resp, err := gh.PRExists(ctx, repoPath)
	if err == nil && exists && resp != nil {
		return resp.URL, nil
	}

	// Generate PR content via Claude
	gitClient := git.New(repoPath)
	title, body := git.GeneratePRContent(ctx, gitClient, branch, prompt)

	// Create PR
	pr, err := gh.CreatePR(ctx, repoPath, &git.PRRequest{
		Title: title,
		Body:  body,
		Head:  branch,
	})
	if err != nil {
		return "", fmt.Errorf("create PR failed: %w", err)
	}

	return pr.URL, nil
}

// defaultMergePR merges the current branch's PR.
func defaultMergePR(ctx context.Context, repoPath, method string) error {
	gh := git.NewGH()
	return gh.MergePR(ctx, repoPath, &git.PRMergeOptions{
		Method:       method,
		DeleteBranch: true,
	})
}

// defaultPush pushes a branch to origin.
func defaultPush(ctx context.Context, repoPath, branch string) error {
	gitClient := git.New(repoPath)
	return gitClient.Push(ctx, branch)
}

// loadMergeWorkstreams loads workstream data from both tmux (live panes) and state file.
func loadMergeWorkstreams(ctx context.Context, repoID, stateDir string) ([]mergeWorkstream, error) {
	// Get live workstream names from tmux panes
	names, err := listWorkstreamNames(ctx, repoID)
	if err != nil {
		return nil, err
	}

	if len(names) == 0 {
		return nil, nil
	}

	// Load state for PR info
	var stateMap map[string]*workstream.SavedWorkstream
	if state, err := workstream.LoadState(stateDir); err == nil {
		stateMap = make(map[string]*workstream.SavedWorkstream)
		for i := range state.Workstreams {
			ws := &state.Workstreams[i]
			stateMap[ws.BranchName] = ws
		}
	}

	var items []mergeWorkstream
	for _, name := range names {
		mw := mergeWorkstream{BranchName: name}
		if sw, ok := stateMap[name]; ok {
			mw.Prompt = sw.Prompt
			mw.PRNumber = sw.PRNumber
			mw.PRURL = sw.PRURL
			mw.HasPR = sw.PRNumber > 0
		}
		items = append(items, mw)
	}
	return items, nil
}
