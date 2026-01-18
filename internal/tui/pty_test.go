package tui

import (
	"testing"
)

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
