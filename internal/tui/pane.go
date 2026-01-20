package tui

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
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

// ansiSGRRegex matches SGR (Select Graphic Rendition) sequences specifically
var ansiSGRRegex = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// basic16Colors maps basic ANSI color codes to RGB values
// Indexes 0-7 are normal colors, 8-15 are bright colors
var basic16Colors = []struct{ r, g, b int }{
	{0, 0, 0},       // 0: Black
	{205, 49, 49},   // 1: Red
	{13, 188, 121},  // 2: Green
	{229, 229, 16},  // 3: Yellow
	{36, 114, 200},  // 4: Blue
	{188, 63, 188},  // 5: Magenta
	{17, 168, 205},  // 6: Cyan
	{229, 229, 229}, // 7: White
	{102, 102, 102}, // 8: Bright Black (Gray)
	{241, 76, 76},   // 9: Bright Red
	{35, 209, 139},  // 10: Bright Green
	{245, 245, 67},  // 11: Bright Yellow
	{59, 142, 234},  // 12: Bright Blue
	{214, 112, 214}, // 13: Bright Magenta
	{41, 184, 219},  // 14: Bright Cyan
	{255, 255, 255}, // 15: Bright White
}

// muteANSI transforms colors in ANSI sequences to be muted (desaturated)
// saturation: 0.0 = grayscale, 1.0 = original
// brightness: multiplier for lightness
func muteANSI(s string, saturation, brightness float64) string {
	return ansiSGRRegex.ReplaceAllStringFunc(s, func(match string) string {
		// Extract the parameters between \x1b[ and m
		submatch := ansiSGRRegex.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		params := submatch[1]
		if params == "" {
			return match // Reset sequence, keep as-is
		}

		// Parse parameters
		parts := strings.Split(params, ";")
		result := make([]string, 0, len(parts))

		for i := 0; i < len(parts); i++ {
			code, err := strconv.Atoi(parts[i])
			if err != nil {
				result = append(result, parts[i])
				continue
			}

			// Handle extended color sequences: 38;5;N or 38;2;R;G;B (foreground)
			// and 48;5;N or 48;2;R;G;B (background)
			if (code == 38 || code == 48) && i+1 < len(parts) {
				colorType, _ := strconv.Atoi(parts[i+1])
				if colorType == 5 && i+2 < len(parts) {
					// 256-color mode: 38;5;N or 48;5;N
					colorIndex, _ := strconv.Atoi(parts[i+2])
					r, g, b := color256ToRGB(colorIndex)
					mr, mg, mb := MuteColor(r, g, b, saturation, brightness)
					result = append(result, fmt.Sprintf("%d;2;%d;%d;%d", code, mr, mg, mb))
					i += 2
					continue
				} else if colorType == 2 && i+4 < len(parts) {
					// True color mode: 38;2;R;G;B or 48;2;R;G;B
					r, _ := strconv.Atoi(parts[i+2])
					g, _ := strconv.Atoi(parts[i+3])
					b, _ := strconv.Atoi(parts[i+4])
					mr, mg, mb := MuteColor(r, g, b, saturation, brightness)
					result = append(result, fmt.Sprintf("%d;2;%d;%d;%d", code, mr, mg, mb))
					i += 4
					continue
				}
			}

			// Handle basic foreground colors (30-37, 90-97)
			if code >= 30 && code <= 37 {
				colorIndex := code - 30
				r, g, b := basic16Colors[colorIndex].r, basic16Colors[colorIndex].g, basic16Colors[colorIndex].b
				mr, mg, mb := MuteColor(r, g, b, saturation, brightness)
				result = append(result, fmt.Sprintf("38;2;%d;%d;%d", mr, mg, mb))
				continue
			}
			if code >= 90 && code <= 97 {
				colorIndex := code - 90 + 8 // Bright colors are 8-15
				r, g, b := basic16Colors[colorIndex].r, basic16Colors[colorIndex].g, basic16Colors[colorIndex].b
				mr, mg, mb := MuteColor(r, g, b, saturation, brightness)
				result = append(result, fmt.Sprintf("38;2;%d;%d;%d", mr, mg, mb))
				continue
			}

			// Handle basic background colors (40-47, 100-107)
			if code >= 40 && code <= 47 {
				colorIndex := code - 40
				r, g, b := basic16Colors[colorIndex].r, basic16Colors[colorIndex].g, basic16Colors[colorIndex].b
				mr, mg, mb := MuteColor(r, g, b, saturation, brightness)
				result = append(result, fmt.Sprintf("48;2;%d;%d;%d", mr, mg, mb))
				continue
			}
			if code >= 100 && code <= 107 {
				colorIndex := code - 100 + 8
				r, g, b := basic16Colors[colorIndex].r, basic16Colors[colorIndex].g, basic16Colors[colorIndex].b
				mr, mg, mb := MuteColor(r, g, b, saturation, brightness)
				result = append(result, fmt.Sprintf("48;2;%d;%d;%d", mr, mg, mb))
				continue
			}

			// Keep other codes as-is (bold, underline, reset, etc.)
			result = append(result, parts[i])
		}

		return "\x1b[" + strings.Join(result, ";") + "m"
	})
}

// color256ToRGB converts a 256-color palette index to RGB
func color256ToRGB(index int) (r, g, b int) {
	if index < 16 {
		// Standard colors (same as basic16Colors)
		return basic16Colors[index].r, basic16Colors[index].g, basic16Colors[index].b
	} else if index < 232 {
		// 216-color cube (6x6x6)
		index -= 16
		r = (index / 36) * 51
		g = ((index / 6) % 6) * 51
		b = (index % 6) * 51
		return r, g, b
	} else {
		// Grayscale (24 shades)
		gray := (index-232)*10 + 8
		return gray, gray, gray
	}
}

// SummarizePhase represents the animation phase during title generation
type SummarizePhase int

const (
	SummarizePhasePrompt SummarizePhase = iota // Showing prompt with spinner
	SummarizePhaseReveal                       // Title revealed, brief highlight
	SummarizePhaseFading                       // Title fading out over initialization
	SummarizePhaseDone                         // Animation complete
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
	initializing    bool           // True while waiting for Claude Code to be ready
	initStartTime   time.Time      // When initialization started (for timeout)
	spinnerFrame    int            // Current spinner animation frame
	initStatus      string         // Status message during initialization
	initStep        int            // Current initialization step (1-3)
	initSteps       int            // Total initialization steps
	scrollback      []string       // Scrollback buffer (lines that scrolled off top)
	scrollMode      bool           // True when viewing scrollback (not following live output)

	// Resize settling state - after resize, give terminal time to redraw before
	// auto-scrolling to bottom, to prevent rapid scroll oscillation
	resizeTime time.Time // When last resize happened

	// Fade animation state
	fading        bool      // True when fade animation is in progress
	fadeStartTime time.Time // When fade started
	fadeProgress  float64   // Progress from 0.0 to 1.0

	// Summarizing state (before container creation)
	summarizing        bool           // True while generating title
	summarizeStart     time.Time      // When summarization/fading started
	summarizePhase     SummarizePhase // Current animation phase
	summarizeTitle     string         // Generated title (set when ready)
	summarizeFadeEndAt time.Time      // When fading overlay should disappear
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

// fadeDuration is how long the fade animation takes
const fadeDuration = 400 * time.Millisecond

// SetInitializing sets the initializing state with a status message
func (p *PaneModel) SetInitializing(initializing bool) {
	wasInitializing := p.initializing
	p.initializing = initializing
	if initializing {
		p.initStartTime = time.Now()
		p.initSteps = 3
		if p.initStatus == "" {
			p.initStatus = "Starting..."
			p.initStep = 1
		}
		// Reset fade state when starting initialization
		p.fading = false
		p.fadeProgress = 0
	} else if wasInitializing {
		// Transition from initializing to ready - start fade animation
		p.fading = true
		p.fadeStartTime = time.Now()
		p.fadeProgress = 0
	}
}

// IsFading returns true if the pane is in fade animation
func (p *PaneModel) IsFading() bool {
	return p.fading
}

// TickFade advances the fade animation, returns true if still fading
func (p *PaneModel) TickFade() bool {
	if !p.fading {
		return false
	}
	elapsed := time.Since(p.fadeStartTime)
	p.fadeProgress = float64(elapsed) / float64(fadeDuration)
	if p.fadeProgress >= 1.0 {
		p.fadeProgress = 1.0
		p.fading = false
		return false
	}
	return true
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

// IsSummarizing returns true if the pane is generating a title
func (p *PaneModel) IsSummarizing() bool {
	return p.summarizing
}

// SetSummarizing starts the summarizing animation
func (p *PaneModel) SetSummarizing(summarizing bool) {
	p.summarizing = summarizing
	if summarizing {
		p.summarizeStart = time.Now()
		p.summarizePhase = SummarizePhasePrompt
		p.summarizeTitle = ""
	}
}

// SetSummarizeTitle sets the generated title
func (p *PaneModel) SetSummarizeTitle(title string) {
	p.summarizeTitle = title
}

// StartSummarizeFade starts the fading animation (called when container starts)
func (p *PaneModel) StartSummarizeFade() {
	p.summarizePhase = SummarizePhaseFading
	p.summarizeStart = time.Now()                          // Reset start time for fade progress
	p.summarizeFadeEndAt = time.Now().Add(4 * time.Second) // Fade out over 4 seconds
}

// SummarizeComplete marks summarization as done and returns the title
func (p *PaneModel) SummarizeComplete() string {
	p.summarizing = false
	p.summarizePhase = SummarizePhaseDone
	return p.summarizeTitle
}

// IsSummarizeFading returns true if in the fading phase
func (p *PaneModel) IsSummarizeFading() bool {
	return p.summarizePhase == SummarizePhaseFading
}

// SummarizeFadeProgress returns 0.0-1.0 progress through the fade (1.0 = fully faded)
func (p *PaneModel) SummarizeFadeProgress() float64 {
	if p.summarizePhase != SummarizePhaseFading {
		return 0
	}
	total := p.summarizeFadeEndAt.Sub(p.summarizeStart)
	elapsed := time.Since(p.summarizeStart)
	if elapsed >= total {
		return 1.0
	}
	return float64(elapsed) / float64(total)
}

// ShouldFinishFade returns true if the fading phase is complete
func (p *PaneModel) ShouldFinishFade() bool {
	return p.summarizePhase == SummarizePhaseFading && time.Now().After(p.summarizeFadeEndAt)
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
					// Kitty keyboard protocol: CSI 13;2u (13=Enter codepoint, 2=Shift modifier)
					data = []byte{27, '[', '1', '3', ';', '2', 'u'}
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
	// Color constants for animation
	const (
		colorPurple    = "#7C3AED" // Initializing state
		colorCyan      = "#0891B2" // Input mode
		colorGreen     = "#059669" // Nav mode
		colorDarkGrey  = "#444444" // Initializing content / unfocused border
		colorMidGrey   = "#555555" // Unfocused content
		colorLightGrey = "#888888" // Nav mode content
	)

	// Determine target colors based on state
	var targetHeaderBg string
	if p.focused {
		if p.inputMode {
			targetHeaderBg = colorCyan
		} else {
			targetHeaderBg = colorGreen
		}
	} else {
		targetHeaderBg = colorPurple // Unfocused stays purple
	}

	// Calculate current header background color (with fade animation)
	var headerBg lipgloss.Color
	if p.initializing {
		headerBg = lipgloss.Color(colorPurple)
	} else if p.fading {
		// Interpolate from purple to target color
		headerBg = LerpColor(colorPurple, targetHeaderBg, p.fadeProgress)
	} else {
		headerBg = lipgloss.Color(targetHeaderBg)
	}

	// Header with index, status, and branch name
	indexStyle := lipgloss.NewStyle().
		Background(headerBg).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1)

	// Mode indicator for focused pane
	var modeIndicator string
	if p.focused && !p.initializing {
		if p.scrollMode {
			// Scroll mode - orange background
			colorOrange := "#D97706"
			modeIndicator = lipgloss.NewStyle().
				Background(lipgloss.Color(colorOrange)).
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true).
				Padding(0, 1).
				Render("SCROLL")
		} else if p.inputMode {
			// Input mode - bright cyan background (also fades in)
			var modeBg lipgloss.Color
			if p.fading {
				modeBg = LerpColor(colorPurple, colorCyan, p.fadeProgress)
			} else {
				modeBg = lipgloss.Color(colorCyan)
			}
			modeIndicator = lipgloss.NewStyle().
				Background(modeBg).
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true).
				Padding(0, 1).
				Render("INPUT")
		} else {
			// Nav mode - green background (also fades in)
			var modeBg lipgloss.Color
			if p.fading {
				modeBg = LerpColor(colorPurple, colorGreen, p.fadeProgress)
			} else {
				modeBg = lipgloss.Color(colorGreen)
			}
			modeIndicator = lipgloss.NewStyle().
				Background(modeBg).
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
	// After a resize, give the terminal time to settle before auto-scrolling,
	// to prevent rapid scroll oscillation as the terminal redraws
	const resizeSettleTime = 500 * time.Millisecond
	isSettling := !p.resizeTime.IsZero() && time.Since(p.resizeTime) < resizeSettleTime

	if p.scrollMode {
		// In scroll mode - try to maintain position
		p.viewport.SetYOffset(yOffset)
	} else if isSettling {
		// During resize settling - maintain position to prevent oscillation
		p.viewport.SetYOffset(yOffset)
	} else if wasAtBottom {
		// Follow live output (was at bottom, stay at bottom)
		p.viewport.GotoBottom()
	}

	outputView := p.viewport.View()

	// Show summarizing animation (before container creation)
	if p.summarizing {
		viewportHeight := p.height - 4
		viewportWidth := p.width - 4
		if viewportHeight > 0 && viewportWidth > 0 {
			// Create empty background
			emptyLine := strings.Repeat(" ", viewportWidth)
			lines := make([]string, viewportHeight)
			for i := range lines {
				lines[i] = emptyLine
			}

			centerY := viewportHeight / 2

			switch p.summarizePhase {
			case SummarizePhasePrompt:
				// Show prompt with "Summarizing..." and spinner
				spinnerChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
				spinner := spinnerChars[p.spinnerFrame%len(spinnerChars)]

				// Spinner line
				spinnerStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("#A855F7")).
					Bold(true)
				labelStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("#888888"))
				spinnerText := spinnerStyle.Render(spinner) + " " + labelStyle.Render("Summarizing...")
				spinnerX := (viewportWidth - lipgloss.Width(spinnerText)) / 2
				if spinnerX < 0 {
					spinnerX = 0
				}
				if centerY-2 >= 0 && centerY-2 < len(lines) {
					lines[centerY-2] = strings.Repeat(" ", spinnerX) + spinnerText
				}

				// Prompt text (truncate if too long)
				prompt := p.workstream.Prompt
				if prompt == "" {
					prompt = "..."
				}
				// Wrap prompt to fit width with some margin
				maxPromptWidth := viewportWidth - 8
				if maxPromptWidth < 20 {
					maxPromptWidth = 20
				}
				promptStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("#CCCCCC")).
					Italic(true).
					Width(maxPromptWidth).
					Align(lipgloss.Center)
				promptRendered := promptStyle.Render(prompt)
				promptLines := strings.Split(promptRendered, "\n")

				// Render prompt lines centered
				startY := centerY
				for i, pLine := range promptLines {
					if startY+i >= 0 && startY+i < len(lines) {
						lineWidth := lipgloss.Width(pLine)
						promptX := (viewportWidth - lineWidth) / 2
						if promptX < 0 {
							promptX = 0
						}
						lines[startY+i] = strings.Repeat(" ", promptX) + pLine
					}
				}

			case SummarizePhaseReveal:
				// Fade the prompt, show the title with highlight
				titleStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("#22C55E")).
					Bold(true)
				title := p.summarizeTitle
				if title == "" {
					title = "Untitled"
				}

				// "Title generated" label
				labelStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("#666666"))
				labelText := labelStyle.Render("✓ Title generated")
				labelX := (viewportWidth - lipgloss.Width(labelText)) / 2
				if labelX < 0 {
					labelX = 0
				}
				if centerY-1 >= 0 && centerY-1 < len(lines) {
					lines[centerY-1] = strings.Repeat(" ", labelX) + labelText
				}

				// Title with highlight
				titleText := titleStyle.Render(title)
				titleX := (viewportWidth - lipgloss.Width(titleText)) / 2
				if titleX < 0 {
					titleX = 0
				}
				if centerY+1 >= 0 && centerY+1 < len(lines) {
					lines[centerY+1] = strings.Repeat(" ", titleX) + titleText
				}

				// Faded prompt below
				prompt := p.workstream.Prompt
				if prompt != "" {
					maxPromptWidth := viewportWidth - 8
					if maxPromptWidth < 20 {
						maxPromptWidth = 20
					}
					promptStyle := lipgloss.NewStyle().
						Foreground(lipgloss.Color("#444444")). // Very faded
						Italic(true).
						Width(maxPromptWidth).
						Align(lipgloss.Center)
					promptRendered := promptStyle.Render(prompt)
					promptLines := strings.Split(promptRendered, "\n")

					startY := centerY + 3
					for i, pLine := range promptLines {
						if startY+i >= 0 && startY+i < len(lines) && i < 2 { // Only show first 2 lines
							lineWidth := lipgloss.Width(pLine)
							promptX := (viewportWidth - lineWidth) / 2
							if promptX < 0 {
								promptX = 0
							}
							lines[startY+i] = strings.Repeat(" ", promptX) + pLine
						}
					}
				}
			}

			outputView = strings.Join(lines, "\n")
		}
	} else if p.initializing {
		// Grey out the output
		outputView = stripANSI(outputView)
		greyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorDarkGrey))
		outputView = greyStyle.Render(outputView)

		// Create spinner overlay
		spinnerChars := []string{"⠋", "⠙", "⠹", "⠸"}
		spinner := spinnerChars[p.spinnerFrame%len(spinnerChars)]
		spinnerStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorCyan)).
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
	} else if p.fading {
		// During fade animation - interpolate content brightness
		// Determine target grey level based on focus/mode
		var targetGrey string
		if p.focused && p.inputMode {
			// Fading to full color - use a brighter grey that fades toward white
			targetGrey = "#CCCCCC"
		} else if p.focused && !p.inputMode {
			targetGrey = colorLightGrey
		} else {
			targetGrey = colorMidGrey
		}
		// Strip ANSI during fade for consistent color application
		outputView = stripANSI(outputView)
		fadeColor := LerpColor(colorDarkGrey, targetGrey, p.fadeProgress)
		greyStyle := lipgloss.NewStyle().Foreground(fadeColor)
		outputView = greyStyle.Render(outputView)
	} else if p.focused && !p.inputMode {
		// Focused but nav mode: lighter grey
		outputView = stripANSI(outputView)
		greyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorLightGrey))
		outputView = greyStyle.Render(outputView)
	} else if !p.focused {
		// Unfocused panes: muted colors (very desaturated, dimmed)
		outputView = muteANSI(outputView, 0.25, 0.6)
	}

	// Overlay fading title during initialization (after container starts)
	if p.IsSummarizeFading() && p.summarizeTitle != "" {
		viewportHeight := p.height - 4
		viewportWidth := p.width - 4
		if viewportHeight > 0 && viewportWidth > 0 {
			lines := strings.Split(outputView, "\n")

			// Calculate fade: start at full green, fade to grey, then invisible
			progress := p.SummarizeFadeProgress()
			var titleColor string
			if progress < 0.5 {
				// First half: bright green
				titleColor = "#22C55E"
			} else if progress < 0.75 {
				// Middle: fade to grey
				titleColor = "#666666"
			} else {
				// Last quarter: very dim before disappearing
				titleColor = "#444444"
			}

			titleStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color(titleColor)).
				Bold(progress < 0.5) // Only bold in first half

			title := p.summarizeTitle
			titleText := titleStyle.Render(title)

			// Position vertically centered
			titleY := viewportHeight / 2
			titleX := (viewportWidth - lipgloss.Width(titleText)) / 2
			if titleX < 0 {
				titleX = 0
			}

			// Ensure we have enough lines
			for len(lines) <= titleY {
				lines = append(lines, strings.Repeat(" ", viewportWidth))
			}

			if titleY >= 0 && titleY < len(lines) {
				// Overlay title on the line
				lines[titleY] = strings.Repeat(" ", titleX) + titleText
			}

			outputView = strings.Join(lines, "\n")
		}
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
		var borderColor lipgloss.Color
		var targetBorderColor string
		if p.inputMode {
			// Input mode - bright cyan border
			targetBorderColor = colorCyan
		} else {
			// Nav mode - green border to match header highlight
			targetBorderColor = colorGreen
		}
		// Fade border color during animation
		if p.fading {
			borderColor = LerpColor(colorDarkGrey, targetBorderColor, p.fadeProgress)
		} else if p.initializing {
			borderColor = lipgloss.Color(colorDarkGrey)
		} else {
			borderColor = lipgloss.Color(targetBorderColor)
		}
		style = lipgloss.NewStyle().
			Border(lipgloss.ThickBorder()).
			BorderForeground(borderColor).
			Width(p.width - 2).
			Height(p.height - 2)
	} else {
		style = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(colorDarkGrey)).
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
	// Track if size is actually changing (to trigger resize settling)
	sizeChanged := p.width != width || p.height != height

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
		// Mark resize time so View() can wait for terminal to settle
		// before auto-scrolling (prevents rapid scroll oscillation)
		if sizeChanged {
			p.resizeTime = time.Now()
		}
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

// SendToPTY sends text to the PTY session as if the user typed it.
// This is used to notify Claude Code about external events like PR creation.
func (p *PaneModel) SendToPTY(text string) error {
	if p.pty == nil || p.pty.IsClosed() {
		return fmt.Errorf("PTY session not available")
	}
	return p.pty.WriteString(text)
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
	p.viewport.PageUp()
}

// ScrollPageDown scrolls the viewport down by one page
func (p *PaneModel) ScrollPageDown() {
	p.viewport.PageDown()
	// Exit scroll mode if at bottom
	if p.viewport.AtBottom() {
		p.scrollMode = false
	}
}

// ScrollLineUp scrolls the viewport up by one line
func (p *PaneModel) ScrollLineUp() {
	p.scrollMode = true
	p.viewport.ScrollUp(1)
}

// ScrollLineDown scrolls the viewport down by one line
func (p *PaneModel) ScrollLineDown() {
	p.viewport.ScrollDown(1)
	// Exit scroll mode if at bottom
	if p.viewport.AtBottom() {
		p.scrollMode = false
	}
}

// EnterScrollMode enters scroll mode without changing position
func (p *PaneModel) EnterScrollMode() {
	p.scrollMode = true
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

// ExportLogs exports the pane's full terminal history to a file.
// Returns the path to the exported file, or an error.
func (p *PaneModel) ExportLogs(logsDir string) (string, error) {
	// Build full content: scrollback + current vterm
	var content strings.Builder

	// Add header with metadata
	content.WriteString("# CCells Log Export\n")
	content.WriteString(fmt.Sprintf("# Branch: %s\n", p.workstream.BranchName))
	content.WriteString(fmt.Sprintf("# Title: %s\n", p.workstream.GetTitle()))
	content.WriteString(fmt.Sprintf("# Exported: %s\n", time.Now().Format(time.RFC3339)))
	content.WriteString(fmt.Sprintf("# Prompt: %s\n", p.workstream.Prompt))
	content.WriteString("\n" + strings.Repeat("=", 60) + "\n\n")

	// Add scrollback buffer
	if len(p.scrollback) > 0 {
		for _, line := range p.scrollback {
			// Strip ANSI codes for clean output
			content.WriteString(stripANSI(line))
			content.WriteString("\n")
		}
	}

	// Add current vterm content
	vtermContent := p.renderVTerm()
	if vtermContent != "" {
		content.WriteString(stripANSI(vtermContent))
	}

	// Generate filename: <branch>-<timestamp>.txt
	safeBranch := strings.ReplaceAll(p.workstream.BranchName, "/", "-")
	safeBranch = strings.ReplaceAll(safeBranch, " ", "-")
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s-%s.txt", safeBranch, timestamp)
	filepath := fmt.Sprintf("%s/%s", logsDir, filename)

	// Write to file
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create logs directory: %w", err)
	}

	if err := os.WriteFile(filepath, []byte(content.String()), 0644); err != nil {
		return "", fmt.Errorf("failed to write log file: %w", err)
	}

	return filepath, nil
}

// PromptMsg is sent when user submits a prompt
type PromptMsg struct {
	WorkstreamID string
	Prompt       string
}
