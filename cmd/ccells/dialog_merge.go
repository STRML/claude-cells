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
	mergeStepSelect  mergeStep = iota // Select workstream (skipped if only 1)
	mergeStepLoading                  // Loading branch info (spinner)
	mergeStepAction                   // Show branch info + action list
	mergeStepWorking                  // Async operation in progress (spinner)
	mergeStepResult                   // Show result
)

// mergeAction defines an action in the merge dialog's action list.
type mergeAction struct {
	label string // Display label
	id    string // Internal identifier
}

// mergeWorkstream holds the display data for a workstream in the merge dialog.
type mergeWorkstream struct {
	BranchName string
	Prompt     string
	PRNumber   int
	PRURL      string
	HasPR      bool
}

// branchDetail holds branch info loaded async for display.
type branchDetail struct {
	Info     string // Formatted commit list + diff stats from GetBranchInfo
	BaseName string // "main" or "master"
}

// branchDetailMsg is the result of async branch info loading.
type branchDetailMsg struct {
	detail *branchDetail
	err    error
}

// mergeResultMsg is the result of an async PR operation.
type mergeResultMsg struct {
	message string
	err     error
}

// mergeTickMsg drives the spinner animation for the merge dialog.
type mergeTickMsg time.Time

var mergeActions = []mergeAction{
	{"Merge into main (squash)", "squash"},
	{"Merge into main (merge commit)", "merge"},
	{"Create Pull Request", "create-pr"},
	{"Push branch only", "push"},
	{"Rebase on main (fetch first)", "rebase"},
	{"Cancel", "cancel"},
}

// mergeDialog is a Bubble Tea model for the interactive merge/PR dialog.
// Invoked via: ccells merge --interactive
//
// Matches the old TUI layout:
//
//	Merge / PR Options
//	Branch: <name>
//	Commits (N): <recent commits with hashes>
//	<diff stat>
//	→ Merge into main (squash)
//	  Merge into main (merge commit)
//	  Create Pull Request
//	  Push branch only
//	  Rebase on main (fetch first)
//	  Cancel
type mergeDialog struct {
	step     mergeStep
	items    []mergeWorkstream
	selected int // workstream selection index
	err      error
	done     bool
	result   string // success message
	frame    int    // spinner frame

	// Branch detail (loaded async after workstream selection)
	detail *branchDetail

	// Action selection
	actionIdx int

	// Working state
	workingMsg string

	// Injectable dependencies for testing
	repoPath      string
	loadDetailFn  func(ctx context.Context, branch string) (*branchDetail, error)
	createPRFn    func(ctx context.Context, branch, prompt string) (string, error)
	mergePRFn     func(ctx context.Context, method string) error
	pushFn        func(ctx context.Context, branch string) error
	fetchRebaseFn func(ctx context.Context) error
}

func newMergeDialog(workstreams []mergeWorkstream, repoPath string) mergeDialog {
	m := mergeDialog{
		items:    workstreams,
		repoPath: repoPath,
		loadDetailFn: func(ctx context.Context, branch string) (*branchDetail, error) {
			return defaultLoadBranchDetail(ctx, repoPath, branch)
		},
		createPRFn: func(ctx context.Context, branch, prompt string) (string, error) {
			return defaultCreatePR(ctx, repoPath, branch, prompt)
		},
		mergePRFn: func(ctx context.Context, method string) error {
			return defaultMergePR(ctx, repoPath, method)
		},
		pushFn: func(ctx context.Context, branch string) error {
			return defaultPush(ctx, repoPath, branch)
		},
		fetchRebaseFn: func(ctx context.Context) error {
			return defaultFetchRebase(ctx, repoPath)
		},
	}
	return m
}

func mergeTickCmd() tea.Cmd {
	return tea.Tick(spinnerInterval, func(t time.Time) tea.Msg {
		return mergeTickMsg(t)
	})
}

func (m mergeDialog) Init() tea.Cmd {
	// Auto-select if only one workstream
	if len(m.items) == 1 {
		m.selected = 0
		return m.loadDetail()
	}
	return nil
}

func (m mergeDialog) loadDetail() tea.Cmd {
	branch := m.items[m.selected].BranchName
	loadFn := m.loadDetailFn
	return func() tea.Msg {
		ctx := context.Background()
		detail, err := loadFn(ctx, branch)
		return branchDetailMsg{detail: detail, err: err}
	}
}

func (m mergeDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case mergeTickMsg:
		if m.step == mergeStepLoading || m.step == mergeStepWorking {
			m.frame++
			return m, mergeTickCmd()
		}
		return m, nil

	case branchDetailMsg:
		if msg.err != nil {
			// Show action list anyway, just without detail
			m.detail = &branchDetail{Info: "(failed to load branch info)"}
		} else {
			m.detail = msg.detail
		}
		m.step = mergeStepAction
		m.actionIdx = 0
		return m, nil

	case mergeResultMsg:
		if msg.err != nil {
			m.err = msg.err
			m.step = mergeStepAction
			return m, nil
		}
		m.result = msg.message
		m.step = mergeStepResult
		return m, nil

	case tea.KeyMsg:
		// Global quit keys (except during working/loading)
		if m.step != mergeStepWorking && m.step != mergeStepLoading {
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
		// If we auto-selected (only 1 item), back = quit
		if len(m.items) <= 1 {
			m.done = true
			return m, tea.Quit
		}
		m.step = mergeStepSelect
		m.detail = nil
		m.err = nil
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
		m.step = mergeStepLoading
		m.frame = 0
		m.err = nil
		return m, tea.Batch(mergeTickCmd(), m.loadDetail())
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
		if m.actionIdx < len(mergeActions)-1 {
			m.actionIdx++
		}
	case "enter":
		action := mergeActions[m.actionIdx]
		ws := m.items[m.selected]

		switch action.id {
		case "squash":
			return m.startMerge(ws, "squash")
		case "merge":
			return m.startMerge(ws, "merge")
		case "create-pr":
			return m.startCreatePR(ws)
		case "push":
			return m.startPush(ws)
		case "rebase":
			return m.startRebase()
		case "cancel":
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m mergeDialog) startMerge(ws mergeWorkstream, method string) (tea.Model, tea.Cmd) {
	m.step = mergeStepWorking
	m.workingMsg = fmt.Sprintf("Merging via %s", method)
	m.frame = 0
	branch := ws.BranchName
	prompt := ws.Prompt
	pushFn := m.pushFn
	createPRFn := m.createPRFn
	mergePRFn := m.mergePRFn
	hasPR := ws.HasPR
	cmd := func() tea.Msg {
		ctx := context.Background()
		// If no PR exists, push + create PR first
		if !hasPR {
			if err := pushFn(ctx, branch); err != nil {
				return mergeResultMsg{err: fmt.Errorf("push failed: %w", err)}
			}
			_, err := createPRFn(ctx, branch, prompt)
			if err != nil {
				return mergeResultMsg{err: fmt.Errorf("create PR failed: %w", err)}
			}
		}
		if err := mergePRFn(ctx, method); err != nil {
			return mergeResultMsg{err: err}
		}
		return mergeResultMsg{message: fmt.Sprintf("PR merged via %s", method)}
	}
	return m, tea.Batch(mergeTickCmd(), cmd)
}

func (m mergeDialog) startCreatePR(ws mergeWorkstream) (tea.Model, tea.Cmd) {
	m.step = mergeStepWorking
	m.workingMsg = "Pushing & creating PR"
	m.frame = 0
	branch := ws.BranchName
	prompt := ws.Prompt
	pushFn := m.pushFn
	createPRFn := m.createPRFn
	cmd := func() tea.Msg {
		ctx := context.Background()
		if err := pushFn(ctx, branch); err != nil {
			return mergeResultMsg{err: fmt.Errorf("push failed: %w", err)}
		}
		url, err := createPRFn(ctx, branch, prompt)
		if err != nil {
			return mergeResultMsg{err: err}
		}
		return mergeResultMsg{message: fmt.Sprintf("PR created: %s", url)}
	}
	return m, tea.Batch(mergeTickCmd(), cmd)
}

func (m mergeDialog) startPush(ws mergeWorkstream) (tea.Model, tea.Cmd) {
	m.step = mergeStepWorking
	m.workingMsg = "Pushing branch"
	m.frame = 0
	branch := ws.BranchName
	pushFn := m.pushFn
	cmd := func() tea.Msg {
		ctx := context.Background()
		if err := pushFn(ctx, branch); err != nil {
			return mergeResultMsg{err: fmt.Errorf("push failed: %w", err)}
		}
		return mergeResultMsg{message: "Branch pushed to origin"}
	}
	return m, tea.Batch(mergeTickCmd(), cmd)
}

func (m mergeDialog) startRebase() (tea.Model, tea.Cmd) {
	m.step = mergeStepWorking
	m.workingMsg = "Fetching & rebasing"
	m.frame = 0
	fetchRebaseFn := m.fetchRebaseFn
	cmd := func() tea.Msg {
		ctx := context.Background()
		if err := fetchRebaseFn(ctx); err != nil {
			return mergeResultMsg{err: err}
		}
		return mergeResultMsg{message: "Rebased on latest main"}
	}
	return m, tea.Batch(mergeTickCmd(), cmd)
}

func (m mergeDialog) View() tea.View {
	if m.done {
		return tea.NewView("")
	}

	var b strings.Builder

	if len(m.items) == 0 {
		b.WriteString(fmt.Sprintf("\n  %sMerge / PR Options%s\n", cCyanBold, cReset))
		b.WriteString(fmt.Sprintf("  %s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n\n", cDim, cReset))
		b.WriteString("  No workstreams available.\n")
		b.WriteString(fmt.Sprintf("\n  %s(Esc to close)%s", cDim, cReset))
		return tea.NewView(b.String())
	}

	switch m.step {
	case mergeStepSelect:
		m.viewSelect(&b)
	case mergeStepLoading:
		m.viewLoading(&b)
	case mergeStepAction:
		m.viewAction(&b)
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
	b.WriteString(fmt.Sprintf("\n  %sMerge / PR Options%s\n", cCyanBold, cReset))
	b.WriteString(fmt.Sprintf("  %s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n\n", cDim, cReset))
	b.WriteString(fmt.Sprintf("  %sSelect workstream:%s\n\n", cDim, cReset))
	for i, ws := range m.items {
		cursor := "  "
		if i == m.selected {
			cursor = fmt.Sprintf("%s→%s ", cCyan, cReset)
		}

		// Status indicator
		var status string
		if ws.HasPR {
			status = fmt.Sprintf(" %sPR#%d%s", cGreen, ws.PRNumber, cReset)
		}

		b.WriteString(fmt.Sprintf("  %s%s%s\n", cursor, ws.BranchName, status))
	}
	b.WriteString(fmt.Sprintf("\n  %s[↑/↓] navigate  [Enter] select  [Esc] Cancel%s", cDim, cReset))
}

func (m mergeDialog) viewLoading(b *strings.Builder) {
	b.WriteString(fmt.Sprintf("\n  %sMerge / PR Options%s\n", cCyanBold, cReset))
	b.WriteString(fmt.Sprintf("  %s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n\n", cDim, cReset))
	frame := spinnerFrames[m.frame%len(spinnerFrames)]
	b.WriteString(fmt.Sprintf("  %s%s%s Loading branch info...\n", cCyan, frame, cReset))
}

func (m mergeDialog) viewAction(b *strings.Builder) {
	ws := m.items[m.selected]
	baseName := "main"
	if m.detail != nil && m.detail.BaseName != "" {
		baseName = m.detail.BaseName
	}

	b.WriteString(fmt.Sprintf("\n  %sMerge / PR Options%s\n", cCyanBold, cReset))
	b.WriteString(fmt.Sprintf("  %s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n\n", cDim, cReset))

	// Branch name
	b.WriteString(fmt.Sprintf("  %sBranch:%s %s%s%s\n\n", cDim, cReset, cMagenta, ws.BranchName, cReset))

	// Branch info (commits + diff stat)
	if m.detail != nil && m.detail.Info != "" {
		for _, line := range strings.Split(m.detail.Info, "\n") {
			b.WriteString(fmt.Sprintf("  %s%s%s\n", cDim, line, cReset))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	// Action list with dynamic base branch name
	for i, action := range mergeActions {
		cursor := "  "
		if i == m.actionIdx {
			cursor = fmt.Sprintf("%s→%s ", cCyan, cReset)
		}
		label := strings.ReplaceAll(action.label, "main", baseName)
		b.WriteString(fmt.Sprintf("  %s%s\n", cursor, label))
	}

	b.WriteString(fmt.Sprintf("\n  %s[↑/↓] navigate  [Enter] select  [Esc] Cancel%s", cDim, cReset))
}

func (m mergeDialog) viewWorking(b *strings.Builder) {
	b.WriteString(fmt.Sprintf("\n  %sMerge / PR Options%s\n", cCyanBold, cReset))
	b.WriteString(fmt.Sprintf("  %s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n\n", cDim, cReset))
	frame := spinnerFrames[m.frame%len(spinnerFrames)]
	b.WriteString(fmt.Sprintf("  %s%s%s %s...\n", cCyan, frame, cReset, m.workingMsg))
}

func (m mergeDialog) viewResult(b *strings.Builder) {
	b.WriteString(fmt.Sprintf("\n  %sMerge / PR Options%s\n", cCyanBold, cReset))
	b.WriteString(fmt.Sprintf("  %s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n\n", cDim, cReset))
	b.WriteString(fmt.Sprintf("  %s✓%s %s\n", cGreenBold, cReset, m.result))
	b.WriteString(fmt.Sprintf("\n  %sPress any key to close%s", cDim, cReset))
}

// defaultLoadBranchDetail loads commit info and diff stats for a branch.
func defaultLoadBranchDetail(ctx context.Context, repoPath, branch string) (*branchDetail, error) {
	gitClient := git.New(repoPath)
	info, err := gitClient.GetBranchInfo(ctx, branch)
	if err != nil {
		return nil, err
	}
	baseName, _ := gitClient.GetBaseBranch(ctx)
	if baseName == "" {
		baseName = "main"
	}
	return &branchDetail{
		Info:     info,
		BaseName: baseName,
	}, nil
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

// defaultFetchRebase fetches main and rebases current branch.
func defaultFetchRebase(ctx context.Context, repoPath string) error {
	gitClient := git.New(repoPath)
	return gitClient.FetchAndRebase(ctx)
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
