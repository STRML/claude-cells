package tui

import (
	"fmt"
	"strings"

	"github.com/STRML/claude-cells/internal/workstream"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hinshun/vt10x"
)

// PaneModel represents a single workstream pane
type PaneModel struct {
	workstream      *workstream.Workstream
	viewport        viewport.Model
	input           textinput.Model
	focused         bool
	inputMode       bool // True when accepting input (different highlight)
	width           int
	height          int
	output          *strings.Builder
	pty             *PTYSession
	vterm           vt10x.Terminal // Virtual terminal for ANSI code handling
	lastVtermRender string         // Cached last successful vterm render
	index           int            // Pane index (1-based for display)
}

// Width returns the pane width
func (p *PaneModel) Width() int {
	return p.width
}

// Height returns the pane height
func (p *PaneModel) Height() int {
	return p.height
}

// NewPaneModel creates a new pane for a workstream
func NewPaneModel(ws *workstream.Workstream) PaneModel {
	ti := textinput.New()
	ti.Placeholder = "Enter prompt..."
	ti.CharLimit = 500

	vp := viewport.New(0, 0)

	// Create virtual terminal with default size (will be resized later)
	vterm := vt10x.New(vt10x.WithSize(80, 24))

	return PaneModel{
		workstream: ws,
		viewport:   vp,
		input:      ti,
		output:     &strings.Builder{},
		vterm:      vterm,
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
			// If we have a PTY session, send keys directly to it
			if p.pty != nil && !p.pty.IsClosed() {
				// Send the key to the PTY
				keyStr := msg.String()
				var data []byte
				switch keyStr {
				case "enter":
					data = []byte("\r")
				case "backspace":
					data = []byte{127}
				case "tab":
					data = []byte("\t")
				case "shift+tab":
					data = []byte{27, '[', 'Z'} // CSI Z - reverse tab
				case "esc":
					data = []byte{27}
				case "ctrl+a":
					data = []byte{1} // SOH - go to beginning of line
				case "ctrl+b":
					data = []byte{2} // STX - move backward one character
				case "ctrl+c":
					data = []byte{3} // ETX - interrupt signal
				case "ctrl+d":
					data = []byte{4} // EOT - end of transmission
				case "ctrl+e":
					data = []byte{5} // ENQ - go to end of line
				case "ctrl+f":
					data = []byte{6} // ACK - move forward one character
				case "ctrl+g":
					data = []byte{7} // BEL - abort
				case "ctrl+h":
					data = []byte{8} // BS - backspace (alternative)
				case "ctrl+k":
					data = []byte{11} // VT - kill line forward (to end)
				case "ctrl+l":
					data = []byte{12} // FF - form feed (clear screen)
				case "ctrl+n":
					data = []byte{14} // SO - next line in history
				case "ctrl+o":
					data = []byte{15} // SI - operate-and-get-next
				case "ctrl+p":
					data = []byte{16} // DLE - previous line in history
				case "ctrl+r":
					data = []byte{18} // DC2 - reverse history search
				case "ctrl+s":
					data = []byte{19} // DC3 - forward history search
				case "ctrl+t":
					data = []byte{20} // DC4 - transpose characters
				case "ctrl+u":
					data = []byte{21} // NAK - kill line backward (to beginning)
				case "ctrl+v":
					data = []byte{22} // SYN - literal next character
				case "ctrl+w":
					data = []byte{23} // ETB - delete word backward
				case "ctrl+y":
					data = []byte{25} // EM - yank (paste from kill buffer)
				case "ctrl+z":
					data = []byte{26} // SUB - suspend
				case "up":
					data = []byte{27, '[', 'A'}
				case "down":
					data = []byte{27, '[', 'B'}
				case "left":
					data = []byte{27, '[', 'D'}
				case "right":
					data = []byte{27, '[', 'C'}
				case "home":
					data = []byte{27, '[', 'H'}
				case "end":
					data = []byte{27, '[', 'F'}
				case "pgup":
					data = []byte{27, '[', '5', '~'}
				case "pgdown":
					data = []byte{27, '[', '6', '~'}
				case "delete":
					data = []byte{27, '[', '3', '~'}
				case "insert":
					data = []byte{27, '[', '2', '~'}
				default:
					// Handle alt+key combinations (ESC + key)
					if strings.HasPrefix(keyStr, "alt+") && len(keyStr) == 5 {
						// Alt key sends ESC followed by the character
						data = []byte{27, keyStr[4]}
					} else if len(keyStr) == 1 {
						// Single character
						data = []byte(keyStr)
					} else if msg.Type == tea.KeyRunes {
						data = []byte(string(msg.Runes))
					}
				}
				if len(data) > 0 {
					_ = p.pty.Write(data)
				}
				return p, nil
			}

			// No PTY - use text input for prompts
			switch msg.String() {
			case "enter":
				if p.input.Value() != "" {
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

		// Only update text input if no PTY
		if p.pty == nil || p.pty.IsClosed() {
			var cmd tea.Cmd
			p.input, cmd = p.input.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	var cmd tea.Cmd
	p.viewport, cmd = p.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return p, tea.Batch(cmds...)
}

// View renders the pane
func (p PaneModel) View() string {
	// Header with index, status, and branch name
	indexStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#7C3AED")).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1)

	// Mode indicator for focused pane
	var modeIndicator string
	if p.focused {
		if p.inputMode {
			// Input mode - bright cyan background
			indexStyle = indexStyle.Background(lipgloss.Color("#0891B2"))
			modeIndicator = lipgloss.NewStyle().
				Background(lipgloss.Color("#0891B2")).
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true).
				Padding(0, 1).
				Render("INPUT")
		} else {
			// Nav mode - green background
			indexStyle = indexStyle.Background(lipgloss.Color("#059669"))
			modeIndicator = lipgloss.NewStyle().
				Background(lipgloss.Color("#059669")).
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true).
				Padding(0, 1).
				Render("NAV")
		}
	}
	indexLabel := indexStyle.Render(fmt.Sprintf("%d", p.index))

	status := StatusStyle(string(p.workstream.GetState()))
	title := PaneTitle.Render(p.workstream.BranchName)
	stateLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(fmt.Sprintf("(%s)", p.workstream.GetState()))

	var header string
	if p.focused && modeIndicator != "" {
		header = fmt.Sprintf("%s %s %s %s %s", indexLabel, modeIndicator, status, title, stateLabel)
	} else {
		header = fmt.Sprintf("%s %s %s %s", indexLabel, status, title, stateLabel)
	}

	// Output content - use virtual terminal if PTY is active
	var outputContent string
	if p.pty != nil && !p.pty.IsClosed() {
		// Render from virtual terminal
		outputContent = p.renderVTerm()
	} else {
		// Use simple output buffer for non-PTY mode
		outputContent = p.output.String()
	}

	p.viewport.SetContent(outputContent)
	outputView := p.viewport.View()

	// Input (only show when focused and no PTY)
	var inputView string
	if p.focused && (p.pty == nil || p.pty.IsClosed()) {
		inputView = "\n" + p.input.View()
	}

	// Combine
	content := header + "\n\n" + outputView + inputView

	// Apply border based on focus and input mode
	var style lipgloss.Style
	if p.focused {
		var borderColor string
		if p.inputMode {
			// Input mode - bright cyan border (more noticeable)
			borderColor = "#0891B2"
		} else {
			// Nav mode - green border
			borderColor = "#059669"
		}
		style = lipgloss.NewStyle().
			Border(lipgloss.ThickBorder()).
			BorderForeground(lipgloss.Color(borderColor)).
			Width(p.width - 2).
			Height(p.height - 2)
	} else {
		style = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#444444")).
			Width(p.width - 2).
			Height(p.height - 2)
	}

	return style.Render(content)
}

// SetInputMode sets whether the pane is in input mode
func (p *PaneModel) SetInputMode(inputMode bool) {
	p.inputMode = inputMode
}

// renderVTerm renders the virtual terminal content as a string with ANSI colors
func (p *PaneModel) renderVTerm() (result string) {
	// Use defer/recover to handle any panics from vt10x
	defer func() {
		if r := recover(); r != nil {
			// Return last successful render on panic
			result = p.lastVtermRender
		}
	}()

	if p.vterm == nil {
		return p.lastVtermRender
	}

	// vt10x.Size() returns (cols, rows) - not (rows, cols)!
	cols, rows := p.vterm.Size()
	if rows <= 0 || cols <= 0 {
		return p.lastVtermRender
	}

	var lines []string
	for row := 0; row < rows; row++ {
		var line strings.Builder
		var lastFG, lastBG vt10x.Color = vt10x.DefaultFG, vt10x.DefaultBG

		for col := 0; col < cols; col++ {
			cell := p.vterm.Cell(col, row)

			// Check if colors changed
			if cell.FG != lastFG || cell.BG != lastBG {
				// Reset and apply new colors
				line.WriteString("\x1b[0m") // Reset
				if cell.FG != vt10x.DefaultFG {
					if cell.FG.ANSI() {
						// Standard ANSI colors (0-15)
						if cell.FG < 8 {
							line.WriteString(fmt.Sprintf("\x1b[%dm", 30+cell.FG))
						} else {
							line.WriteString(fmt.Sprintf("\x1b[%dm", 90+cell.FG-8))
						}
					} else {
						// 256-color mode
						line.WriteString(fmt.Sprintf("\x1b[38;5;%dm", cell.FG))
					}
				}
				if cell.BG != vt10x.DefaultBG {
					if cell.BG.ANSI() {
						// Standard ANSI colors (0-15)
						if cell.BG < 8 {
							line.WriteString(fmt.Sprintf("\x1b[%dm", 40+cell.BG))
						} else {
							line.WriteString(fmt.Sprintf("\x1b[%dm", 100+cell.BG-8))
						}
					} else {
						// 256-color mode
						line.WriteString(fmt.Sprintf("\x1b[48;5;%dm", cell.BG))
					}
				}
				lastFG, lastBG = cell.FG, cell.BG
			}

			if cell.Char == 0 {
				line.WriteRune(' ')
			} else {
				line.WriteRune(cell.Char)
			}
		}
		// Reset colors at end of line
		line.WriteString("\x1b[0m")
		// Trim trailing spaces (but keep color codes)
		lineStr := line.String()
		// Only trim if line ends with spaces after the reset
		if strings.HasSuffix(lineStr, "\x1b[0m") {
			prefix := lineStr[:len(lineStr)-4]
			prefix = strings.TrimRight(prefix, " ")
			lineStr = prefix + "\x1b[0m"
		}
		lines = append(lines, lineStr)
	}
	// Remove trailing empty lines (just reset codes)
	for len(lines) > 0 && (lines[len(lines)-1] == "" || lines[len(lines)-1] == "\x1b[0m") {
		lines = lines[:len(lines)-1]
	}

	result = strings.Join(lines, "\n")
	p.lastVtermRender = result
	return result
}

// SetSize sets the pane dimensions
func (p *PaneModel) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.viewport.Width = width - 4   // Account for border and padding
	p.viewport.Height = height - 6 // Account for header, input, borders

	// Calculate inner dimensions
	innerWidth := width - 4
	innerHeight := height - 6
	if innerWidth < 40 {
		innerWidth = 40
	}
	if innerHeight < 10 {
		innerHeight = 10
	}

	// Resize virtual terminal
	p.vterm.Resize(innerWidth, innerHeight)

	// Resize PTY if active
	if p.pty != nil && !p.pty.IsClosed() {
		_ = p.pty.Resize(innerWidth, innerHeight)
	}
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

// SetIndex sets the pane index (1-based)
func (p *PaneModel) SetIndex(idx int) {
	p.index = idx
}

// AppendOutput adds text to the output buffer (for non-PTY output like status messages)
func (p *PaneModel) AppendOutput(text string) {
	p.output.WriteString(text)
	p.viewport.SetContent(p.output.String())
	p.viewport.GotoBottom()
}

// WritePTYOutput writes raw PTY output to the virtual terminal
func (p *PaneModel) WritePTYOutput(data []byte) {
	p.vterm.Write(data)
}

// Workstream returns the underlying workstream
func (p *PaneModel) Workstream() *workstream.Workstream {
	return p.workstream
}

// SetPTY sets the PTY session for this pane
func (p *PaneModel) SetPTY(pty *PTYSession) {
	p.pty = pty
}

// PTY returns the PTY session for this pane
func (p *PaneModel) PTY() *PTYSession {
	return p.pty
}

// HasPTY returns true if the pane has an active PTY session
func (p *PaneModel) HasPTY() bool {
	return p.pty != nil && !p.pty.IsClosed()
}

// GetFullOutput returns the complete output buffer content including vterm history.
// This is useful for showing logs to the user.
func (p *PaneModel) GetFullOutput() string {
	var result strings.Builder

	// Add status messages (pre-PTY output)
	statusOutput := p.output.String()
	if statusOutput != "" {
		result.WriteString(statusOutput)
	}

	// Add vterm content if we have PTY output
	if p.vterm != nil {
		vtermContent := p.renderVTerm()
		if vtermContent != "" {
			if result.Len() > 0 {
				result.WriteString("\n")
			}
			result.WriteString(vtermContent)
		}
	}

	return result.String()
}

// PromptMsg is sent when user submits a prompt
type PromptMsg struct {
	WorkstreamID string
	Prompt       string
}
