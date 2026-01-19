package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
}

// NewDestroyDialog creates a destroy confirmation dialog
func NewDestroyDialog(branchName, workstreamID string) DialogModel {
	ti := textinput.New()
	ti.Placeholder = "type 'destroy' to confirm"
	ti.Width = 40
	ti.Focus()
	ti.CharLimit = 20

	// Style the textinput
	ti.Prompt = "› "
	ti.PromptStyle = DialogInputPrompt
	ti.TextStyle = DialogInputText
	ti.PlaceholderStyle = DialogInputPlaceholder
	ti.Cursor.Style = DialogInputCursor

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
	ta.FocusedStyle.Base = lipgloss.NewStyle() // No border
	ta.FocusedStyle.Text = DialogInputText
	ta.FocusedStyle.Placeholder = DialogInputPlaceholder
	ta.FocusedStyle.CursorLine = DialogInputText
	ta.FocusedStyle.EndOfBuffer = lipgloss.NewStyle().Foreground(lipgloss.Color("#333333"))
	ta.BlurredStyle = ta.FocusedStyle

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
	ti.Width = 50
	ti.SetValue(title)
	ti.Focus()
	ti.CharLimit = 100

	// Style the textinput
	ti.Prompt = "› "
	ti.PromptStyle = DialogInputPrompt
	ti.TextStyle = DialogInputText
	ti.PlaceholderStyle = DialogInputPlaceholder
	ti.Cursor.Style = DialogInputCursor

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
	MergeActionCreatePR   MergeAction = "create_pr"
	MergeActionMergeMain  MergeAction = "merge_main"
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
func NewMergeDialog(branchName, workstreamID string) DialogModel {
	body := fmt.Sprintf("Branch: %s\n", branchName)

	return DialogModel{
		Type:         DialogMerge,
		Title:        "Merge / PR Options",
		Body:         body,
		WorkstreamID: workstreamID,
		MenuItems: []string{
			"Create Pull Request",
			"Merge into main",
			"Push branch only",
			"Cancel",
		},
		MenuSelection: 0,
	}
}

// NewCommitBeforeMergeDialog creates a dialog asking if user wants to commit uncommitted changes
func NewCommitBeforeMergeDialog(branchName, workstreamID string) DialogModel {
	body := fmt.Sprintf("Branch '%s' has uncommitted changes.\n\nWould you like Claude to commit them first?", branchName)

	return DialogModel{
		Type:         DialogCommitBeforeMerge,
		Title:        "Uncommitted Changes",
		Body:         body,
		WorkstreamID: workstreamID,
		MenuItems: []string{
			"Yes, commit changes",
			"No, continue without committing",
			"Cancel",
		},
		MenuSelection: 0,
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
	ti.Width = 40
	ti.Focus()
	ti.CharLimit = 20

	// Style the textinput
	ti.Prompt = "› "
	ti.PromptStyle = DialogInputPrompt
	ti.TextStyle = DialogInputText
	ti.PlaceholderStyle = DialogInputPlaceholder
	ti.Cursor.Style = DialogInputCursor

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
	ti.Width = 40
	ti.Focus()
	ti.CharLimit = 20

	// Style the textinput
	ti.Prompt = "› "
	ti.PromptStyle = DialogInputPrompt
	ti.TextStyle = DialogInputText
	ti.PlaceholderStyle = DialogInputPlaceholder
	ti.Cursor.Style = DialogInputCursor

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
		switch msg.String() {
		case "esc", "ctrl+c":
			// Progress dialog can't be dismissed while in progress
			if d.Type == DialogProgress && d.inProgress {
				return d, nil
			}
			return d, func() tea.Msg { return DialogCancelMsg{} }
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
					action = MergeActionCreatePR
				case 1:
					action = MergeActionMergeMain
				case 2:
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
				return d, func() tea.Msg {
					return CommitBeforeMergeConfirmMsg{Action: action, WorkstreamID: d.WorkstreamID}
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
			// Handle log dialog scrolling
			if d.Type == DialogLog {
				if d.scrollOffset > 0 {
					d.scrollOffset--
				}
				return d, nil
			}
			// Only handle for menu dialogs, otherwise pass to input
			if d.Type == DialogSettings || d.Type == DialogMerge || d.Type == DialogBranchConflict || d.Type == DialogCommitBeforeMerge {
				if d.MenuSelection > 0 {
					d.MenuSelection--
				}
				return d, nil
			}
		case "down", "j":
			// Handle log dialog scrolling
			if d.Type == DialogLog {
				if d.scrollOffset < d.scrollMax {
					d.scrollOffset++
				}
				return d, nil
			}
			// Only handle for menu dialogs, otherwise pass to input
			if d.Type == DialogSettings || d.Type == DialogMerge || d.Type == DialogBranchConflict || d.Type == DialogCommitBeforeMerge {
				if d.MenuSelection < len(d.MenuItems)-1 {
					d.MenuSelection++
				}
				return d, nil
			}
		case "pgup", "ctrl+u":
			// Page up for log dialog
			if d.Type == DialogLog {
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
			// Page down for log dialog
			if d.Type == DialogLog {
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
			// Go to top for log dialog
			if d.Type == DialogLog {
				d.scrollOffset = 0
				return d, nil
			}
		case "end", "G":
			// Go to bottom for log dialog
			if d.Type == DialogLog {
				d.scrollOffset = d.scrollMax
				return d, nil
			}
		}
	}

	// For menu-style, log, and progress dialogs, don't pass keys to input
	if d.Type == DialogSettings || d.Type == DialogMerge || d.Type == DialogBranchConflict || d.Type == DialogCommitBeforeMerge || d.Type == DialogLog || d.Type == DialogProgress {
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

		if d.scrollOffset < len(lines) {
			visibleContent := strings.Join(lines[d.scrollOffset:endLine], "\n")
			content.WriteString(visibleContent)
		}
		content.WriteString("\n\n")

		// Scroll indicator
		if d.scrollMax > 0 {
			scrollPct := 100 * d.scrollOffset / d.scrollMax
			content.WriteString(fmt.Sprintf("Line %d-%d/%d (%d%%)\n", d.scrollOffset+1, endLine, len(lines), scrollPct))
		} else {
			content.WriteString(fmt.Sprintf("Lines: %d\n", len(lines)))
		}
		content.WriteString(KeyHint("↑↓", " scroll") + "  " + KeyHint("PgUp/Dn", " page") + "  " + KeyHint("g/G", " top/bottom") + "  " + KeyHint("Enter/Esc", " close"))
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

	content.WriteString(d.Body)
	content.WriteString("\n\n")

	// Menu-style dialogs render a selection list
	if d.Type == DialogSettings || d.Type == DialogMerge || d.Type == DialogBranchConflict || d.Type == DialogCommitBeforeMerge {
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
		content.WriteString(KeyHint("↑/↓", " navigate") + "  " + KeyHint("Enter", " select") + "  " + KeyHintStyle.Render("[Esc] Cancel"))
	} else if d.useTextArea {
		content.WriteString(inputStyle.Render(d.TextArea.View()))
		content.WriteString("\n\n")
		content.WriteString(KeyHint("Enter", " create") + "  " + KeyHintStyle.Render("[Esc] Cancel"))
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

	// Update textarea width to match dialog content area
	// Account for dialog box padding (4 on each side = 8) and input style padding (2)
	if d.useTextArea {
		textareaWidth := width - 12
		if textareaWidth < 20 {
			textareaWidth = 20
		}
		d.TextArea.SetWidth(textareaWidth)
	}

	// Recalculate scrollMax for log dialogs based on actual visible lines
	if d.Type == DialogLog {
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
}

// BranchConflictConfirmMsg is sent when a branch conflict resolution is confirmed
type BranchConflictConfirmMsg struct {
	Action       BranchConflictAction
	WorkstreamID string
}
