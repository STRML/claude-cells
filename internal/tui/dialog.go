package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// DialogType represents the type of dialog
type DialogType int

const (
	DialogNone DialogType = iota
	DialogDestroy
	DialogNewWorkstream
	DialogPRPreview
	DialogSettings
	DialogMerge
	DialogLog
	DialogProgress
	DialogBranchConflict
	DialogPruneAllConfirm
	DialogPruneProjectConfirm
	DialogCommitBeforeMerge
	DialogResourceUsage
	DialogPostMergeDestroy     // Confirmation to destroy container after successful merge
	DialogMergeConflict        // Merge failed due to conflicts - offer rebase
	DialogFirstRunIntroduction // First-run introduction modal
	DialogQuitConfirm          // Confirm quit with y/n
)

// DialogModel represents a modal dialog
type DialogModel struct {
	Type         DialogType
	Title        string
	Body         string
	Input        textinput.Model
	TextArea     textarea.Model // For multi-line input (new workstream)
	useTextArea  bool           // Whether to use textarea instead of textinput
	ConfirmWord  string         // Word required to confirm (e.g., "destroy")
	WorkstreamID string
	BranchInfo   string // Branch statistics to pass through dialogs
	width        int
	height       int
	// Menu-style dialogs
	MenuItems     []string
	MenuSelection int
	// Log dialog scrolling
	scrollOffset int
	scrollMax    int
	// Progress dialog
	inProgress bool
	// Resource usage dialog
	isGlobalView bool   // True for global (all ccells), false for project only
	statsLoading bool   // True while fetching stats
	statsError   string // Error message if stats fetch failed
}

// NewDestroyDialog creates a destroy confirmation dialog
func NewDestroyDialog(branchName, workstreamID string) DialogModel {
	ti := textinput.New()
	ti.Placeholder = "type 'destroy' to confirm"
	ti.SetWidth(40)
	ti.Focus()
	ti.CharLimit = 20

	// Style the textinput
	ti.Prompt = "› "
	ti.SetStyles(textinput.Styles{
		Focused: textinput.StyleState{
			Prompt:      DialogInputPrompt,
			Text:        DialogInputText,
			Placeholder: DialogInputPlaceholder,
		},
		Blurred: textinput.StyleState{
			Prompt:      DialogInputPrompt,
			Text:        DialogInputText,
			Placeholder: DialogInputPlaceholder,
		},
	})

	body := `This will:
  • Stop and remove the container
  • End any active Mutagen sync
  • Delete local branch (remote untouched)

Type "destroy" to confirm:`

	return DialogModel{
		Type:         DialogDestroy,
		Title:        fmt.Sprintf("Destroy workstream %q?", branchName),
		Body:         body,
		Input:        ti,
		ConfirmWord:  "destroy",
		WorkstreamID: workstreamID,
	}
}

// NewWorkstreamDialog creates a new workstream prompt dialog
func NewWorkstreamDialog() DialogModel {
	ta := textarea.New()
	ta.Placeholder = "describe the task..."
	ta.CharLimit = 500
	// Width will be set properly when SetSize is called
	// Start with a reasonable default
	ta.SetWidth(60)
	ta.SetHeight(4)
	ta.Focus()
	ta.ShowLineNumbers = false

	// Style the textarea - remove default border, minimal chrome
	ta.Prompt = "" // No per-line prompt
	styleState := textarea.StyleState{
		Base:        lipgloss.NewStyle(), // No border
		Text:        DialogInputText,
		Placeholder: DialogInputPlaceholder,
		CursorLine:  DialogInputText,
		EndOfBuffer: lipgloss.NewStyle().Foreground(lipgloss.Color("#333333")),
	}
	ta.SetStyles(textarea.Styles{
		Focused: styleState,
		Blurred: styleState,
	})

	return DialogModel{
		Type:        DialogNewWorkstream,
		Title:       "New Workstream",
		Body:        "Enter a prompt for Claude:",
		TextArea:    ta,
		useTextArea: true,
	}
}

// NewPRDialog creates a PR preview/edit dialog
func NewPRDialog(branchName, title, body string) DialogModel {
	ti := textinput.New()
	ti.Placeholder = "PR title..."
	ti.SetWidth(50)
	ti.SetValue(title)
	ti.Focus()
	ti.CharLimit = 100

	// Style the textinput
	ti.Prompt = "› "
	ti.SetStyles(textinput.Styles{
		Focused: textinput.StyleState{
			Prompt:      DialogInputPrompt,
			Text:        DialogInputText,
			Placeholder: DialogInputPlaceholder,
		},
		Blurred: textinput.StyleState{
			Prompt:      DialogInputPrompt,
			Text:        DialogInputText,
			Placeholder: DialogInputPlaceholder,
		},
	})

	return DialogModel{
		Type:  DialogPRPreview,
		Title: fmt.Sprintf("Create PR for %q", branchName),
		Body:  body,
		Input: ti,
	}
}

// SettingsAction represents a settings menu action
type SettingsAction string

const (
	SettingsActionPruneStopped SettingsAction = "prune_stopped"
	SettingsActionPruneProject SettingsAction = "prune_project"
	SettingsActionPruneAll     SettingsAction = "prune_all"
	SettingsActionCancel       SettingsAction = "cancel"
)

// MergeAction represents a merge menu action
type MergeAction string

const (
	MergeActionMergeMain  MergeAction = "merge_main"
	MergeActionSquashMain MergeAction = "squash_main"
	MergeActionCreatePR   MergeAction = "create_pr"
	MergeActionPush       MergeAction = "push"
	MergeActionCancel     MergeAction = "cancel"
)

// CommitBeforeMergeAction represents a commit-before-merge dialog action
type CommitBeforeMergeAction string

const (
	CommitBeforeMergeYes    CommitBeforeMergeAction = "yes"
	CommitBeforeMergeNo     CommitBeforeMergeAction = "no"
	CommitBeforeMergeCancel CommitBeforeMergeAction = "cancel"
)

// BranchConflictAction represents a branch conflict resolution action
type BranchConflictAction string

const (
	BranchConflictUseExisting BranchConflictAction = "use_existing"
	BranchConflictCreateNew   BranchConflictAction = "create_new"
	BranchConflictDelete      BranchConflictAction = "delete"
	BranchConflictCancel      BranchConflictAction = "cancel"
)

// NewBranchConflictDialog creates a dialog for handling branch conflicts
func NewBranchConflictDialog(branchName, workstreamID, branchInfo string) DialogModel {
	body := fmt.Sprintf("Branch '%s' already exists.", branchName)
	if branchInfo != "" {
		body += "\n\n" + branchInfo
	}
	body += "\n\nWhat would you like to do?"

	return DialogModel{
		Type:         DialogBranchConflict,
		Title:        "Branch Already Exists",
		Body:         body,
		WorkstreamID: workstreamID,
		MenuItems: []string{
			"Use existing branch",
			"Create new branch with unique name",
			"Delete branch and recreate",
			"Cancel",
		},
		MenuSelection: 0,
	}
}

// NewMergeDialog creates a merge/PR menu dialog
func NewMergeDialog(branchName, workstreamID, branchInfo string) DialogModel {
	var body strings.Builder
	body.WriteString(fmt.Sprintf("Branch: %s\n", branchName))

	if branchInfo != "" {
		body.WriteString("\n")
		body.WriteString(branchInfo)
		body.WriteString("\n")
	}

	return DialogModel{
		Type:         DialogMerge,
		Title:        "Merge / PR Options",
		Body:         body.String(),
		WorkstreamID: workstreamID,
		MenuItems: []string{
			"Merge into main (merge commit)",
			"Merge into main (squash)",
			"Create Pull Request",
			"Push branch only",
			"Cancel",
		},
		MenuSelection: 0,
	}
}

// NewCommitBeforeMergeDialog creates a dialog asking if user wants to commit uncommitted changes
func NewCommitBeforeMergeDialog(branchName, workstreamID, branchInfo string) DialogModel {
	body := fmt.Sprintf("Branch '%s' has uncommitted changes.\n\nWould you like Claude to commit them first?", branchName)

	return DialogModel{
		Type:         DialogCommitBeforeMerge,
		Title:        "Uncommitted Changes",
		Body:         body,
		WorkstreamID: workstreamID,
		BranchInfo:   branchInfo, // Store for passing to merge dialog
		MenuItems: []string{
			"Yes, commit changes",
			"No, continue without committing",
			"Cancel",
		},
		MenuSelection: 0,
	}
}

// NewPostMergeDestroyDialog creates a dialog asking if user wants to destroy the container after merge
func NewPostMergeDestroyDialog(branchName, workstreamID string) DialogModel {
	body := fmt.Sprintf("Branch '%s' has been merged into main.\n\nThe work is complete. Would you like to destroy this container?", branchName)

	return DialogModel{
		Type:         DialogPostMergeDestroy,
		Title:        "Merge Successful",
		Body:         body,
		WorkstreamID: workstreamID,
		MenuItems: []string{
			"Yes, destroy container (recommended)",
			"No, keep container",
		},
		MenuSelection: 0,
	}
}

// NewMergeConflictDialog creates a dialog for handling merge conflicts
func NewMergeConflictDialog(branchName, workstreamID string, conflictFiles []string) DialogModel {
	body := fmt.Sprintf("Merge of '%s' into main has conflicts.\n\nConflicting files:\n", branchName)
	for _, f := range conflictFiles {
		body += fmt.Sprintf("  • %s\n", f)
	}
	body += "\nYou can rebase onto main and resolve conflicts in the container,\nor cancel and keep the current state."

	return DialogModel{
		Type:         DialogMergeConflict,
		Title:        "Merge Conflict",
		Body:         body,
		WorkstreamID: workstreamID,
		MenuItems: []string{
			"Rebase onto main (resolve conflicts in container)",
			"Cancel (keep current state)",
		},
		MenuSelection: 0,
	}
}

// NewFirstRunIntroductionDialog creates the first-run introduction dialog
func NewFirstRunIntroductionDialog() DialogModel {
	body := `Welcome to Claude Cells!

Run parallel Claude Code instances in isolated Docker containers.

NAVIGATION
  NAV mode   Navigate between panes, create/destroy workstreams
  INPUT mode Type directly into the focused pane (Claude)

  Press [i] or click a pane to enter INPUT mode
  Press [Esc] twice quickly to return to NAV mode

KEYBOARD SHORTCUTS (NAV mode)
  [n]        New workstream      [d]        Destroy workstream
  [←/→]      Switch pane         [m]        Merge/PR options
  [l]        View logs           [p]        Toggle pairing mode
  [?]        Help                [q]        Quit (containers pause)

TMUX-STYLE SHORTCUTS (INPUT mode)
  [Ctrl+B] then...
  [Esc]      Return to NAV mode
  [←/→]      Switch pane
  [n]        New workstream
  [d]        Destroy

PAIRING MODE
  Sync your local editor with a container for real-time collaboration.

For more details: https://github.com/STRML/claude-cells`

	return DialogModel{
		Type:  DialogFirstRunIntroduction,
		Title: "Getting Started",
		Body:  body,
	}
}

// NewQuitConfirmDialog creates a quit confirmation dialog
func NewQuitConfirmDialog() DialogModel {
	return DialogModel{
		Type:  DialogQuitConfirm,
		Title: "Quit ccells?",
		Body:  "Active containers will be paused and can resume later.\n\nAre you sure you want to quit?",
		MenuItems: []string{
			"Yes",
			"No",
		},
		MenuSelection: 1, // Default to "No" for safety
	}
}

// NewSettingsDialog creates a settings menu dialog
func NewSettingsDialog(containerCount int, projectName string) DialogModel {
	body := fmt.Sprintf("Containers managed by ccells: %d\n", containerCount)

	return DialogModel{
		Type:  DialogSettings,
		Title: "Settings",
		Body:  body,
		MenuItems: []string{
			"Prune stopped ccells containers",
			fmt.Sprintf("Destroy all containers for %q", projectName),
			"Destroy ALL ccells containers (all projects!)",
			"Cancel",
		},
		MenuSelection: 0,
	}
}

// NewPruneProjectConfirmDialog creates a confirmation dialog for pruning containers for the current project
func NewPruneProjectConfirmDialog(projectName string) DialogModel {
	ti := textinput.New()
	ti.Placeholder = "type 'destroy' to confirm"
	ti.SetWidth(40)
	ti.Focus()
	ti.CharLimit = 20

	// Style the textinput
	ti.Prompt = "› "
	ti.SetStyles(textinput.Styles{
		Focused: textinput.StyleState{
			Prompt:      DialogInputPrompt,
			Text:        DialogInputText,
			Placeholder: DialogInputPlaceholder,
		},
		Blurred: textinput.StyleState{
			Prompt:      DialogInputPrompt,
			Text:        DialogInputText,
			Placeholder: DialogInputPlaceholder,
		},
	})

	body := fmt.Sprintf(`This will:
  • Stop and remove all ccells containers for %q
  • Delete any local branches with no commits
  • Containers for other projects are NOT affected

Type "destroy" to confirm:`, projectName)

	return DialogModel{
		Type:        DialogPruneProjectConfirm,
		Title:       fmt.Sprintf("Destroy %q Workstreams?", projectName),
		Body:        body,
		Input:       ti,
		ConfirmWord: "destroy",
	}
}

// NewPruneAllConfirmDialog creates a confirmation dialog for pruning ALL containers globally
func NewPruneAllConfirmDialog() DialogModel {
	ti := textinput.New()
	ti.Placeholder = "type 'destroy' to confirm"
	ti.SetWidth(40)
	ti.Focus()
	ti.CharLimit = 20

	// Style the textinput
	ti.Prompt = "› "
	ti.SetStyles(textinput.Styles{
		Focused: textinput.StyleState{
			Prompt:      DialogInputPrompt,
			Text:        DialogInputText,
			Placeholder: DialogInputPlaceholder,
		},
		Blurred: textinput.StyleState{
			Prompt:      DialogInputPrompt,
			Text:        DialogInputText,
			Placeholder: DialogInputPlaceholder,
		},
	})

	body := `⚠️  WARNING: This affects ALL projects!

This will:
  • Stop and remove ALL ccells containers
    (from every repository, not just this one)
  • Delete any local branches with no commits

Type "destroy" to confirm:`

	return DialogModel{
		Type:        DialogPruneAllConfirm,
		Title:       "Destroy ALL Workstreams (All Projects)?",
		Body:        body,
		Input:       ti,
		ConfirmWord: "destroy",
	}
}

// NewResourceUsageDialog creates a resource usage dialog
func NewResourceUsageDialog(isGlobal bool) DialogModel {
	title := "Resource Usage (Project)"
	if isGlobal {
		title = "Resource Usage (Global)"
	}

	return DialogModel{
		Type:         DialogResourceUsage,
		Title:        title,
		Body:         "Loading...",
		isGlobalView: isGlobal,
		statsLoading: true,
	}
}

// NewLogDialog creates a log viewer dialog with scrollable content
func NewLogDialog(branchName string, logContent string) DialogModel {
	// Calculate scroll max based on content lines
	lines := strings.Split(logContent, "\n")
	scrollMax := 0
	if len(lines) > 20 { // Assume 20 visible lines
		scrollMax = len(lines) - 20
	}

	return DialogModel{
		Type:      DialogLog,
		Title:     fmt.Sprintf("Logs: %s", branchName),
		Body:      logContent,
		scrollMax: scrollMax,
	}
}

// NewProgressDialog creates a dialog showing an operation in progress
func NewProgressDialog(title, message string, workstreamID string) DialogModel {
	return DialogModel{
		Type:         DialogProgress,
		Title:        title,
		Body:         message,
		WorkstreamID: workstreamID,
		inProgress:   true,
	}
}

// SetComplete marks the progress dialog as complete with a result message
func (d *DialogModel) SetComplete(message string) {
	d.inProgress = false
	d.Body = message
}

// AppendBody appends text to the dialog body
func (d *DialogModel) AppendBody(text string) {
	d.Body += text
}

// SetStatsContent updates the resource dialog with stats data
func (d *DialogModel) SetStatsContent(body string) {
	d.Body = body
	d.statsLoading = false
	d.statsError = ""
}

// SetStatsError updates the resource dialog with an error message
func (d *DialogModel) SetStatsError(err string) {
	d.Body = "Error: " + err
	d.statsLoading = false
	d.statsError = err
}

// IsGlobalView returns whether the dialog shows global (all ccells) or project stats
func (d *DialogModel) IsGlobalView() bool {
	return d.isGlobalView
}

// Init initializes the dialog
func (d DialogModel) Init() tea.Cmd {
	if d.useTextArea {
		return textarea.Blink
	}
	return textinput.Blink
}

// Update handles dialog input
func (d DialogModel) Update(msg tea.Msg) (DialogModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		keyStr := msg.String()
		// Debug: log key events in dialogs to help diagnose shift+enter issues
		if keyStr == "enter" || keyStr == "shift+enter" {
			LogDebug("Dialog received key: %q", keyStr)
		}
		switch keyStr {
		case "esc", "ctrl+c":
			// Progress dialog can't be dismissed while in progress
			if d.Type == DialogProgress && d.inProgress {
				return d, nil
			}
			// Resource dialog can be dismissed even while loading
			return d, func() tea.Msg { return DialogCancelMsg{} }
		case "tab":
			// Tab toggles view in resource usage dialog
			if d.Type == DialogResourceUsage {
				d.isGlobalView = !d.isGlobalView
				d.statsLoading = true
				if d.isGlobalView {
					d.Title = "Resource Usage (Global)"
				} else {
					d.Title = "Resource Usage (Project)"
				}
				d.Body = "Loading..."
				return d, func() tea.Msg { return ResourceStatsToggleMsg{IsGlobal: d.isGlobalView} }
			}
		case "r":
			// 'r' refreshes in resource usage dialog
			if d.Type == DialogResourceUsage && !d.statsLoading {
				d.statsLoading = true
				d.Body = "Loading..."
				return d, func() tea.Msg { return ResourceStatsRefreshMsg{IsGlobal: d.isGlobalView} }
			}
		case "y", "Y":
			// 'y' confirms quit dialog
			if d.Type == DialogQuitConfirm {
				return d, func() tea.Msg {
					return DialogConfirmMsg{Type: d.Type}
				}
			}
		case "n", "N":
			// 'n' cancels quit dialog
			if d.Type == DialogQuitConfirm {
				return d, func() tea.Msg { return DialogCancelMsg{} }
			}
		case "shift+enter", "ctrl+j":
			// Insert newline in textarea dialogs
			// ctrl+j is the legacy escape sequence some terminals send for shift+enter
			if d.useTextArea {
				d.TextArea.InsertRune('\n')
				return d, nil
			}
		case "enter":
			// Progress dialog: dismiss only if complete
			if d.Type == DialogProgress {
				if !d.inProgress {
					return d, func() tea.Msg { return DialogCancelMsg{} }
				}
				return d, nil
			}
			// Log dialog dismisses on enter
			if d.Type == DialogLog {
				return d, func() tea.Msg { return DialogCancelMsg{} }
			}
			// First-run introduction dialog dismisses on enter
			if d.Type == DialogFirstRunIntroduction {
				return d, func() tea.Msg {
					return DialogConfirmMsg{Type: d.Type}
				}
			}
			// Resource usage dialog dismisses on enter
			if d.Type == DialogResourceUsage {
				return d, func() tea.Msg { return DialogCancelMsg{} }
			}
			// Handle menu-style dialogs
			if d.Type == DialogSettings {
				var action SettingsAction
				switch d.MenuSelection {
				case 0:
					action = SettingsActionPruneStopped
				case 1:
					action = SettingsActionPruneProject
				case 2:
					action = SettingsActionPruneAll
				default:
					action = SettingsActionCancel
				}
				if action == SettingsActionCancel {
					return d, func() tea.Msg { return DialogCancelMsg{} }
				}
				return d, func() tea.Msg {
					return SettingsConfirmMsg{Action: action}
				}
			}

			if d.Type == DialogMerge {
				var action MergeAction
				switch d.MenuSelection {
				case 0:
					action = MergeActionMergeMain
				case 1:
					action = MergeActionSquashMain
				case 2:
					action = MergeActionCreatePR
				case 3:
					action = MergeActionPush
				default:
					action = MergeActionCancel
				}
				if action == MergeActionCancel {
					return d, func() tea.Msg { return DialogCancelMsg{} }
				}
				return d, func() tea.Msg {
					return MergeConfirmMsg{Action: action, WorkstreamID: d.WorkstreamID}
				}
			}

			if d.Type == DialogCommitBeforeMerge {
				var action CommitBeforeMergeAction
				switch d.MenuSelection {
				case 0:
					action = CommitBeforeMergeYes
				case 1:
					action = CommitBeforeMergeNo
				default:
					action = CommitBeforeMergeCancel
				}
				if action == CommitBeforeMergeCancel {
					return d, func() tea.Msg { return DialogCancelMsg{} }
				}
				branchInfo := d.BranchInfo
				return d, func() tea.Msg {
					return CommitBeforeMergeConfirmMsg{Action: action, WorkstreamID: d.WorkstreamID, BranchInfo: branchInfo}
				}
			}

			if d.Type == DialogBranchConflict {
				var action BranchConflictAction
				switch d.MenuSelection {
				case 0:
					action = BranchConflictUseExisting
				case 1:
					action = BranchConflictCreateNew
				case 2:
					action = BranchConflictDelete
				default:
					action = BranchConflictCancel
				}
				if action == BranchConflictCancel {
					return d, func() tea.Msg { return DialogCancelMsg{} }
				}
				return d, func() tea.Msg {
					return BranchConflictConfirmMsg{Action: action, WorkstreamID: d.WorkstreamID}
				}
			}

			if d.Type == DialogPostMergeDestroy {
				// Selection 0 = "Yes, destroy container", 1 = "No, keep container"
				selection := d.MenuSelection
				if selection == 1 {
					return d, func() tea.Msg { return DialogCancelMsg{} }
				}
				return d, func() tea.Msg {
					return DialogConfirmMsg{
						Type:         d.Type,
						WorkstreamID: d.WorkstreamID,
						Value:        fmt.Sprintf("%d", selection),
					}
				}
			}

			if d.Type == DialogMergeConflict {
				// Selection 0 = "Rebase onto main", 1 = "Cancel"
				selection := d.MenuSelection
				if selection == 1 {
					return d, func() tea.Msg { return DialogCancelMsg{} }
				}
				return d, func() tea.Msg {
					return DialogConfirmMsg{
						Type:         d.Type,
						WorkstreamID: d.WorkstreamID,
						Value:        fmt.Sprintf("%d", selection),
					}
				}
			}

			if d.Type == DialogQuitConfirm {
				// Selection 0 = "Yes", 1 = "No"
				if d.MenuSelection == 1 {
					return d, func() tea.Msg { return DialogCancelMsg{} }
				}
				return d, func() tea.Msg {
					return DialogConfirmMsg{Type: d.Type}
				}
			}

			if d.Type == DialogDestroy || d.Type == DialogPruneAllConfirm || d.Type == DialogPruneProjectConfirm {
				if strings.ToLower(d.Input.Value()) == d.ConfirmWord {
					return d, func() tea.Msg {
						return DialogConfirmMsg{
							Type:         d.Type,
							WorkstreamID: d.WorkstreamID,
							Value:        d.Input.Value(),
						}
					}
				}
				// Enter pressed but confirm word doesn't match - ignore
				return d, nil
			} else if d.useTextArea {
				// For textarea dialogs, get value from textarea
				value := strings.TrimSpace(d.TextArea.Value())
				if value != "" {
					return d, func() tea.Msg {
						return DialogConfirmMsg{
							Type:         d.Type,
							WorkstreamID: d.WorkstreamID,
							Value:        value,
						}
					}
				}
				// Enter pressed but input is empty - ignore
				return d, nil
			} else {
				if d.Input.Value() != "" {
					return d, func() tea.Msg {
						return DialogConfirmMsg{
							Type:         d.Type,
							WorkstreamID: d.WorkstreamID,
							Value:        d.Input.Value(),
						}
					}
				}
				// Enter pressed but input is empty - ignore
				return d, nil
			}
		case "up", "k":
			// Handle scrollable dialog scrolling
			if d.Type == DialogLog || d.Type == DialogFirstRunIntroduction {
				if d.scrollOffset > 0 {
					d.scrollOffset--
				}
				return d, nil
			}
			// Only handle for menu dialogs, otherwise pass to input
			if d.Type == DialogSettings || d.Type == DialogMerge || d.Type == DialogBranchConflict || d.Type == DialogCommitBeforeMerge || d.Type == DialogPostMergeDestroy || d.Type == DialogMergeConflict || d.Type == DialogQuitConfirm {
				if d.MenuSelection > 0 {
					d.MenuSelection--
				}
				return d, nil
			}
		case "down", "j":
			// Handle scrollable dialog scrolling
			if d.Type == DialogLog || d.Type == DialogFirstRunIntroduction {
				if d.scrollOffset < d.scrollMax {
					d.scrollOffset++
				}
				return d, nil
			}
			// Only handle for menu dialogs, otherwise pass to input
			if d.Type == DialogSettings || d.Type == DialogMerge || d.Type == DialogBranchConflict || d.Type == DialogCommitBeforeMerge || d.Type == DialogPostMergeDestroy || d.Type == DialogMergeConflict || d.Type == DialogQuitConfirm {
				if d.MenuSelection < len(d.MenuItems)-1 {
					d.MenuSelection++
				}
				return d, nil
			}
		case "pgup", "ctrl+u":
			// Page up for scrollable dialogs
			if d.Type == DialogLog || d.Type == DialogFirstRunIntroduction {
				visibleLines := d.height - 8
				if visibleLines < 5 {
					visibleLines = 5
				}
				d.scrollOffset -= visibleLines
				if d.scrollOffset < 0 {
					d.scrollOffset = 0
				}
				return d, nil
			}
		case "pgdown", "ctrl+d":
			// Page down for scrollable dialogs
			if d.Type == DialogLog || d.Type == DialogFirstRunIntroduction {
				visibleLines := d.height - 8
				if visibleLines < 5 {
					visibleLines = 5
				}
				d.scrollOffset += visibleLines
				if d.scrollOffset > d.scrollMax {
					d.scrollOffset = d.scrollMax
				}
				return d, nil
			}
		case "home", "g":
			// Go to top for scrollable dialogs
			if d.Type == DialogLog || d.Type == DialogFirstRunIntroduction {
				d.scrollOffset = 0
				return d, nil
			}
		case "end", "G":
			// Go to bottom for scrollable dialogs
			if d.Type == DialogLog || d.Type == DialogFirstRunIntroduction {
				d.scrollOffset = d.scrollMax
				return d, nil
			}
		}
	}

	// For menu-style, log, progress, resource, and introduction dialogs, don't pass keys to input
	if d.Type == DialogSettings || d.Type == DialogMerge || d.Type == DialogBranchConflict || d.Type == DialogCommitBeforeMerge || d.Type == DialogPostMergeDestroy || d.Type == DialogMergeConflict || d.Type == DialogQuitConfirm || d.Type == DialogLog || d.Type == DialogProgress || d.Type == DialogResourceUsage || d.Type == DialogFirstRunIntroduction {
		return d, nil
	}

	// Pass to text input or textarea for text-based dialogs
	var cmd tea.Cmd
	if d.useTextArea {
		d.TextArea, cmd = d.TextArea.Update(msg)
	} else {
		d.Input, cmd = d.Input.Update(msg)
	}
	return d, cmd
}

// View renders the dialog
func (d DialogModel) View() string {
	titleStyle := DialogTitle
	inputStyle := DialogInputFocused.Width(d.width - 10)

	// Resource usage dialog renders stats with custom header
	if d.Type == DialogResourceUsage {
		var content strings.Builder
		// Show toggle hint in title area
		toggleHint := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("[Tab] toggle view")
		titleLine := titleStyle.Render(d.Title) + "  " + toggleHint
		content.WriteString(titleLine)
		content.WriteString("\n")
		content.WriteString(strings.Repeat("═", d.width-8))
		content.WriteString("\n\n")

		// Body contains the stats table (or loading/error message)
		content.WriteString(d.Body)
		content.WriteString("\n\n")

		// Footer hints
		if d.statsLoading {
			content.WriteString(KeyHintStyle.Render("Loading..."))
		} else {
			content.WriteString(KeyHint("r", " Refresh") + "    " + KeyHint("Esc", " Close"))
		}
		return DialogBox.Width(d.width).Render(content.String())
	}

	var content strings.Builder
	content.WriteString(titleStyle.Render(d.Title))
	content.WriteString("\n\n")

	// Log dialog renders scrollable content
	if d.Type == DialogLog {
		lines := strings.Split(d.Body, "\n")
		visibleLines := d.height - 8 // Account for title, padding, hints
		if visibleLines < 5 {
			visibleLines = 5
		}

		endLine := d.scrollOffset + visibleLines
		if endLine > len(lines) {
			endLine = len(lines)
		}

		// Calculate content width: dialog width minus border (2) and padding (4)
		contentWidth := d.width - 6
		if contentWidth < 20 {
			contentWidth = 20
		}

		if d.scrollOffset < len(lines) {
			// Truncate each line to fit the content width
			var truncatedLines []string
			for i := d.scrollOffset; i < endLine; i++ {
				line := lines[i]
				if len(line) > contentWidth {
					line = line[:contentWidth-3] + "..."
				}
				truncatedLines = append(truncatedLines, line)
			}
			content.WriteString(strings.Join(truncatedLines, "\n"))
		}
		content.WriteString("\n\n")

		// Bottom bar with scroll indicator integrated
		var lineInfo string
		if d.scrollMax > 0 {
			scrollPct := 100 * d.scrollOffset / d.scrollMax
			lineInfo = fmt.Sprintf("Line %d-%d/%d (%d%%)", d.scrollOffset+1, endLine, len(lines), scrollPct)
		} else {
			lineInfo = fmt.Sprintf("Lines: %d", len(lines))
		}
		content.WriteString(KeyHint("↑↓", " scroll") + "  " + KeyHint("PgUp/Dn", " page") + "  " + KeyHint("g/G", " top/bottom") + "  " + KeyHint("Enter/Esc", " close") + "  " + KeyHintStyle.Render(lineInfo))
		return DialogBox.Width(d.width).Render(content.String())
	}

	// Progress dialog shows operation status
	if d.Type == DialogProgress {
		content.WriteString(d.Body)
		content.WriteString("\n\n")
		if d.inProgress {
			content.WriteString(KeyHintStyle.Render("Working..."))
		} else {
			content.WriteString(KeyHint("Enter", " close") + "  " + KeyHintStyle.Render("[Esc] Cancel"))
		}
		return DialogBox.Width(d.width).Render(content.String())
	}

	// First-run introduction dialog renders scrollable content
	if d.Type == DialogFirstRunIntroduction {
		lines := strings.Split(d.Body, "\n")
		visibleLines := d.height - 8 // Account for title, padding, hints
		if visibleLines < 5 {
			visibleLines = 5
		}

		endLine := d.scrollOffset + visibleLines
		if endLine > len(lines) {
			endLine = len(lines)
		}

		if d.scrollOffset < len(lines) {
			visibleContent := strings.Join(lines[d.scrollOffset:endLine], "\n")
			content.WriteString(visibleContent)
		}
		content.WriteString("\n\n")

		// Footer with scroll hints if content is scrollable
		if d.scrollMax > 0 {
			content.WriteString(KeyHint("↑↓", " scroll") + "  ")
		}
		content.WriteString(KeyHint("Enter", " continue"))
		return DialogBox.Width(d.width).Render(content.String())
	}

	content.WriteString(d.Body)
	content.WriteString("\n\n")

	// Menu-style dialogs render a selection list
	if d.Type == DialogSettings || d.Type == DialogMerge || d.Type == DialogBranchConflict || d.Type == DialogCommitBeforeMerge || d.Type == DialogPostMergeDestroy || d.Type == DialogMergeConflict || d.Type == DialogQuitConfirm {
		for i, item := range d.MenuItems {
			if i == d.MenuSelection {
				content.WriteString("→ ")
				content.WriteString(item)
			} else {
				content.WriteString("  ")
				content.WriteString(item)
			}
			content.WriteString("\n")
		}
		content.WriteString("\n")
		if d.Type == DialogQuitConfirm {
			content.WriteString(KeyHint("y", " yes") + "  " + KeyHint("n", " no") + "  " + KeyHint("↑/↓", " navigate") + "  " + KeyHint("Enter", " select"))
		} else {
			content.WriteString(KeyHint("↑/↓", " navigate") + "  " + KeyHint("Enter", " select") + "  " + KeyHintStyle.Render("[Esc] Cancel"))
		}
	} else if d.useTextArea {
		content.WriteString(inputStyle.Render(d.TextArea.View()))
		content.WriteString("\n\n")
		content.WriteString(KeyHint("Shift+Enter", " newline") + "  " + KeyHint("Enter", " create") + "  " + KeyHintStyle.Render("[Esc] Cancel"))
	} else {
		content.WriteString(inputStyle.Render(d.Input.View()))
		content.WriteString("\n\n")

		// Footer with hints
		var hints string
		switch d.Type {
		case DialogDestroy:
			hints = KeyHintStyle.Render("[Esc] Cancel")
		case DialogNewWorkstream:
			hints = KeyHint("Enter", " create") + "  " + KeyHintStyle.Render("[Esc] Cancel")
		case DialogPRPreview:
			hints = KeyHint("Enter", " create") + "  " + KeyHintStyle.Render("[Esc] Cancel")
		}
		content.WriteString(hints)
	}

	return DialogBox.Width(d.width).Render(content.String())
}

// SetSize sets the dialog dimensions
func (d *DialogModel) SetSize(width, height int) {
	d.width = width
	d.height = height

	// Update textarea width to match the inputStyle container's inner content area
	// In View(), inputStyle = DialogInputFocused.Width(d.width - 10)
	// DialogInputFocused has border (2) + padding (2) = 4 total horizontal overhead
	// So inputStyle inner width = (width - 10) - 4 = width - 14
	if d.useTextArea {
		textareaWidth := width - 14
		if textareaWidth < 20 {
			textareaWidth = 20
		}
		d.TextArea.SetWidth(textareaWidth)
	}

	// Recalculate scrollMax for scrollable dialogs based on actual visible lines
	if d.Type == DialogLog || d.Type == DialogFirstRunIntroduction {
		lines := strings.Split(d.Body, "\n")
		visibleLines := height - 8 // Account for title, padding, hints
		if visibleLines < 5 {
			visibleLines = 5
		}
		if len(lines) > visibleLines {
			d.scrollMax = len(lines) - visibleLines
		} else {
			d.scrollMax = 0
		}
	}
}

// ViewInPane renders the dialog to fill a pane's content area (no outer box border)
// Uses the same styling as the regular View() method for consistency
func (d DialogModel) ViewInPane() string {
	// Use full available width/height for in-pane rendering
	contentWidth := d.width
	contentHeight := d.height

	var content strings.Builder
	content.WriteString(DialogTitle.Render(d.Title))
	content.WriteString("\n\n")

	// Body content
	content.WriteString(d.Body)
	content.WriteString("\n\n")

	// Progress dialog shows operation status - same styling as View()
	if d.Type == DialogProgress {
		if d.inProgress {
			content.WriteString(KeyHintStyle.Render("Working..."))
		} else {
			content.WriteString(KeyHint("Enter", " close") + "  " + KeyHintStyle.Render("[Esc] Cancel"))
		}
	} else if d.Type == DialogSettings || d.Type == DialogMerge || d.Type == DialogBranchConflict || d.Type == DialogCommitBeforeMerge || d.Type == DialogPostMergeDestroy || d.Type == DialogMergeConflict || d.Type == DialogQuitConfirm {
		// Menu items (for menu-style dialogs like merge) - same styling as View()
		for i, item := range d.MenuItems {
			if i == d.MenuSelection {
				content.WriteString("→ ")
				content.WriteString(item)
			} else {
				content.WriteString("  ")
				content.WriteString(item)
			}
			content.WriteString("\n")
		}
		content.WriteString("\n")
		if d.Type == DialogQuitConfirm {
			content.WriteString(KeyHint("y", " yes") + "  " + KeyHint("n", " no") + "  " + KeyHint("↑/↓", " navigate") + "  " + KeyHint("Enter", " select"))
		} else {
			content.WriteString(KeyHint("↑/↓", " navigate") + "  " + KeyHint("Enter", " select") + "  " + KeyHintStyle.Render("[Esc] Cancel"))
		}
	} else if d.useTextArea {
		inputStyle := DialogInputFocused.Width(contentWidth - 4)
		content.WriteString(inputStyle.Render(d.TextArea.View()))
		content.WriteString("\n\n")
		content.WriteString(KeyHint("Shift+Enter", " newline") + "  " + KeyHint("Enter", " create") + "  " + KeyHintStyle.Render("[Esc] Cancel"))
	} else if d.ConfirmWord != "" {
		inputStyle := DialogInputFocused.Width(contentWidth - 4)
		content.WriteString(inputStyle.Render(d.Input.View()))
		content.WriteString("\n\n")
		content.WriteString(KeyHintStyle.Render("[Esc] Cancel"))
	}

	// Create the final styled output that fills the pane with padding
	style := lipgloss.NewStyle().
		Width(contentWidth).
		Height(contentHeight).
		Padding(1, 2)

	return style.Render(content.String())
}

// DialogConfirmMsg is sent when dialog is confirmed
type DialogConfirmMsg struct {
	Type         DialogType
	WorkstreamID string
	Value        string
}

// DialogCancelMsg is sent when dialog is cancelled
type DialogCancelMsg struct{}

// SettingsConfirmMsg is sent when a settings action is confirmed
type SettingsConfirmMsg struct {
	Action SettingsAction
}

// PruneResultMsg is sent when container pruning completes
type PruneResultMsg struct {
	PrunedCount int
	Error       error
}

// MergeConfirmMsg is sent when a merge action is confirmed
type MergeConfirmMsg struct {
	Action       MergeAction
	WorkstreamID string
}

// CommitBeforeMergeConfirmMsg is sent when a commit-before-merge action is confirmed
type CommitBeforeMergeConfirmMsg struct {
	Action       CommitBeforeMergeAction
	WorkstreamID string
	BranchInfo   string // Branch statistics for the merge dialog
}

// BranchConflictConfirmMsg is sent when a branch conflict resolution is confirmed
type BranchConflictConfirmMsg struct {
	Action       BranchConflictAction
	WorkstreamID string
}

// ResourceStatsToggleMsg is sent when the resource dialog view is toggled
type ResourceStatsToggleMsg struct {
	IsGlobal bool
}

// ResourceStatsRefreshMsg is sent when the resource dialog needs to refresh stats
type ResourceStatsRefreshMsg struct {
	IsGlobal bool
}
