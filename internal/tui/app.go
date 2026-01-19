package tui

import (
	"context"
	"fmt"
	"os"
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

// spinnerTickCmd returns a command that sends a spinner tick after a delay
func spinnerTickCmd() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg{}
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
				// Check for Ctrl+B prefix (exit to nav mode)
				if m.tmuxPrefix && time.Since(m.tmuxPrefixTime) < tmuxPrefixTimeout {
					m.tmuxPrefix = false
					m.inputMode = false
					return m, tea.HideCursor
				}
				m.tmuxPrefix = false
				// Check for double-escape (exit to nav mode)
				if time.Since(m.lastEscapeTime) < escapeTimeout {
					m.lastEscapeTime = time.Time{} // Reset
					m.inputMode = false
					return m, tea.HideCursor
				}
				// Single escape - send to pane (for vim, etc.) but remember time for potential double-tap
				m.lastEscapeTime = time.Now()
				var cmd tea.Cmd
				m.panes[m.focusedPane], cmd = m.panes[m.focusedPane].Update(msg)
				return m, cmd
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

		case "n":
			// New workstream dialog
			dialog := NewWorkstreamDialog()
			dialog.SetSize(50, 15)
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
			// Merge/PR menu
			if len(m.panes) > 0 && m.focusedPane < len(m.panes) {
				ws := m.panes[m.focusedPane].Workstream()
				dialog := NewMergeDialog(ws.BranchName, ws.ID)
				dialog.SetSize(50, 15)
				m.dialog = &dialog
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

				// If initializing or no output, fetch container logs
				if pane.IsInitializing() || pane.GetFullOutput() == "" {
					if ws.ContainerID != "" {
						// Show a loading dialog while fetching logs
						dialog := NewLogDialog(ws.BranchName, "Fetching container logs...")
						dialog.SetSize(m.width-10, m.height-6)
						m.dialog = &dialog
						return m, FetchContainerLogsCmd(ws)
					}
					// No container yet
					dialog := NewLogDialog(ws.BranchName, "(Container not started yet)")
					dialog.SetSize(m.width-10, m.height-6)
					m.dialog = &dialog
					return m, nil
				}

				logContent := pane.GetFullOutput()
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

		case "s":
			// Settings dialog - first get container count
			return m, ListContainersCmd()

		case "?":
			// Show help dialog
			helpText := `Navigation Mode:
  ←→ ↑↓       Switch between panes
  i, Enter    Enter input mode (interact with Claude)
  n           New workstream
  d           Destroy workstream
  m           Merge/PR options
  p           Toggle pairing mode
  s           Settings
  l           Show logs
  L           Cycle layout (Grid/Main+Stack/Main+Row/Rows/Columns)
  Space       Move focused pane to main (largest) position
  1-9         Focus pane by number
  Tab         Cycle focus
  q           Quit (pauses containers)
  Esc Esc     Quit

Input Mode:
  Esc Esc     Exit to nav mode
  Ctrl+B Esc  Exit to nav mode
  Ctrl+B ←→   Switch pane (without exiting input mode)
  Ctrl+B 1-9  Switch pane by number
  All other keys sent to Claude Code`
			dialog := NewLogDialog("Help", helpText)
			dialog.SetSize(60, 27)
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
			// Collect existing branch names to ensure uniqueness
			var existingBranches []string
			for _, pane := range m.panes {
				existingBranches = append(existingBranches, pane.Workstream().BranchName)
			}

			// Create new workstream with unique branch name
			ws := workstream.NewWithUniqueBranch(msg.Value, existingBranches)
			if err := m.manager.Add(ws); err != nil {
				m.toast = fmt.Sprintf("Cannot create workstream: %v", err)
				m.toastExpiry = time.Now().Add(toastDuration * 2)
				return m, nil
			}
			pane := NewPaneModel(ws)
			pane.SetIndex(m.nextPaneIndex) // Assign permanent index
			m.nextPaneIndex++
			pane.SetInitializing(true)
			pane.SetInitStatus("Starting container...")
			m.panes = append(m.panes, pane)
			m.updateLayout()
			// Focus the new pane
			if m.focusedPane < len(m.panes)-1 && m.focusedPane < len(m.panes) {
				m.panes[m.focusedPane].SetFocused(false)
			}
			m.focusedPane = len(m.panes) - 1
			m.panes[m.focusedPane].SetFocused(true)
			// Start container asynchronously and spinner animation
			return m, tea.Batch(StartContainerCmd(ws), spinnerTickCmd())

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

		case DialogPruneAllConfirm:
			// User typed "destroy" - close all panes, prune containers and branches
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
			// Prune all containers and empty branches
			return m, PruneAllContainersAndBranchesCmd()
		}
		return m, nil

	case ContainerStartedMsg:
		// Container successfully started - now start PTY session
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				ws.SetContainerID(msg.ContainerID)
				m.panes[i].SetInitStatus("Starting Claude Code...")
				// Calculate PTY dimensions from pane size (account for borders/padding)
				ptyWidth := m.panes[i].Width() - 4
				ptyHeight := m.panes[i].Height() - 6
				if ptyWidth < 40 {
					ptyWidth = 40
				}
				if ptyHeight < 10 {
					ptyHeight = 10
				}
				// Start PTY session with initial prompt and terminal size
				return m, StartPTYCmd(ws, ws.Prompt, ptyWidth, ptyHeight)
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
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				ws.SetState(workstream.StateRunning)
				m.panes[i].SetPTY(msg.Session)
				m.panes[i].SetInitStatus("Starting Claude Code...")
				// Start the read loop in a goroutine
				// The session needs the program reference to send messages
				go msg.Session.StartReadLoop()
				break
			}
		}
		return m, nil

	case spinnerTickMsg:
		// Animate spinner for any initializing panes and check for timeout
		anyInitializing := false
		for i := range m.panes {
			if m.panes[i].IsInitializing() {
				m.panes[i].TickSpinner()
				anyInitializing = true

				// Check for initialization timeout
				if m.panes[i].InitTimedOut() {
					ws := m.panes[i].Workstream()
					elapsed := m.panes[i].InitElapsed().Round(time.Second)
					m.panes[i].SetInitializing(false)
					m.panes[i].AppendOutput(fmt.Sprintf("\n[Error] Startup timed out after %v\n", elapsed))
					m.panes[i].AppendOutput("Press 'l' to view container logs, or 'd' to destroy and retry.\n")
					ws.SetError(fmt.Errorf("startup timed out after %v", elapsed))
				}
			}
		}
		// Continue ticking if any pane is still initializing
		if anyInitializing {
			return m, spinnerTickCmd()
		}
		return m, nil

	case PTYOutputMsg:
		// Output from PTY - write to virtual terminal
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				// Check if Claude Code is ready (bypass permissions accepted)
				if m.panes[i].IsInitializing() {
					outputStr := string(msg.Output)
					if strings.Contains(outputStr, "bypass permissions on") ||
						strings.Contains(outputStr, "What would you like to do?") {
						m.panes[i].SetInitializing(false)
						// Discard this chunk - it may contain permissions dialog remnants
						// Start fresh from next output
						// Auto-enter input mode if this is the focused pane
						if i == m.focusedPane {
							m.inputMode = true
							return m, tea.ShowCursor
						}
					}
					// Don't write output while initializing - discard it
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
				if msg.Error != nil {
					m.panes[i].AppendOutput(fmt.Sprintf("\nSession ended: %v\n", msg.Error))
				} else {
					m.panes[i].AppendOutput("\nSession ended.\n")
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
			dialog := NewSettingsDialog(0)
			dialog.SetSize(50, 15)
			m.dialog = &dialog
		} else {
			dialog := NewSettingsDialog(msg.Count)
			dialog.SetSize(50, 15)
			m.dialog = &dialog
		}
		return m, nil

	case SettingsConfirmMsg:
		m.dialog = nil
		switch msg.Action {
		case SettingsActionPruneStopped:
			return m, PruneStoppedContainersCmd()
		case SettingsActionPruneAll:
			// Show destroy confirmation dialog instead of directly pruning
			dialog := NewPruneAllConfirmDialog()
			dialog.SetSize(50, 15)
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
		mode = titleStyle.Copy().Background(lipgloss.Color("#059669")).Render(" INPUT ")
	} else {
		mode = titleStyle.Copy().Background(lipgloss.Color("#7C3AED")).Render(" NAV ")
	}

	// App title
	title := titleStyle.Render(" Claude Cells ")

	// Keybinds hint
	var hints string
	if m.inputMode {
		hints = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("  [Esc Esc] nav  [Ctrl+B ←→] switch pane")
	} else {
		hints = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("  [←→]panes  [n]ew  [m]erge  [d]estroy  [l]ogs  [i]nput  [?]help  [Esc Esc]quit")
	}

	left := mode + title
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
