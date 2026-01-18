package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/samuelreed/docker-tui/internal/docker"
	"github.com/samuelreed/docker-tui/internal/workstream"
)

const escapeTimeout = 300 * time.Millisecond

const toastDuration = 2 * time.Second

// AppModel is the main application model
type AppModel struct {
	manager        *workstream.Manager
	panes          []PaneModel
	focusedPane    int
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
}

const tmuxPrefixTimeout = 2 * time.Second

// NewAppModel creates a new application model
func NewAppModel() AppModel {
	cwd, _ := os.Getwd()
	return AppModel{
		manager:    workstream.NewManager(),
		statusBar:  NewStatusBarModel(),
		workingDir: cwd,
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
func SaveStateAndQuitCmd(dir string, workstreams []*workstream.Workstream, focusedIndex int) tea.Cmd {
	return func() tea.Msg {
		// Save state synchronously before returning
		err := workstream.SaveState(dir, workstreams, focusedIndex)
		return StateSavedMsg{Error: err}
	}
}

// PauseAllAndSaveCmd gracefully stops claude processes, pauses containers, then saves state
func PauseAllAndSaveCmd(dir string, workstreams []*workstream.Workstream, focusedIndex int) tea.Cmd {
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
		saveErr := workstream.SaveState(dir, workstreams, focusedIndex)
		return StateSavedMsg{Error: saveErr}
	}
}

// Init initializes the application
func (m AppModel) Init() tea.Cmd {
	// Try to load saved state on startup
	return LoadStateCmd(m.workingDir)
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
				// Check for double-escape (send escape to pane)
				if time.Since(m.lastEscapeTime) < escapeTimeout {
					m.lastEscapeTime = time.Time{} // Reset
					var cmd tea.Cmd
					m.panes[m.focusedPane], cmd = m.panes[m.focusedPane].Update(msg)
					return m, cmd
				}
				// Single escape - exit input mode but remember time for potential double-tap
				m.lastEscapeTime = time.Now()
				m.inputMode = false
				return m, nil
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
						m.panes[m.focusedPane].SetFocused(false)
						switch msg.String() {
						case "left", "up":
							m.focusedPane--
							if m.focusedPane < 0 {
								m.focusedPane = len(m.panes) - 1
							}
						case "right", "down":
							m.focusedPane++
							if m.focusedPane >= len(m.panes) {
								m.focusedPane = 0
							}
						}
						m.panes[m.focusedPane].SetFocused(true)
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

		// Check for double-escape even in nav mode (to send escape to pane)
		if msg.String() == "esc" && len(m.panes) > 0 && m.focusedPane < len(m.panes) {
			if time.Since(m.lastEscapeTime) < escapeTimeout {
				// Double escape from nav mode - enter input mode and send escape
				m.lastEscapeTime = time.Time{}
				m.inputMode = true
				var cmd tea.Cmd
				m.panes[m.focusedPane], cmd = m.panes[m.focusedPane].Update(msg)
				return m, cmd
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
				return m, PauseAllAndSaveCmd(m.workingDir, workstreams, m.focusedPane)
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
			// Check for tmux-style prefix (ctrl-b + arrow)
			if m.tmuxPrefix && time.Since(m.tmuxPrefixTime) < tmuxPrefixTimeout {
				m.tmuxPrefix = false
				if len(m.panes) > 1 {
					m.panes[m.focusedPane].SetFocused(false)
					switch msg.String() {
					case "left", "up":
						m.focusedPane--
						if m.focusedPane < 0 {
							m.focusedPane = len(m.panes) - 1
						}
					case "right", "down":
						m.focusedPane++
						if m.focusedPane >= len(m.panes) {
							m.focusedPane = 0
						}
					}
					m.panes[m.focusedPane].SetFocused(true)
				}
				return m, nil
			}
			// Reset prefix if not used
			m.tmuxPrefix = false

		case "n":
			// New workstream dialog
			dialog := NewWorkstreamDialog()
			dialog.SetSize(50, 15)
			m.dialog = &dialog
			return m, nil

		case "d":
			// Destroy focused workstream
			if len(m.panes) > 0 && m.focusedPane < len(m.panes) {
				ws := m.panes[m.focusedPane].Workstream()
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
			}
			return m, nil

		case "l":
			// Show logs dialog for focused workstream
			if len(m.panes) > 0 && m.focusedPane < len(m.panes) {
				pane := m.panes[m.focusedPane]
				ws := pane.Workstream()
				logContent := pane.GetFullOutput()
				if logContent == "" {
					logContent = "(No output yet)"
				}
				dialog := NewLogDialog(ws.BranchName, logContent)
				dialog.SetSize(m.width-10, m.height-6)
				m.dialog = &dialog
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
			// Direct focus by number (stay in nav mode)
			idx := int(msg.String()[0] - '1')
			if idx < len(m.panes) {
				if m.focusedPane < len(m.panes) {
					m.panes[m.focusedPane].SetFocused(false)
				}
				m.focusedPane = idx
				m.panes[m.focusedPane].SetFocused(true)
			}
			return m, nil

		case "s":
			// Settings dialog - first get container count
			return m, ListContainersCmd()

		case "?":
			// Help (placeholder)
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
			// Create new workstream
			ws := workstream.New(msg.Value)
			m.manager.Add(ws)
			pane := NewPaneModel(ws)
			pane.AppendOutput("Starting container...\n")
			m.panes = append(m.panes, pane)
			m.updateLayout()
			// Focus the new pane
			if m.focusedPane < len(m.panes)-1 && m.focusedPane < len(m.panes) {
				m.panes[m.focusedPane].SetFocused(false)
			}
			m.focusedPane = len(m.panes) - 1
			m.panes[m.focusedPane].SetFocused(true)
			// Start container asynchronously
			return m, StartContainerCmd(ws)

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
		}
		return m, nil

	case ContainerStartedMsg:
		// Container successfully started - now start PTY session
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				ws.SetContainerID(msg.ContainerID)
				m.panes[i].AppendOutput("Container started.\n")
				m.panes[i].AppendOutput(fmt.Sprintf("Container ID: %s\n\n", msg.ContainerID[:12]))
				m.panes[i].AppendOutput("Starting Claude Code...\n")
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
		// Container failed to start
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				ws.SetError(msg.Error)
				m.panes[i].AppendOutput(fmt.Sprintf("Error: %v\n", msg.Error))
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
				m.panes[i].AppendOutput("Claude Code connected.\n\n")
				// Start the read loop in a goroutine
				// The session needs the program reference to send messages
				go msg.Session.StartReadLoop()
				// Auto-enter input mode if this is the focused pane
				if i == m.focusedPane {
					m.inputMode = true
				}
				break
			}
		}
		return m, nil

	case PTYOutputMsg:
		// Output from PTY - write to virtual terminal
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				m.panes[i].WritePTYOutput(msg.Output)
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
		// Show container logs in pane
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
			return m, PruneAllContainersCmd()
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
					dialog.SetSize(60, 15)
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
			m.manager.Add(ws)

			pane := NewPaneModel(ws)
			pane.AppendOutput("Resuming session...\n")
			m.panes = append(m.panes, pane)

			// Resume container
			if ws.ContainerID != "" {
				cmds = append(cmds, ResumeContainerCmd(ws, 80, 24))
			}
		}

		// Restore focus
		if msg.State.FocusedIndex >= 0 && msg.State.FocusedIndex < len(m.panes) {
			m.focusedPane = msg.State.FocusedIndex
		}
		if len(m.panes) > 0 {
			m.panes[m.focusedPane].SetFocused(true)
		}

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
			Height(m.height - 4).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color("#666666")).
			Render("No workstreams. Press [n] to create one.")
		sections = append(sections, empty)
	}

	// Bottom status bar
	m.statusBar.SetWidth(m.width)
	m.statusBar.SetWorkstreamCount(m.manager.Count())
	m.statusBar.SetInputMode(m.inputMode)
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
	title := titleStyle.Render(" Docker TUI ")

	// Keybinds hint
	var hints string
	if m.inputMode {
		hints = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("  [Esc] nav mode  [Esc Esc] send escape")
	} else {
		hints = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render("  [n]ew  [p]air  [m]erge/PR  [d]estroy  [i]nput  [s]ettings  [q]uit")
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

// renderPanes renders all panes in a grid layout
func (m AppModel) renderPanes() string {
	if len(m.panes) == 0 {
		return ""
	}

	// Set indices and input mode on all panes
	for i := range m.panes {
		m.panes[i].SetIndex(i + 1)
		// Only focused pane can be in input mode
		if i == m.focusedPane && m.inputMode {
			m.panes[i].SetInputMode(true)
		} else {
			m.panes[i].SetInputMode(false)
		}
	}

	// Simple horizontal split for up to 2 panes
	// More complex layouts can be added later
	if len(m.panes) == 1 {
		return m.panes[0].View()
	}

	// Two panes side by side
	if len(m.panes) == 2 {
		left := m.panes[0].View()
		right := m.panes[1].View()
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}

	// More than 2: first two side by side, rest below
	left := m.panes[0].View()
	right := m.panes[1].View()
	top := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	var bottomPanes []string
	for i := 2; i < len(m.panes); i++ {
		bottomPanes = append(bottomPanes, m.panes[i].View())
	}
	bottom := lipgloss.JoinVertical(lipgloss.Left, bottomPanes...)

	return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
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

	// Create a completely fresh view with dialog overlaid
	// This avoids any ANSI corruption issues
	var result strings.Builder
	dialogLines := strings.Split(dialog, "\n")
	bgLines := strings.Split(background, "\n")

	// Ensure we have enough lines
	for len(bgLines) < m.height {
		bgLines = append(bgLines, "")
	}

	for row := 0; row < m.height; row++ {
		// Check if this row is within the dialog area
		dialogRow := row - y
		if dialogRow >= 0 && dialogRow < len(dialogLines) {
			// This row contains part of the dialog
			leftPad := strings.Repeat(" ", x)
			dLine := dialogLines[dialogRow]
			result.WriteString(leftPad)
			result.WriteString(dLine)
		} else {
			// This row is outside the dialog - use background
			if row < len(bgLines) {
				result.WriteString(bgLines[row])
			}
		}
		if row < m.height-1 {
			result.WriteString("\n")
		}
	}

	return result.String()
}

// updateLayout recalculates pane sizes
func (m *AppModel) updateLayout() {
	titleBarHeight := 1
	statusBarHeight := 1
	availableHeight := m.height - titleBarHeight - statusBarHeight

	if len(m.panes) == 0 {
		return
	}

	if len(m.panes) == 1 {
		m.panes[0].SetSize(m.width, availableHeight)
		return
	}

	// Two or more: split horizontally
	paneWidth := m.width / 2
	paneHeight := availableHeight

	if len(m.panes) > 2 {
		// Reserve space for additional panes below
		paneHeight = availableHeight * 2 / 3
	}

	for i := range m.panes {
		if i < 2 {
			m.panes[i].SetSize(paneWidth, paneHeight)
		} else {
			m.panes[i].SetSize(m.width, (availableHeight-paneHeight)/(len(m.panes)-2))
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
