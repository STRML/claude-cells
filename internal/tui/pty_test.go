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
	}

	err := session.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if !session.IsClosed() {
		t.Error("Session should be closed after Close()")
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
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"hello world", "hello world"},
		{`hello "world"`, `hello \"world\"`},
		{`hello\world`, `hello\\world`},
		{"hello $VAR", `hello \$VAR`},
		{"hello `cmd`", "hello \\`cmd\\`"},
	}

	for _, tt := range tests {
		result := escapeShellArg(tt.input)
		if result != tt.expected {
			t.Errorf("escapeShellArg(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
