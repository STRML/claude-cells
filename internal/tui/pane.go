package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/samuelreed/docker-tui/internal/workstream"
)

// PaneModel represents a single workstream pane
type PaneModel struct {
	workstream *workstream.Workstream
	viewport   viewport.Model
	input      textinput.Model
	focused    bool
	width      int
	height     int
	output     strings.Builder
}

// NewPaneModel creates a new pane for a workstream
func NewPaneModel(ws *workstream.Workstream) PaneModel {
	ti := textinput.New()
	ti.Placeholder = "Enter prompt..."
	ti.CharLimit = 500

	vp := viewport.New(0, 0)

	return PaneModel{
		workstream: ws,
		viewport:   vp,
		input:      ti,
	}
}

// Init initializes the pane
func (p PaneModel) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (p PaneModel) Update(msg tea.Msg) (PaneModel, tea.Cmd) {
	var cmds []tea.Cmd

	if p.focused {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				if p.input.Value() != "" {
					// Return a command to send the prompt
					prompt := p.input.Value()
					p.input.Reset()
					return p, func() tea.Msg {
						return PromptMsg{
							WorkstreamID: p.workstream.ID,
							Prompt:       prompt,
						}
					}
				}
			}
		}

		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		cmds = append(cmds, cmd)
	}

	var cmd tea.Cmd
	p.viewport, cmd = p.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return p, tea.Batch(cmds...)
}

// View renders the pane
func (p PaneModel) View() string {
	// Header with status and branch name
	status := StatusStyle(string(p.workstream.GetState()))
	title := PaneTitle.Render(p.workstream.BranchName)
	stateLabel := fmt.Sprintf("(%s)", p.workstream.GetState())
	header := fmt.Sprintf("%s %s %s", status, title, stateLabel)

	// Output viewport
	p.viewport.SetContent(p.output.String())
	outputView := p.viewport.View()

	// Input (only show when focused)
	var inputView string
	if p.focused {
		inputView = "\n" + p.input.View()
	}

	// Combine
	content := header + "\n\n" + outputView + inputView

	// Apply border based on focus
	var style lipgloss.Style
	if p.focused {
		style = PaneBorderActive.Width(p.width - 2).Height(p.height - 2)
	} else {
		style = PaneBorderInactive.Width(p.width - 2).Height(p.height - 2)
	}

	return style.Render(content)
}

// SetSize sets the pane dimensions
func (p *PaneModel) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.viewport.Width = width - 4  // Account for border and padding
	p.viewport.Height = height - 6 // Account for header, input, borders
}

// SetFocused sets the focus state
func (p *PaneModel) SetFocused(focused bool) {
	p.focused = focused
	if focused {
		p.input.Focus()
	} else {
		p.input.Blur()
	}
}

// AppendOutput adds text to the output buffer
func (p *PaneModel) AppendOutput(text string) {
	p.output.WriteString(text)
	p.viewport.SetContent(p.output.String())
	p.viewport.GotoBottom()
}

// Workstream returns the underlying workstream
func (p *PaneModel) Workstream() *workstream.Workstream {
	return p.workstream
}

// PromptMsg is sent when user submits a prompt
type PromptMsg struct {
	WorkstreamID string
	Prompt       string
}
