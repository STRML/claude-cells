package workstream

import (
	"testing"
	"time"
)

func TestWorkstreamState(t *testing.T) {
	tests := []struct {
		name     string
		state    State
		isActive bool
	}{
		{"running is active", StateRunning, true},
		{"idle is active", StateIdle, true},
		{"pairing is active", StatePairing, true},
		{"stopped is not active", StateStopped, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.state.IsActive() != tt.isActive {
				t.Errorf("State(%q).IsActive() = %v, want %v", tt.state, tt.state.IsActive(), tt.isActive)
			}
		})
	}
}

func TestNewWorkstream(t *testing.T) {
	prompt := "add user authentication"
	ws := New(prompt)

	if ws.Prompt != prompt {
		t.Errorf("Prompt = %q, want %q", ws.Prompt, prompt)
	}
	if ws.BranchName != "add-user-authentication" {
		t.Errorf("BranchName = %q, want %q", ws.BranchName, "add-user-authentication")
	}
	if ws.State != StateStopped {
		t.Errorf("State = %q, want %q", ws.State, StateStopped)
	}
	if ws.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestWorkstream_SetState(t *testing.T) {
	ws := New("test")
	ws.SetState(StateRunning)

	if ws.State != StateRunning {
		t.Errorf("State = %q, want %q", ws.State, StateRunning)
	}
}

func TestWorkstream_SetContainerID(t *testing.T) {
	ws := New("test")
	ws.SetContainerID("abc123")

	if ws.ContainerID != "abc123" {
		t.Errorf("ContainerID = %q, want %q", ws.ContainerID, "abc123")
	}
}

func TestWorkstream_LastActivity(t *testing.T) {
	ws := New("test")
	before := time.Now()

	ws.UpdateActivity()

	if ws.LastActivity.Before(before) {
		t.Error("LastActivity should be updated to current time")
	}
}

func TestWorkstream_String(t *testing.T) {
	ws := New("add auth")
	ws.SetState(StateRunning)

	str := ws.String()
	if str == "" {
		t.Error("String() should return non-empty")
	}
}
