package claude

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestQueryOptions_Defaults(t *testing.T) {
	// Verify that nil options work (uses defaults)
	opts := &QueryOptions{}
	if opts.Timeout != 0 {
		t.Errorf("Expected zero timeout in empty options, got %v", opts.Timeout)
	}
	if opts.OutputFormat != "" {
		t.Errorf("Expected empty output format in empty options, got %s", opts.OutputFormat)
	}
}

func TestQuery_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := Query(ctx, "test prompt", nil)
	if err == nil {
		t.Error("Expected error with canceled context")
	}
}

func TestQueryWithTimeout_SetsTimeout(t *testing.T) {
	// Very short timeout should cause failure
	_, err := QueryWithTimeout("test prompt", 1*time.Nanosecond)
	if err == nil {
		t.Error("Expected error with 1ns timeout")
	}
}

func TestEphemeralSessionID_IsDescriptive(t *testing.T) {
	// The session ID should make it clear this is internal
	if EphemeralSessionID == "" {
		t.Error("EphemeralSessionID should not be empty")
	}
	// Should contain indicators that it's not for user use
	expectedSubstrings := []string{"internal", "do-not-use"}
	for _, s := range expectedSubstrings {
		found := false
		for i := 0; i <= len(EphemeralSessionID)-len(s); i++ {
			if EphemeralSessionID[i:i+len(s)] == s {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("EphemeralSessionID %q should contain %q", EphemeralSessionID, s)
		}
	}
}

func TestQuery_MutexPreventsConcurrency(t *testing.T) {
	// This test verifies the mutex is being used.
	// We can't easily test true concurrent behavior without a real Claude CLI,
	// but we can verify that the mutex exists and is accessible.

	// The queryMutex is package-level and should be initialized
	// Just verify we can reference it without panic
	queryMutex.Lock()
	queryMutex.Unlock()
}

func TestLoadState_MissingFile(t *testing.T) {
	// Save original state file path logic by using a temp dir
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	state, needsClear := loadState()

	// Missing file should return fresh state and needsClear=true
	if state.PromptCount != 0 {
		t.Errorf("Expected PromptCount=0, got %d", state.PromptCount)
	}
	if !needsClear {
		t.Error("Expected needsClear=true for missing file")
	}
}

func TestLoadState_CorruptedFile(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	// Create corrupted state file
	stateDir := filepath.Join(tempDir, ".claude-cells")
	os.MkdirAll(stateDir, 0755)
	stateFile := filepath.Join(stateDir, "ephemeral-query-state.json")
	os.WriteFile(stateFile, []byte("not valid json{{{"), 0644)

	state, needsClear := loadState()

	// Corrupted file should return fresh state and needsClear=true
	if state.PromptCount != 0 {
		t.Errorf("Expected PromptCount=0, got %d", state.PromptCount)
	}
	if !needsClear {
		t.Error("Expected needsClear=true for corrupted file")
	}
}

func TestLoadState_NegativeCount(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	// Create state file with negative count (corruption)
	stateDir := filepath.Join(tempDir, ".claude-cells")
	os.MkdirAll(stateDir, 0755)
	stateFile := filepath.Join(stateDir, "ephemeral-query-state.json")
	os.WriteFile(stateFile, []byte(`{"prompt_count":-5}`), 0644)

	state, needsClear := loadState()

	// Negative count should return fresh state and needsClear=true
	if state.PromptCount != 0 {
		t.Errorf("Expected PromptCount=0, got %d", state.PromptCount)
	}
	if !needsClear {
		t.Error("Expected needsClear=true for negative count")
	}
}

func TestLoadState_ValidFile(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	// Create valid state file
	stateDir := filepath.Join(tempDir, ".claude-cells")
	os.MkdirAll(stateDir, 0755)
	stateFile := filepath.Join(stateDir, "ephemeral-query-state.json")
	os.WriteFile(stateFile, []byte(`{"prompt_count":25}`), 0644)

	state, needsClear := loadState()

	if state.PromptCount != 25 {
		t.Errorf("Expected PromptCount=25, got %d", state.PromptCount)
	}
	if needsClear {
		t.Error("Expected needsClear=false for valid file under threshold")
	}
}

func TestSaveState_CreatesDirectory(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	// Save state to non-existent directory
	saveState(ephemeralState{PromptCount: 42})

	// Verify file was created
	stateFile := filepath.Join(tempDir, ".claude-cells", "ephemeral-query-state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("Failed to read state file: %v", err)
	}

	expected := `{"prompt_count":42}`
	if string(data) != expected {
		t.Errorf("Expected %q, got %q", expected, string(data))
	}
}

func TestClearThreshold_IsReasonable(t *testing.T) {
	// Verify the threshold is in a reasonable range
	if clearThreshold < 10 {
		t.Error("clearThreshold too low - would clear too frequently")
	}
	if clearThreshold > 500 {
		t.Error("clearThreshold too high - session could grow very large")
	}
}
