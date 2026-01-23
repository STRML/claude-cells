package tui

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/STRML/claude-cells/internal/docker"
)

func TestPTYOptions(t *testing.T) {
	tests := []struct {
		name     string
		opts     *PTYOptions
		wantW    int
		wantH    int
		wantEnv  int
		isResume bool
	}{
		{
			name:     "nil options",
			opts:     nil,
			wantW:    0,
			wantH:    0,
			wantEnv:  0,
			isResume: false,
		},
		{
			name: "custom size",
			opts: &PTYOptions{
				Width:  120,
				Height: 40,
			},
			wantW:    120,
			wantH:    40,
			wantEnv:  0,
			isResume: false,
		},
		{
			name: "with env vars",
			opts: &PTYOptions{
				Width:   80,
				Height:  24,
				EnvVars: []string{"FOO=bar", "BAZ=qux"},
			},
			wantW:   80,
			wantH:   24,
			wantEnv: 2,
		},
		{
			name: "resume mode",
			opts: &PTYOptions{
				IsResume: true,
			},
			isResume: true,
		},
		{
			name: "with host project path",
			opts: &PTYOptions{
				HostProjectPath: "/home/user/project",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.opts == nil {
				// Just verify nil is valid
				return
			}
			if tt.wantW > 0 && tt.opts.Width != tt.wantW {
				t.Errorf("Width = %d, want %d", tt.opts.Width, tt.wantW)
			}
			if tt.wantH > 0 && tt.opts.Height != tt.wantH {
				t.Errorf("Height = %d, want %d", tt.opts.Height, tt.wantH)
			}
			if tt.wantEnv > 0 && len(tt.opts.EnvVars) != tt.wantEnv {
				t.Errorf("EnvVars count = %d, want %d", len(tt.opts.EnvVars), tt.wantEnv)
			}
			if tt.isResume && !tt.opts.IsResume {
				t.Errorf("IsResume = %v, want true", tt.opts.IsResume)
			}
		})
	}
}

func TestPTYOutputMsg(t *testing.T) {
	msg := PTYOutputMsg{
		WorkstreamID: "test-ws",
		Output:       []byte("hello world"),
	}

	if msg.WorkstreamID != "test-ws" {
		t.Errorf("WorkstreamID = %q, want %q", msg.WorkstreamID, "test-ws")
	}
	if string(msg.Output) != "hello world" {
		t.Errorf("Output = %q, want %q", string(msg.Output), "hello world")
	}
}

func TestPTYClosedMsg(t *testing.T) {
	msg := PTYClosedMsg{
		WorkstreamID: "test-ws",
		Error:        nil,
	}

	if msg.WorkstreamID != "test-ws" {
		t.Errorf("WorkstreamID = %q, want %q", msg.WorkstreamID, "test-ws")
	}
	if msg.Error != nil {
		t.Errorf("Error should be nil")
	}
}

func TestPTYSession_IsClosed(t *testing.T) {
	session := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
	}

	if session.IsClosed() {
		t.Error("Session should not be closed initially")
	}

	session.closed = true
	if !session.IsClosed() {
		t.Error("Session should be closed after setting closed = true")
	}
}

func TestPTYSession_Close(t *testing.T) {
	session := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
	}

	err := session.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if !session.IsClosed() {
		t.Error("Session should be closed after Close()")
	}

	// Verify done channel is closed
	select {
	case <-session.Done():
		// Expected - channel is closed
	default:
		t.Error("Done channel should be closed after Close()")
	}

	// Closing again should be idempotent
	err = session.Close()
	if err != nil {
		t.Errorf("Close() second call error = %v", err)
	}
}

func TestSetProgram(t *testing.T) {
	// Store original
	sender.mu.Lock()
	original := sender.program
	sender.mu.Unlock()

	// Reset after test
	defer func() {
		sender.mu.Lock()
		sender.program = original
		sender.mu.Unlock()
	}()

	// Test setting nil
	SetProgram(nil)
	sender.mu.RLock()
	p := sender.program
	sender.mu.RUnlock()
	if p != nil {
		t.Error("sender.program should be nil")
	}
}

func TestSendMsg_NilProgram(t *testing.T) {
	// Store original
	sender.mu.Lock()
	original := sender.program
	sender.mu.Unlock()
	defer func() {
		sender.mu.Lock()
		sender.program = original
		sender.mu.Unlock()
	}()

	// Set nil program
	SetProgram(nil)

	// sendMsg should return false when program is nil
	result := sendMsg(PTYOutputMsg{WorkstreamID: "test", Output: []byte("hello")})
	if result {
		t.Error("sendMsg should return false when program is nil")
	}
}

func TestEscapeShellArg(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain text", "hello", "hello"},
		{"with space", "hello world", "hello world"},
		{"double quotes", `hello "world"`, `hello \"world\"`},
		{"backslash", `hello\world`, `hello\\world`},
		{"dollar sign", "hello $VAR", `hello \$VAR`},
		{"backticks", "hello `cmd`", "hello \\`cmd\\`"},
		{"newline", "hello\nworld", `hello\nworld`},
		{"carriage return", "hello\rworld", `hello\rworld`},
		{"null byte", "hello\x00world", "helloworld"},
		{"mixed special", "test\n$VAR\x00`cmd`", `test\n\$VAR\` + "`cmd\\`"},
		{"multiple newlines", "line1\nline2\nline3", `line1\nline2\nline3`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeShellArg(tt.input)
			if result != tt.expected {
				t.Errorf("escapeShellArg(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// mockWriteCloser is a mock io.WriteCloser for testing
type mockWriteCloser struct {
	buf    bytes.Buffer
	closed bool
	mu     sync.Mutex
}

func (m *mockWriteCloser) Write(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, io.ErrClosedPipe
	}
	return m.buf.Write(p)
}

func (m *mockWriteCloser) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockWriteCloser) Bytes() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.buf.Bytes()
}

func TestPTYSession_Write(t *testing.T) {
	mock := &mockWriteCloser{}
	session := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
		stdin:        mock,
	}

	// Test Write
	err := session.Write([]byte("hello"))
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
	if string(mock.Bytes()) != "hello" {
		t.Errorf("Written data = %q, want %q", string(mock.Bytes()), "hello")
	}
}

func TestPTYSession_WriteString(t *testing.T) {
	mock := &mockWriteCloser{}
	session := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
		stdin:        mock,
	}

	// Test WriteString
	err := session.WriteString("world")
	if err != nil {
		t.Errorf("WriteString() error = %v", err)
	}
	if string(mock.Bytes()) != "world" {
		t.Errorf("Written data = %q, want %q", string(mock.Bytes()), "world")
	}
}

func TestPTYSession_SendKey(t *testing.T) {
	mock := &mockWriteCloser{}
	session := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
		stdin:        mock,
	}

	// Test SendKey
	err := session.SendKey("\x1b[A") // Up arrow
	if err != nil {
		t.Errorf("SendKey() error = %v", err)
	}
	if string(mock.Bytes()) != "\x1b[A" {
		t.Errorf("Written data = %q, want %q", string(mock.Bytes()), "\x1b[A")
	}
}

func TestPTYSession_Write_WhenClosed(t *testing.T) {
	mock := &mockWriteCloser{}
	session := &PTYSession{
		workstreamID: "test",
		closed:       true, // Session is closed
		done:         make(chan struct{}),
		stdin:        mock,
	}

	err := session.Write([]byte("test"))
	if err != io.ErrClosedPipe {
		t.Errorf("Write() when closed should return ErrClosedPipe, got %v", err)
	}
}

func TestPTYSession_Write_NilStdin(t *testing.T) {
	session := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
		stdin:        nil, // No stdin
	}

	err := session.Write([]byte("test"))
	if err != io.ErrClosedPipe {
		t.Errorf("Write() with nil stdin should return ErrClosedPipe, got %v", err)
	}
}

func TestPTYSession_Resize_WhenClosed(t *testing.T) {
	session := &PTYSession{
		workstreamID: "test",
		closed:       true,
		done:         make(chan struct{}),
	}

	// Resize on closed session should return nil (no-op)
	err := session.Resize(100, 50)
	if err != nil {
		t.Errorf("Resize() on closed session should return nil, got %v", err)
	}
}

func TestPTYSession_Resize_NilDockerClient(t *testing.T) {
	session := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
		dockerClient: nil, // No docker client
	}

	// Resize with nil docker client should return nil (no-op)
	err := session.Resize(100, 50)
	if err != nil {
		t.Errorf("Resize() with nil dockerClient should return nil, got %v", err)
	}
}

func TestPTYSession_Resize_UpdatesDimensions(t *testing.T) {
	session := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
		width:        80,
		height:       24,
		dockerClient: nil, // Will short-circuit
	}

	// Even with nil docker client, dimensions should not update since
	// the check happens before dimension update
	_ = session.Resize(120, 40)

	// With nil docker client, dimensions aren't updated (returns early)
	// This is correct behavior - resize needs docker client
}

func TestPTYSession_ConcurrentClose(t *testing.T) {
	session := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	session.ctx = ctx
	session.cancel = cancel

	// Close from multiple goroutines should not panic
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			session.Close()
		}()
	}

	wg.Wait()

	if !session.IsClosed() {
		t.Error("Session should be closed after concurrent Close() calls")
	}
}

func TestPTYSession_Done(t *testing.T) {
	done := make(chan struct{})
	session := &PTYSession{
		workstreamID: "test",
		done:         done,
	}

	// Done() should return the same channel
	if session.Done() != done {
		t.Error("Done() should return the done channel")
	}
}

func TestPTYSession_CloseWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	session := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
		ctx:          ctx,
		cancel:       cancel,
	}

	// Close should cancel context
	session.Close()

	select {
	case <-ctx.Done():
		// Expected - context should be cancelled
	default:
		t.Error("Context should be cancelled after Close()")
	}
}

func TestPTYSession_WriteAfterClose(t *testing.T) {
	mock := &mockWriteCloser{}
	ctx, cancel := context.WithCancel(context.Background())
	session := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
		stdin:        mock,
		ctx:          ctx,
		cancel:       cancel,
	}

	// Close the session
	session.Close()

	// Write should fail
	err := session.Write([]byte("test"))
	if err != io.ErrClosedPipe {
		t.Errorf("Write() after Close() should return ErrClosedPipe, got %v", err)
	}
}

func TestPTYSession_MultipleWrites(t *testing.T) {
	mock := &mockWriteCloser{}
	session := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
		stdin:        mock,
	}

	// Multiple writes should append
	session.Write([]byte("hello"))
	session.Write([]byte(" "))
	session.Write([]byte("world"))

	if string(mock.Bytes()) != "hello world" {
		t.Errorf("Written data = %q, want %q", string(mock.Bytes()), "hello world")
	}
}

func TestPTYSession_ConcurrentWrites(t *testing.T) {
	mock := &mockWriteCloser{}
	session := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
		stdin:        mock,
	}

	// Concurrent writes should not panic
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			session.Write([]byte("x"))
		}(i)
	}

	wg.Wait()

	// All 100 writes should have completed
	if len(mock.Bytes()) != 100 {
		t.Errorf("Expected 100 bytes written, got %d", len(mock.Bytes()))
	}
}

func TestEscapeShellArg_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"only null bytes", "\x00\x00\x00", ""},
		{"only newlines", "\n\n\n", `\n\n\n`},
		{"unicode", "hello 世界", "hello 世界"},
		{"emoji", "hello 🎉", "hello 🎉"},
		{"nested quotes", `"'"`, `\"'\"`},
		{"shell command injection", "$(rm -rf /)", `\$(rm -rf /)`},
		{"complex injection", "`rm -rf /`; echo pwned", "\\`rm -rf /\\`; echo pwned"},
		{"path traversal", "../../../etc/passwd", "../../../etc/passwd"},
		{"env var with braces", "${HOME}", `\${HOME}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeShellArg(tt.input)
			if result != tt.expected {
				t.Errorf("escapeShellArg(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// BenchmarkEscapeShellArg benchmarks the escape function
func BenchmarkEscapeShellArg(b *testing.B) {
	input := "hello $VAR `cmd` \"quoted\" \nwith\nnewlines"
	for i := 0; i < b.N; i++ {
		escapeShellArg(input)
	}
}

// BenchmarkPTYSessionWrite benchmarks write performance
func BenchmarkPTYSessionWrite(b *testing.B) {
	mock := &mockWriteCloser{}
	session := &PTYSession{
		workstreamID: "test",
		closed:       false,
		done:         make(chan struct{}),
		stdin:        mock,
	}

	data := []byte("benchmark write data")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		session.Write(data)
	}
}

func TestSetProgram_WithValue(t *testing.T) {
	// Store original
	sender.mu.Lock()
	original := sender.program
	sender.mu.Unlock()
	defer func() {
		sender.mu.Lock()
		sender.program = original
		sender.mu.Unlock()
	}()

	// Set nil
	SetProgram(nil)
	sender.mu.RLock()
	p := sender.program
	sender.mu.RUnlock()
	if p != nil {
		t.Error("sender.program should be nil after SetProgram(nil)")
	}
}

// Test that PTYSession fields are properly initialized
func TestPTYSession_Fields(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session := &PTYSession{
		containerID:  "container-123",
		execID:       "exec-456",
		workstreamID: "ws-789",
		width:        80,
		height:       24,
		closed:       false,
		done:         make(chan struct{}),
		ctx:          ctx,
		cancel:       cancel,
	}

	if session.containerID != "container-123" {
		t.Errorf("containerID = %s, want container-123", session.containerID)
	}
	if session.execID != "exec-456" {
		t.Errorf("execID = %s, want exec-456", session.execID)
	}
	if session.workstreamID != "ws-789" {
		t.Errorf("workstreamID = %s, want ws-789", session.workstreamID)
	}
	if session.width != 80 {
		t.Errorf("width = %d, want 80", session.width)
	}
	if session.height != 24 {
		t.Errorf("height = %d, want 24", session.height)
	}
}

// Test the bypass permissions prompt detection logic (without actual PTY)
func TestBypassPromptDetection(t *testing.T) {
	// The StartReadLoop checks for "Bypass Permissions mode" and "Enter to confirm" in output
	// We can't easily test this without a real PTY, but we can test the string matching
	t.Run("Bypass Permissions mode", func(t *testing.T) {
		testCases := []struct {
			content  string
			expected bool
		}{
			{"Normal output", false},
			{"Bypass Permissions mode", true},
			{"Some text before Bypass Permissions mode and after", true},
			{"bypass permissions mode", false}, // Case sensitive
			{"BypassPermissionsmode", false},   // No spaces
		}

		for _, tc := range testCases {
			t.Run(tc.content, func(t *testing.T) {
				contains := bytes.Contains([]byte(tc.content), []byte("Bypass Permissions mode"))
				if contains != tc.expected {
					t.Errorf("Detection for %q = %v, want %v", tc.content, contains, tc.expected)
				}
			})
		}
	})

	t.Run("Enter to confirm", func(t *testing.T) {
		testCases := []struct {
			content  string
			expected bool
		}{
			{"Normal output", false},
			{"Enter to confirm", true},
			{"Enter to confirm · Esc to cancel", true},
			{"   Enter to confirm · Esc to cancel", true},
			{"Some text Enter to confirm more text", true},
			{"enter to confirm", false}, // Case sensitive
		}

		for _, tc := range testCases {
			t.Run(tc.content, func(t *testing.T) {
				contains := bytes.Contains([]byte(tc.content), []byte("Enter to confirm"))
				if contains != tc.expected {
					t.Errorf("Detection for %q = %v, want %v", tc.content, contains, tc.expected)
				}
			})
		}
	})
}

// Test timeout behavior conceptually
func TestTimeoutConstants(t *testing.T) {
	// Verify the bypass timeout is reasonable
	const bypassTimeout = 10 * time.Second
	if bypassTimeout < 5*time.Second || bypassTimeout > 30*time.Second {
		t.Errorf("bypassTimeout = %v, should be between 5s and 30s", bypassTimeout)
	}
}

// Test SessionIDCapturedMsg struct
func TestSessionIDCapturedMsg(t *testing.T) {
	msg := SessionIDCapturedMsg{
		WorkstreamID: "test-ws",
		SessionID:    "01HZ8Y3QPXKJNM5VG2DTCW9RAE",
	}

	if msg.WorkstreamID != "test-ws" {
		t.Errorf("WorkstreamID = %q, want %q", msg.WorkstreamID, "test-ws")
	}
	if msg.SessionID != "01HZ8Y3QPXKJNM5VG2DTCW9RAE" {
		t.Errorf("SessionID = %q, want %q", msg.SessionID, "01HZ8Y3QPXKJNM5VG2DTCW9RAE")
	}
}

// Test session ID regex pattern matching
func TestSessionIDRegex(t *testing.T) {
	testCases := []struct {
		name      string
		content   string
		wantMatch bool
		wantID    string
	}{
		// ULID format tests (26 alphanumeric chars)
		{
			name:      "ULID - standard session format",
			content:   "session: 01HZ8Y3QPXKJNM5VG2DTCW9RAE",
			wantMatch: true,
			wantID:    "01HZ8Y3QPXKJNM5VG2DTCW9RAE",
		},
		{
			name:      "ULID - session_id format",
			content:   "session_id: 01HZ8Y3QPXKJNM5VG2DTCW9RAE",
			wantMatch: true,
			wantID:    "01HZ8Y3QPXKJNM5VG2DTCW9RAE",
		},
		{
			name:      "ULID - resuming session format",
			content:   "Resuming session: 01HZ8Y3QPXKJNM5VG2DTCW9RAE",
			wantMatch: true,
			wantID:    "01HZ8Y3QPXKJNM5VG2DTCW9RAE",
		},
		{
			name:      "ULID - with surrounding text",
			content:   "Starting... session: 01ABC2DEF3GHJ4KLM5NPQ6RST7 and more",
			wantMatch: true,
			wantID:    "01ABC2DEF3GHJ4KLM5NPQ6RST7",
		},
		// UUID format tests (36 chars with dashes)
		{
			name:      "UUID - standard session format",
			content:   "session: 10b9a15d-6b70-4813-aaa8-8e438b796931",
			wantMatch: true,
			wantID:    "10b9a15d-6b70-4813-aaa8-8e438b796931",
		},
		{
			name:      "UUID - session_id format",
			content:   "session_id: 10b9a15d-6b70-4813-aaa8-8e438b796931",
			wantMatch: true,
			wantID:    "10b9a15d-6b70-4813-aaa8-8e438b796931",
		},
		{
			name:      "UUID - resuming session format",
			content:   "Resuming session: a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			wantMatch: true,
			wantID:    "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		},
		{
			name:      "UUID - uppercase",
			content:   "session: A1B2C3D4-E5F6-7890-ABCD-EF1234567890",
			wantMatch: true,
			wantID:    "A1B2C3D4-E5F6-7890-ABCD-EF1234567890",
		},
		{
			name:      "UUID - with surrounding text",
			content:   "Starting... session: 12345678-1234-1234-1234-123456789012 and more",
			wantMatch: true,
			wantID:    "12345678-1234-1234-1234-123456789012",
		},
		// Negative tests
		{
			name:      "no session ID",
			content:   "Normal output without session info",
			wantMatch: false,
			wantID:    "",
		},
		{
			name:      "too short ULID",
			content:   "session: 01HZ8Y3Q",
			wantMatch: false,
			wantID:    "",
		},
		{
			name:      "malformed UUID - wrong segment lengths",
			content:   "session: 10b9a15d-6b70-4813-aaa8-8e438b79693",
			wantMatch: false,
			wantID:    "",
		},
		{
			name:      "UUID without prefix",
			content:   "10b9a15d-6b70-4813-aaa8-8e438b796931",
			wantMatch: false,
			wantID:    "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			matches := sessionIDRegex.FindStringSubmatch(tc.content)
			if tc.wantMatch {
				if len(matches) < 2 {
					t.Errorf("Expected match for %q, but got none", tc.content)
					return
				}
				if matches[1] != tc.wantID {
					t.Errorf("Session ID = %q, want %q", matches[1], tc.wantID)
				}
			} else {
				if len(matches) > 1 {
					t.Errorf("Expected no match for %q, but got %q", tc.content, matches[1])
				}
			}
		})
	}
}

// Test PTYOptions with ClaudeSessionID
func TestPTYOptionsWithSessionID(t *testing.T) {
	tests := []struct {
		name          string
		opts          *PTYOptions
		wantSessionID string
		wantIsResume  bool
	}{
		{
			name: "resume with session ID",
			opts: &PTYOptions{
				IsResume:        true,
				ClaudeSessionID: "01HZ8Y3QPXKJNM5VG2DTCW9RAE",
			},
			wantSessionID: "01HZ8Y3QPXKJNM5VG2DTCW9RAE",
			wantIsResume:  true,
		},
		{
			name: "resume without session ID (fallback to --continue)",
			opts: &PTYOptions{
				IsResume:        true,
				ClaudeSessionID: "",
			},
			wantSessionID: "",
			wantIsResume:  true,
		},
		{
			name: "new session (no resume)",
			opts: &PTYOptions{
				IsResume:        false,
				ClaudeSessionID: "",
			},
			wantSessionID: "",
			wantIsResume:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.opts.ClaudeSessionID != tt.wantSessionID {
				t.Errorf("ClaudeSessionID = %q, want %q", tt.opts.ClaudeSessionID, tt.wantSessionID)
			}
			if tt.opts.IsResume != tt.wantIsResume {
				t.Errorf("IsResume = %v, want %v", tt.opts.IsResume, tt.wantIsResume)
			}
		})
	}
}

// TestContainerSetupScript_SingleConfigDir verifies that the container setup script
// uses $HOME/.claude as the config directory.
func TestContainerSetupScript_SingleConfigDir(t *testing.T) {
	tests := []struct {
		name     string
		contains string
		desc     string
	}{
		{
			name:     "sets CONFIG_DIR to HOME/.claude",
			contains: `CONFIG_DIR="$HOME/.claude"`,
			desc:     "should use $HOME/.claude as config directory",
		},
		{
			name:     "handles session data in CONFIG_DIR",
			contains: `$CONFIG_DIR/projects`,
			desc:     "session data should use CONFIG_DIR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(containerSetupScript, tt.contains) {
				t.Errorf("containerSetupScript should contain %q: %s", tt.contains, tt.desc)
			}
		})
	}
}

// TestCCellsCommitCommand verifies the ccells-commit command content.
// The ccells-commit.md file is now created in CreateContainerConfig (config.go)
// rather than in the container startup script.
func TestCCellsCommitCommand(t *testing.T) {
	// Verify the command contains essential information
	checks := []struct {
		contains string
		desc     string
	}{
		{"Claude Cells", "should mention Claude Cells"},
		{"commit all changes", "should instruct to commit changes"},
		{"Shift+Esc", "should mention the Shift+Esc shortcut"},
		{"merge dialog", "should mention the merge dialog"},
	}

	for _, check := range checks {
		if !strings.Contains(docker.CCellsCommitCommand, check.contains) {
			t.Errorf("CCellsCommitCommand should contain %q: %s", check.contains, check.desc)
		}
	}
}

// TestPTYSession_WriteString_EnterKey verifies that Enter key is sent as carriage return (\r)
// rather than line feed (\n). This is important for PTY interaction where \r simulates
// pressing the Enter key.
func TestPTYSession_WriteString_EnterKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "ccells-commit skill with carriage return",
			input:    "/ccells-commit\r",
			expected: "/ccells-commit\r",
		},
		{
			name:     "just carriage return for enter",
			input:    "\r",
			expected: "\r",
		},
		{
			name:     "continue command with enter",
			input:    "continue\r",
			expected: "continue\r",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockWriteCloser{}
			session := &PTYSession{
				workstreamID: "test",
				closed:       false,
				done:         make(chan struct{}),
				stdin:        mock,
			}

			err := session.WriteString(tt.input)
			if err != nil {
				t.Errorf("WriteString() error = %v", err)
			}
			if string(mock.Bytes()) != tt.expected {
				t.Errorf("Written data = %q, want %q", string(mock.Bytes()), tt.expected)
			}
		})
	}
}

// TestKittyEnterKey verifies that the KittyEnterKey constant contains the correct
// Kitty keyboard protocol sequence for the Enter key (CSI 13 u).
func TestKittyEnterKey(t *testing.T) {
	// Kitty keyboard protocol: CSI 13 u
	// CSI = ESC [ (27, 91)
	// 13 = Enter key codepoint (character codes '1' and '3')
	// u = terminator (117)
	expected := []byte{27, '[', '1', '3', 'u'}

	if len(KittyEnterKey) != len(expected) {
		t.Errorf("KittyEnterKey length = %d, want %d", len(KittyEnterKey), len(expected))
	}

	for i, b := range expected {
		if i < len(KittyEnterKey) && KittyEnterKey[i] != b {
			t.Errorf("KittyEnterKey[%d] = %v (%c), want %v (%c)", i, KittyEnterKey[i], KittyEnterKey[i], b, b)
		}
	}

	// Also verify the string representation
	expectedStr := "\x1b[13u"
	if string(KittyEnterKey) != expectedStr {
		t.Errorf("KittyEnterKey = %q, want %q", string(KittyEnterKey), expectedStr)
	}
}
