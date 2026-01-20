package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/STRML/claude-cells/internal/docker"
	"github.com/STRML/claude-cells/internal/workstream"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const escapeTimeout = 300 * time.Millisecond

const toastDuration = 2 * time.Second

// Version info - set by main via SetVersionInfo
var (
	versionInfo = "dev"
	commitHash  = "unknown"
)

// SetVersionInfo sets the version info displayed in the help dialog
func SetVersionInfo(version, commit string) {
	versionInfo = version
	commitHash = commit
}

// AppModel is the main application model
type AppModel struct {
	ctx            context.Context // App-level context for cancellation
	manager        *workstream.Manager
	panes          []PaneModel
	focusedPane    int
	nextPaneIndex  int        // Counter for assigning permanent pane indices
	layout         LayoutType // Current pane layout
	statusBar      StatusBarModel
	dialog         *DialogModel
	width          int
	height         int
	quitting       bool
	inputMode      bool      // True when input is being routed to focused pane
	lastEscapeTime time.Time // For double-escape detection
	pendingEscape  bool      // True when first Esc pressed, waiting to see if double-tap
	toast          string    // Temporary notification message
	toastExpiry    time.Time // When toast should disappear
	workingDir     string    // Current working directory for state file
	resuming       bool      // True if resuming from saved state
	tmuxPrefix     bool      // True after ctrl-b is pressed (tmux-style prefix)
	tmuxPrefixTime time.Time // When prefix was pressed
	// Pairing mode state
	pairingWorkstreamID string // ID of workstream in pairing mode (empty if none)
	pairingPrevBranch   string // Branch to restore when pairing ends
	pairingStashed      bool   // True if we stashed changes when enabling pairing
	// Pane swap state
	lastSwapPosition int // Position to swap back to when pressing Space at main (0 = none)
}

const tmuxPrefixTimeout = 2 * time.Second

// NewAppModel creates a new application model
func NewAppModel(ctx context.Context) AppModel {
	cwd, _ := os.Getwd()
	return AppModel{
		ctx:           ctx,
		manager:       workstream.NewManager(),
		statusBar:     NewStatusBarModel(),
		workingDir:    cwd,
		nextPaneIndex: 1, // Start pane numbering at 1
	}
}

// projectName returns the project name derived from the working directory
func (m *AppModel) projectName() string {
	name := filepath.Base(m.workingDir)
	if name == "" || name == "." {
		return "workspace"
	}
	return name
}

// StateLoadedMsg is sent when state has been loaded from disk
type StateLoadedMsg struct {
	State *workstream.AppState
	Error error
}

// StateSavedMsg is sent when state has been saved
type StateSavedMsg struct {
	Error error
}

// spinnerTickMsg is sent to animate the spinner
type spinnerTickMsg struct{}

// fadeTickMsg is sent to animate the fade transition
type fadeTickMsg struct{}

// escapeTimeoutMsg is sent when the escape timeout expires (first Esc should be forwarded)
type escapeTimeoutMsg struct {
	timestamp time.Time // The timestamp of the Esc that started this timeout
}

// spinnerTickCmd returns a command that sends a spinner tick after a delay
func spinnerTickCmd() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

// fadeTickCmd returns a command that sends a fade tick after a short delay (60fps)
func fadeTickCmd() tea.Cmd {
	return tea.Tick(16*time.Millisecond, func(t time.Time) tea.Msg {
		return fadeTickMsg{}
	})
}

// LoadStateCmd loads saved state from disk
func LoadStateCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		if !workstream.StateExists(dir) {
			return StateLoadedMsg{State: nil, Error: nil}
		}
		state, err := workstream.LoadState(dir)
		return StateLoadedMsg{State: state, Error: err}
	}
}

// SaveStateAndQuitCmd pauses containers, saves state, and quits
func SaveStateAndQuitCmd(dir string, workstreams []*workstream.Workstream, focusedIndex int, layout int) tea.Cmd {
	return func() tea.Msg {
		// Save state synchronously before returning
		err := workstream.SaveState(dir, workstreams, focusedIndex, layout)
		return StateSavedMsg{Error: err}
	}
}

// PauseAllAndSaveCmd gracefully stops claude processes, pauses containers, then saves state
func PauseAllAndSaveCmd(dir string, workstreams []*workstream.Workstream, focusedIndex int, layout int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		dockerClient, err := docker.NewClient()
		if err == nil {
			// First, send SIGTERM to claude processes in all containers
			// This allows them to finish writing config files before we pause
			for _, ws := range workstreams {
				if ws.ContainerID != "" {
					_ = dockerClient.SignalProcess(ctx, ws.ContainerID, "claude", "TERM")
				}
			}

			// Wait for processes to handle the signal and finish writes
			time.Sleep(500 * time.Millisecond)

			// Now pause all containers
			for _, ws := range workstreams {
				if ws.ContainerID != "" {
					_ = dockerClient.PauseContainer(ctx, ws.ContainerID)
				}
			}
			dockerClient.Close()
		}

		// Then save state
		saveErr := workstream.SaveState(dir, workstreams, focusedIndex, layout)
		return StateSavedMsg{Error: saveErr}
	}
}

// Init initializes the application
func (m AppModel) Init() tea.Cmd {
	// Try to load saved state on startup
	// Start with cursor hidden since we begin in nav mode
	return tea.Batch(LoadStateCmd(m.workingDir), tea.HideCursor)
}

// Update handles messages
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()
		return m, nil

	case tea.MouseMsg:
		// Handle mouse clicks to focus panes and enter input mode
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			// Don't handle clicks when dialog is active
			if m.dialog != nil {
				return m, nil
			}

			// Calculate pane bounds (title bar is 1 line, panes start at Y=1)
			titleBarHeight := 1
			statusBarHeight := 1
			availableHeight := m.height - titleBarHeight - statusBarHeight

			bounds := CalculatePaneBounds(m.layout, len(m.panes), m.width, availableHeight, titleBarHeight)
			clickedPane := FindPaneAtPosition(bounds, msg.X, msg.Y)

			if clickedPane >= 0 && clickedPane < len(m.panes) {
				// Focus the clicked pane
				if m.focusedPane < len(m.panes) {
					m.panes[m.focusedPane].SetFocused(false)
				}
				m.focusedPane = clickedPane
				m.panes[m.focusedPane].SetFocused(true)

				// Enter input mode
				m.inputMode = true
				return m, tea.ShowCursor
			}
		}
		return m, nil

	case tea.KeyMsg:
		// If dialog is active, handle dialog input
		if m.dialog != nil {
			newDialog, cmd := m.dialog.Update(msg)
			m.dialog = &newDialog
			return m, cmd
		}

		// Handle input mode (keys routed to focused pane)
		if m.inputMode && len(m.panes) > 0 && m.focusedPane < len(m.panes) {
			switch msg.String() {
			case "esc":
				// Check for Ctrl+B prefix - exit scroll mode or nav mode
				if m.tmuxPrefix && time.Since(m.tmuxPrefixTime) < tmuxPrefixTimeout {
					m.tmuxPrefix = false
					m.pendingEscape = false
					// If in scroll mode, exit scroll mode first
					if m.panes[m.focusedPane].IsScrollMode() {
						m.panes[m.focusedPane].ScrollToBottom()
						return m, nil
					}
					// Otherwise exit to nav mode
					m.inputMode = false
					return m, tea.HideCursor
				}
				m.tmuxPrefix = false
				// Check for double-escape (exit to nav mode)
				// Only triggers if we have a pending escape waiting
				if m.pendingEscape && time.Since(m.lastEscapeTime) < escapeTimeout {
					m.lastEscapeTime = time.Time{} // Reset
					m.pendingEscape = false
					m.inputMode = false
					return m, tea.HideCursor
				}
				// First escape - defer sending to pane, wait to see if it's a double-tap
				m.lastEscapeTime = time.Now()
				m.pendingEscape = true
				// Start timer - if it fires, we'll send the Esc to the pane
				escTime := m.lastEscapeTime
				return m, tea.Tick(escapeTimeout, func(t time.Time) tea.Msg {
					return escapeTimeoutMsg{timestamp: escTime}
				})
			case "ctrl+c":
				// Send ctrl+c to the pane (for interrupting processes)
				var cmd tea.Cmd
				m.panes[m.focusedPane], cmd = m.panes[m.focusedPane].Update(msg)
				return m, cmd
			case "ctrl+b":
				// Tmux-style prefix - works in input mode too
				m.tmuxPrefix = true
				m.tmuxPrefixTime = time.Now()
				return m, nil
			case "left", "right", "up", "down":
				// Check for tmux-style prefix (ctrl-b + arrow) - works in input mode
				if m.tmuxPrefix && time.Since(m.tmuxPrefixTime) < tmuxPrefixTimeout {
					m.tmuxPrefix = false
					if len(m.panes) > 1 {
						var dir Direction
						switch msg.String() {
						case "left":
							dir = DirLeft
						case "right":
							dir = DirRight
						case "up":
							dir = DirUp
						case "down":
							dir = DirDown
						}

						neighbor := FindNeighbor(m.layout, len(m.panes), m.focusedPane, dir)
						if neighbor >= 0 {
							m.panes[m.focusedPane].SetFocused(false)
							m.focusedPane = neighbor
							m.panes[m.focusedPane].SetFocused(true)
						}
					}
					return m, nil
				}
				// Not a tmux sequence - pass to pane
				m.tmuxPrefix = false
				var cmd tea.Cmd
				m.panes[m.focusedPane], cmd = m.panes[m.focusedPane].Update(msg)
				return m, cmd
			case "1", "2", "3", "4", "5", "6", "7", "8", "9":
				// Check for tmux-style prefix (ctrl-b + number) - switch panes by number
				if m.tmuxPrefix && time.Since(m.tmuxPrefixTime) < tmuxPrefixTimeout {
					m.tmuxPrefix = false
					targetIndex := int(msg.String()[0] - '0') // Convert to 1-based index
					for i, pane := range m.panes {
						if pane.Index() == targetIndex {
							if m.focusedPane < len(m.panes) {
								m.panes[m.focusedPane].SetFocused(false)
							}
							m.focusedPane = i
							m.panes[i].SetFocused(true)
							break
						}
					}
					return m, nil
				}
				// Not a tmux sequence - pass to pane
				m.tmuxPrefix = false
				var cmd tea.Cmd
				m.panes[m.focusedPane], cmd = m.panes[m.focusedPane].Update(msg)
				return m, cmd
			case "pgup", "pgdown":
				// Check for tmux-style prefix (ctrl-b + pgup/pgdown) - scroll pane
				if m.tmuxPrefix && time.Since(m.tmuxPrefixTime) < tmuxPrefixTimeout {
					m.tmuxPrefix = false
					if len(m.panes) > 0 && m.focusedPane < len(m.panes) {
						if msg.String() == "pgup" {
							m.panes[m.focusedPane].ScrollPageUp()
						} else {
							m.panes[m.focusedPane].ScrollPageDown()
						}
					}
					return m, nil
				}
				// Not a tmux sequence - pass to pane
				m.tmuxPrefix = false
				var cmd tea.Cmd
				m.panes[m.focusedPane], cmd = m.panes[m.focusedPane].Update(msg)
				return m, cmd
			default:
				// Reset tmux prefix on any other key
				m.tmuxPrefix = false
				// All other keys go to focused pane
				var cmd tea.Cmd
				m.panes[m.focusedPane], cmd = m.panes[m.focusedPane].Update(msg)
				return m, cmd
			}
		}

		// In nav mode, double-escape quits ccells
		if msg.String() == "esc" {
			// First check if we're in scroll mode - single escape exits scroll
			if len(m.panes) > 0 && m.focusedPane < len(m.panes) && m.panes[m.focusedPane].IsScrollMode() {
				m.panes[m.focusedPane].ScrollToBottom()
				return m, nil
			}

			if time.Since(m.lastEscapeTime) < escapeTimeout {
				// Double escape in nav mode - quit
				m.lastEscapeTime = time.Time{}
				if len(m.panes) > 0 {
					var workstreams []*workstream.Workstream
					for _, pane := range m.panes {
						ws := pane.Workstream()
						workstreams = append(workstreams, ws)
						if pane.HasPTY() {
							pane.PTY().Close()
						}
					}
					m.quitting = true
					return m, PauseAllAndSaveCmd(m.workingDir, workstreams, m.focusedPane, int(m.layout))
				}
				m.quitting = true
				_ = workstream.DeleteState(m.workingDir)
				return m, tea.Quit
			}
			m.lastEscapeTime = time.Now()
			return m, nil
		}

		// Navigation mode keybinds
		switch msg.String() {
		case "q", "ctrl+c":
			// Pause containers and save state before quitting
			if len(m.panes) > 0 {
				var workstreams []*workstream.Workstream
				for _, pane := range m.panes {
					ws := pane.Workstream()
					workstreams = append(workstreams, ws)
					// Close PTY session if active
					if pane.HasPTY() {
						pane.PTY().Close()
					}
				}
				m.quitting = true
				// Use synchronous pause+save command
				return m, PauseAllAndSaveCmd(m.workingDir, workstreams, m.focusedPane, int(m.layout))
			}
			m.quitting = true
			// Delete state file if no panes (clean exit)
			_ = workstream.DeleteState(m.workingDir)
			return m, tea.Quit

		case "ctrl+b":
			// Tmux-style prefix
			m.tmuxPrefix = true
			m.tmuxPrefixTime = time.Now()
			return m, nil

		case "left", "right", "up", "down":
			// Arrow keys use spatial navigation based on layout
			m.tmuxPrefix = false
			if len(m.panes) > 1 {
				var dir Direction
				switch msg.String() {
				case "left":
					dir = DirLeft
				case "right":
					dir = DirRight
				case "up":
					dir = DirUp
				case "down":
					dir = DirDown
				}

				neighbor := FindNeighbor(m.layout, len(m.panes), m.focusedPane, dir)
				if neighbor >= 0 {
					m.panes[m.focusedPane].SetFocused(false)
					m.focusedPane = neighbor
					m.panes[m.focusedPane].SetFocused(true)
				}
			}
			return m, nil

		case "pgup":
			// Scroll focused pane up
			if len(m.panes) > 0 && m.focusedPane < len(m.panes) {
				m.panes[m.focusedPane].ScrollPageUp()
			}
			return m, nil

		case "pgdown":
			// Scroll focused pane down
			if len(m.panes) > 0 && m.focusedPane < len(m.panes) {
				m.panes[m.focusedPane].ScrollPageDown()
			}
			return m, nil

		case "n":
			// New workstream dialog
			dialog := NewWorkstreamDialog()
			dialog.SetSize(70, 15)
			m.dialog = &dialog
			return m, nil

		case "d":
			// Destroy focused workstream
			if len(m.panes) > 0 && m.focusedPane < len(m.panes) {
				pane := m.panes[m.focusedPane]
				ws := pane.Workstream()
				// Skip confirmation for errored workstreams - nothing to lose
				if ws.GetState() == workstream.StateError {
					// Same logic as DialogDestroy handler
					m.manager.Remove(ws.ID)
					m.panes = append(m.panes[:m.focusedPane], m.panes[m.focusedPane+1:]...)
					if m.focusedPane >= len(m.panes) && len(m.panes) > 0 {
						m.focusedPane = len(m.panes) - 1
					}
					if len(m.panes) > 0 {
						m.panes[m.focusedPane].SetFocused(true)
					}
					m.updateLayout()
					return m, StopContainerCmd(ws)
				}
				dialog := NewDestroyDialog(ws.BranchName, ws.ID)
				dialog.SetSize(50, 15)
				m.dialog = &dialog
			}
			return m, nil

		case "p":
			// Toggle pairing mode
			if len(m.panes) == 0 || m.focusedPane >= len(m.panes) {
				return m, nil
			}
			ws := m.panes[m.focusedPane].Workstream()
			if ws.ContainerID == "" {
				m.toast = "Container must be running to enable pairing"
				m.toastExpiry = time.Now().Add(toastDuration)
				return m, nil
			}

			// If this workstream is already in pairing mode, disable it
			if m.pairingWorkstreamID == ws.ID {
				m.panes[m.focusedPane].AppendOutput("\nDisabling pairing mode...\n")
				return m, DisablePairingCmd(ws, m.pairingPrevBranch, m.pairingStashed)
			}

			// If another workstream is in pairing mode, disable it first
			if m.pairingWorkstreamID != "" {
				// Find and disable the other pairing
				for i := range m.panes {
					if m.panes[i].Workstream().ID == m.pairingWorkstreamID {
						oldWs := m.panes[i].Workstream()
						m.panes[i].AppendOutput("\nDisabling pairing mode (switching to new workstream)...\n")
						// Disable old pairing then enable new one
						return m, tea.Sequence(
							DisablePairingCmd(oldWs, m.pairingPrevBranch, m.pairingStashed),
							EnablePairingCmd(ws),
						)
					}
				}
			}

			// Get current branch before enabling pairing
			m.panes[m.focusedPane].AppendOutput("\nEnabling pairing mode...\n")
			return m, EnablePairingCmd(ws)

		case "m":
			// Merge/PR menu - first check for uncommitted changes
			if len(m.panes) > 0 && m.focusedPane < len(m.panes) {
				ws := m.panes[m.focusedPane].Workstream()
				// Check for uncommitted changes before showing merge dialog
				return m, CheckUncommittedChangesCmd(ws)
			}
			return m, nil

		case "i", "enter":
			// Enter input mode for focused pane
			if len(m.panes) > 0 && m.focusedPane < len(m.panes) {
				m.inputMode = true
				return m, tea.ShowCursor
			}
			return m, nil

		case "l":
			// Show logs dialog for focused workstream
			if len(m.panes) > 0 && m.focusedPane < len(m.panes) {
				pane := m.panes[m.focusedPane]
				ws := pane.Workstream()

				var logContent string
				paneOutput := pane.GetFullOutput()

				if paneOutput != "" {
					logContent = paneOutput
				} else if pane.IsInitializing() {
					// Show initialization status
					var status strings.Builder
					status.WriteString("=== Container Initialization ===\n\n")
					status.WriteString(fmt.Sprintf("Branch: %s\n", ws.BranchName))
					status.WriteString(fmt.Sprintf("Container ID: %s\n", ws.ContainerID))
					status.WriteString(fmt.Sprintf("Status: %s\n", pane.GetInitStatus()))
					elapsed := time.Since(pane.GetInitStartTime())
					status.WriteString(fmt.Sprintf("Elapsed: %v\n", elapsed.Round(time.Second)))
					status.WriteString("\nWaiting for Claude Code to start...\n")
					status.WriteString("This may take a few minutes if building an image.\n")
					logContent = status.String()
				} else if ws.ContainerID == "" {
					logContent = "(Container not started yet)"
				} else {
					logContent = "(No output captured yet)"
				}

				dialog := NewLogDialog(ws.BranchName, logContent)
				dialog.SetSize(m.width-10, m.height-6)
				m.dialog = &dialog
			}
			return m, nil

		case "L":
			// Cycle through layout types
			m.layout = m.layout.Next()
			m.updateLayout()
			m.toast = fmt.Sprintf("Layout: %s", m.layout.String())
			m.toastExpiry = time.Now().Add(toastDuration)
			return m, nil

		case " ":
			// Toggle focused pane with main pane (position 0)
			if len(m.panes) > 1 {
				if m.focusedPane > 0 {
					// Swap focused pane to main position
					swapPos := m.focusedPane
					m.panes[0], m.panes[swapPos] = m.panes[swapPos], m.panes[0]
					m.panes[swapPos].SetFocused(false)
					m.focusedPane = 0
					m.panes[0].SetFocused(true)
					m.lastSwapPosition = swapPos // Remember for toggle back
					m.updateLayout()
					m.toast = "Moved to main pane"
					m.toastExpiry = time.Now().Add(toastDuration)
				} else if m.lastSwapPosition > 0 && m.lastSwapPosition < len(m.panes) {
					// At main position - swap back to previous position
					swapPos := m.lastSwapPosition
					m.panes[0], m.panes[swapPos] = m.panes[swapPos], m.panes[0]
					m.panes[0].SetFocused(false)
					m.focusedPane = swapPos
					m.panes[swapPos].SetFocused(true)
					m.lastSwapPosition = 0 // Clear after swap back
					m.updateLayout()
					m.toast = "Moved back"
					m.toastExpiry = time.Now().Add(toastDuration)
				}
			}
			return m, nil

		case "tab":
			// Cycle focus (stay in nav mode)
			if len(m.panes) > 0 {
				m.panes[m.focusedPane].SetFocused(false)
				m.focusedPane = (m.focusedPane + 1) % len(m.panes)
				m.panes[m.focusedPane].SetFocused(true)
			}
			return m, nil

		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			// Direct focus by pane number (searches by permanent index, not position)
			targetIndex := int(msg.String()[0] - '0') // Convert to 1-based index
			for i, pane := range m.panes {
				if pane.Index() == targetIndex {
					if m.focusedPane < len(m.panes) {
						m.panes[m.focusedPane].SetFocused(false)
					}
					m.focusedPane = i
					m.panes[i].SetFocused(true)
					break
				}
			}
			return m, nil

		case "r":
			// Resource usage dialog
			dialog := NewResourceUsageDialog(false) // Start with project view
			dialog.SetSize(65, 30)                  // Taller to accommodate disk usage section
			m.dialog = &dialog
			return m, FetchResourceStatsCmd(false, m.getContainerIDs())

		case "s":
			// Settings dialog - first get container count
			return m, ListContainersCmd()

		case "?":
			// Show help dialog
			helpText := fmt.Sprintf(`Claude Cells %s (%s)

Navigation Mode:
  ←→ ↑↓       Switch between panes
  i, Enter    Enter input mode (interact with Claude)
  n           New workstream
  d           Destroy workstream
  m           Merge/PR options
  p           Toggle pairing mode
  r           Resource usage (CPU/memory)
  s           Settings
  l           Show logs
  L           Cycle layout (Grid/Main+Stack/Main+Row/Rows/Columns)
  Space       Move focused pane to main (largest) position
  1-9         Focus pane by number
  Tab         Cycle focus
  PgUp/PgDn   Scroll pane (enters scroll mode)
  q           Quit (pauses containers)
  Esc Esc     Quit

Input Mode:
  Esc Esc     Exit to nav mode
  Ctrl+B Esc  Exit to nav mode (or scroll mode)
  Ctrl+B ←→   Switch pane (without exiting input mode)
  Ctrl+B 1-9  Switch pane by number
  Ctrl+B PgUp/Dn  Scroll pane (enters scroll mode)
  Shift+Enter Insert newline
  All other keys sent to Claude Code

Scroll Mode:
  PgUp/PgDn   Continue scrolling
  Esc         Exit scroll mode (return to live)`, versionInfo, commitHash)
			dialog := NewLogDialog("Help", helpText)
			dialog.SetSize(60, 34)
			m.dialog = &dialog
			return m, nil

		default:
			// Block unhandled keys in nav mode - show toast hint
			if len(m.panes) > 0 {
				m.toast = "Press [i] to enter input mode"
				m.toastExpiry = time.Now().Add(toastDuration)
			}
			return m, nil
		}

	case DialogConfirmMsg:
		m.dialog = nil
		switch msg.Type {
		case DialogNewWorkstream:
			// Create new workstream for summarizing (branch name derived from title later)
			ws := workstream.NewForSummarizing(msg.Value)
			if err := m.manager.Add(ws); err != nil {
				m.toast = fmt.Sprintf("Cannot create workstream: %v", err)
				m.toastExpiry = time.Now().Add(toastDuration * 2)
				return m, nil
			}
			pane := NewPaneModel(ws)
			pane.SetIndex(m.nextPaneIndex) // Assign permanent index
			m.nextPaneIndex++
			pane.SetSummarizing(true) // Start with summarizing animation
			m.panes = append(m.panes, pane)
			m.updateLayout()
			// Focus the new pane
			if m.focusedPane < len(m.panes)-1 && m.focusedPane < len(m.panes) {
				m.panes[m.focusedPane].SetFocused(false)
			}
			m.focusedPane = len(m.panes) - 1
			m.panes[m.focusedPane].SetFocused(true)
			// Generate title first (container starts after title is ready)
			return m, tea.Batch(GenerateTitleCmd(ws), spinnerTickCmd())

		case DialogDestroy:
			// Destroy workstream
			for i, pane := range m.panes {
				if pane.Workstream().ID == msg.WorkstreamID {
					ws := pane.Workstream()
					m.manager.Remove(msg.WorkstreamID)
					m.panes = append(m.panes[:i], m.panes[i+1:]...)
					if m.focusedPane >= len(m.panes) && len(m.panes) > 0 {
						m.focusedPane = len(m.panes) - 1
					}
					if len(m.panes) > 0 {
						m.panes[m.focusedPane].SetFocused(true)
					}
					m.updateLayout()
					// Stop container asynchronously
					return m, StopContainerCmd(ws)
				}
			}

		case DialogPruneProjectConfirm:
			// User typed "destroy" - close panes for this project, prune project containers and branches
			// Close all PTY sessions first (they're all for this project)
			for _, pane := range m.panes {
				if pty := pane.PTY(); pty != nil {
					pty.Close()
				}
				m.manager.Remove(pane.Workstream().ID)
			}
			// Clear all panes
			m.panes = nil
			m.focusedPane = 0
			m.updateLayout()
			// Prune containers and empty branches for this project only
			return m, PruneProjectContainersAndBranchesCmd(m.projectName())

		case DialogPruneAllConfirm:
			// User typed "destroy" - close all panes, prune ALL containers globally
			// Close all PTY sessions first
			for _, pane := range m.panes {
				if pty := pane.PTY(); pty != nil {
					pty.Close()
				}
				m.manager.Remove(pane.Workstream().ID)
			}
			// Clear all panes
			m.panes = nil
			m.focusedPane = 0
			m.updateLayout()
			// Prune all containers and empty branches (globally!)
			return m, PruneAllContainersAndBranchesCmd()
		}
		return m, nil

	case ContainerStartedMsg:
		// Container successfully started - now start PTY session
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				ws.SetContainerID(msg.ContainerID)
				if msg.IsResume {
					m.panes[i].SetInitStatus("Resuming Claude Code...")
				} else {
					m.panes[i].SetInitStatus("Starting Claude Code...")
				}
				// Calculate PTY dimensions from pane size (account for borders/padding)
				ptyWidth := m.panes[i].Width() - 4
				ptyHeight := m.panes[i].Height() - 6
				if ptyWidth < 40 {
					ptyWidth = 40
				}
				if ptyHeight < 10 {
					ptyHeight = 10
				}
				// Start PTY session with initial prompt (or --continue for resume)
				return m, StartPTYCmd(ws, ws.Prompt, ptyWidth, ptyHeight, msg.IsResume)
			}
		}
		return m, nil

	case ContainerErrorMsg:
		// Container failed to start or resume
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				ws.SetError(msg.Error)
				m.panes[i].SetInitializing(false) // Stop the spinner
				m.panes[i].AppendOutput(fmt.Sprintf("\nError: %v\n", msg.Error))
				break
			}
		}
		return m, nil

	case ContainerNotFoundMsg:
		// Container no longer exists - rebuild it and resume with --continue
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				// Clear the old container ID
				ws.ContainerID = ""
				m.panes[i].AppendOutput("\nContainer not found, rebuilding...\n")
				m.panes[i].SetInitializing(true)
				return m, RebuildContainerCmd(ws)
			}
		}
		return m, nil

	case BranchConflictMsg:
		// Branch already exists - show conflict resolution dialog with branch info
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				m.panes[i].AppendOutput(fmt.Sprintf("Branch '%s' already exists.\n", msg.BranchName))
				dialog := NewBranchConflictDialog(msg.BranchName, msg.WorkstreamID, msg.BranchInfo)
				// Make dialog taller to show branch info
				height := 18
				if msg.BranchInfo != "" {
					height = 25
				}
				dialog.SetSize(60, height)
				m.dialog = &dialog
				break
			}
		}
		return m, nil

	case TitleGeneratedMsg:
		// Title generated via Claude CLI - update workstream and start container immediately
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()

				// Determine the title to use
				title := msg.Title
				if msg.Error != nil || title == "" {
					// Fallback to generating from prompt if title generation failed
					title = ws.Prompt
					if len(title) > 50 {
						title = title[:47] + "..."
					}
				}

				// Set the title
				ws.SetTitle(title)

				// If pane is summarizing, derive branch name and start container immediately
				if m.panes[i].IsSummarizing() {
					// Collect existing branch names for uniqueness
					var existingBranches []string
					for _, pane := range m.panes {
						if bn := pane.Workstream().BranchName; bn != "" {
							existingBranches = append(existingBranches, bn)
						}
					}
					// Derive branch name from the generated title
					ws.SetBranchNameFromTitle(title, existingBranches)

					// Set title and start fading animation immediately
					m.panes[i].SetSummarizeTitle(title)
					m.panes[i].StartSummarizeFade()

					// Start container immediately (don't wait for animation)
					m.panes[i].SetInitializing(true)
					m.panes[i].SetInitStatus("Starting container...")

					return m, StartContainerCmd(ws)
				}
				break
			}
		}
		return m, nil

	case UncommittedChangesMsg:
		// Result of checking for uncommitted changes before merge
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				if msg.Error != nil {
					// Error checking - just show merge dialog anyway
					dialog := NewMergeDialog(ws.BranchName, ws.ID)
					dialog.SetSize(50, 15)
					m.dialog = &dialog
				} else if msg.HasChanges {
					// Has uncommitted changes - ask if user wants to commit first
					dialog := NewCommitBeforeMergeDialog(ws.BranchName, ws.ID)
					dialog.SetSize(55, 12)
					m.dialog = &dialog
				} else {
					// No uncommitted changes - show merge dialog directly
					dialog := NewMergeDialog(ws.BranchName, ws.ID)
					dialog.SetSize(50, 15)
					m.dialog = &dialog
				}
				break
			}
		}
		return m, nil

	case CommitBeforeMergeConfirmMsg:
		m.dialog = nil
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				switch msg.Action {
				case CommitBeforeMergeYes:
					// Send /commit command to Claude Code in the container
					if m.panes[i].HasPTY() {
						m.panes[i].AppendOutput("\nAsking Claude to commit changes...\n")
						_ = m.panes[i].PTY().WriteString("/commit\n")
					} else {
						m.panes[i].AppendOutput("\nNo active session to commit.\n")
					}
					// Don't show merge dialog yet - user can press 'm' again after commit
				case CommitBeforeMergeNo:
					// Continue to merge dialog without committing
					dialog := NewMergeDialog(ws.BranchName, ws.ID)
					dialog.SetSize(50, 15)
					m.dialog = &dialog
				}
				break
			}
		}
		return m, nil

	case BranchConflictConfirmMsg:
		m.dialog = nil
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				switch msg.Action {
				case BranchConflictUseExisting:
					m.panes[i].AppendOutput("Using existing branch...\n")
					return m, StartContainerWithExistingBranchCmd(ws)
				case BranchConflictCreateNew:
					// Collect existing branch names for uniqueness check
					var existingBranches []string
					for _, pane := range m.panes {
						existingBranches = append(existingBranches, pane.Workstream().BranchName)
					}
					m.panes[i].AppendOutput("Creating new branch with unique name...\n")
					return m, StartContainerWithNewBranchCmd(ws, existingBranches)
				case BranchConflictDelete:
					m.panes[i].AppendOutput("Deleting and recreating branch...\n")
					return m, DeleteAndRestartContainerCmd(ws)
				}
				break
			}
		}
		return m, nil

	case ContainerStoppedMsg:
		// Container stopped (already removed from panes in DialogDestroy)
		return m, nil

	case PTYReadyMsg:
		// PTY session is ready - connect it to the pane
		// Note: StartReadLoop is already running (started in NewPTYSession)
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				ws.SetState(workstream.StateRunning)
				m.panes[i].SetPTY(msg.Session)
				m.panes[i].SetInitStatus("Starting Claude Code...")
				break
			}
		}
		return m, nil

	case spinnerTickMsg:
		// Animate spinner for any initializing or summarizing panes
		anyAnimating := false
		var cmds []tea.Cmd
		for i := range m.panes {
			// Handle summarizing panes (waiting for title generation)
			if m.panes[i].IsSummarizing() && !m.panes[i].IsSummarizeFading() {
				m.panes[i].TickSpinner()
				anyAnimating = true
			}

			// Handle fading overlay (runs alongside initialization)
			if m.panes[i].IsSummarizeFading() {
				anyAnimating = true
				// Check if fade is complete
				if m.panes[i].ShouldFinishFade() {
					m.panes[i].SummarizeComplete()
				}
			}

			// Handle initializing panes
			if m.panes[i].IsInitializing() {
				m.panes[i].TickSpinner()
				anyAnimating = true

				// Check for initialization timeout
				if m.panes[i].InitTimedOut() {
					ws := m.panes[i].Workstream()
					elapsed := m.panes[i].InitElapsed().Round(time.Second)
					m.panes[i].SetInitializing(false)
					m.panes[i].AppendOutput(fmt.Sprintf("\n[Error] Startup timed out after %v\n", elapsed))
					m.panes[i].AppendOutput("Press 'l' to view container logs, or 'd' to destroy and retry.\n")
					ws.SetError(fmt.Errorf("startup timed out after %v", elapsed))
					// Start fade animation
					if m.panes[i].IsFading() {
						cmds = append(cmds, fadeTickCmd())
					}
				}
			}
		}
		// Continue ticking if any pane is still animating
		if anyAnimating {
			cmds = append(cmds, spinnerTickCmd())
		}
		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case fadeTickMsg:
		// Animate fade transition for any fading panes
		anyFading := false
		for i := range m.panes {
			if m.panes[i].IsFading() {
				if m.panes[i].TickFade() {
					anyFading = true
				}
			}
		}
		// Continue ticking if any pane is still fading
		if anyFading {
			return m, fadeTickCmd()
		}
		return m, nil

	case PTYOutputMsg:
		// Output from PTY - write to virtual terminal
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				outputStr := string(msg.Output)

				// Check if Claude Code is ready
				if m.panes[i].IsInitializing() {
					// Detect various indicators that Claude is ready:
					// - "bypass permissions on" - after accepting bypass permissions
					// - "What would you like to do?" - initial prompt
					// - "Resuming" - resuming a previous session
					// - ">" at start of line - Claude's input prompt
					// - Contains project path - Claude showing the project
					claudeReady := strings.Contains(outputStr, "bypass permissions on") ||
						strings.Contains(outputStr, "What would you like to do?") ||
						strings.Contains(outputStr, "Resuming") ||
						strings.Contains(outputStr, "/workspace") ||
						strings.Contains(outputStr, "\n> ") ||
						strings.Contains(outputStr, "\r> ")
					if claudeReady {
						m.panes[i].SetInitializing(false)
						// Start fade animation and auto-enter input mode if focused
						var cmds []tea.Cmd
						if m.panes[i].IsFading() {
							cmds = append(cmds, fadeTickCmd())
						}
						if i == m.focusedPane {
							m.inputMode = true
							cmds = append(cmds, tea.ShowCursor)
						}
						if len(cmds) > 0 {
							return m, tea.Batch(cmds...)
						}
					}
					// Show ALL output during initialization for debugging
					// Only filter out the permissions dialog spinner
					if !strings.Contains(outputStr, "Bypass Permissions") {
						m.panes[i].WritePTYOutput(msg.Output)
					}
				} else {
					m.panes[i].WritePTYOutput(msg.Output)
				}
				break
			}
		}
		return m, nil

	case PTYClosedMsg:
		// PTY session closed
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				ws.SetState(workstream.StateIdle)
				m.panes[i].SetInitializing(false) // Stop spinner if still initializing
				if msg.Error != nil {
					m.panes[i].AppendOutput(fmt.Sprintf("\nSession ended: %v\n", msg.Error))
				} else {
					m.panes[i].AppendOutput("\nSession ended.\n")
				}
				// Start fade animation if needed
				if m.panes[i].IsFading() {
					return m, fadeTickCmd()
				}
				break
			}
		}
		return m, nil

	case ContainerLogsMsg:
		// If there's a log dialog open, update it with the fetched logs
		if m.dialog != nil && m.dialog.Type == DialogLog {
			var logContent string
			if msg.Error != nil {
				logContent = fmt.Sprintf("Error fetching logs: %v", msg.Error)
			} else if msg.Logs == "" {
				logContent = "(No container logs available yet)\n\nThe container may still be starting up."
			} else {
				logContent = fmt.Sprintf("Container Logs (last 100 lines):\n\n%s", msg.Logs)
			}
			// Extract branch name from existing title (strip "Logs: " prefix)
			branchName := strings.TrimPrefix(m.dialog.Title, "Logs: ")
			dialog := NewLogDialog(branchName, logContent)
			dialog.SetSize(m.width-10, m.height-6)
			m.dialog = &dialog
			return m, nil
		}

		// Otherwise append to pane output
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				if msg.Error != nil {
					m.panes[i].AppendOutput(fmt.Sprintf("\n--- Container Logs Error ---\n%v\n", msg.Error))
				} else {
					m.panes[i].AppendOutput(fmt.Sprintf("\n--- Container Logs (last 100 lines) ---\n%s\n--- End Logs ---\n", msg.Logs))
				}
				break
			}
		}
		return m, nil

	case DialogCancelMsg:
		m.dialog = nil
		return m, nil

	case ContainerCountMsg:
		// Show settings dialog with container count
		if msg.Error != nil {
			// Still show dialog with count of 0 on error
			dialog := NewSettingsDialog(0, m.projectName())
			dialog.SetSize(55, 17)
			m.dialog = &dialog
		} else {
			dialog := NewSettingsDialog(msg.Count, m.projectName())
			dialog.SetSize(55, 17)
			m.dialog = &dialog
		}
		return m, nil

	case SettingsConfirmMsg:
		m.dialog = nil
		switch msg.Action {
		case SettingsActionPruneStopped:
			return m, PruneStoppedContainersCmd()
		case SettingsActionPruneProject:
			// Show project-scoped destroy confirmation dialog
			dialog := NewPruneProjectConfirmDialog(m.projectName())
			dialog.SetSize(55, 17)
			m.dialog = &dialog
			return m, nil
		case SettingsActionPruneAll:
			// Show global destroy confirmation dialog
			dialog := NewPruneAllConfirmDialog()
			dialog.SetSize(55, 17)
			m.dialog = &dialog
			return m, nil
		}
		return m, nil

	case MergeConfirmMsg:
		// Find the workstream and show progress dialog
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				switch msg.Action {
				case MergeActionCreatePR:
					m.panes[i].AppendOutput("\nCreating pull request...\n")
					dialog := NewProgressDialog("Creating Pull Request", fmt.Sprintf("Branch: %s\n\nPushing and creating PR...", ws.BranchName), ws.ID)
					dialog.SetSize(80, 15) // Wide enough for PR URLs to be cmd-clickable
					m.dialog = &dialog
					return m, CreatePRCmd(ws)
				case MergeActionMergeMain:
					m.panes[i].AppendOutput("\nMerging branch into main...\n")
					dialog := NewProgressDialog("Merging Branch", fmt.Sprintf("Branch: %s\n\nMerging into main...", ws.BranchName), ws.ID)
					dialog.SetSize(60, 15)
					m.dialog = &dialog
					return m, MergeBranchCmd(ws)
				case MergeActionPush:
					m.panes[i].AppendOutput("\nPushing branch to origin...\n")
					dialog := NewProgressDialog("Pushing Branch", fmt.Sprintf("Branch: %s\n\nPushing to origin...", ws.BranchName), ws.ID)
					dialog.SetSize(60, 15)
					m.dialog = &dialog
					return m, PushBranchCmd(ws)
				}
				break
			}
		}
		m.dialog = nil
		return m, nil

	case PushBranchResultMsg:
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				if msg.Error != nil {
					m.panes[i].AppendOutput(fmt.Sprintf("Push failed: %v\n", msg.Error))
					// Update progress dialog if open
					if m.dialog != nil && m.dialog.Type == DialogProgress && m.dialog.WorkstreamID == msg.WorkstreamID {
						m.dialog.SetComplete(fmt.Sprintf("Push Failed\n\n%v", msg.Error))
					}
				} else {
					m.panes[i].AppendOutput("Branch pushed successfully!\n")
					// Update progress dialog if open
					if m.dialog != nil && m.dialog.Type == DialogProgress && m.dialog.WorkstreamID == msg.WorkstreamID {
						m.dialog.SetComplete("Branch pushed successfully!\n\nPress Enter or Esc to close.")
					}
				}
				break
			}
		}
		return m, nil

	case PRCreatedMsg:
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				if msg.Error != nil {
					m.panes[i].AppendOutput(fmt.Sprintf("PR creation failed: %v\n", msg.Error))
					// Update progress dialog if open
					if m.dialog != nil && m.dialog.Type == DialogProgress && m.dialog.WorkstreamID == msg.WorkstreamID {
						m.dialog.SetComplete(fmt.Sprintf("PR Creation Failed\n\n%v", msg.Error))
					}
				} else {
					m.panes[i].AppendOutput(fmt.Sprintf("PR created: %s\n", msg.PRURL))
					// Update progress dialog if open
					if m.dialog != nil && m.dialog.Type == DialogProgress && m.dialog.WorkstreamID == msg.WorkstreamID {
						m.dialog.SetComplete(fmt.Sprintf("Pull Request Created!\n\nPR #%d: %s\n\nPress Enter or Esc to close.", msg.PRNumber, msg.PRURL))
					}
				}
				break
			}
		}
		return m, nil

	case MergeBranchMsg:
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				if msg.Error != nil {
					m.panes[i].AppendOutput(fmt.Sprintf("Merge failed: %v\n", msg.Error))
					// Update progress dialog if open
					if m.dialog != nil && m.dialog.Type == DialogProgress && m.dialog.WorkstreamID == msg.WorkstreamID {
						m.dialog.SetComplete(fmt.Sprintf("Merge Failed\n\n%v", msg.Error))
					}
				} else {
					m.panes[i].AppendOutput("Branch merged into main successfully!\n")
					// Update progress dialog if open
					if m.dialog != nil && m.dialog.Type == DialogProgress && m.dialog.WorkstreamID == msg.WorkstreamID {
						m.dialog.SetComplete("Branch merged into main!\n\nPress Enter or Esc to close.")
					}
				}
				break
			}
		}
		return m, nil

	case PairingEnabledMsg:
		if msg.Error != nil {
			for i := range m.panes {
				if m.panes[i].Workstream().ID == msg.WorkstreamID {
					m.panes[i].AppendOutput(fmt.Sprintf("Pairing failed: %v\n", msg.Error))
					break
				}
			}
			m.toast = "Pairing mode failed"
			m.toastExpiry = time.Now().Add(toastDuration)
			return m, nil
		}

		// Store pairing state
		m.pairingWorkstreamID = msg.WorkstreamID
		m.pairingStashed = msg.StashedChanges
		// Get current branch (we'll use "main" as fallback, ideally we'd get it before enabling)
		m.pairingPrevBranch = "main"

		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				m.panes[i].AppendOutput("Pairing mode enabled!\n")
				m.panes[i].AppendOutput(fmt.Sprintf("Local branch: %s\n", ws.BranchName))
				m.panes[i].AppendOutput("Mutagen sync active - changes sync bidirectionally\n")
				if msg.StashedChanges {
					m.panes[i].AppendOutput("(Local changes have been stashed)\n")
				}
				// Set workstream state to pairing
				ws.SetState(workstream.StatePairing)
				break
			}
		}

		m.toast = "Pairing mode enabled"
		m.toastExpiry = time.Now().Add(toastDuration)
		return m, nil

	case PairingDisabledMsg:
		if msg.Error != nil {
			for i := range m.panes {
				if m.panes[i].Workstream().ID == msg.WorkstreamID {
					m.panes[i].AppendOutput(fmt.Sprintf("Pairing disable error: %v\n", msg.Error))
					break
				}
			}
		}

		// Clear pairing state
		m.pairingWorkstreamID = ""
		m.pairingPrevBranch = ""
		m.pairingStashed = false

		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				m.panes[i].AppendOutput("Pairing mode disabled.\n")
				if msg.StashedChanges {
					m.panes[i].AppendOutput("Run 'git stash pop' to restore your stashed changes.\n")
				}
				// Set workstream state back to running
				ws.SetState(workstream.StateRunning)
				break
			}
		}

		m.toast = "Pairing mode disabled"
		m.toastExpiry = time.Now().Add(toastDuration)
		return m, nil

	case PruneResultMsg:
		// Show result as toast (always visible) and in first pane if available
		if msg.Error != nil {
			m.toast = fmt.Sprintf("Prune error: %v", msg.Error)
			m.toastExpiry = time.Now().Add(toastDuration * 2) // Longer for errors
			if len(m.panes) > 0 {
				m.panes[0].AppendOutput(fmt.Sprintf("\n[Settings] Prune error: %v\n", msg.Error))
			}
		} else {
			m.toast = fmt.Sprintf("Pruned %d container(s)", msg.PrunedCount)
			m.toastExpiry = time.Now().Add(toastDuration)
			if len(m.panes) > 0 {
				m.panes[0].AppendOutput(fmt.Sprintf("\n[Settings] Pruned %d container(s)\n", msg.PrunedCount))
			}
		}
		return m, nil

	case PruneAllResultMsg:
		// Show result for full prune all operation (containers + branches)
		if msg.Error != nil {
			m.toast = fmt.Sprintf("Prune error: %v", msg.Error)
			m.toastExpiry = time.Now().Add(toastDuration * 2)
			if len(m.panes) > 0 {
				m.panes[0].AppendOutput(fmt.Sprintf("\n[Settings] Prune error: %v\n", msg.Error))
			}
		} else {
			m.toast = fmt.Sprintf("Destroyed %d container(s), %d empty branch(es)", msg.ContainersPruned, msg.BranchesPruned)
			m.toastExpiry = time.Now().Add(toastDuration * 2)
			if len(m.panes) > 0 {
				m.panes[0].AppendOutput(fmt.Sprintf("\n[Settings] Destroyed %d container(s), %d empty branch(es)\n", msg.ContainersPruned, msg.BranchesPruned))
			}
		}
		return m, nil

	case ResourceStatsMsg:
		// Update resource dialog with stats
		if m.dialog != nil && m.dialog.Type == DialogResourceUsage {
			if msg.Error != nil {
				m.dialog.SetStatsError(msg.Error.Error())
			} else {
				// Format stats as table
				body := m.formatResourceStats(msg.Stats, msg.TotalCPU, msg.TotalMemory, msg.DiskUsage)
				m.dialog.SetStatsContent(body)
			}
		}
		return m, nil

	case ResourceStatsToggleMsg:
		// Handle view toggle - fetch new stats
		if m.dialog != nil && m.dialog.Type == DialogResourceUsage {
			var containerIDs []string
			if !msg.IsGlobal {
				containerIDs = m.getContainerIDs()
			}
			return m, FetchResourceStatsCmd(msg.IsGlobal, containerIDs)
		}
		return m, nil

	case ResourceStatsRefreshMsg:
		// Handle refresh - fetch new stats
		if m.dialog != nil && m.dialog.Type == DialogResourceUsage {
			var containerIDs []string
			if !msg.IsGlobal {
				containerIDs = m.getContainerIDs()
			}
			return m, FetchResourceStatsCmd(msg.IsGlobal, containerIDs)
		}
		return m, nil

	case PromptMsg:
		// Handle prompt from pane
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				m.panes[i].AppendOutput(fmt.Sprintf("> %s\n\n", msg.Prompt))
				m.panes[i].AppendOutput("Claude: Processing...\n")
				// In real implementation, this would send to Claude
			}
		}
		return m, nil

	case StateLoadedMsg:
		// Handle loaded state - resume workstreams
		if msg.Error != nil {
			m.toast = fmt.Sprintf("Failed to load state: %v", msg.Error)
			m.toastExpiry = time.Now().Add(toastDuration * 2)
			return m, nil
		}
		if msg.State == nil || len(msg.State.Workstreams) == 0 {
			// No saved state
			return m, nil
		}

		// Restore workstreams
		m.resuming = true
		var cmds []tea.Cmd
		for _, saved := range msg.State.Workstreams {
			ws := workstream.NewWithID(saved.ID, saved.BranchName, saved.Prompt)
			ws.ContainerID = saved.ContainerID
			ws.CreatedAt = saved.CreatedAt
			ws.Title = saved.Title // Restore generated title
			if err := m.manager.Add(ws); err != nil {
				// Skip workstreams that exceed the limit during restore
				continue
			}

			pane := NewPaneModel(ws)
			pane.SetIndex(m.nextPaneIndex) // Assign permanent index
			m.nextPaneIndex++
			pane.SetInitializing(true)
			pane.SetInitStatus("Resuming session...")
			m.panes = append(m.panes, pane)

			// Resume container
			if ws.ContainerID != "" {
				cmds = append(cmds, ResumeContainerCmd(ws, 80, 24))
			}
			cmds = append(cmds, spinnerTickCmd())
		}

		// Restore focus
		if msg.State.FocusedIndex >= 0 && msg.State.FocusedIndex < len(m.panes) {
			m.focusedPane = msg.State.FocusedIndex
		}
		if len(m.panes) > 0 {
			m.panes[m.focusedPane].SetFocused(true)
		}

		// Restore layout
		m.layout = LayoutType(msg.State.Layout)
		m.updateLayout()
		m.toast = fmt.Sprintf("Resumed %d workstream(s)", len(msg.State.Workstreams))
		m.toastExpiry = time.Now().Add(toastDuration)

		// Delete state file after successful resume
		_ = workstream.DeleteState(m.workingDir)

		return m, tea.Batch(cmds...)

	case StateSavedMsg:
		// State was saved, now quit
		if msg.Error != nil {
			m.toast = fmt.Sprintf("Failed to save state: %v", msg.Error)
			m.toastExpiry = time.Now().Add(toastDuration * 2)
		}
		return m, tea.Quit

	case escapeTimeoutMsg:
		// Escape timeout fired - if we still have a pending escape from the same timestamp,
		// forward it to the pane now. If pendingEscape was cleared (double-tap happened),
		// or if we're no longer in input mode, ignore this message.
		if m.pendingEscape && m.inputMode && msg.timestamp.Equal(m.lastEscapeTime) {
			m.pendingEscape = false
			// Send the deferred Esc to the focused pane
			if len(m.panes) > 0 && m.focusedPane < len(m.panes) {
				var cmd tea.Cmd
				m.panes[m.focusedPane], cmd = m.panes[m.focusedPane].Update(tea.KeyMsg{Type: tea.KeyEscape})
				return m, cmd
			}
		}
		return m, nil
	}

	return m, nil
}

// View renders the application
func (m AppModel) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	var sections []string

	// Top title bar
	titleBar := m.renderTitleBar()
	sections = append(sections, titleBar)

	// Panes section
	if len(m.panes) > 0 {
		paneViews := m.renderPanes()
		sections = append(sections, paneViews)
	} else {
		empty := lipgloss.NewStyle().
			Width(m.width).
			Height(m.height-4).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color("#666666")).
			Render("No workstreams. Press [n] to create one.")
		sections = append(sections, empty)
	}

	// Bottom status bar
	m.statusBar.SetWidth(m.width)
	m.statusBar.SetWorkstreamCount(m.manager.Count())
	m.statusBar.SetInputMode(m.inputMode)
	m.statusBar.SetLayoutName(m.layout.String())
	if pairing := m.manager.GetPairing(); pairing != nil {
		m.statusBar.SetPairingBranch(pairing.BranchName)
	} else {
		m.statusBar.SetPairingBranch("")
	}
	sections = append(sections, m.statusBar.View())

	view := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Ensure view fills full terminal height (prevents artifacts from dialog overlay)
	view = m.padToHeight(view)

	// Overlay toast if active
	if m.toast != "" && time.Now().Before(m.toastExpiry) {
		view = m.overlayToast(view)
	}

	// Overlay dialog if active
	if m.dialog != nil {
		view = m.overlayDialog(view)
	}

	return view
}

// padToHeight ensures the view has exactly m.height lines
// This prevents artifacts when transitioning from dialog overlay to normal view
func (m AppModel) padToHeight(view string) string {
	lines := strings.Split(view, "\n")

	// If we already have the right number of lines, return as-is
	if len(lines) >= m.height {
		return view
	}

	// Pad with empty lines to reach m.height
	for len(lines) < m.height {
		lines = append(lines, strings.Repeat(" ", m.width))
	}

	return strings.Join(lines, "\n")
}

// renderTitleBar renders the top title bar
func (m AppModel) renderTitleBar() string {
	titleStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#7C3AED")).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1)

	// Mode indicator
	var mode string
	if m.inputMode {
		inputStyle := titleStyle.Background(lipgloss.Color("#059669"))
		mode = inputStyle.Render(" INPUT ")
	} else {
		navStyle := titleStyle.Background(lipgloss.Color("#7C3AED"))
		mode = navStyle.Render(" NAV ")
	}

	// Scroll mode indicator
	var scrollIndicator string
	if len(m.panes) > 0 && m.focusedPane < len(m.panes) && m.panes[m.focusedPane].IsScrollMode() {
		scrollStyle := titleStyle.Background(lipgloss.Color("#D97706"))
		scrollIndicator = scrollStyle.Render(" SCROLL ")
	}

	// App title
	title := titleStyle.Render(" Claude Cells ")

	// Keybinds hint
	var hints string
	if m.inputMode {
		if scrollIndicator != "" {
			hints = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("  [Ctrl+B PgUp/Dn] scroll  [Esc] exit scroll")
		} else {
			hints = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("  [Esc Esc] nav  [Ctrl+B ←→] switch  [Ctrl+B PgUp/Dn] scroll")
		}
	} else {
		if scrollIndicator != "" {
			hints = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("  [PgUp/Dn] scroll  [Esc] exit scroll  [i]nput")
		} else {
			hints = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("  [←→]panes  [n]ew  [m]erge  [d]estroy  [l]ogs  [i]nput  [?]help  [Esc Esc]quit")
		}
	}

	left := mode + scrollIndicator + title
	right := hints

	// Calculate spacing
	spacing := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if spacing < 0 {
		spacing = 0
	}

	bar := lipgloss.NewStyle().
		Width(m.width).
		Background(lipgloss.Color("#1F2937")).
		Render(left + strings.Repeat(" ", spacing) + right)

	return bar
}

// overlayToast overlays a toast notification
func (m AppModel) overlayToast(background string) string {
	toastStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#374151")).
		Foreground(lipgloss.Color("#FBBF24")).
		Padding(0, 2).
		Bold(true)

	toast := toastStyle.Render(m.toast)
	toastWidth := lipgloss.Width(toast)

	// Position at bottom center, above status bar
	x := (m.width - toastWidth) / 2
	y := m.height - 3

	if x < 0 {
		x = 0
	}

	lines := strings.Split(background, "\n")
	if y >= 0 && y < len(lines) {
		// Build new line: left padding + toast + right padding
		leftPad := strings.Repeat(" ", x)
		restWidth := m.width - x - toastWidth
		if restWidth < 0 {
			restWidth = 0
		}
		rest := strings.Repeat(" ", restWidth)
		lines[y] = leftPad + toast + rest
	}

	return strings.Join(lines, "\n")
}

// renderPanes renders all panes according to the current layout
func (m AppModel) renderPanes() string {
	if len(m.panes) == 0 {
		return ""
	}

	// Set input mode on panes (indices are assigned permanently at creation)
	for i := range m.panes {
		// Only focused pane can be in input mode
		if i == m.focusedPane && m.inputMode {
			m.panes[i].SetInputMode(true)
		} else {
			m.panes[i].SetInputMode(false)
		}
	}

	// Calculate available height for layout rendering
	titleBarHeight := 1
	statusBarHeight := 1
	availableHeight := m.height - titleBarHeight - statusBarHeight

	// Use the layout system to render panes
	return RenderPanesWithLayout(m.panes, m.layout, m.width, availableHeight)
}

// overlayDialog overlays the dialog on top of the view
func (m AppModel) overlayDialog(background string) string {
	dialog := m.dialog.View()

	// Center the dialog
	dialogWidth := lipgloss.Width(dialog)
	dialogHeight := lipgloss.Height(dialog)

	x := (m.width - dialogWidth) / 2
	y := (m.height - dialogHeight) / 2

	// For destroy dialogs, position over the target pane
	if m.dialog.Type == DialogDestroy && m.dialog.WorkstreamID != "" {
		// Find the pane with this workstream
		titleBarHeight := 1
		statusBarHeight := 1
		availableHeight := m.height - titleBarHeight - statusBarHeight
		bounds := CalculatePaneBounds(m.layout, len(m.panes), m.width, availableHeight, titleBarHeight)

		for i, pane := range m.panes {
			if pane.Workstream().ID == m.dialog.WorkstreamID && i < len(bounds) {
				// Center dialog within this pane's bounds
				paneBounds := bounds[i]
				x = paneBounds.X + (paneBounds.Width-dialogWidth)/2
				y = paneBounds.Y + (paneBounds.Height-dialogHeight)/2
				break
			}
		}
	}

	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	// Overlay dialog on background, preserving background content outside dialog area
	var result strings.Builder
	dialogLines := strings.Split(dialog, "\n")
	bgLines := strings.Split(background, "\n")

	// Ensure we have enough lines
	for len(bgLines) < m.height {
		bgLines = append(bgLines, strings.Repeat(" ", m.width))
	}

	for row := 0; row < m.height; row++ {
		bgLine := ""
		if row < len(bgLines) {
			bgLine = bgLines[row]
		}

		// Check if this row is within the dialog area
		dialogRow := row - y
		if dialogRow >= 0 && dialogRow < len(dialogLines) {
			// This row contains part of the dialog - blend with background
			dLine := dialogLines[dialogRow]
			dLineWidth := lipgloss.Width(dLine)

			// Build the composite line using ANSI-aware truncation:
			// 1. Left part of background (up to x)
			// 2. Dialog content
			// 3. Right part of background (after dialog)

			// Left portion - use ANSI-aware truncation to preserve styled background
			if x > 0 {
				leftPart := ansi.Truncate(bgLine, x, "")
				result.WriteString(leftPart)
				// Pad if the left part was shorter than expected
				leftWidth := ansi.StringWidth(leftPart)
				if leftWidth < x {
					result.WriteString(strings.Repeat(" ", x-leftWidth))
				}
			}

			// Dialog content
			result.WriteString(dLine)

			// Right portion - extract from background using ANSI-aware functions
			rightStart := x + dLineWidth
			if rightStart < m.width {
				bgWidth := ansi.StringWidth(bgLine)
				if rightStart < bgWidth {
					// Use TruncateLeft to skip the first rightStart characters
					rightPart := ansi.TruncateLeft(bgLine, rightStart, "")
					result.WriteString(rightPart)
					// Pad if the right part was shorter than expected
					rightWidth := ansi.StringWidth(rightPart)
					remaining := m.width - rightStart
					if rightWidth < remaining {
						result.WriteString(strings.Repeat(" ", remaining-rightWidth))
					}
				} else {
					// Background is shorter than dialog position - just pad
					result.WriteString(strings.Repeat(" ", m.width-rightStart))
				}
			}
		} else {
			// This row is outside the dialog - use background as-is
			result.WriteString(bgLine)
		}
		if row < m.height-1 {
			result.WriteString("\n")
		}
	}

	return result.String()
}

// updateLayout recalculates pane sizes based on the current layout type
func (m *AppModel) updateLayout() {
	titleBarHeight := 1
	statusBarHeight := 1
	availableHeight := m.height - titleBarHeight - statusBarHeight

	if len(m.panes) == 0 {
		return
	}

	// Calculate sizes using the layout system
	sizes := CalculateLayout(m.layout, len(m.panes), m.width, availableHeight)

	// Apply sizes to panes
	for i := range m.panes {
		if i < len(sizes) {
			m.panes[i].SetSize(sizes[i].Width, sizes[i].Height)
		}
	}
}

// Manager returns the workstream manager
func (m *AppModel) Manager() *workstream.Manager {
	return m.manager
}

// InputMode returns true if the app is in input mode
func (m *AppModel) InputMode() bool {
	return m.inputMode
}

// getContainerIDs returns the container IDs for all current panes
func (m *AppModel) getContainerIDs() []string {
	var ids []string
	for _, pane := range m.panes {
		ws := pane.Workstream()
		if ws.ContainerID != "" {
			ids = append(ids, ws.ContainerID)
		}
	}
	return ids
}

// formatResourceStats formats resource stats as a table for display
func (m *AppModel) formatResourceStats(stats []docker.ContainerStats, totalCPU float64, totalMemory uint64, diskUsage *docker.DiskUsage) string {
	var sb strings.Builder

	// Container stats section
	if len(stats) == 0 {
		sb.WriteString("No containers found.\n")
	} else {
		// Header
		header := fmt.Sprintf("%-24s %8s %12s %8s", "Container", "CPU%", "Memory", "Mem%")
		sb.WriteString(header)
		sb.WriteString("\n")
		sb.WriteString(strings.Repeat("─", 56))
		sb.WriteString("\n")

		// Rows
		for _, s := range stats {
			name := s.ContainerName
			if len(name) > 24 {
				name = name[:21] + "..."
			}
			memStr := docker.FormatBytes(s.MemoryUsage)
			row := fmt.Sprintf("%-24s %7.1f%% %12s %7.1f%%", name, s.CPUPercent, memStr, s.MemoryPercent)
			sb.WriteString(row)
			sb.WriteString("\n")
		}

		// Total line
		sb.WriteString(strings.Repeat("─", 56))
		sb.WriteString("\n")
		totalMemStr := docker.FormatBytes(totalMemory)
		totalRow := fmt.Sprintf("%-24s %7.1f%% %12s", "TOTAL", totalCPU, totalMemStr)
		sb.WriteString(totalRow)
	}

	// Disk usage section
	if diskUsage != nil {
		sb.WriteString("\n\n")
		sb.WriteString("Disk Usage\n")
		sb.WriteString(strings.Repeat("─", 56))
		sb.WriteString("\n")

		sb.WriteString(fmt.Sprintf("%-24s %12s   (%d)\n", "Containers",
			docker.FormatBytesInt64(diskUsage.ContainersSize), diskUsage.ContainersCount))
		sb.WriteString(fmt.Sprintf("%-24s %12s\n", "  ccells containers",
			docker.FormatBytesInt64(diskUsage.CCellsContainerSize)))
		sb.WriteString(fmt.Sprintf("%-24s %12s   (%d)\n", "Images",
			docker.FormatBytesInt64(diskUsage.ImagesSize), diskUsage.ImagesCount))
		sb.WriteString(fmt.Sprintf("%-24s %12s   (%d)\n", "Volumes",
			docker.FormatBytesInt64(diskUsage.VolumesSize), diskUsage.VolumesCount))
		sb.WriteString(fmt.Sprintf("%-24s %12s\n", "Build cache",
			docker.FormatBytesInt64(diskUsage.BuildCacheSize)))
		sb.WriteString(strings.Repeat("─", 56))
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("%-24s %12s\n", "TOTAL",
			docker.FormatBytesInt64(diskUsage.TotalSize)))
	}

	return sb.String()
}
