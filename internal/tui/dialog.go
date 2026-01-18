package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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
)

// DialogModel represents a modal dialog
type DialogModel struct {
	Type         DialogType
	Title        string
	Body         string
	Input        textinput.Model
	ConfirmWord  string // Word required to confirm (e.g., "destroy")
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
	ti := textinput.New()
	ti.Placeholder = "describe the task..."
	ti.CharLimit = 200
	ti.Width = 40
	ti.Focus()

	return DialogModel{
		Type:  DialogNewWorkstream,
		Title: "New Workstream",
		Body:  "Enter a prompt for Claude:",
		Input: ti,
	}
}

// NewPRDialog creates a PR preview/edit dialog
func NewPRDialog(branchName, title, body string) DialogModel {
	ti := textinput.New()
	ti.Placeholder = "PR title..."
	ti.Width = 40
	ti.SetValue(title)
	ti.Focus()
	ti.CharLimit = 100

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
	SettingsActionPruneAll     SettingsAction = "prune_all"
	SettingsActionCancel       SettingsAction = "cancel"
)

// MergeAction represents a merge menu action
type MergeAction string

const (
	MergeActionCreatePR MergeAction = "create_pr"
	MergeActionPush     MergeAction = "push"
	MergeActionCancel   MergeAction = "cancel"
)

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
			"Push branch only",
			"Cancel",
		},
		MenuSelection: 0,
	}
}

// NewSettingsDialog creates a settings menu dialog
func NewSettingsDialog(containerCount int) DialogModel {
	body := fmt.Sprintf("Containers managed by docker-tui: %d\n", containerCount)

	return DialogModel{
		Type:  DialogSettings,
		Title: "Settings",
		Body:  body,
		MenuItems: []string{
			"Prune stopped docker-tui containers",
			"Prune ALL docker-tui containers (stops running)",
			"Cancel",
		},
		MenuSelection: 0,
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

			if d.Type == DialogDestroy {
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
			if d.Type == DialogSettings || d.Type == DialogMerge {
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
			if d.Type == DialogSettings || d.Type == DialogMerge {
				if d.MenuSelection < len(d.MenuItems)-1 {
					d.MenuSelection++
				}
				return d, nil
			}
		}
	}

	// For menu-style, log, and progress dialogs, don't pass keys to input
	if d.Type == DialogSettings || d.Type == DialogMerge || d.Type == DialogLog || d.Type == DialogProgress {
		return d, nil
	}

	// Pass to text input for text-based dialogs
	var cmd tea.Cmd
	d.Input, cmd = d.Input.Update(msg)
	return d, cmd
}

// View renders the dialog
func (d DialogModel) View() string {
	titleStyle := DialogTitle
	inputStyle := DialogInput.Width(d.width - 10)

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
			content.WriteString(fmt.Sprintf("Line %d/%d (%d%%)\n", d.scrollOffset+1, len(lines), scrollPct))
		}
		content.WriteString(KeyHint("↑/↓", " scroll") + "  " + KeyHint("Enter", " close") + "  " + KeyHintStyle.Render("[Esc] Cancel"))
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
	if d.Type == DialogSettings || d.Type == DialogMerge {
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
