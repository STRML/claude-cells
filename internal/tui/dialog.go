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
}

// NewDestroyDialog creates a destroy confirmation dialog
func NewDestroyDialog(branchName, workstreamID string) DialogModel {
	ti := textinput.New()
	ti.Placeholder = "type 'destroy' to confirm"
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
	ti.Focus()
	ti.CharLimit = 200

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

// Init initializes the dialog
func (d DialogModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles dialog input
func (d DialogModel) Update(msg tea.Msg) (DialogModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return d, func() tea.Msg { return DialogCancelMsg{} }
		case "enter":
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
			}
		}
	}

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
	content.WriteString(d.Body)
	content.WriteString("\n\n")
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
