package tui

import (
	"context"
	"io"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

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

// PTYOptions holds options for creating a PTY session.
type PTYOptions struct {
	Width    int
	Height   int
	EnvVars  []string // Additional environment variables in "KEY=value" format
	IsResume bool     // If true, use 'claude --continue' instead of starting new session
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
	setupScript := `
echo "[ccells] Starting container setup..."
echo "[ccells] User: $(whoami), Home: $HOME"

# Ensure PATH includes user's local bin directories
export PATH="$HOME/.local/bin:$HOME/.claude/local/bin:$PATH"

# Kill any existing claude CLI processes (from previous sessions that weren't cleaned up)
# This happens when ccells quits - the container stays running but the PTY is orphaned
# Use specific pattern to avoid killing this script (which contains "claude" in paths)
if pgrep -x "claude" >/dev/null 2>&1; then
  echo "[ccells] Killing existing Claude processes from previous session..."
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
  echo "[ccells] Copying credentials from $CREDS_SRC..."
  mkdir -p "$HOME/.claude" 2>/dev/null
  cp "$CREDS_SRC" "$HOME/.claude/.credentials.json" 2>/dev/null
fi

# Copy essential .claude files (NOT the whole directory - it's huge!)
if test -d "/home/claude/.claude" && test "$HOME" != "/home/claude"; then
  echo "[ccells] Copying .claude config from /home/claude..."
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
  # Copy session data for /workspace project (needed for --continue)
  if test -d "/home/claude/.claude/projects/-workspace"; then
    echo "[ccells] Copying session data for --continue..."
    mkdir -p "$HOME/.claude/projects"
    cp -r "/home/claude/.claude/projects/-workspace" "$HOME/.claude/projects/" 2>/dev/null
  fi
fi

# Copy .claude.json (onboarding state, settings) if mounted at /home/claude
if test -f "/home/claude/.claude.json" && test "$HOME" != "/home/claude"; then
  echo "[ccells] Copying .claude.json from /home/claude..."
  cp /home/claude/.claude.json "$HOME/.claude.json" 2>/dev/null || true
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

echo "[ccells] Claude Code found at: $(which claude)"
echo "[ccells] Starting Claude Code..."
`
	var setupCmd string
	if opts != nil && opts.IsResume {
		// Resume existing session with --continue
		setupCmd = setupScript + `exec claude --dangerously-skip-permissions --continue`
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
	if opts != nil && len(opts.EnvVars) > 0 {
		env = append(env, opts.EnvVars...)
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
// It also handles auto-accepting the bypass permissions prompt if it appears.
func (p *PTYSession) StartReadLoop() {
	buf := make([]byte, 4096)

	// For detecting the bypass permissions prompt during startup
	// Always check - even with --continue, fresh containers show the prompt
	var accumulated strings.Builder
	bypassHandled := false
	startTime := time.Now()
	const bypassTimeout = 10 * time.Second

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
				// Check for bypass permissions prompt during startup (first 10 seconds)
				if !bypassHandled && time.Since(startTime) < bypassTimeout {
					accumulated.Write(buf[:result.n])
					content := accumulated.String()
					if strings.Contains(content, "Bypass Permissions mode") {
						// Wait a moment for the full prompt to render
						time.Sleep(100 * time.Millisecond)
						// Send down arrow to select "Yes, I accept"
						p.Write([]byte{27, '[', 'B'})
						time.Sleep(50 * time.Millisecond)
						// Send enter to confirm
						p.Write([]byte{'\r'})
						bypassHandled = true
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
