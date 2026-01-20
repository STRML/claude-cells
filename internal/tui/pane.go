package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/STRML/claude-cells/internal/workstream"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hinshun/vt10x"
)

// initTimeout is how long to wait for initialization before showing a warning
// 5 minutes allows time for image builds and Claude Code installation
const initTimeout = 5 * time.Minute

// ansiRegex matches ANSI escape sequences
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// stripANSI removes all ANSI escape sequences from a string
func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

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
	initializing    bool           // True while waiting for Claude Code to be ready
	initStartTime   time.Time      // When initialization started (for timeout)
	spinnerFrame    int            // Current spinner animation frame
	initStatus      string         // Status message during initialization
	initStep        int            // Current initialization step (1-3)
	initSteps       int            // Total initialization steps
	scrollback      []string       // Scrollback buffer (lines that scrolled off top)
	scrollMode      bool           // True when viewing scrollback (not following live output)
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

// SetInitializing sets the initializing state with a status message
func (p *PaneModel) SetInitializing(initializing bool) {
	p.initializing = initializing
	if initializing {
		p.initStartTime = time.Now()
		p.initSteps = 3
		if p.initStatus == "" {
			p.initStatus = "Starting..."
			p.initStep = 1
		}
	}
}

// InitTimedOut returns true if initialization has taken longer than initTimeout
func (p *PaneModel) InitTimedOut() bool {
	if !p.initializing || p.initStartTime.IsZero() {
		return false
	}
	return time.Since(p.initStartTime) > initTimeout
}

// InitElapsed returns how long initialization has been running
func (p *PaneModel) InitElapsed() time.Duration {
	if p.initStartTime.IsZero() {
		return 0
	}
	return time.Since(p.initStartTime)
}

// GetInitStatus returns the current initialization status message
func (p *PaneModel) GetInitStatus() string {
	return p.initStatus
}

// GetInitStartTime returns when initialization started
func (p *PaneModel) GetInitStartTime() time.Time {
	return p.initStartTime
}

// SetInitStatus sets the initialization status message and step
func (p *PaneModel) SetInitStatus(status string) {
	p.initStatus = status
	// Auto-advance step based on status
	switch {
	case strings.Contains(status, "container"):
		p.initStep = 1
	case strings.Contains(status, "Claude Code"):
		p.initStep = 2
	case strings.Contains(status, "Resuming"):
		p.initStep = 2
	default:
		if p.initStep == 0 {
			p.initStep = 1
		}
	}
}

// IsInitializing returns true if the pane is still initializing
func (p *PaneModel) IsInitializing() bool {
	return p.initializing
}

// TickSpinner advances the spinner animation
func (p *PaneModel) TickSpinner() {
	p.spinnerFrame = (p.spinnerFrame + 1) % 4
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
				case "shift+enter":
					data = []byte("\n") // Insert literal newline (multiline input)
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

			// No PTY - don't accept input
			// This can happen if the container failed to start or the session ended
			return p, nil
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
	title := PaneTitle.Render(p.workstream.GetTitle())
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
		// Combine scrollback with current vterm content
		var fullContent strings.Builder
		if len(p.scrollback) > 0 {
			fullContent.WriteString(strings.Join(p.scrollback, "\n"))
			fullContent.WriteString("\n")
		}
		fullContent.WriteString(p.renderVTerm())
		outputContent = fullContent.String()
	} else {
		// Use simple output buffer for non-PTY mode
		outputContent = p.output.String()
	}

	// Remember scroll position before setting content
	wasAtBottom := p.viewport.AtBottom()
	yOffset := p.viewport.YOffset

	p.viewport.SetContent(outputContent)

	// Restore scroll position or go to bottom
	if p.scrollMode {
		// In scroll mode - try to maintain position
		p.viewport.SetYOffset(yOffset)
	} else if wasAtBottom || !p.scrollMode {
		// Follow live output
		p.viewport.GotoBottom()
	}

	outputView := p.viewport.View()

	// Show spinner overlay while initializing
	if p.initializing {
		// Grey out the output
		outputView = stripANSI(outputView)
		greyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#444444"))
		outputView = greyStyle.Render(outputView)

		// Create spinner overlay
		spinnerChars := []string{"⠋", "⠙", "⠹", "⠸"}
		spinner := spinnerChars[p.spinnerFrame%len(spinnerChars)]
		spinnerStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#0891B2")).
			Bold(true)
		messageStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#AAAAAA"))

		statusMsg := p.initStatus
		if statusMsg == "" {
			statusMsg = "Starting..."
		}

		// Add elapsed time to status
		elapsed := p.InitElapsed().Round(time.Second)
		if elapsed > 0 {
			statusMsg = fmt.Sprintf("%s (%v)", statusMsg, elapsed)
		}

		spinnerText := spinnerStyle.Render(spinner) + " " + messageStyle.Render(statusMsg)

		// Center the spinner in the viewport
		viewportHeight := p.height - 4 // Account for header and borders
		viewportWidth := p.width - 4
		if viewportHeight > 0 && viewportWidth > 0 {
			lines := strings.Split(outputView, "\n")
			centerY := viewportHeight / 2
			centerX := (viewportWidth - lipgloss.Width(spinnerText)) / 2
			if centerX < 0 {
				centerX = 0
			}
			if centerY >= 0 && centerY < len(lines) {
				padding := strings.Repeat(" ", centerX)
				lines[centerY] = padding + spinnerText
			}
			outputView = strings.Join(lines, "\n")
		}
	} else if p.focused && !p.inputMode {
		// Focused but nav mode: lighter grey
		outputView = stripANSI(outputView)
		greyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
		outputView = greyStyle.Render(outputView)
	} else if !p.focused {
		// Unfocused panes: dark grey
		outputView = stripANSI(outputView)
		greyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
		outputView = greyStyle.Render(outputView)
	}

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
			// Nav mode - dim border to reinforce "inactive" feel
			borderColor = "#666666"
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
					} else if cell.FG > 255 {
						// Truecolor RGB - vt10x stores as r<<16 | g<<8 | b
						r := (cell.FG >> 16) & 0xFF
						g := (cell.FG >> 8) & 0xFF
						b := cell.FG & 0xFF
						line.WriteString(fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b))
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
					} else if cell.BG > 255 {
						// Truecolor RGB - vt10x stores as r<<16 | g<<8 | b
						r := (cell.BG >> 16) & 0xFF
						g := (cell.BG >> 8) & 0xFF
						b := cell.BG & 0xFF
						line.WriteString(fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b))
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

// Index returns the pane's permanent index (1-based)
func (p *PaneModel) Index() int {
	return p.index
}

// AppendOutput adds text to the output buffer (for non-PTY output like status messages)
func (p *PaneModel) AppendOutput(text string) {
	p.output.WriteString(text)
	p.viewport.SetContent(p.output.String())
	p.viewport.GotoBottom()
}

// WritePTYOutput writes raw PTY output to the virtual terminal
func (p *PaneModel) WritePTYOutput(data []byte) {
	// Capture current first line before write (to detect scrolling)
	firstLineBefore := p.getVtermLine(0)

	p.vterm.Write(data)

	// Check if content scrolled (first line changed)
	firstLineAfter := p.getVtermLine(0)
	if firstLineBefore != "" && firstLineBefore != firstLineAfter {
		// Content scrolled - save old first line to scrollback
		// Limit scrollback to 10000 lines to prevent memory issues
		if len(p.scrollback) >= 10000 {
			// Remove oldest 1000 lines when limit reached
			p.scrollback = p.scrollback[1000:]
		}
		p.scrollback = append(p.scrollback, firstLineBefore)
	}
}

// getVtermLine returns a single line from the vterm (0-indexed)
func (p *PaneModel) getVtermLine(row int) string {
	if p.vterm == nil {
		return ""
	}
	cols, rows := p.vterm.Size()
	if row < 0 || row >= rows || cols <= 0 {
		return ""
	}

	var line strings.Builder
	for col := 0; col < cols; col++ {
		cell := p.vterm.Cell(col, row)
		if cell.Char == 0 {
			line.WriteRune(' ')
		} else {
			line.WriteRune(cell.Char)
		}
	}
	return strings.TrimRight(line.String(), " ")
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

// ScrollPageUp scrolls the viewport up by one page
func (p *PaneModel) ScrollPageUp() {
	p.scrollMode = true
	p.viewport.ViewUp()
}

// ScrollPageDown scrolls the viewport down by one page
func (p *PaneModel) ScrollPageDown() {
	p.viewport.ViewDown()
	// Exit scroll mode if at bottom
	if p.viewport.AtBottom() {
		p.scrollMode = false
	}
}

// ScrollToBottom scrolls to the bottom and exits scroll mode
func (p *PaneModel) ScrollToBottom() {
	p.viewport.GotoBottom()
	p.scrollMode = false
}

// IsScrollMode returns true if the pane is in scroll mode (not following live output)
func (p *PaneModel) IsScrollMode() bool {
	return p.scrollMode
}

// PromptMsg is sent when user submits a prompt
type PromptMsg struct {
	WorkstreamID string
	Prompt       string
}
