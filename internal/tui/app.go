package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/STRML/claude-cells/internal/config"
	"github.com/STRML/claude-cells/internal/docker"
	"github.com/STRML/claude-cells/internal/sync"
	"github.com/STRML/claude-cells/internal/workstream"
	"github.com/charmbracelet/x/ansi"
)

const escapeTimeout = 300 * time.Millisecond

const toastDuration = 2 * time.Second

const pairingHealthCheckInterval = 30 * time.Second

// formatFileList formats a list of files for display
func formatFileList(files []string) string {
	var sb strings.Builder
	for _, f := range files {
		sb.WriteString("  • ")
		sb.WriteString(f)
		sb.WriteString("\n")
	}
	return sb.String()
}

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

// dragModifier returns the key name for bypassing terminal mouse capture.
// On macOS, Option (Alt) is typically used; on other platforms, Shift is used.
func dragModifier() string {
	if runtime.GOOS == "darwin" {
		return "Option"
	}
	return "Shift"
}

// AppModel is the main application model
type AppModel struct {
	ctx            context.Context // App-level context for cancellation
	manager        *workstream.PersistentManager
	panes          []PaneModel
	focusedPane    int
	nextPaneIndex  int        // Counter for assigning permanent pane indices
	layout         LayoutType // Current pane layout
	statusBar      StatusBarModel
	dialog         *DialogModel
	width          int
	height         int
	quitting       bool
	inputMode      bool                 // True when input is being routed to focused pane
	mouseEnabled   bool                 // True when mouse capture is enabled (click-to-focus)
	dragHintShown  bool                 // True if we've shown the drag modifier hint this session
	ctrlVHintShown bool                 // True if we've shown the Ctrl+V paste hint this session
	lastEscapeTime time.Time            // For double-escape detection
	pendingEscape  bool                 // True when first Esc pressed, waiting to see if double-tap
	toast          string               // Temporary notification message
	toastExpiry    time.Time            // When toast should disappear
	workingDir     string               // Current working directory (git repo path)
	stateDir       string               // State file directory (~/.claude-cells/state/<repo-id>/)
	repoInfo       *workstream.RepoInfo // Repo metadata for state file
	resuming       bool                 // True if resuming from saved state
	tmuxPrefix     bool                 // True after ctrl-b is pressed (tmux-style prefix)
	tmuxPrefixTime time.Time            // When prefix was pressed
	// Pairing mode orchestrator (single source of truth)
	pairingOrchestrator *sync.Pairing
	// Pane swap state
	lastSwapPosition int // Position to swap back to when pressing Space at main (0 = none)
	// Log panel
	logPanel *LogPanelModel
	// Keyboard enhancement support (Kitty protocol)
	keyboardEnhanced bool // True if terminal supports enhanced keyboard (shift+enter, etc.)
}

const tmuxPrefixTimeout = 2 * time.Second

// NewAppModel creates a new application model
func NewAppModel(ctx context.Context) AppModel {
	cwd, _ := os.Getwd()
	logPanel := NewLogPanelModel()
	SetLogPanel(logPanel) // Set global for logging functions

	// Initialize pairing orchestrator with real git and mutagen implementations
	gitOps := GitClientFactory(cwd)
	mutagenOps := sync.NewMutagen()

	// Compute state directory based on repo ID
	stateDir := cwd // Fallback to cwd if we can't get repo ID
	var repoInfo *workstream.RepoInfo
	repoID, err := gitOps.RepoID(ctx)
	if err == nil && repoID != "" {
		// Get new state directory
		newStateDir, err := workstream.GetStateDir(repoID)
		if err == nil {
			stateDir = newStateDir

			// Migrate old state file if it exists
			_, _ = workstream.MigrateStateFile(cwd, stateDir)

			// Get remote URL for RepoInfo
			remoteURL := ""
			if out, err := gitOps.RemoteURL(ctx, "origin"); err == nil {
				remoteURL = out
			}

			// Build RepoInfo for new state files
			repoInfo = &workstream.RepoInfo{
				Name:      filepath.Base(cwd),
				Path:      cwd,
				Remote:    remoteURL,
				RepoID:    repoID,
				CreatedAt: time.Now(),
			}
		}
	}

	manager := workstream.NewPersistentManager(stateDir)
	if repoInfo != nil {
		manager.SetRepoInfo(repoInfo)
	}

	return AppModel{
		ctx:                 ctx,
		manager:             manager,
		statusBar:           NewStatusBarModel(),
		workingDir:          cwd,
		stateDir:            stateDir,
		repoInfo:            repoInfo,
		nextPaneIndex:       1,    // Start pane numbering at 1
		mouseEnabled:        true, // Enable mouse click-to-focus by default
		logPanel:            logPanel,
		pairingOrchestrator: sync.NewPairing(gitOps, mutagenOps),
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

// setFocusedPane updates the focused pane and syncs with persistent manager.
func (m *AppModel) setFocusedPane(idx int) {
	m.focusedPane = idx
	m.manager.SetFocused(idx)
}

// setLayout updates the layout and syncs with persistent manager.
func (m *AppModel) setLayout(layout LayoutType) {
	m.layout = layout
	m.manager.SetLayout(int(layout))
}

// StateLoadedMsg is sent when state has been loaded from disk
type StateLoadedMsg struct {
	State *workstream.AppState
	Error error
}

// StateSavedMsg is sent when state has been saved
type StateSavedMsg struct {
	Error         error
	RepairMessage string // Optional message about state repair (shown to user if not empty)
}

// spinnerTickMsg is sent to animate the spinner
type spinnerTickMsg struct{}

// fadeTickMsg is sent to animate the fade transition
type fadeTickMsg struct{}

// escapeTimeoutMsg is sent when the escape timeout expires (first Esc should be forwarded)
type escapeTimeoutMsg struct {
	timestamp time.Time // The timestamp of the Esc that started this timeout
}

// pairingHealthTickMsg is sent periodically to check pairing sync health
type pairingHealthTickMsg struct{}

// autoContinueMsg is sent when we need to auto-continue an interrupted session
type autoContinueMsg struct {
	WorkstreamID string
}

// pairingHealthTickCmd returns a command that sends a health tick after a delay
func pairingHealthTickCmd() tea.Cmd {
	return tea.Tick(pairingHealthCheckInterval, func(t time.Time) tea.Msg {
		return pairingHealthTickMsg{}
	})
}

// autoContinueCmd returns a command that sends an auto-continue message after a short delay
// The delay gives Claude time to fully initialize before we send the continue command
func autoContinueCmd(workstreamID string) tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return autoContinueMsg{WorkstreamID: workstreamID}
	})
}

// autoPressEnterMsg is sent when we need to auto-press enter for a workstream
// that started with "continue" as its prompt
type autoPressEnterMsg struct {
	WorkstreamID string
}

// autoPressEnterCmd returns a command that sends an auto-press-enter message after a short delay
// The delay gives Claude time to fully initialize before we send the enter key
func autoPressEnterCmd(workstreamID string) tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return autoPressEnterMsg{WorkstreamID: workstreamID}
	})
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

// PauseAllAndSaveCmd gracefully stops claude processes, pauses containers, validates state, then saves
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

			// Persist sessions from container runtime location to mount point
			// This ensures sessions survive container rebuilds
			for _, ws := range workstreams {
				if ws.ContainerID != "" {
					// Ignore errors - session persistence is best-effort
					_ = dockerClient.PersistSessions(ctx, ws.ContainerID)
				}
			}

			// Validate and repair state before pausing containers
			// This extracts session IDs from running containers
			repairResult, repairErr := workstream.ValidateAndRepairState(ctx, workstreams)
			var stateRepairMsg string
			if repairErr != nil {
				stateRepairMsg = fmt.Sprintf("State validation error: %v", repairErr)
			} else if repairResult.WasRepaired() {
				stateRepairMsg = fmt.Sprintf("State repaired: %s", repairResult.Summary())
			} else if repairResult.IsCorrupted() {
				stateRepairMsg = fmt.Sprintf("State issues: %s", repairResult.Summary())
			}

			// Now pause all containers
			for _, ws := range workstreams {
				if ws.ContainerID != "" {
					_ = dockerClient.PauseContainer(ctx, ws.ContainerID)
				}
			}
			dockerClient.Close()

			// Save state (with any repairs applied)
			saveErr := workstream.SaveState(dir, workstreams, focusedIndex, layout)
			return StateSavedMsg{Error: saveErr, RepairMessage: stateRepairMsg}
		}

		// Fallback: no Docker client, just save state
		saveErr := workstream.SaveState(dir, workstreams, focusedIndex, layout)
		return StateSavedMsg{Error: saveErr}
	}
}

// Init initializes the application
func (m AppModel) Init() tea.Cmd {
	// Try to load saved state on startup
	// Cursor visibility is now controlled via View().Cursor
	return LoadStateCmd(m.stateDir)
}

// Update handles messages
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()
		return m, nil

	case tea.KeyboardEnhancementsMsg:
		// Terminal responded with keyboard enhancement capabilities
		// This means the terminal supports Kitty keyboard protocol
		// If we get this message, shift+enter should work
		m.keyboardEnhanced = true
		LogDebug("Keyboard enhancements enabled - Shift+Enter will work")
		return m, nil

	case tea.MouseClickMsg:
		// Handle mouse clicks to focus panes and enter input mode
		if !m.mouseEnabled {
			return m, nil
		}
		if msg.Button == tea.MouseLeft {
			// Don't handle clicks when dialog is active
			if m.dialog != nil {
				return m, nil
			}

			// Calculate pane bounds (title bar is 1 line, panes start at Y=1)
			titleBarHeight := 1
			statusBarHeight := 1
			logPanelH := m.logPanelHeight()
			availableHeight := m.height - titleBarHeight - statusBarHeight - logPanelH

			bounds := CalculatePaneBounds(m.layout, len(m.panes), m.width, availableHeight, titleBarHeight)
			clickedPane := FindPaneAtPosition(bounds, msg.X, msg.Y)

			if clickedPane >= 0 && clickedPane < len(m.panes) {
				// Focus the clicked pane
				if m.focusedPane < len(m.panes) {
					m.panes[m.focusedPane].SetFocused(false)
				}
				m.setFocusedPane(clickedPane)
				m.panes[m.focusedPane].SetFocused(true)

				// Enter input mode (cursor visibility handled in View)
				m.inputMode = true
				return m, nil
			}
		}
		return m, nil

	case tea.MouseMotionMsg:
		// Detect dragging and show hint about modifier+drag for text selection
		if m.mouseEnabled && !m.dragHintShown && msg.Button == tea.MouseLeft {
			m.dragHintShown = true
			m.toast = fmt.Sprintf("Tip: Hold %s while dragging to select text, or Ctrl+B m to toggle mouse mode", dragModifier())
			m.toastExpiry = time.Now().Add(5 * time.Second) // Longer duration for this tip
		}
		return m, nil

	case tea.PasteMsg:
		// Handle paste from clipboard (cmd+v / ctrl+shift+v)
		// If dialog is active, forward paste to dialog
		if m.dialog != nil {
			newDialog, cmd := m.dialog.Update(msg)
			m.dialog = &newDialog
			return m, cmd
		}
		// If focused pane has an in-pane dialog, forward paste to that dialog
		if len(m.panes) > 0 && m.focusedPane < len(m.panes) && m.panes[m.focusedPane].HasInPaneDialog() {
			dialog := m.panes[m.focusedPane].GetInPaneDialog()
			newDialog, cmd := dialog.Update(msg)
			m.panes[m.focusedPane].SetInPaneDialog(&newDialog)
			return m, cmd
		}
		// Forward pasted content to the focused pane when in input mode
		if m.inputMode && len(m.panes) > 0 && m.focusedPane < len(m.panes) {
			if m.panes[m.focusedPane].HasPTY() {
				_ = m.panes[m.focusedPane].SendToPTY(msg.Content)
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

		// If focused pane has an in-pane dialog, handle that dialog's input
		if len(m.panes) > 0 && m.focusedPane < len(m.panes) && m.panes[m.focusedPane].HasInPaneDialog() {
			dialog := m.panes[m.focusedPane].GetInPaneDialog()
			newDialog, cmd := dialog.Update(msg)
			m.panes[m.focusedPane].SetInPaneDialog(&newDialog)
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
					return m, nil
				}
				m.tmuxPrefix = false
				// Check for double-escape (exit to nav mode)
				// Only triggers if we have a pending escape waiting
				if m.pendingEscape && time.Since(m.lastEscapeTime) < escapeTimeout {
					m.lastEscapeTime = time.Time{} // Reset
					m.pendingEscape = false
					m.inputMode = false
					return m, nil
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
							m.setFocusedPane(neighbor)
							m.panes[m.focusedPane].SetFocused(true)
						}
					}
					return m, nil
				}
				// In scroll mode, arrow keys scroll instead of going to pane
				if len(m.panes) > 0 && m.focusedPane < len(m.panes) && m.panes[m.focusedPane].IsScrollMode() {
					switch msg.String() {
					case "up":
						m.panes[m.focusedPane].ScrollLineUp()
					case "down":
						m.panes[m.focusedPane].ScrollLineDown()
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
							m.setFocusedPane(i)
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
				// In scroll mode, pgup/pgdown work directly without prefix
				if len(m.panes) > 0 && m.focusedPane < len(m.panes) && m.panes[m.focusedPane].IsScrollMode() {
					if msg.String() == "pgup" {
						m.panes[m.focusedPane].ScrollPageUp()
					} else {
						m.panes[m.focusedPane].ScrollPageDown()
					}
					return m, nil
				}
				// Not a tmux sequence - pass to pane
				m.tmuxPrefix = false
				var cmd tea.Cmd
				m.panes[m.focusedPane], cmd = m.panes[m.focusedPane].Update(msg)
				return m, cmd
			case "ctrl+u", "ctrl+d":
				// Half-page scrolling (vim-style)
				// Works in scroll mode directly, or with tmux prefix from input mode
				if m.tmuxPrefix && time.Since(m.tmuxPrefixTime) < tmuxPrefixTimeout {
					m.tmuxPrefix = false
					if len(m.panes) > 0 && m.focusedPane < len(m.panes) {
						if msg.String() == "ctrl+u" {
							m.panes[m.focusedPane].ScrollHalfPageUp()
						} else {
							m.panes[m.focusedPane].ScrollHalfPageDown()
						}
					}
					return m, nil
				}
				// In scroll mode, ctrl+u/d work directly
				if len(m.panes) > 0 && m.focusedPane < len(m.panes) && m.panes[m.focusedPane].IsScrollMode() {
					if msg.String() == "ctrl+u" {
						m.panes[m.focusedPane].ScrollHalfPageUp()
					} else {
						m.panes[m.focusedPane].ScrollHalfPageDown()
					}
					return m, nil
				}
				// Not in scroll mode and no prefix - pass to pane
				m.tmuxPrefix = false
				var cmd tea.Cmd
				m.panes[m.focusedPane], cmd = m.panes[m.focusedPane].Update(msg)
				return m, cmd
			case "[":
				// Check for tmux-style prefix (ctrl-b + [) - enter scroll/copy mode
				if m.tmuxPrefix && time.Since(m.tmuxPrefixTime) < tmuxPrefixTimeout {
					m.tmuxPrefix = false
					if len(m.panes) > 0 && m.focusedPane < len(m.panes) {
						m.panes[m.focusedPane].EnterScrollMode()
					}
					return m, nil
				}
				// Not a tmux sequence - pass to pane
				m.tmuxPrefix = false
				var cmd tea.Cmd
				m.panes[m.focusedPane], cmd = m.panes[m.focusedPane].Update(msg)
				return m, cmd
			case "m":
				// Check for tmux-style prefix (ctrl-b + m) - toggle mouse mode
				if m.tmuxPrefix && time.Since(m.tmuxPrefixTime) < tmuxPrefixTimeout {
					m.tmuxPrefix = false
					m.mouseEnabled = !m.mouseEnabled
					if m.mouseEnabled {
						m.toast = fmt.Sprintf("Mouse mode ON (click to focus, %s+drag to select)", dragModifier())
					} else {
						m.toast = "Mouse mode OFF (drag to select text)"
					}
					m.toastExpiry = time.Now().Add(toastDuration)
					return m, nil
				}
				// Not a tmux sequence - pass to pane
				m.tmuxPrefix = false
				var cmd tea.Cmd
				m.panes[m.focusedPane], cmd = m.panes[m.focusedPane].Update(msg)
				return m, cmd
			case "ctrl+v":
				// Show one-time hint about paste shortcut
				// Ctrl+V in Unix terminals means "literal next character", not paste
				if !m.ctrlVHintShown {
					m.ctrlVHintShown = true
					m.toast = "Tip: Use Ctrl+Shift+V to paste (Ctrl+V is 'literal next' in terminals)"
					m.toastExpiry = time.Now().Add(5 * time.Second)
				}
				// Still send to pane (preserves standard terminal behavior)
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
				// Double escape in nav mode - quit (with confirmation if workstreams exist)
				m.lastEscapeTime = time.Time{}
				if len(m.panes) == 0 {
					// No workstreams - quit immediately without confirmation
					m.quitting = true
					m.manager.Close()
					return m, SaveStateAndQuitCmd(m.stateDir, nil, 0, int(m.layout))
				}
				dialog := NewQuitConfirmDialog()
				dialog.SetSize(50, 12)
				m.dialog = &dialog
				return m, nil
			}
			m.lastEscapeTime = time.Now()
			return m, nil
		}

		// Navigation mode keybinds
		switch msg.String() {
		case "q", "ctrl+c":
			// Quit (with confirmation if workstreams exist)
			if len(m.panes) == 0 {
				// No workstreams - quit immediately without confirmation
				m.quitting = true
				m.manager.Close()
				return m, SaveStateAndQuitCmd(m.stateDir, nil, 0, int(m.layout))
			}
			dialog := NewQuitConfirmDialog()
			dialog.SetSize(50, 12)
			m.dialog = &dialog
			return m, nil

		case "ctrl+b":
			// Tmux-style prefix
			m.tmuxPrefix = true
			m.tmuxPrefixTime = time.Now()
			return m, nil

		case "left", "right", "up", "down":
			m.tmuxPrefix = false
			// In scroll mode, up/down arrows scroll instead of navigating panes
			if len(m.panes) > 0 && m.focusedPane < len(m.panes) && m.panes[m.focusedPane].IsScrollMode() {
				switch msg.String() {
				case "up":
					m.panes[m.focusedPane].ScrollLineUp()
					return m, nil
				case "down":
					m.panes[m.focusedPane].ScrollLineDown()
					return m, nil
				}
				// left/right fall through to navigate panes
			}
			// Arrow keys use spatial navigation based on layout
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
					m.setFocusedPane(neighbor)
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

		case "ctrl+u":
			// Half-page scroll up (vim-style)
			if len(m.panes) > 0 && m.focusedPane < len(m.panes) {
				m.panes[m.focusedPane].ScrollHalfPageUp()
			}
			return m, nil

		case "ctrl+d":
			// Half-page scroll down (vim-style)
			if len(m.panes) > 0 && m.focusedPane < len(m.panes) {
				m.panes[m.focusedPane].ScrollHalfPageDown()
			}
			return m, nil

		case "[":
			// Enter scroll/copy mode (tmux-style)
			if len(m.panes) > 0 && m.focusedPane < len(m.panes) {
				m.panes[m.focusedPane].EnterScrollMode()
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
					ws := m.removePane(m.focusedPane)
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

			// Get current pairing state from orchestrator
			pairingState := m.pairingOrchestrator.GetState()

			// If this workstream is already in pairing mode, disable it
			if pairingState.Active && pairingState.ContainerID == ws.ContainerID {
				m.panes[m.focusedPane].AppendOutput("\nDisabling pairing mode...\n")
				return m, DisablePairingCmd(m.pairingOrchestrator, ws)
			}

			// If another workstream is in pairing mode, disable it first
			if pairingState.Active {
				// Find and disable the other pairing
				for i := range m.panes {
					otherWs := m.panes[i].Workstream()
					if otherWs.ContainerID == pairingState.ContainerID {
						m.panes[i].AppendOutput("\nDisabling pairing mode (switching to new workstream)...\n")
						// Capture previousBranch BEFORE async dispatch to avoid race
						gitOps := GitClientFactory(m.workingDir)
						previousBranch, _ := gitOps.CurrentBranch(m.ctx)
						// Disable old pairing then enable new one
						return m, tea.Sequence(
							DisablePairingCmd(m.pairingOrchestrator, otherWs),
							EnablePairingCmd(m.pairingOrchestrator, ws, previousBranch),
						)
					}
				}
			}

			// Capture previousBranch BEFORE async dispatch to avoid race
			gitOps := GitClientFactory(m.workingDir)
			previousBranch, _ := gitOps.CurrentBranch(m.ctx)

			m.panes[m.focusedPane].AppendOutput("\nEnabling pairing mode...\n")
			return m, EnablePairingCmd(m.pairingOrchestrator, ws, previousBranch)

		case "m":
			// Ctrl+B m toggles mouse mode
			if m.tmuxPrefix && time.Since(m.tmuxPrefixTime) < tmuxPrefixTimeout {
				m.tmuxPrefix = false
				m.mouseEnabled = !m.mouseEnabled
				if m.mouseEnabled {
					m.toast = fmt.Sprintf("Mouse mode ON (click to focus, %s+drag to select)", dragModifier())
				} else {
					m.toast = "Mouse mode OFF (drag to select text)"
				}
				m.toastExpiry = time.Now().Add(toastDuration)
				return m, nil
			}
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
				return m, nil
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
			m.setLayout(m.layout.Next())
			m.updateLayout()
			m.toast = fmt.Sprintf("Layout: %s", m.layout.String())
			m.toastExpiry = time.Now().Add(toastDuration)
			return m, nil

		case "`":
			// Toggle log panel visibility
			if m.logPanel != nil {
				m.logPanel.Toggle()
				m.updateLayout()
				if m.logPanel.IsVisible() {
					m.toast = fmt.Sprintf("Log panel: %s+", m.logPanel.FilterLevel().String())
				} else {
					m.toast = "Log panel hidden"
				}
				m.toastExpiry = time.Now().Add(toastDuration)
			}
			return m, nil

		case "~":
			// Cycle log panel filter level (when visible)
			if m.logPanel != nil && m.logPanel.IsVisible() {
				m.logPanel.CycleFilter()
				m.toast = fmt.Sprintf("Log filter: %s+", m.logPanel.FilterLevel().String())
				m.toastExpiry = time.Now().Add(toastDuration)
			}
			return m, nil

		case "f":
			// Toggle log panel fullscreen (when visible)
			if m.logPanel != nil && m.logPanel.IsVisible() {
				m.logPanel.ToggleFullscreen()
				m.updateLayout()
				if m.logPanel.IsFullscreen() {
					m.toast = "Logs: fullscreen"
				} else {
					m.toast = "Logs: normal"
				}
				m.toastExpiry = time.Now().Add(toastDuration)
				return m, nil
			}

		case "E":
			// Export system logs (when log panel is visible)
			if m.logPanel != nil && m.logPanel.IsVisible() {
				logsDir := filepath.Join(m.workingDir, "ccells-logs")
				exportPath, err := m.logPanel.Export(logsDir)
				if err != nil {
					m.toast = fmt.Sprintf("Export failed: %v", err)
				} else {
					m.toast = fmt.Sprintf("Logs exported: %s", filepath.Base(exportPath))
				}
				m.toastExpiry = time.Now().Add(toastDuration * 2) // Longer for export
				return m, nil
			}

		case "space":
			// Toggle focused pane with main pane (position 0)
			if len(m.panes) > 1 {
				if m.focusedPane > 0 {
					// Swap focused pane to main position
					swapPos := m.focusedPane
					m.panes[0], m.panes[swapPos] = m.panes[swapPos], m.panes[0]
					m.panes[swapPos].SetFocused(false)
					m.setFocusedPane(0)
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
					m.setFocusedPane(swapPos)
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
				m.setFocusedPane((m.focusedPane + 1) % len(m.panes))
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
					m.setFocusedPane(i)
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

		case "e":
			// Export logs for focused pane
			if len(m.panes) > 0 && m.focusedPane < len(m.panes) {
				pane := &m.panes[m.focusedPane]
				logsDir := filepath.Join(m.workingDir, "ccells-logs")
				exportPath, err := pane.ExportLogs(logsDir)
				if err != nil {
					m.toast = fmt.Sprintf("Export failed: %v", err)
					m.toastExpiry = time.Now().Add(toastDuration * 2)
				} else {
					m.toast = fmt.Sprintf("Logs exported: %s", exportPath)
					m.toastExpiry = time.Now().Add(toastDuration * 2)
				}
			}
			return m, nil

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
  e           Export logs to file
  L           Cycle layout
  `+"`"+`           Toggle log panel (system logs)
  ~           Cycle log filter (DEBUG/INFO/WARN/ERR)
  f           Fullscreen log panel (when visible)
  E           Export system logs (when visible)
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
  Shift+Enter Insert newline*
  All other keys sent to Claude Code

* Shift+Enter requires a terminal with Kitty keyboard
  protocol support (kitty, WezTerm, Ghostty, foot, etc.)

Scroll Mode:
  PgUp/PgDn   Continue scrolling
  Esc         Exit scroll mode (return to live)`, versionInfo, commitHash)
			dialog := NewLogDialog("Help", helpText)
			dialog.SetSize(60, 40)
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
			m.setFocusedPane(len(m.panes) - 1)
			m.panes[m.focusedPane].SetFocused(true)
			// Generate title first (container starts after title is ready)
			return m, tea.Batch(GenerateTitleCmd(ws), spinnerTickCmd())

		case DialogDestroy:
			// Destroy workstream
			for i, pane := range m.panes {
				if pane.Workstream().ID == msg.WorkstreamID {
					ws := m.removePane(i)
					return m, StopContainerCmd(ws)
				}
			}

		case DialogPruneProjectConfirm:
			// User typed "destroy" - close panes for this project, prune project containers and branches
			m.clearAllPanes()
			// Prune containers and empty branches for this project only
			return m, PruneProjectContainersAndBranchesCmd(m.projectName())

		case DialogPruneAllConfirm:
			// User typed "destroy" - close all panes, prune ALL containers globally
			m.clearAllPanes()
			// Prune all containers and empty branches (globally!)
			return m, PruneAllContainersAndBranchesCmd()

		case DialogPostMergeDestroy:
			// Value is "0" for "Yes, destroy container", "1" for "No, keep container"
			if msg.Value == "0" {
				// User chose to destroy
				for i, pane := range m.panes {
					if pane.Workstream().ID == msg.WorkstreamID {
						ws := m.removePane(i)
						m.toast = "Destroying merged container..."
						m.toastExpiry = time.Now().Add(toastDuration)
						return m, StopContainerCmd(ws)
					}
				}
			}
			// User chose to keep - do nothing

		case DialogMergeConflict:
			// Clear the in-pane dialog
			for i := range m.panes {
				if m.panes[i].Workstream().ID == msg.WorkstreamID {
					m.panes[i].ClearInPaneDialog()
					break
				}
			}
			// Value is "0" for "Ask Claude to fix", "1" for "Abort"
			if msg.Value == "0" {
				// User chose to have Claude fix the conflicts
				for i := range m.panes {
					if m.panes[i].Workstream().ID == msg.WorkstreamID {
						m.toast = "Asking Claude to fix conflicts..."
						m.toastExpiry = time.Now().Add(toastDuration)
						m.panes[i].AppendOutput("\nAsking Claude to resolve merge conflicts...\n")

						// Build the prompt for Claude
						fileList := strings.Join(msg.ConflictFiles, ", ")
						prompt := fmt.Sprintf("Please run `git fetch origin main && git rebase origin/main` to start the rebase, then resolve the merge conflicts in these files: %s. After resolving each conflict, run `git add <file>` and then `git rebase --continue`. Let me know when done.", fileList)

						// Send the prompt to Claude with Enter (uses Kitty keyboard protocol)
						_ = m.panes[i].SendToPTYWithEnter(prompt)
						return m, nil
					}
				}
			}
			// User chose to abort - nothing else to do

		case DialogFirstRunIntroduction:
			// Mark introduction as shown and persist
			if err := config.MarkIntroductionShown(); err != nil {
				// Log error but don't fail - user can still use the app
				m.toast = "Note: Could not save introduction state"
				m.toastExpiry = time.Now().Add(toastDuration)
			}
			// Write default security config with documentation
			if err := docker.WriteDefaultGlobalConfig(); err != nil {
				// Log error but don't fail
				m.toast = "Note: Could not write default config"
				m.toastExpiry = time.Now().Add(toastDuration)
			}

		case DialogForcePushConfirm:
			// User typed "force push" - execute force push
			for i := range m.panes {
				if m.panes[i].Workstream().ID == msg.WorkstreamID {
					m.panes[i].ClearInPaneDialog()
					ws := m.panes[i].Workstream()
					m.panes[i].AppendOutput("\nForce pushing branch to origin (--force-with-lease)...\n")
					dialog := NewProgressDialog("Force Pushing Branch", fmt.Sprintf("Branch: %s\n\nForce pushing with --force-with-lease...", ws.BranchName), ws.ID)
					m.panes[i].SetInPaneDialog(&dialog)
					return m, ForcePushBranchCmd(ws)
				}
			}

		case DialogQuitConfirm:
			// User confirmed quit - pause containers and save state
			if len(m.panes) > 0 {
				var workstreams []*workstream.Workstream
				for i := range m.panes {
					ws := m.panes[i].Workstream()
					// Check if Claude was working before closing PTY (need vterm content)
					if m.panes[i].HasPTY() {
						ws.WasInterrupted = m.panes[i].IsClaudeWorking()
						m.panes[i].PTY().Close()
					}
					workstreams = append(workstreams, ws)
				}
				m.quitting = true
				m.manager.Close() // Final flush before quit
				return m, PauseAllAndSaveCmd(m.stateDir, workstreams, m.focusedPane, int(m.layout))
			}
			m.quitting = true
			m.manager.Close() // Final flush before quit
			// Save empty state (no panes) so next startup is clean
			return m, SaveStateAndQuitCmd(m.stateDir, nil, 0, int(m.layout))
		}
		return m, nil

	case ContainerStartedMsg:
		// Container successfully started - now start PTY session
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				ws.SetContainerID(msg.ContainerID)
				m.manager.UpdateWorkstream(ws.ID)
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

	case UntrackedFilesPromptMsg:
		// Untracked files detected - show dialog to ask if user wants to copy them
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				dialog := NewCopyUntrackedFilesDialog(msg.WorkstreamID, msg.UntrackedFiles)
				// Adjust dialog height based on number of files shown
				height := 15
				if len(msg.UntrackedFiles) > 3 {
					height = 18
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

				// If pane is summarizing, derive branch name and check for untracked files
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

					// Check for untracked files before starting container
					m.panes[i].SetInitializing(true)
					m.panes[i].SetInitStatus("Checking for untracked files...")

					return m, CheckUntrackedFilesCmd(ws)
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
					dialog := NewMergeDialog(ws.BranchName, ws.ID, msg.BranchInfo, ws.GetHasBeenPushed(), ws.PRURL)
					m.panes[i].SetInPaneDialog(&dialog)
				} else if msg.HasChanges {
					// Has uncommitted changes - ask if user wants to commit first
					dialog := NewCommitBeforeMergeDialog(ws.BranchName, ws.ID, msg.BranchInfo)
					m.panes[i].SetInPaneDialog(&dialog)
				} else {
					// No uncommitted changes - show merge dialog directly
					dialog := NewMergeDialog(ws.BranchName, ws.ID, msg.BranchInfo, ws.GetHasBeenPushed(), ws.PRURL)
					m.panes[i].SetInPaneDialog(&dialog)
				}
				break
			}
		}
		return m, nil

	case CommitBeforeMergeConfirmMsg:
		// Clear any global dialog and in-pane dialog
		m.dialog = nil
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				m.panes[i].ClearInPaneDialog()
				ws := m.panes[i].Workstream()
				switch msg.Action {
				case CommitBeforeMergeYes:
					// Send /ccells-commit skill to Claude Code in the container (uses Kitty Enter)
					if m.panes[i].HasPTY() {
						m.panes[i].AppendOutput("\nAsking Claude to commit changes...\n")
						_ = m.panes[i].SendToPTYWithEnter("/ccells-commit")
					} else {
						m.panes[i].AppendOutput("\nNo active session to commit.\n")
					}
					// Don't show merge dialog yet - user can press 'm' again after commit
				case CommitBeforeMergeNo:
					// Continue to merge dialog without committing (in-pane)
					dialog := NewMergeDialog(ws.BranchName, ws.ID, msg.BranchInfo, ws.GetHasBeenPushed(), ws.PRURL)
					m.panes[i].SetInPaneDialog(&dialog)
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

	case CopyUntrackedFilesConfirmMsg:
		m.dialog = nil
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				copyFiles := msg.Action == CopyUntrackedFilesYes
				if copyFiles {
					m.panes[i].AppendOutput("Starting container with untracked files...\n")
				} else {
					m.panes[i].AppendOutput("Starting container with clean worktree...\n")
				}
				m.panes[i].SetInitStatus("Starting container...")
				return m, StartContainerWithCopyUntrackedFilesCmd(ws, copyFiles)
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
				// PersistentManager auto-saves state
				m.manager.UpdateWorkstream(ws.ID)
				return m, nil
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
						// Clear scrollback so setup messages don't appear when scrolling up
						m.panes[i].ClearScrollback()
						ws := m.panes[i].Workstream()
						// Start fade animation and auto-enter input mode if focused
						var cmds []tea.Cmd
						if m.panes[i].IsFading() {
							cmds = append(cmds, fadeTickCmd())
						}
						if i == m.focusedPane {
							m.inputMode = true
							cmds = append(cmds, nil)
						}
						// Check if we need to auto-continue an interrupted session
						if ws.WasInterrupted {
							ws.WasInterrupted = false // Clear flag so we don't continue again
							cmds = append(cmds, autoContinueCmd(ws.ID))
						} else if strings.HasPrefix(strings.ToLower(strings.TrimSpace(ws.Prompt)), "continue") {
							// Auto-press enter for prompts that start with "continue"
							cmds = append(cmds, autoPressEnterCmd(ws.ID))
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

	case autoContinueMsg:
		// Auto-continue an interrupted session by sending "continue" to Claude
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				if m.panes[i].HasPTY() {
					// Send "continue" followed by enter to resume the interrupted task (uses Kitty Enter)
					_ = m.panes[i].SendToPTYWithEnter("continue")
				}
				break
			}
		}
		return m, nil

	case autoPressEnterMsg:
		// Auto-press enter for workstreams that started with "continue" as prompt
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				if m.panes[i].HasPTY() {
					// Just send enter to confirm the continue prompt (uses Kitty Enter)
					_ = m.panes[i].SendToPTYWithEnter("")
				}
				break
			}
		}
		return m, nil

	case SessionIDCapturedMsg:
		// Session ID captured from Claude output - store it for future --resume
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				ws.SetClaudeSessionID(msg.SessionID)
				m.manager.UpdateWorkstream(ws.ID) // Auto-persists
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
		// Also clear any in-pane dialog on the focused pane
		if len(m.panes) > 0 && m.focusedPane < len(m.panes) {
			m.panes[m.focusedPane].ClearInPaneDialog()
		}
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
		// Find the workstream and show progress dialog inside the pane
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				switch msg.Action {
				case MergeActionCreatePR:
					m.panes[i].AppendOutput("\nCreating pull request...\n")
					dialog := NewProgressDialog("Creating Pull Request", fmt.Sprintf("Branch: %s\n\nPushing and creating PR...", ws.BranchName), ws.ID)
					m.panes[i].SetInPaneDialog(&dialog)
					return m, CreatePRCmd(ws)
				case MergeActionMergeMain:
					m.panes[i].AppendOutput("\nMerging branch into main (merge commit)...\n")
					dialog := NewProgressDialog("Merging Branch", fmt.Sprintf("Branch: %s\n\nMerging into main (merge commit)...", ws.BranchName), ws.ID)
					m.panes[i].SetInPaneDialog(&dialog)
					return m, MergeBranchCmd(ws)
				case MergeActionSquashMain:
					m.panes[i].AppendOutput("\nMerging branch into main (squash)...\n")
					dialog := NewProgressDialog("Squash Merging Branch", fmt.Sprintf("Branch: %s\n\nSquash merging into main...", ws.BranchName), ws.ID)
					m.panes[i].SetInPaneDialog(&dialog)
					return m, SquashMergeBranchCmd(ws)
				case MergeActionPush:
					m.panes[i].AppendOutput("\nPushing branch to origin...\n")
					dialog := NewProgressDialog("Pushing Branch", fmt.Sprintf("Branch: %s\n\nPushing to origin...", ws.BranchName), ws.ID)
					m.panes[i].SetInPaneDialog(&dialog)
					return m, PushBranchCmd(ws)
				case MergeActionForcePush:
					// Show force push confirmation dialog (requires typing "force push")
					dialog := NewForcePushConfirmDialog(ws.BranchName, ws.ID)
					m.panes[i].SetInPaneDialog(&dialog)
					return m, nil
				}
				break
			}
		}
		return m, nil

	case PushBranchResultMsg:
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				pushType := "Push"
				if msg.ForcePush {
					pushType = "Force push"
				}
				if msg.Error != nil {
					m.panes[i].AppendOutput(fmt.Sprintf("%s failed: %v\n", pushType, msg.Error))
					// Update in-pane progress dialog if open
					if dialog := m.panes[i].GetInPaneDialog(); dialog != nil && dialog.Type == DialogProgress {
						dialog.SetComplete(fmt.Sprintf("%s Failed\n\n%v", pushType, msg.Error))
					}
				} else {
					m.panes[i].AppendOutput(fmt.Sprintf("%s successful!\n", pushType))
					// Mark branch as pushed - this enables force push option in merge dialog
					// and signals to Claude not to use commit amend
					ws.SetHasBeenPushed(true)
					// Notify Claude Code about the push (uses Kitty Enter)
					_ = m.panes[i].SendToPTYWithEnter(fmt.Sprintf("[ccells] ✓ Branch '%s' pushed to remote (avoid using commit --amend)", ws.BranchName))
					// Update in-pane progress dialog if open
					if dialog := m.panes[i].GetInPaneDialog(); dialog != nil && dialog.Type == DialogProgress {
						dialog.SetComplete(fmt.Sprintf("%s successful!\n\nPress Enter or Esc to close.", pushType))
					}
				}
				break
			}
		}
		return m, nil

	case PRCreatedMsg:
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				if msg.Error != nil {
					m.panes[i].AppendOutput(fmt.Sprintf("PR creation failed: %v\n", msg.Error))
					// Update in-pane progress dialog if open
					if dialog := m.panes[i].GetInPaneDialog(); dialog != nil && dialog.Type == DialogProgress {
						dialog.SetComplete(fmt.Sprintf("PR Creation Failed\n\n%v", msg.Error))
					}
				} else {
					m.panes[i].AppendOutput(fmt.Sprintf("PR created: %s\n", msg.PRURL))
					// Store PR info and mark as pushed (PR creation pushes the branch)
					ws.SetPRInfo(msg.PRNumber, msg.PRURL)
					ws.SetHasBeenPushed(true)
					// Notify Claude Code about the PR creation (uses Kitty Enter)
					_ = m.panes[i].SendToPTYWithEnter(fmt.Sprintf("[ccells] ✓ PR #%d created: %s (avoid using commit --amend)", msg.PRNumber, msg.PRURL))
					// Update in-pane progress dialog if open
					if dialog := m.panes[i].GetInPaneDialog(); dialog != nil && dialog.Type == DialogProgress {
						dialog.SetComplete(fmt.Sprintf("Pull Request Created!\n\nPR #%d: %s\n\nPress Enter or Esc to close.", msg.PRNumber, msg.PRURL))
					}
				}
				break
			}
		}
		return m, nil

	case MergeBranchMsg:
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				if msg.Error != nil {
					// Check if this is a conflict error
					if len(msg.ConflictFiles) > 0 {
						m.panes[i].AppendOutput(fmt.Sprintf("Merge conflict: %d files need resolution\n", len(msg.ConflictFiles)))
						// Show merge conflict dialog in pane
						dialog := NewMergeConflictDialog(ws.BranchName, ws.ID, msg.ConflictFiles)
						m.panes[i].SetInPaneDialog(&dialog)
					} else {
						m.panes[i].AppendOutput(fmt.Sprintf("Merge failed: %v\n", msg.Error))
						// Update in-pane progress dialog if open
						if dialog := m.panes[i].GetInPaneDialog(); dialog != nil && dialog.Type == DialogProgress {
							dialog.SetComplete(fmt.Sprintf("Merge Failed\n\n%v", msg.Error))
						}
					}
				} else {
					m.panes[i].AppendOutput("Branch merged into main successfully!\n")
					// Notify Claude Code about the merge (uses Kitty Enter)
					_ = m.panes[i].SendToPTYWithEnter(fmt.Sprintf("[ccells] ✓ Branch '%s' merged into main", ws.BranchName))
					// Show post-merge destroy dialog in pane
					dialog := NewPostMergeDestroyDialog(ws.BranchName, ws.ID)
					m.panes[i].SetInPaneDialog(&dialog)
				}
				break
			}
		}
		return m, nil

	case RebaseBranchMsg:
		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				if msg.Error != nil {
					if len(msg.ConflictFiles) > 0 {
						// Rebase has conflicts - notify Claude to resolve (uses Kitty Enter)
						m.panes[i].AppendOutput("Rebase has conflicts. Resolve in container and run 'git rebase --continue'\n")
						_ = m.panes[i].SendToPTYWithEnter(fmt.Sprintf("[ccells] ⚠ Rebase has conflicts. Please resolve the following files and run 'git rebase --continue': %s", formatFileList(msg.ConflictFiles)))
					} else {
						m.panes[i].AppendOutput(fmt.Sprintf("Rebase failed: %v\n", msg.Error))
					}
				} else {
					m.panes[i].AppendOutput("Rebase successful! Branch is now up to date with main.\n")
					// Notify Claude Code about the rebase (uses Kitty Enter)
					_ = m.panes[i].SendToPTYWithEnter(fmt.Sprintf("[ccells] ✓ Branch '%s' rebased onto main. You can now try merging again.", ws.BranchName))
					m.toast = "Rebase successful"
					m.toastExpiry = time.Now().Add(toastDuration)
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

		// Get state from orchestrator (single source of truth)
		pairingState := m.pairingOrchestrator.GetState()

		for i := range m.panes {
			if m.panes[i].Workstream().ID == msg.WorkstreamID {
				ws := m.panes[i].Workstream()
				m.panes[i].AppendOutput("Pairing mode enabled!\n")
				m.panes[i].AppendOutput(fmt.Sprintf("Local branch: %s\n", ws.BranchName))
				m.panes[i].AppendOutput("Mutagen sync active - changes sync bidirectionally\n")
				if pairingState.StashedChanges {
					m.panes[i].AppendOutput("(Local changes have been stashed)\n")
				}
				// Set workstream state to pairing
				ws.SetState(workstream.StatePairing)
				break
			}
		}

		m.toast = "Pairing mode enabled"
		m.toastExpiry = time.Now().Add(toastDuration)
		// Start health monitoring ticker
		return m, pairingHealthTickCmd()

	case PairingDisabledMsg:
		if msg.Error != nil {
			for i := range m.panes {
				if m.panes[i].Workstream().ID == msg.WorkstreamID {
					m.panes[i].AppendOutput(fmt.Sprintf("Pairing disable error: %v\n", msg.Error))
					break
				}
			}
		}

		// State is managed by orchestrator (already cleared via Disable())

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

		// Check for first run and show introduction dialog
		if config.IsFirstRun() {
			dialog := NewFirstRunIntroductionDialog()
			dialog.SetSize(70, m.height-4)
			m.dialog = &dialog
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
			ws.Title = saved.Title                     // Restore generated title
			ws.ClaudeSessionID = saved.ClaudeSessionID // Restore session ID for --resume
			ws.WasInterrupted = saved.WasInterrupted   // Restore interrupted state for auto-continue
			ws.HasBeenPushed = saved.HasBeenPushed     // Restore push status
			ws.PRNumber = saved.PRNumber               // Restore PR number if created
			ws.PRURL = saved.PRURL                     // Restore PR URL if created
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
			m.setFocusedPane(msg.State.FocusedIndex)
		}
		if len(m.panes) > 0 {
			m.panes[m.focusedPane].SetFocused(true)
		}

		// Restore layout
		m.setLayout(LayoutType(msg.State.Layout))
		m.updateLayout()
		m.toast = fmt.Sprintf("Resumed %d workstream(s)", len(msg.State.Workstreams))
		m.toastExpiry = time.Now().Add(toastDuration)

		// Don't delete state file here - keep it as backup until containers are confirmed running.
		// The state will be overwritten on next save (quit), so stale data is not a concern.
		// This prevents state loss if resume fails (e.g., container errors, session not found).

		return m, tea.Batch(cmds...)

	case StateSavedMsg:
		// State was saved, now quit
		// Print any repair messages to stderr so user sees them after TUI exits
		if msg.RepairMessage != "" {
			fmt.Fprintf(os.Stderr, "\n[ccells] %s\n", msg.RepairMessage)
		}
		if msg.Error != nil {
			fmt.Fprintf(os.Stderr, "[ccells] Failed to save state: %v\n", msg.Error)
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
				m.panes[m.focusedPane], cmd = m.panes[m.focusedPane].Update(tea.KeyPressMsg{Code: tea.KeyEscape})
				return m, cmd
			}
		}
		return m, nil

	case pairingHealthTickMsg:
		// Periodic health check for pairing mode
		if m.pairingOrchestrator.IsActive() {
			// Run health check and schedule next tick
			return m, tea.Batch(
				CheckPairingSyncHealthCmd(m.pairingOrchestrator),
				pairingHealthTickCmd(),
			)
		}
		// Pairing not active, don't schedule another tick
		return m, nil

	case PairingSyncHealthMsg:
		// Handle health check result
		if msg.Error != nil {
			m.toast = fmt.Sprintf("Pairing sync issue: %v", msg.Error)
			m.toastExpiry = time.Now().Add(toastDuration * 2)
		} else if len(msg.Conflicts) > 0 {
			m.toast = fmt.Sprintf("Pairing: %d sync conflict(s)", len(msg.Conflicts))
			m.toastExpiry = time.Now().Add(toastDuration * 2)
		}
		return m, nil
	}

	return m, nil
}

// View renders the application
func (m AppModel) View() tea.View {
	if m.quitting {
		return tea.NewView("Goodbye!\n")
	}

	var sections []string

	// Top title bar
	titleBar := m.renderTitleBar()
	sections = append(sections, titleBar)

	// Panes section
	logPanelH := m.logPanelHeight()
	if len(m.panes) > 0 {
		paneViews := m.renderPanes()
		sections = append(sections, paneViews)
	} else {
		empty := lipgloss.NewStyle().
			Width(m.width).
			Height(m.height-4-logPanelH).
			Align(lipgloss.Center, lipgloss.Center).
			Foreground(lipgloss.Color("#666666")).
			Render("No workstreams. Press [n] to create one.")
		sections = append(sections, empty)
	}

	// Log panel (if visible)
	if m.logPanel != nil && m.logPanel.IsVisible() {
		m.logPanel.SetSize(m.width, logPanelH)
		sections = append(sections, m.logPanel.View())
	}

	// Bottom status bar
	m.statusBar.SetWidth(m.width)
	m.statusBar.SetWorkstreamCount(m.manager.Count())
	m.statusBar.SetInputMode(m.inputMode)
	m.statusBar.SetLayoutName(m.layout.String())
	m.statusBar.SetRepoPath(m.workingDir)
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

	// Create tea.View - basic keyboard enhancements (shift+enter) enabled by default in v2
	v := tea.NewView(view)
	v.AltScreen = true
	// Enable mouse mode for click-to-focus panes (toggleable with Ctrl+B m)
	// Note: Hold modifier key (Option on macOS, Shift elsewhere) while dragging for native text selection
	if m.mouseEnabled {
		v.MouseMode = tea.MouseModeCellMotion
	}
	// Show cursor in input mode at the correct position in the focused pane
	if m.inputMode && m.dialog == nil && len(m.panes) > 0 && m.focusedPane < len(m.panes) {
		pane := m.panes[m.focusedPane]
		cursorPos := pane.GetCursorPosition()
		if cursorPos.Visible {
			// Calculate pane bounds to get absolute screen position
			titleBarHeight := 1
			statusBarHeight := 1
			logPanelH := m.logPanelHeight()
			availableHeight := m.height - titleBarHeight - statusBarHeight - logPanelH
			bounds := CalculatePaneBounds(m.layout, len(m.panes), m.width, availableHeight, titleBarHeight)

			if m.focusedPane < len(bounds) {
				paneBounds := bounds[m.focusedPane]
				// Absolute cursor position = pane position + cursor offset within pane
				v.Cursor = tea.NewCursor(
					paneBounds.X+cursorPos.X,
					paneBounds.Y+cursorPos.Y,
				)
			}
		}
	}
	return v
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

	// Keybinds hint - top bar shows navigation/mode hints (context-sensitive)
	// Bottom status bar shows action keys, so avoid duplication here
	var hints string
	if m.inputMode {
		if scrollIndicator != "" {
			hints = "  " + KeyHint("↑↓/PgUp/Dn", " scroll") + "  " + KeyHint("Esc", " exit scroll")
		} else {
			hints = "  " + KeyHint("Esc Esc", " nav") + "  " + KeyHint("Ctrl+B ←→", " switch pane") + "  " + KeyHint("Ctrl+B PgUp/Dn", " scroll")
		}
	} else {
		if scrollIndicator != "" {
			hints = "  " + KeyHint("↑↓/PgUp/Dn", " scroll") + "  " + KeyHint("Esc", " exit") + "  " + KeyHint("i", " input mode")
		} else {
			hints = "  " + KeyHint("←→", " panes") + "  " + KeyHint("Tab", " cycle") + "  " + KeyHint("1-9", " focus") + "  " + KeyHint("Space", " promote") + "  " + KeyHint("L", " layout") + "  " + KeyHint("i", " input") + "  " + KeyHint("?", " help")
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

	// Sync focus and input mode state on all panes every render
	// This ensures pane state stays consistent with app state
	for i := range m.panes {
		m.panes[i].SetFocused(i == m.focusedPane)
		m.panes[i].SetInputMode(i == m.focusedPane && m.inputMode)
	}

	// Calculate available height for layout rendering
	titleBarHeight := 1
	statusBarHeight := 1
	logPanelH := m.logPanelHeight()
	availableHeight := m.height - titleBarHeight - statusBarHeight - logPanelH

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

	// For destroy dialogs and post-merge destroy dialogs, position over the target pane
	if (m.dialog.Type == DialogDestroy || m.dialog.Type == DialogPostMergeDestroy) && m.dialog.WorkstreamID != "" {
		// Find the pane with this workstream
		titleBarHeight := 1
		statusBarHeight := 1
		logPanelH := m.logPanelHeight()
		availableHeight := m.height - titleBarHeight - statusBarHeight - logPanelH
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

// logPanelHeight returns the height of the log panel (0 if hidden)
func (m *AppModel) logPanelHeight() int {
	if m.logPanel != nil && m.logPanel.IsVisible() {
		if m.logPanel.IsFullscreen() {
			// In fullscreen, take all available space except title bar (1) and status bar (1)
			return m.height - 2
		}
		return DefaultLogPanelHeight
	}
	return 0
}

// updateLayout recalculates pane sizes based on the current layout type
func (m *AppModel) updateLayout() {
	titleBarHeight := 1
	statusBarHeight := 1
	logPanelH := m.logPanelHeight()
	availableHeight := m.height - titleBarHeight - statusBarHeight - logPanelH

	// Update log panel size
	if m.logPanel != nil {
		m.logPanel.SetSize(m.width, logPanelH)
	}

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
func (m *AppModel) Manager() *workstream.PersistentManager {
	return m.manager
}

// InputMode returns true if the app is in input mode
func (m *AppModel) InputMode() bool {
	return m.inputMode
}

// renumberPanes renumbers all panes sequentially starting from 1
// This should be called after destroying a pane to keep indices contiguous
func (m *AppModel) renumberPanes() {
	for i := range m.panes {
		m.panes[i].SetIndex(i + 1) // 1-based indexing
	}
	m.nextPaneIndex = len(m.panes) + 1
	// Reset swap position to avoid invalid indices after destruction
	m.lastSwapPosition = 0
}

// removePane removes a pane at the given index and handles all bookkeeping:
// removes from slice, adjusts focus, renumbers remaining panes, and updates layout.
// Returns the removed workstream so caller can issue StopContainerCmd if needed.
func (m *AppModel) removePane(index int) *workstream.Workstream {
	if index < 0 || index >= len(m.panes) {
		return nil
	}
	ws := m.panes[index].Workstream()
	m.manager.Remove(ws.ID)
	m.panes = append(m.panes[:index], m.panes[index+1:]...)
	if m.focusedPane >= len(m.panes) && len(m.panes) > 0 {
		m.setFocusedPane(len(m.panes) - 1)
	}
	if len(m.panes) > 0 {
		m.panes[m.focusedPane].SetFocused(true)
	}
	m.renumberPanes()
	m.updateLayout()
	return ws
}

// clearAllPanes removes all panes and resets state.
// Closes PTY sessions and removes workstreams from manager.
func (m *AppModel) clearAllPanes() {
	for _, pane := range m.panes {
		if pty := pane.PTY(); pty != nil {
			pty.Close()
		}
		m.manager.Remove(pane.Workstream().ID)
	}
	m.panes = nil
	m.setFocusedPane(0)
	m.renumberPanes()
	m.updateLayout()
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
