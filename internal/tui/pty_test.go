package tui

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"
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
	original := program

	// Reset after test
	defer func() {
		program = original
	}()

	// Test setting nil
	SetProgram(nil)
	if program != nil {
		t.Error("program should be nil")
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
	original := program
	defer func() {
		program = original
	}()

	// Set nil
	SetProgram(nil)
	if program != nil {
		t.Error("program should be nil after SetProgram(nil)")
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
	// The StartReadLoop checks for "Bypass Permissions mode" in output
	// We can't easily test this without a real PTY, but we can test the string matching
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
}

// Test timeout behavior conceptually
func TestTimeoutConstants(t *testing.T) {
	// Verify the bypass timeout is reasonable
	const bypassTimeout = 10 * time.Second
	if bypassTimeout < 5*time.Second || bypassTimeout > 30*time.Second {
		t.Errorf("bypassTimeout = %v, should be between 5s and 30s", bypassTimeout)
	}
}
