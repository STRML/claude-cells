package tui

import (
	"fmt"
	"image/color"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/STRML/claude-cells/internal/git"
	"github.com/STRML/claude-cells/internal/sync"
	"github.com/STRML/claude-cells/internal/workstream"
	"github.com/hinshun/vt10x"
)

// initTimeout is how long to wait for initialization before showing a warning
// 5 minutes allows time for image builds and Claude Code installation
const initTimeout = 5 * time.Minute

// resizeSettleTime is how long to wait after resize before auto-scrolling.
// This prevents scroll oscillation as the terminal redraws.
const resizeSettleTime = 500 * time.Millisecond

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

	// In-pane dialog (e.g., merge dialog shown inside the pane)
	inPaneDialog *DialogModel

	// PR status for merge dialog enhancement
	prStatus        *git.PRStatusInfo
	prStatusLoading bool

	// Synopsis display
	synopsisHidden bool // True to hide synopsis in header (app-level toggle)

	// Pairing state (set by app from pairingOrchestrator)
	pairingState *sync.PairingState
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

	vp := viewport.New()
	vp.FillHeight = true // Ensure consistent height regardless of content

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
		case tea.KeyPressMsg:
			// If we have a PTY session, send keys directly to it
			if p.pty != nil && !p.pty.IsClosed() {
				// Send the key to the PTY
				keyStr := msg.String()
				var data []byte
				switch keyStr {
				case "enter":
					// Kitty keyboard protocol: CSI 13 u (13=Enter/CR codepoint, no modifiers)
					// This is the standard format for Enter in terminals that support Kitty protocol.
					// Using this instead of \r ensures Enter works in Claude Code (bubbletea app)
					// which enables Kitty keyboard protocol for enhanced key handling.
					data = []byte{27, '[', '1', '3', 'u'}
				case "shift+enter", "ctrl+j":
					// Kitty keyboard protocol: CSI 13;2u (13=Enter codepoint, 2=Shift modifier)
					// ctrl+j is the legacy escape sequence some terminals send for shift+enter
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
					} else if msg.Text != "" {
						// Printable characters (replaces KeyRunes in v2)
						data = []byte(msg.Text)
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
	// If there's an in-pane dialog, render it instead of normal content
	if p.inPaneDialog != nil {
		return p.viewWithInPaneDialog()
	}

	// Color constants for animation (use centralized mode colors from styles.go)
	const (
		colorPurple    = ModeInitBadge   // Initializing state
		colorCyan      = ModeInputBadge  // Input mode
		colorGreen     = ModeNavBadge    // Nav mode
		colorOrange    = ModeScrollBadge // Scroll mode
		colorDarkGrey  = "#444444"       // Initializing content / unfocused border
		colorMidGrey   = "#555555"       // Unfocused content
		colorLightGrey = "#888888"       // Nav mode content
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
	var headerBg color.Color
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
			// Scroll mode - orange background (colorOrange defined above)
			modeIndicator = lipgloss.NewStyle().
				Background(lipgloss.Color(colorOrange)).
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true).
				Padding(0, 1).
				Render("SCROLL")
		} else if p.inputMode {
			// Input mode - bright cyan background (also fades in)
			var modeBg color.Color
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
			var modeBg color.Color
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

	// Build left side of header
	var headerLeft string
	if p.focused && modeIndicator != "" {
		headerLeft = fmt.Sprintf("%s %s %s %s %s", indexLabel, modeIndicator, status, title, stateLabel)
	} else {
		headerLeft = fmt.Sprintf("%s %s %s %s", indexLabel, status, title, stateLabel)
	}

	// Pairing status badges (shown after state label when this pane is being paired)
	if p.pairingState != nil && p.pairingState.Active {
		// Pairing mode label
		pairingLabelStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorPairingSynced)).
			Bold(true)
		headerLeft += " " + pairingLabelStyle.Render("(pairing)")

		// Sync status badge (uses helper from styles.go)
		if syncBadge := RenderSyncBadge(p.pairingState.SyncStatus, len(p.pairingState.Conflicts)); syncBadge != "" {
			headerLeft += " " + syncBadge
		}

		// Stash indicator
		if p.pairingState.StashedChanges {
			headerLeft += " " + RenderStashBadge()
		}
	}

	// PR status badge (shown at top right when PR exists)
	var prBadge string
	if p.workstream.PRURL != "" && p.prStatus != nil {
		var parts []string

		// Check status indicator with icon and summary (e.g., "✓ 3/3")
		if p.prStatus.CheckStatus != git.PRCheckStatusUnknown {
			var checkIcon string
			var checkStyle lipgloss.Style
			switch p.prStatus.CheckStatus {
			case git.PRCheckStatusSuccess:
				checkStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#00FF88")).
					Bold(true)
				checkIcon = "✓"
			case git.PRCheckStatusPending:
				checkStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#FFD700")).
					Bold(true)
				checkIcon = "⏳"
			case git.PRCheckStatusFailure:
				checkStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#FF4466")).
					Bold(true)
				checkIcon = "✗"
			}
			// Include summary like "3/3" if available
			if p.prStatus.ChecksSummary != "" {
				parts = append(parts, checkStyle.Render(checkIcon+" "+p.prStatus.ChecksSummary))
			} else {
				parts = append(parts, checkStyle.Render(checkIcon))
			}
		}

		// Unpushed commits indicator - bright magenta
		if p.prStatus.UnpushedCount > 0 {
			unpushedStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF66FF")). // Bright magenta
				Bold(true)
			parts = append(parts, unpushedStyle.Render(fmt.Sprintf("↑%d", p.prStatus.UnpushedCount)))
		}

		// Divergence warning - bright orange/red
		if p.prStatus.IsDiverged {
			divergedStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF6B35")). // Bright orange
				Bold(true)
			parts = append(parts, divergedStyle.Render(fmt.Sprintf("⚠↓%d", p.prStatus.DivergedCount)))
		}

		if len(parts) > 0 {
			// Create a compact badge with PR number
			prNumStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00DDFF")). // Bright cyan
				Bold(true)
			prBadge = prNumStyle.Render(fmt.Sprintf("PR #%d", p.prStatus.Number)) + " " + strings.Join(parts, " ")
		}
	}

	// Combine header with PR badge on the right
	var header string
	if prBadge != "" {
		// Calculate available width for spacing (account for border and padding)
		availableWidth := p.width - 4 // 2 for border, 2 for padding
		leftWidth := lipgloss.Width(headerLeft)
		rightWidth := lipgloss.Width(prBadge)
		spacing := availableWidth - leftWidth - rightWidth
		if spacing < 1 {
			spacing = 1
		}
		header = headerLeft + strings.Repeat(" ", spacing) + prBadge
	} else {
		header = headerLeft
	}

	// Add synopsis line below header if height > 80 and synopsis exists
	if p.height > 80 && !p.synopsisHidden {
		synopsis := p.workstream.GetSynopsis()
		if synopsis != "" {
			// Style the synopsis line in dim grey
			synopsisStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#666666")).
				Italic(true)
			// Truncate synopsis if too wide
			maxWidth := p.width - 4
			if len(synopsis) > maxWidth {
				synopsis = synopsis[:maxWidth-3] + "..."
			}
			header = header + "\n" + synopsisStyle.Render("  "+synopsis)
		}
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
	yOffset := p.viewport.YOffset()

	p.viewport.SetContent(outputContent)

	// Restore scroll position or go to bottom
	// After a resize, give the terminal time to settle before auto-scrolling,
	// to prevent rapid scroll oscillation as the terminal redraws
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
		viewportHeight := p.height - 6 // Must match viewport.SetHeight in SetSize()
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
		viewportHeight := p.height - 6 // Must match viewport.SetHeight in SetSize()
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
		// Focused but nav mode: muted colors but less aggressive than unfocused
		// colorLightGrey (#888888) = RGB(136, 136, 136)
		outputView = muteANSI(outputView, 0.5, 0.85, [3]int{136, 136, 136})
		mutedNavStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorLightGrey))
		outputView = mutedNavStyle.Render(outputView)
	} else if !p.focused {
		// Unfocused panes: muted colors (very desaturated, dimmed)
		// Mute explicit ANSI colors and replace reset/default sequences with muted grey
		// colorMidGrey (#555555) = RGB(85, 85, 85)
		outputView = muteANSI(outputView, 0.25, 0.6, [3]int{85, 85, 85})
		// Apply muted default foreground for text without any ANSI codes at all
		mutedDefaultStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorMidGrey))
		outputView = mutedDefaultStyle.Render(outputView)
	}

	// Overlay fading title during initialization (after container starts)
	if p.IsSummarizeFading() && p.summarizeTitle != "" {
		viewportHeight := p.height - 6 // Must match viewport.SetHeight in SetSize()
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

	// Scroll hints (only show when focused and there's scrollback content)
	var scrollHint string
	if p.focused && len(p.scrollback) > 0 {
		hintStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			Italic(true)

		if p.scrollMode {
			// In scroll mode - show navigation hints
			scrollHint = hintStyle.Render("SCROLL: ↑↓/pgup/pgdn  |  ctrl+u/d half-page  |  esc exit")
		} else if !p.viewport.AtBottom() {
			// Not at bottom - hint that we're scrolled up
			scrollHint = hintStyle.Render("↑ scrolled up — esc to return")
		}
	}

	// Combine
	var content string
	if scrollHint != "" {
		content = header + "\n\n" + outputView + "\n" + scrollHint + inputView
	} else {
		content = header + "\n\n" + outputView + inputView
	}

	// Apply border based on focus and input mode
	var style lipgloss.Style
	if p.focused {
		var borderColor color.Color
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
			Padding(0, 1). // 1 char padding on left and right to match vterm width
			Width(p.width).
			Height(p.height)
	} else {
		style = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(colorDarkGrey)).
			Padding(0, 1). // 1 char padding on left and right to match vterm width
			Width(p.width).
			Height(p.height)
	}

	return style.Render(content)
}

// SetInputMode sets whether the pane is in input mode
func (p *PaneModel) SetInputMode(inputMode bool) {
	p.inputMode = inputMode
}

// viewWithInPaneDialog renders the pane with the in-pane dialog filling the content area
func (p PaneModel) viewWithInPaneDialog() string {
	// Use centralized dialog mode colors from styles.go
	// Choose colors based on focus state
	var borderColor, headerBgColor string
	var borderStyle lipgloss.Border
	if p.focused {
		borderColor = ModeDialogBadge
		headerBgColor = ModeDialogBadge
		borderStyle = lipgloss.ThickBorder()
	} else {
		borderColor = ModeDialogDimmed
		headerBgColor = ModeDialogDimmed
		borderStyle = lipgloss.NormalBorder()
	}

	// Header with index, status, and branch name (same as normal view)
	headerBg := lipgloss.Color(headerBgColor)
	indexStyle := lipgloss.NewStyle().
		Background(headerBg).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1)

	indexLabel := indexStyle.Render(fmt.Sprintf("%d", p.index))
	status := StatusStyle(string(p.workstream.GetState()))
	title := PaneTitle.Render(p.workstream.GetTitle())
	stateLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(fmt.Sprintf("(%s)", p.workstream.GetState()))

	// Mode indicator for dialog
	modeIndicator := lipgloss.NewStyle().
		Background(lipgloss.Color(headerBgColor)).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1).
		Render("DIALOG")

	header := fmt.Sprintf("%s %s %s %s %s", indexLabel, modeIndicator, status, title, stateLabel)

	// Render the dialog content to fill the pane
	dialogContent := p.inPaneDialog.ViewInPane()

	// Combine header and dialog content
	content := header + "\n\n" + dialogContent

	// Apply border - bright purple when focused, dimmed purple when unfocused
	style := lipgloss.NewStyle().
		Border(borderStyle).
		BorderForeground(lipgloss.Color(borderColor)).
		Padding(0, 1). // 1 char padding on left and right for consistency
		Width(p.width).
		Height(p.height)

	return style.Render(content)
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

	// NOTE: We don't try to render our own cursor at a calculated position.
	// Claude Code (the inner app) renders its cursor as styled text (virtual cursor),
	// so we just need to preserve that styling.

	var lines []string
	for row := 0; row < rows; row++ {
		var line strings.Builder
		var lastFG, lastBG vt10x.Color = vt10x.DefaultFG, vt10x.DefaultBG
		var lastInverse bool

		for col := 0; col < cols; col++ {
			cell := p.vterm.Cell(col, row)

			// Detect inverse video via Mode flag (bit 0)
			// This works for both plain inverse and inverse+color combinations
			isInverse := cell.Mode&1 != 0

			// Check if colors or inverse state changed
			if cell.FG != lastFG || cell.BG != lastBG || isInverse != lastInverse {
				// Reset and apply new colors
				line.WriteString("\x1b[0m") // Reset

				if isInverse {
					// Output inverse video attribute
					line.WriteString("\x1b[7m")
				}
				// Output explicit colors
				// Skip default sentinels (DefaultFG, DefaultBG) which vt10x uses as placeholders
				if cell.FG != vt10x.DefaultFG && cell.FG != vt10x.DefaultBG {
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
				if cell.BG != vt10x.DefaultBG && cell.BG != vt10x.DefaultFG {
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
				lastInverse = isInverse
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

// SetSize sets the pane dimensions.
// This sends refresh signals (Ctrl+L, Ctrl+O) to the PTY when size changes.
// For layout-induced resizes where refresh signals are not wanted, use SetSizeQuiet.
func (p *PaneModel) SetSize(width, height int) {
	p.setSizeInternal(width, height, true)
}

// SetSizeQuiet sets the pane dimensions without sending refresh signals to the PTY.
// Use this for layout-induced resizes (pane count changes) to avoid sending
// Ctrl+L/Ctrl+O which can interfere with Claude Code's input handling.
func (p *PaneModel) SetSizeQuiet(width, height int) {
	p.setSizeInternal(width, height, false)
}

// setSizeInternal is the internal implementation of SetSize/SetSizeQuiet.
// If sendRefresh is true, sends Ctrl+L and Ctrl+O to the PTY to trigger redraw.
func (p *PaneModel) setSizeInternal(width, height int, sendRefresh bool) {
	// Track if size is actually changing (to trigger resize settling)
	sizeChanged := p.width != width || p.height != height

	// Track previous vterm dimensions
	oldVtermWidth, oldVtermHeight := p.vterm.Size()

	p.width = width
	p.height = height
	p.viewport.SetWidth(width - 4)   // Account for border and padding
	p.viewport.SetHeight(height - 6) // Account for header, input, borders

	// Calculate inner dimensions
	innerWidth := width - 4
	innerHeight := height - 6
	if innerWidth < 40 {
		innerWidth = 40
	}
	if innerHeight < 10 {
		innerHeight = 10
	}

	// Check if vterm size is actually changing
	vtermSizeChanged := innerWidth != oldVtermWidth || innerHeight != oldVtermHeight

	// Resize PTY FIRST if active - this sends SIGWINCH to the process so it knows
	// about the new size before we resize the vterm. This prevents the race condition
	// where output generated for the old size is written to the new-sized vterm.
	if p.pty != nil && !p.pty.IsClosed() && vtermSizeChanged {
		_ = p.pty.Resize(innerWidth, innerHeight)
		// Mark resize time so View() can wait for terminal to settle
		// before auto-scrolling (prevents rapid scroll oscillation)
		if sizeChanged {
			p.resizeTime = time.Now()
		}

		// Only send refresh signals if requested (i.e., for actual window resizes)
		// Layout-induced resizes (pane count changes) should not send Ctrl+L as
		// it can cause visual disruption.
		if sendRefresh {
			// Send Ctrl+L on resize to trigger a full redraw.
			// This ensures the process redraws for the new size, preventing corruption
			// from output generated for the old size.
			// Note: Previously we also sent Ctrl+O twice here, but that was causing
			// newlines to be inserted in bash/readline and Claude Code's input area.
			// SIGWINCH from PTY resize + Ctrl+L is sufficient for proper redraw.
			_ = p.pty.Write([]byte{12}) // Ctrl+L (form feed) - triggers screen redraw
		}
	}

	// Now resize the vterm. Creating a fresh vterm instead of resizing in-place
	// avoids corruption from content that was generated for the old size.
	// The Ctrl+L we sent above will cause the process to redraw everything.
	if vtermSizeChanged {
		// Create a fresh vterm at the new size
		p.vterm = vt10x.New(vt10x.WithSize(innerWidth, innerHeight))
		// Clear the cached render since content is now invalid
		p.lastVtermRender = ""
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

// SendInput sends text to the PTY session as if the user typed it.
// If pressEnter is true, it also sends an Enter key press using the Kitty keyboard protocol
// to ensure it works in Claude Code (bubbletea app) which enables Kitty protocol.
// This is used to notify Claude Code about external events like PR creation.
func (p *PaneModel) SendInput(text string, pressEnter bool) error {
	if p.pty == nil || p.pty.IsClosed() {
		return fmt.Errorf("PTY session not available")
	}
	if err := p.pty.WriteString(text); err != nil {
		return err
	}
	if pressEnter {
		return p.pty.Write(KittyEnterKey)
	}
	return nil
}

// SendToPTY sends text to the PTY session without pressing Enter.
// Deprecated: Use SendInput(text, false) instead.
func (p *PaneModel) SendToPTY(text string) error {
	return p.SendInput(text, false)
}

// SendToPTYWithEnter sends text to the PTY followed by an Enter key press.
// Deprecated: Use SendInput(text, true) instead.
func (p *PaneModel) SendToPTYWithEnter(text string) error {
	return p.SendInput(text, true)
}

// WritePTYOutput writes raw PTY output to the virtual terminal
func (p *PaneModel) WritePTYOutput(data []byte) {
	// Capture current first line before write (to detect scrolling)
	// Use plain text for comparison, but we'll save the colored version
	firstLineBefore := p.getVtermLine(0)
	// Render with colors before the write changes the vterm
	coloredFirstLine := p.renderVTermLine(0)

	p.vterm.Write(data)

	// Check if content scrolled (first line changed)
	firstLineAfter := p.getVtermLine(0)
	if firstLineBefore != "" && firstLineBefore != firstLineAfter {
		// Content scrolled - save old first line to scrollback WITH colors
		// Limit scrollback to 10000 lines to prevent memory issues
		if len(p.scrollback) >= 10000 {
			// Remove oldest 1000 lines when limit reached
			p.scrollback = p.scrollback[1000:]
		}
		p.scrollback = append(p.scrollback, coloredFirstLine)
	}
}

// getVtermLine returns a single line from the vterm (0-indexed) as plain text
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

// renderVTermLine renders a single line from vterm with ANSI color codes preserved
// This is used for scrollback so that colors are maintained when scrolling up
func (p *PaneModel) renderVTermLine(row int) string {
	if p.vterm == nil {
		return ""
	}
	cols, rows := p.vterm.Size()
	if row < 0 || row >= rows || cols <= 0 {
		return ""
	}

	var line strings.Builder
	var lastFG, lastBG vt10x.Color = vt10x.DefaultFG, vt10x.DefaultBG
	var lastInverse bool

	for col := 0; col < cols; col++ {
		cell := p.vterm.Cell(col, row)

		// Detect inverse video via Mode flag (bit 0)
		isInverse := cell.Mode&1 != 0

		// Check if colors or inverse state changed
		if cell.FG != lastFG || cell.BG != lastBG || isInverse != lastInverse {
			// Reset and apply new colors
			line.WriteString("\x1b[0m") // Reset

			if isInverse {
				// Output inverse video attribute
				line.WriteString("\x1b[7m")
			}
			// Output explicit colors
			// Skip default sentinels (DefaultFG, DefaultBG) which vt10x uses as placeholders
			if cell.FG != vt10x.DefaultFG && cell.FG != vt10x.DefaultBG {
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
			if cell.BG != vt10x.DefaultBG && cell.BG != vt10x.DefaultFG {
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
			lastInverse = isInverse
		}

		if cell.Char == 0 {
			line.WriteRune(' ')
		} else {
			line.WriteRune(cell.Char)
		}
	}
	// Reset colors at end of line
	line.WriteString("\x1b[0m")

	// Trim trailing spaces but keep color codes
	lineStr := line.String()
	if strings.HasSuffix(lineStr, "\x1b[0m") {
		prefix := lineStr[:len(lineStr)-4]
		prefix = strings.TrimRight(prefix, " ")
		lineStr = prefix + "\x1b[0m"
	}
	return lineStr
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

// SetInPaneDialog sets the dialog to show inside the pane
func (p *PaneModel) SetInPaneDialog(dialog *DialogModel) {
	p.inPaneDialog = dialog
	if dialog != nil {
		// Size the dialog to fill the pane's content area
		// Account for border (2) and padding (2) on each side = 4 total horizontal
		// And header (2 lines), border (2), padding (2) vertical
		dialogWidth := p.width - 4
		dialogHeight := p.height - 6
		if dialogWidth < 30 {
			dialogWidth = 30
		}
		if dialogHeight < 10 {
			dialogHeight = 10
		}
		dialog.SetSize(dialogWidth, dialogHeight)
	}
}

// GetInPaneDialog returns the current in-pane dialog, if any
func (p *PaneModel) GetInPaneDialog() *DialogModel {
	return p.inPaneDialog
}

// ClearInPaneDialog removes the in-pane dialog
func (p *PaneModel) ClearInPaneDialog() {
	p.inPaneDialog = nil
}

// HasInPaneDialog returns true if the pane has an active in-pane dialog
func (p *PaneModel) HasInPaneDialog() bool {
	return p.inPaneDialog != nil
}

// GetPRStatus returns the current PR status, if any
func (p *PaneModel) GetPRStatus() *git.PRStatusInfo {
	return p.prStatus
}

// SetPRStatus sets the PR status
func (p *PaneModel) SetPRStatus(status *git.PRStatusInfo) {
	p.prStatus = status
	p.prStatusLoading = false
}

// SetPRStatusLoading sets whether PR status is currently being loaded
func (p *PaneModel) SetPRStatusLoading(loading bool) {
	p.prStatusLoading = loading
}

// IsPRStatusLoading returns true if PR status is being loaded
func (p *PaneModel) IsPRStatusLoading() bool {
	return p.prStatusLoading
}

// SetSynopsisHidden sets whether the synopsis should be hidden in the header
func (p *PaneModel) SetSynopsisHidden(hidden bool) {
	p.synopsisHidden = hidden
}

// SetPairingState sets the pairing state for this pane.
// Pass nil to clear pairing status (pane is not being paired).
// Makes a defensive copy to avoid holding a pointer to caller's stack variable.
func (p *PaneModel) SetPairingState(state *sync.PairingState) {
	if state != nil {
		copied := *state
		p.pairingState = &copied
	} else {
		p.pairingState = nil
	}
}

// GetPairingState returns the current pairing state, if any.
func (p *PaneModel) GetPairingState() *sync.PairingState {
	return p.pairingState
}

// renderPRFooter renders a compact PR status footer line.
// Returns a string like "PR #123: ✓ 3/3 | ↑2 unpushed" or "PR #123: ⏳ 2/3 | ⚠ diverged"
func (p *PaneModel) renderPRFooter() string {
	if p.prStatus == nil {
		return ""
	}

	// Style for the footer
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888"))

	// Build the footer parts
	var parts []string

	// PR number
	parts = append(parts, fmt.Sprintf("PR #%d:", p.prStatus.Number))

	// Check status with icon
	var checkIcon string
	switch p.prStatus.CheckStatus {
	case git.PRCheckStatusSuccess:
		checkIcon = "✓"
	case git.PRCheckStatusPending:
		checkIcon = "⏳"
	case git.PRCheckStatusFailure:
		checkIcon = "✗"
	default:
		checkIcon = "?"
	}
	parts = append(parts, fmt.Sprintf("%s %s", checkIcon, p.prStatus.ChecksSummary))

	// Unpushed commits
	if p.prStatus.UnpushedCount > 0 {
		parts = append(parts, fmt.Sprintf("↑%d unpushed", p.prStatus.UnpushedCount))
	}

	// Divergence warning
	if p.prStatus.IsDiverged {
		parts = append(parts, "⚠ diverged")
	}

	return footerStyle.Render(strings.Join(parts, " | "))
}

// IsClaudeWorking returns true if Claude appears to be actively working.
// This is detected by looking for the "(ctrl+c to interrupt)" message in the terminal output,
// which Claude Code shows when it's processing a task.
func (p *PaneModel) IsClaudeWorking() bool {
	if p.vterm == nil {
		return false
	}

	cols, rows := p.vterm.Size()
	if rows <= 0 || cols <= 0 {
		return false
	}

	// Check all rows for the working indicator (the message could be anywhere on screen)
	for row := 0; row < rows; row++ {
		line := p.getVtermLine(row)
		// Empty lines indicate we've passed the content area
		if line == "" {
			continue
		}
		// Check for the interrupt message that Claude Code shows when working
		// The "·" character makes this specific to Claude's status line (user won't type it)
		if strings.Contains(line, "ctrl+c to interrupt ·") ||
			strings.Contains(line, "ctrl-c to interrupt ·") {
			return true
		}
	}

	return false
}

// NOTE: CursorPosition type and GetCursorPosition() were removed.
// Claude Code uses virtual cursor (renders cursor as styled text), not hardware cursor.
// The vterm.Cursor() position doesn't reflect where Claude Code's visual cursor is,
// so attempting to position a hardware cursor based on it caused misplacement.
// We now rely on Claude Code's own cursor rendering (inverse video in its output).

// PromptMsg is sent when user submits a prompt
type PromptMsg struct {
	WorkstreamID string
	Prompt       string
}
