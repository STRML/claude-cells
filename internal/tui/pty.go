package tui

import (
	"context"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// sessionIDRegex matches Claude Code session IDs in output.
// Claude outputs session ID at startup like: "Resuming session: xxxxxxxx-xxxx-..."
// Session IDs can be UUIDs (36 chars with dashes) or ULIDs (26 alphanumeric chars).
// UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
// ULID format: 26 alphanumeric characters
var sessionIDRegex = regexp.MustCompile(`(?:session(?:[_\s]?id)?[:\s]+|Resuming session[:\s]+)([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}|[0-9A-Za-z]{26})`)

// escapeShellArg escapes a string for safe use in a shell command.
// It handles special characters that could break out of double-quoted strings
// or cause command injection.
func escapeShellArg(s string) string {
	// Remove null bytes entirely - they can't be safely escaped
	s = strings.ReplaceAll(s, "\x00", "")

	// Escape backslashes first (must be done before other escapes)
	s = strings.ReplaceAll(s, `\`, `\\`)

	// Escape double quotes
	s = strings.ReplaceAll(s, `"`, `\"`)

	// Escape dollar signs (variable expansion)
	s = strings.ReplaceAll(s, `$`, `\$`)

	// Escape backticks (command substitution)
	s = strings.ReplaceAll(s, "`", "\\`")

	// Escape newlines - convert to escaped form that won't break the command
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)

	return s
}

// program holds the tea.Program reference for sending messages from goroutines
var program *tea.Program

// SetProgram sets the program reference for PTY sessions to use
func SetProgram(p *tea.Program) {
	program = p
}

// PTYSession manages a PTY session inside a Docker container.
type PTYSession struct {
	containerID  string
	execID       string
	conn         *types.HijackedResponse
	stdin        io.WriteCloser
	mu           sync.Mutex
	closed       bool
	done         chan struct{} // Signals goroutines to exit (legacy, kept for Done() method)
	ctx          context.Context
	cancel       context.CancelFunc
	workstreamID string
	dockerClient *client.Client
	width        int
	height       int
}

// PTYOutputMsg is sent when there's output from the PTY.
type PTYOutputMsg struct {
	WorkstreamID string
	Output       []byte
}

// PTYClosedMsg is sent when the PTY session closes.
type PTYClosedMsg struct {
	WorkstreamID string
	Error        error
}

// SessionIDCapturedMsg is sent when a Claude session ID is captured from output.
type SessionIDCapturedMsg struct {
	WorkstreamID string
	SessionID    string
}

// PTYOptions holds options for creating a PTY session.
type PTYOptions struct {
	Width           int
	Height          int
	EnvVars         []string // Additional environment variables in "KEY=value" format
	IsResume        bool     // If true, use 'claude --resume' instead of starting new session
	ClaudeSessionID string   // Claude session ID for --resume (if available)
	HostProjectPath string   // Host project path for finding session data (encoded for .claude/projects/)
}

// NewPTYSession creates a new PTY session for running Claude Code in a container.
func NewPTYSession(ctx context.Context, dockerClient *client.Client, containerID, workstreamID, initialPrompt string, opts *PTYOptions) (*PTYSession, error) {
	// Default terminal size
	width := 80
	height := 24
	if opts != nil {
		if opts.Width > 0 {
			width = opts.Width
		}
		if opts.Height > 0 {
			height = opts.Height
		}
	}

	// Build the command to run Claude Code
	// Setup steps:
	// 1. Copy credentials file to expected location (uses $HOME for portability)
	// 2. Create ~/.local/bin and symlink claude if needed (suppresses "native install" warning)
	// 3. If claude is not installed, install it via npm (for devcontainer images without claude)
	// 4. Run claude with --dangerously-skip-permissions since we're in an isolated container
	//    - If resuming, use --continue to resume the previous session
	//    - If new session with prompt, pass the prompt as argument
	//
	// On resume (IS_RESUME=1), the script is quieter since container is already configured.
	setupScript := `
# Helper function for logging (quiet on resume)
log() {
  test -z "$IS_RESUME" && echo "[ccells] $1"
}

log "Starting container setup..."
log "User: $(whoami), Home: $HOME"

# Ensure PATH includes user's local bin directories
export PATH="$HOME/.local/bin:$HOME/.claude/local/bin:$PATH"

# Kill any existing claude CLI processes (from previous sessions that weren't cleaned up)
# This happens when ccells quits - the container stays running but the PTY is orphaned
# Use specific pattern to avoid killing this script (which contains "claude" in paths)
if pgrep -x "claude" >/dev/null 2>&1; then
  log "Killing existing Claude processes from previous session..."
  pkill -9 -x "claude" 2>/dev/null
  sleep 1
fi

# Setup credentials - check both $HOME and /home/claude (for devcontainers running as root)
CREDS_SRC=""
if test -f "$HOME/.claude-credentials"; then
  CREDS_SRC="$HOME/.claude-credentials"
elif test -f "/home/claude/.claude-credentials"; then
  CREDS_SRC="/home/claude/.claude-credentials"
fi

if test -n "$CREDS_SRC"; then
  log "Copying credentials from $CREDS_SRC..."
  mkdir -p "$HOME/.claude" 2>/dev/null
  cp "$CREDS_SRC" "$HOME/.claude/.credentials.json" 2>/dev/null
fi

# Copy essential .claude files (NOT the whole directory - it's huge!)
if test -d "/home/claude/.claude" && test "$HOME" != "/home/claude"; then
  log "Copying .claude config from /home/claude..."
  # Only copy essential config files, not cache/telemetry/shell-snapshots
  for f in settings.json CLAUDE.md statsig; do
    test -e "/home/claude/.claude/$f" && cp -r "/home/claude/.claude/$f" "$HOME/.claude/" 2>/dev/null
  done
  # Copy plugins config if it exists
  if test -d "/home/claude/.claude/plugins"; then
    mkdir -p "$HOME/.claude/plugins"
    test -f "/home/claude/.claude/plugins/installed_plugins.json" && \
      cp "/home/claude/.claude/plugins/installed_plugins.json" "$HOME/.claude/plugins/" 2>/dev/null
  fi
  # Copy session data for --continue
  # Session data on host is stored under encoded HOST path (e.g., -Users-samuelreed-git-oss-docker-tui)
  # But container runs in /workspace, so Claude looks for -workspace
  # We need to copy from host-encoded path to -workspace
  if test -n "$HOST_PROJECT_PATH"; then
    # Encode host path: replace / with -
    HOST_ENCODED=$(echo "$HOST_PROJECT_PATH" | sed 's|/|-|g')
    HOST_SESSION_DIR="/home/claude/.claude/projects/$HOST_ENCODED"
    if test -d "$HOST_SESSION_DIR"; then
      log "Copying session data from $HOST_ENCODED to -workspace..."
      mkdir -p "$HOME/.claude/projects/-workspace"
      cp -r "$HOST_SESSION_DIR"/* "$HOME/.claude/projects/-workspace/" 2>/dev/null
    else
      log "No session data found at $HOST_SESSION_DIR"
    fi
  fi
fi

# Copy .claude.json (onboarding state, settings) if mounted at /home/claude
if test -f "/home/claude/.claude.json" && test "$HOME" != "/home/claude"; then
  log "Copying .claude.json from /home/claude..."
  cp /home/claude/.claude.json "$HOME/.claude.json" 2>/dev/null || true
fi

# Copy .gitconfig for git user identity (name/email) - this is critical!
# Without this, commits show as "Claude" instead of the actual user
if test -f "/home/claude/.gitconfig" && test "$HOME" != "/home/claude"; then
  log "Copying .gitconfig from /home/claude..."
  cp /home/claude/.gitconfig "$HOME/.gitconfig" 2>/dev/null || true
fi

mkdir -p "$HOME/.local/bin" 2>/dev/null
test ! -f "$HOME/.local/bin/claude" && which claude >/dev/null 2>&1 && ln -sf "$(which claude)" "$HOME/.local/bin/claude" 2>/dev/null

# Install Claude Code if not available
if ! which claude >/dev/null 2>&1; then
  echo "[ccells] Claude Code not found, installing..."
  curl -fsSL https://claude.ai/install.sh | bash 2>&1
  export PATH="$HOME/.claude/local/bin:$PATH"
  if ! which claude >/dev/null 2>&1; then
    echo "[ccells] ERROR: Claude Code installation failed!"
    echo "[ccells] Checking install location..."
    ls -la "$HOME/.claude/local/bin/" 2>&1 || echo "[ccells] Install dir not found"
    exit 1
  fi
fi

log "Claude Code found at: $(which claude)"
log "Starting Claude Code..."
`
	var setupCmd string
	if opts != nil && opts.IsResume {
		// Resume existing session
		if opts.ClaudeSessionID != "" {
			// Use --resume with explicit session ID (preferred)
			setupCmd = setupScript + `exec claude --dangerously-skip-permissions --resume "` + escapeShellArg(opts.ClaudeSessionID) + `"`
		} else {
			// Fall back to --continue if no session ID available
			setupCmd = setupScript + `exec claude --dangerously-skip-permissions --continue`
		}
	} else if initialPrompt != "" {
		// New session with initial prompt
		setupCmd = setupScript + `exec claude --dangerously-skip-permissions "` + escapeShellArg(initialPrompt) + `"`
	} else {
		// New session without prompt
		setupCmd = setupScript + `exec claude --dangerously-skip-permissions`
	}
	cmd := []string{"/bin/bash", "-c", setupCmd}

	// Build environment variables
	// Don't hardcode HOME or user-specific paths - let the container determine them
	// IS_SANDBOX=1 tells Claude Code it's in a sandboxed container (allows --dangerously-skip-permissions as root)
	env := []string{
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"IS_SANDBOX=1",
	}
	if opts != nil {
		if len(opts.EnvVars) > 0 {
			env = append(env, opts.EnvVars...)
		}
		// Pass host project path for session data copying
		if opts.HostProjectPath != "" {
			env = append(env, "HOST_PROJECT_PATH="+opts.HostProjectPath)
		}
		// Pass IS_RESUME to make setup script quieter on resume
		if opts.IsResume {
			env = append(env, "IS_RESUME=1")
		}
	}

	// Create exec with TTY and terminal size
	// Don't specify User - let Docker use the container's default user
	// (base.Dockerfile sets USER claude, devcontainers typically use vscode/node)
	execCfg := container.ExecOptions{
		Cmd:          cmd,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		WorkingDir:   "/workspace",
		Env:          env,
		ConsoleSize:  &[2]uint{uint(height), uint(width)},
	}

	execResp, err := dockerClient.ContainerExecCreate(ctx, containerID, execCfg)
	if err != nil {
		return nil, err
	}

	// Attach to the exec session
	attachResp, err := dockerClient.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{
		Tty:         true,
		ConsoleSize: &[2]uint{uint(height), uint(width)},
	})
	if err != nil {
		return nil, err
	}

	// Create a cancellable context for this session
	sessionCtx, sessionCancel := context.WithCancel(context.Background())

	session := &PTYSession{
		containerID:  containerID,
		execID:       execResp.ID,
		conn:         &attachResp,
		stdin:        attachResp.Conn,
		done:         make(chan struct{}),
		ctx:          sessionCtx,
		cancel:       sessionCancel,
		workstreamID: workstreamID,
		dockerClient: dockerClient,
		width:        width,
		height:       height,
	}

	// Start the read loop immediately - it will handle both:
	// 1. Forwarding output to the pane
	// 2. Auto-accepting the bypass permissions prompt if it appears
	go session.StartReadLoop()

	return session, nil
}

// Resize changes the terminal size of the PTY session.
func (p *PTYSession) Resize(width, height int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed || p.dockerClient == nil {
		return nil
	}

	p.width = width
	p.height = height

	// Use a short timeout for resize - it should be fast
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return p.dockerClient.ContainerExecResize(ctx, p.execID, container.ResizeOptions{
		Height: uint(height),
		Width:  uint(width),
	})
}

// StartReadLoop starts reading from the PTY and sending output messages.
// This should be called in a goroutine. Uses context-based cancellation
// to avoid race conditions with Close().
// It also handles auto-accepting the bypass permissions prompt if it appears,
// and captures the Claude session ID from output.
func (p *PTYSession) StartReadLoop() {
	buf := make([]byte, 4096)

	// For detecting the bypass permissions prompt during startup
	// Always check - even with --continue, fresh containers show the prompt
	var accumulated strings.Builder
	bypassHandled := false
	sessionIDCaptured := false
	startTime := time.Now()
	const bypassTimeout = 10 * time.Second
	const sessionIDTimeout = 30 * time.Second // Session ID may appear later in output

	for {
		// Check context first (non-blocking)
		select {
		case <-p.ctx.Done():
			return
		default:
		}

		// Get connection reference under lock
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return
		}
		conn := p.conn
		p.mu.Unlock()

		if conn == nil {
			return
		}

		// Wrap read in a goroutine so we can select on context cancellation.
		// This prevents the race condition where Close() is called while
		// we're blocked on Read().
		type readResult struct {
			n   int
			err error
		}
		readCh := make(chan readResult, 1)

		go func() {
			n, err := conn.Reader.Read(buf)
			readCh <- readResult{n, err}
		}()

		// Wait for either read completion or context cancellation
		select {
		case <-p.ctx.Done():
			// Context cancelled (Close() was called)
			// The read goroutine will eventually complete and the channel
			// will be garbage collected. This is acceptable since we're shutting down.
			return
		case result := <-readCh:
			if result.err != nil {
				// Check context before sending error message
				// This prevents sending spurious error messages during shutdown
				select {
				case <-p.ctx.Done():
					return
				default:
				}

				if program != nil {
					program.Send(PTYClosedMsg{
						WorkstreamID: p.workstreamID,
						Error:        result.err,
					})
				}
				return
			}

			if result.n > 0 {
				// Accumulate output during startup period for pattern matching
				needsAccumulation := (!bypassHandled && time.Since(startTime) < bypassTimeout) ||
					(!sessionIDCaptured && time.Since(startTime) < sessionIDTimeout)

				if needsAccumulation {
					accumulated.Write(buf[:result.n])
					content := accumulated.String()

					// Check for bypass permissions prompt during startup (first 10 seconds)
					// Claude Code may show different prompts:
					// 1. "Bypass Permissions mode" - the full bypass dialog (needs down arrow + enter)
					// 2. "Enter to confirm" - a simple confirmation prompt (just needs enter)
					// 3. "Resume Session" picker - shown when --resume can't find the session (press Escape)
					if !bypassHandled && time.Since(startTime) < bypassTimeout {
						if strings.Contains(content, "Bypass Permissions mode") {
							// Wait a moment for the full prompt to render
							time.Sleep(100 * time.Millisecond)
							// Send down arrow to select "Yes, I accept"
							p.Write([]byte{27, '[', 'B'})
							time.Sleep(50 * time.Millisecond)
							// Send enter to confirm
							p.Write([]byte{'\r'})
							bypassHandled = true
						} else if strings.Contains(content, "Enter to confirm") {
							// Simple confirmation prompt - just send enter
							time.Sleep(100 * time.Millisecond)
							p.Write([]byte{'\r'})
							bypassHandled = true
						} else if strings.Contains(content, "Resume Session") {
							// Session picker appeared - the specified session ID wasn't found
							// Press Escape to cancel and start a fresh session
							time.Sleep(100 * time.Millisecond)
							p.Write([]byte{27}) // Escape key
							bypassHandled = true
						}
					}

					// Check for session ID in output (within first 30 seconds)
					if !sessionIDCaptured && time.Since(startTime) < sessionIDTimeout {
						if matches := sessionIDRegex.FindStringSubmatch(content); len(matches) > 1 {
							sessionID := matches[1]
							sessionIDCaptured = true
							if program != nil {
								program.Send(SessionIDCapturedMsg{
									WorkstreamID: p.workstreamID,
									SessionID:    sessionID,
								})
							}
						}
					}
				}

				if program != nil {
					// Make a copy of the buffer to send
					output := make([]byte, result.n)
					copy(output, buf[:result.n])
					program.Send(PTYOutputMsg{
						WorkstreamID: p.workstreamID,
						Output:       output,
					})
				}
			}
		}
	}
}

// Write sends input to the PTY.
func (p *PTYSession) Write(data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed || p.stdin == nil {
		return io.ErrClosedPipe
	}

	_, err := p.stdin.Write(data)
	return err
}

// WriteString sends a string to the PTY.
func (p *PTYSession) WriteString(s string) error {
	return p.Write([]byte(s))
}

// SendKey sends a key to the PTY.
func (p *PTYSession) SendKey(key string) error {
	return p.WriteString(key)
}

// Close closes the PTY session and releases all resources.
// Thread-safe: can be called concurrently with StartReadLoop.
func (p *PTYSession) Close() error {
	// Cancel context first to signal goroutines to exit.
	// This must be done BEFORE acquiring the lock to avoid deadlock
	// with StartReadLoop which holds a reference to conn.
	if p.cancel != nil {
		p.cancel()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true

	// Signal goroutines via done channel (legacy, for Done() method)
	if p.done != nil {
		close(p.done)
	}

	// Close the connection - any blocked Read() will return with error
	if p.conn != nil {
		p.conn.Close()
	}

	// Close the Docker client to avoid resource leak
	if p.dockerClient != nil {
		p.dockerClient.Close()
		p.dockerClient = nil
	}

	return nil
}

// IsClosed returns true if the session is closed.
func (p *PTYSession) IsClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

// Done returns a channel that's closed when the session is closed.
// This can be used to wait for the session to end.
func (p *PTYSession) Done() <-chan struct{} {
	return p.done
}
