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
		{"starting is active", StateStarting, true},
		{"running is active", StateRunning, true},
		{"idle is active", StateIdle, true},
		{"pairing is active", StatePairing, true},
		{"stopped is not active", StateStopped, false},
		{"error is not active", StateError, false},
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
	if ws.State != StateStarting {
		t.Errorf("State = %q, want %q", ws.State, StateStarting)
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

func TestWorkstream_SetError(t *testing.T) {
	ws := New("test")
	testErr := &testError{msg: "container failed"}

	ws.SetError(testErr)

	if ws.State != StateError {
		t.Errorf("State = %q, want %q", ws.State, StateError)
	}
	if ws.ErrorMessage != "container failed" {
		t.Errorf("ErrorMessage = %q, want %q", ws.ErrorMessage, "container failed")
	}
}

func TestWorkstream_SetError_Nil(t *testing.T) {
	ws := New("test")

	ws.SetError(nil)

	if ws.State != StateError {
		t.Errorf("State = %q, want %q", ws.State, StateError)
	}
	if ws.ErrorMessage != "" {
		t.Errorf("ErrorMessage = %q, want empty", ws.ErrorMessage)
	}
}

func TestWorkstream_SetState_ClearsError(t *testing.T) {
	ws := New("test")
	ws.SetError(&testError{msg: "some error"})

	ws.SetState(StateRunning)

	if ws.ErrorMessage != "" {
		t.Errorf("ErrorMessage should be cleared, got %q", ws.ErrorMessage)
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestWorkstream_SetTitle(t *testing.T) {
	ws := New("implement user auth")
	ws.SetTitle("User Auth Implementation")

	if ws.Title != "User Auth Implementation" {
		t.Errorf("Title = %q, want %q", ws.Title, "User Auth Implementation")
	}
}

func TestWorkstream_GetTitle_ReturnsTitle(t *testing.T) {
	ws := New("implement user auth")
	ws.SetTitle("User Auth Implementation")

	if got := ws.GetTitle(); got != "User Auth Implementation" {
		t.Errorf("GetTitle() = %q, want %q", got, "User Auth Implementation")
	}
}

func TestWorkstream_GetTitle_FallbackToBranchName(t *testing.T) {
	ws := New("implement user auth")
	// Title not set, should fallback to branch name

	if got := ws.GetTitle(); got != ws.BranchName {
		t.Errorf("GetTitle() = %q, want %q (branch name)", got, ws.BranchName)
	}
}

func TestWorkstream_SetClaudeSessionID(t *testing.T) {
	ws := New("test workstream")
	sessionID := "01HZ8Y3QPXKJNM5VG2DTCW9RAE"

	ws.SetClaudeSessionID(sessionID)

	if ws.ClaudeSessionID != sessionID {
		t.Errorf("ClaudeSessionID = %q, want %q", ws.ClaudeSessionID, sessionID)
	}
}

func TestWorkstream_GetClaudeSessionID(t *testing.T) {
	ws := New("test workstream")
	sessionID := "01HZ8Y3QPXKJNM5VG2DTCW9RAE"
	ws.ClaudeSessionID = sessionID

	if got := ws.GetClaudeSessionID(); got != sessionID {
		t.Errorf("GetClaudeSessionID() = %q, want %q", got, sessionID)
	}
}

func TestWorkstream_GetClaudeSessionID_Empty(t *testing.T) {
	ws := New("test workstream")

	if got := ws.GetClaudeSessionID(); got != "" {
		t.Errorf("GetClaudeSessionID() = %q, want empty string", got)
	}
}

func TestNewWithUniqueBranch(t *testing.T) {
	tests := []struct {
		name             string
		prompt           string
		existingBranches []string
		wantBranch       string
	}{
		{
			name:             "no conflicts",
			prompt:           "add user auth",
			existingBranches: []string{},
			wantBranch:       "add-user-auth",
		},
		{
			name:             "conflict adds suffix",
			prompt:           "add user auth",
			existingBranches: []string{"add-user-auth"},
			wantBranch:       "add-user-auth-2",
		},
		{
			name:             "multiple conflicts",
			prompt:           "add user auth",
			existingBranches: []string{"add-user-auth", "add-user-auth-2", "add-user-auth-3"},
			wantBranch:       "add-user-auth-4",
		},
		{
			name:             "nil existing branches",
			prompt:           "fix bug",
			existingBranches: nil,
			wantBranch:       "fix-bug",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := NewWithUniqueBranch(tt.prompt, tt.existingBranches)
			if ws.BranchName != tt.wantBranch {
				t.Errorf("BranchName = %q, want %q", ws.BranchName, tt.wantBranch)
			}
			if ws.Prompt != tt.prompt {
				t.Errorf("Prompt = %q, want %q", ws.Prompt, tt.prompt)
			}
			if ws.State != StateStarting {
				t.Errorf("State = %q, want %q", ws.State, StateStarting)
			}
			if ws.ID == "" {
				t.Error("ID should not be empty")
			}
		})
	}
}

func TestWorkstream_SetBranchNameFromTitle(t *testing.T) {
	ws := NewForSummarizing("implement user authentication")
	if ws.BranchName != "" {
		t.Errorf("BranchName should be empty initially, got %q", ws.BranchName)
	}

	ws.SetBranchNameFromTitle("User Auth System", nil)
	if ws.BranchName == "" {
		t.Error("BranchName should be set after SetBranchNameFromTitle")
	}
	if ws.BranchName != "user-auth-system" {
		t.Errorf("BranchName = %q, want %q", ws.BranchName, "user-auth-system")
	}
}

func TestWorkstream_SetBranchNameFromTitle_WithConflicts(t *testing.T) {
	ws := NewForSummarizing("fix login bug")
	ws.SetBranchNameFromTitle("Fix Login Bug", []string{"fix-login-bug"})
	if ws.BranchName != "fix-login-bug-2" {
		t.Errorf("BranchName = %q, want %q", ws.BranchName, "fix-login-bug-2")
	}
}

func TestWorkstream_SetHasBeenPushed(t *testing.T) {
	ws := New("test")

	// Default should be false
	if ws.GetHasBeenPushed() {
		t.Error("HasBeenPushed should default to false")
	}

	// Set to true
	ws.SetHasBeenPushed(true)
	if !ws.GetHasBeenPushed() {
		t.Error("GetHasBeenPushed() should return true after SetHasBeenPushed(true)")
	}

	// Set back to false
	ws.SetHasBeenPushed(false)
	if ws.GetHasBeenPushed() {
		t.Error("GetHasBeenPushed() should return false after SetHasBeenPushed(false)")
	}
}

func TestWorkstream_SetPRInfo(t *testing.T) {
	ws := New("test")

	// Default should be zero values
	prNum, prURL := ws.GetPRInfo()
	if prNum != 0 {
		t.Errorf("PRNumber default = %d, want 0", prNum)
	}
	if prURL != "" {
		t.Errorf("PRURL default = %q, want empty", prURL)
	}

	// Set PR info
	ws.SetPRInfo(42, "https://github.com/org/repo/pull/42")

	prNum, prURL = ws.GetPRInfo()
	if prNum != 42 {
		t.Errorf("PRNumber = %d, want 42", prNum)
	}
	if prURL != "https://github.com/org/repo/pull/42" {
		t.Errorf("PRURL = %q, want %q", prURL, "https://github.com/org/repo/pull/42")
	}

	// Overwrite
	ws.SetPRInfo(99, "https://github.com/org/repo/pull/99")
	prNum, prURL = ws.GetPRInfo()
	if prNum != 99 {
		t.Errorf("PRNumber after overwrite = %d, want 99", prNum)
	}
}

func TestWorkstream_SetSynopsis(t *testing.T) {
	ws := New("test")

	// Default should be empty
	if ws.GetSynopsis() != "" {
		t.Errorf("Synopsis default = %q, want empty", ws.GetSynopsis())
	}

	// Set synopsis
	ws.SetSynopsis("Added OAuth2 login flow with Google provider")
	if ws.GetSynopsis() != "Added OAuth2 login flow with Google provider" {
		t.Errorf("GetSynopsis() = %q, want %q", ws.GetSynopsis(), "Added OAuth2 login flow with Google provider")
	}

	// Overwrite
	ws.SetSynopsis("Updated synopsis")
	if ws.GetSynopsis() != "Updated synopsis" {
		t.Errorf("GetSynopsis() after overwrite = %q, want %q", ws.GetSynopsis(), "Updated synopsis")
	}
}

func TestNewForSummarizing(t *testing.T) {
	ws := NewForSummarizing("implement dark mode")

	if ws.Prompt != "implement dark mode" {
		t.Errorf("Prompt = %q, want %q", ws.Prompt, "implement dark mode")
	}
	if ws.BranchName != "" {
		t.Errorf("BranchName should be empty for summarizing, got %q", ws.BranchName)
	}
	if ws.State != StateStarting {
		t.Errorf("State = %q, want %q", ws.State, StateStarting)
	}
	if ws.ID == "" {
		t.Error("ID should not be empty")
	}
}

func TestNewWithID(t *testing.T) {
	ws := NewWithID("custom-id-123", "my-branch", "my prompt")

	if ws.ID != "custom-id-123" {
		t.Errorf("ID = %q, want %q", ws.ID, "custom-id-123")
	}
	if ws.BranchName != "my-branch" {
		t.Errorf("BranchName = %q, want %q", ws.BranchName, "my-branch")
	}
	if ws.Prompt != "my prompt" {
		t.Errorf("Prompt = %q, want %q", ws.Prompt, "my prompt")
	}
}

func TestWorkstream_GetState(t *testing.T) {
	ws := New("test")
	if ws.GetState() != StateStarting {
		t.Errorf("GetState() = %q, want %q", ws.GetState(), StateStarting)
	}

	ws.SetState(StateRunning)
	if ws.GetState() != StateRunning {
		t.Errorf("GetState() = %q, want %q", ws.GetState(), StateRunning)
	}
}
