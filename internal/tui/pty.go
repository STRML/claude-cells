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
	done         chan struct{} // Signals goroutines to exit
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
	Width   int
	Height  int
	EnvVars []string // Additional environment variables in "KEY=value" format
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
	// 1. Copy credentials file to expected location
	// 2. Create ~/.local/bin and symlink claude if needed (suppresses "native install" warning)
	// 3. Run claude with --dangerously-skip-permissions since we're in an isolated container
	setupScript := `
test -f /home/claude/.claude-credentials && cp /home/claude/.claude-credentials /home/claude/.claude/.credentials.json 2>/dev/null
mkdir -p /home/claude/.local/bin 2>/dev/null
test ! -f /home/claude/.local/bin/claude && which claude >/dev/null 2>&1 && ln -sf "$(which claude)" /home/claude/.local/bin/claude 2>/dev/null
`
	setupCmd := setupScript + `exec claude --dangerously-skip-permissions`
	if initialPrompt != "" {
		setupCmd = setupScript + `exec claude --dangerously-skip-permissions "` + escapeShellArg(initialPrompt) + `"`
	}
	cmd := []string{"/bin/bash", "-c", setupCmd}

	// Build environment variables
	env := []string{
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"PATH=/home/claude/.local/bin:/usr/local/bin:/usr/bin:/bin",
		"HOME=/home/claude",
	}
	if opts != nil && len(opts.EnvVars) > 0 {
		env = append(env, opts.EnvVars...)
	}

	// Create exec with TTY and terminal size
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

	session := &PTYSession{
		containerID:  containerID,
		execID:       execResp.ID,
		conn:         &attachResp,
		stdin:        attachResp.Conn,
		done:         make(chan struct{}),
		workstreamID: workstreamID,
		dockerClient: dockerClient,
		width:        width,
		height:       height,
	}

	// Auto-accept the bypass permissions prompt
	// Read output until we see the prompt, then send down+enter to accept
	if err := session.autoAcceptBypassPermissions(); err != nil {
		// Log but don't fail - the prompt may not appear
		// (e.g., if already accepted or different Claude version)
	}

	return session, nil
}

// autoAcceptBypassPermissions reads PTY output looking for the bypass permissions
// prompt and automatically accepts it by sending down arrow + enter.
func (p *PTYSession) autoAcceptBypassPermissions() error {
	// Buffer to accumulate output
	var accumulated strings.Builder
	buf := make([]byte, 1024)
	timeout := time.After(10 * time.Second)

	for {
		select {
		case <-timeout:
			// Timeout - prompt may not have appeared, continue anyway
			return nil
		default:
		}

		// Set a short read deadline to avoid blocking forever
		// Note: Docker hijacked connections don't support SetReadDeadline,
		// so we use a goroutine with channel instead
		readDone := make(chan struct {
			n   int
			err error
		}, 1)

		go func() {
			n, err := p.conn.Reader.Read(buf)
			readDone <- struct {
				n   int
				err error
			}{n, err}
		}()

		select {
		case <-timeout:
			return nil
		case result := <-readDone:
			if result.err != nil {
				return result.err
			}
			if result.n > 0 {
				accumulated.Write(buf[:result.n])
				content := accumulated.String()

				// Check if we see the bypass permissions prompt
				if strings.Contains(content, "Bypass Permissions mode") {
					// Wait a moment for the full prompt to render
					time.Sleep(100 * time.Millisecond)

					// Send down arrow to select "Yes, I accept"
					p.Write([]byte{27, '[', 'B'}) // Down arrow
					time.Sleep(50 * time.Millisecond)

					// Send enter to confirm
					p.Write([]byte{'\r'})

					// Don't forward the permissions dialog to the pane - discard it
					// The pane will start fresh after permissions are accepted
					return nil
				}
			}
		}
	}
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
// This should be called in a goroutine.
func (p *PTYSession) StartReadLoop() {
	buf := make([]byte, 4096)
	for {
		// Check if we should exit via the done channel (non-blocking)
		select {
		case <-p.done:
			return
		default:
		}

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

		n, err := conn.Reader.Read(buf)
		if err != nil {
			// Check done channel before sending error message
			// This prevents sending spurious error messages during shutdown
			select {
			case <-p.done:
				return
			default:
			}

			if program != nil {
				program.Send(PTYClosedMsg{
					WorkstreamID: p.workstreamID,
					Error:        err,
				})
			}
			return
		}

		if n > 0 && program != nil {
			// Make a copy of the buffer to send
			output := make([]byte, n)
			copy(output, buf[:n])
			program.Send(PTYOutputMsg{
				WorkstreamID: p.workstreamID,
				Output:       output,
			})
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

// Close closes the PTY session.
func (p *PTYSession) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true

	// Signal goroutines to exit
	if p.done != nil {
		close(p.done)
	}

	if p.conn != nil {
		p.conn.Close()
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
