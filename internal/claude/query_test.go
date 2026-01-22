package claude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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

func TestEphemeralSessionID_IsValidUUID(t *testing.T) {
	// The session ID must be a valid UUID format for Claude CLI
	if EphemeralSessionID == "" {
		t.Error("EphemeralSessionID should not be empty")
	}

	// UUID format: 8-4-4-4-12 hex characters
	// Example: cccc0000-ce11-5000-0000-000000000001
	parts := strings.Split(EphemeralSessionID, "-")
	if len(parts) != 5 {
		t.Errorf("EphemeralSessionID %q should have 5 parts separated by dashes", EphemeralSessionID)
	}

	expectedLengths := []int{8, 4, 4, 4, 12}
	for i, part := range parts {
		if len(part) != expectedLengths[i] {
			t.Errorf("Part %d of EphemeralSessionID should be %d chars, got %d: %q",
				i, expectedLengths[i], len(part), part)
		}
		// Verify all characters are hex
		for _, c := range part {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				t.Errorf("EphemeralSessionID contains non-hex character: %c", c)
			}
		}
	}

	// Should contain "cccc" or "ce11" to hint at "ccells"
	if !strings.Contains(EphemeralSessionID, "cccc") && !strings.Contains(EphemeralSessionID, "ce11") {
		t.Logf("Warning: EphemeralSessionID %q doesn't contain ccells identifier", EphemeralSessionID)
	}
}

func TestQuery_MutexPreventsConcurrency(t *testing.T) {
	// This test verifies the mutex is being used.
	// We can't easily test true concurrent behavior without a real Claude CLI,
	// but we can verify that the mutex exists and is accessible.

	// The queryMutex is package-level and should be initialized
	// Just verify we can reference it without panic
	queryMutex.Lock()
	_ = struct{}{} // Satisfy staticcheck SA2001 (non-empty critical section)
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

// =============================================================================
// Tests for buildQueryArgs - the core command construction logic
// =============================================================================

func TestBuildQueryArgs_BasicPrompt(t *testing.T) {
	prompt := "Generate a title"
	needsClear := false
	outputFormat := ""

	args, finalPrompt := buildQueryArgs(prompt, needsClear, outputFormat)

	// Verify prompt is NOT modified (--no-session-persistence means no /clear needed)
	if finalPrompt != prompt {
		t.Errorf("Expected prompt %q, got %q", prompt, finalPrompt)
	}

	// Verify basic args structure uses --no-session-persistence
	expectedArgs := []string{"-p", prompt, "--no-session-persistence"}
	if !slicesEqual(args, expectedArgs) {
		t.Errorf("Expected args %v, got %v", expectedArgs, args)
	}
}

func TestBuildQueryArgs_WithClear(t *testing.T) {
	prompt := "Generate a title"
	needsClear := true
	outputFormat := ""

	args, finalPrompt := buildQueryArgs(prompt, needsClear, outputFormat)

	// With --no-session-persistence, needsClear is ignored (each query is fresh)
	// Prompt should be unchanged
	if finalPrompt != prompt {
		t.Errorf("Expected prompt %q (unchanged), got %q", prompt, finalPrompt)
	}

	// Verify args use the original prompt
	if args[1] != prompt {
		t.Errorf("Expected args[1] to be original prompt, got %q", args[1])
	}
}

func TestBuildQueryArgs_WithOutputFormat(t *testing.T) {
	prompt := "Generate a title"
	needsClear := false
	outputFormat := "json"

	args, _ := buildQueryArgs(prompt, needsClear, outputFormat)

	// Verify output format is included
	expectedArgs := []string{"-p", prompt, "--no-session-persistence", "--output-format", "json"}
	if !slicesEqual(args, expectedArgs) {
		t.Errorf("Expected args %v, got %v", expectedArgs, args)
	}
}

func TestBuildQueryArgs_ClearAndOutputFormat(t *testing.T) {
	prompt := "Generate a title"
	needsClear := true
	outputFormat := "text"

	args, finalPrompt := buildQueryArgs(prompt, needsClear, outputFormat)

	// With --no-session-persistence, needsClear is ignored
	// Prompt should be unchanged
	if finalPrompt != prompt {
		t.Errorf("Expected prompt %q (unchanged), got %q", prompt, finalPrompt)
	}

	expectedArgs := []string{"-p", prompt, "--no-session-persistence", "--output-format", "text"}
	if !slicesEqual(args, expectedArgs) {
		t.Errorf("Expected args %v, got %v", expectedArgs, args)
	}
}

func TestBuildQueryArgs_UsesNoSessionPersistence(t *testing.T) {
	args, _ := buildQueryArgs("test", false, "")

	// Find --no-session-persistence flag
	found := false
	for _, arg := range args {
		if arg == "--no-session-persistence" {
			found = true
			break
		}
	}
	if !found {
		t.Error("--no-session-persistence flag not found in args")
	}
}

// =============================================================================
// Tests for determineClearNeeded - the clear threshold logic
// =============================================================================

func TestDetermineClearNeeded_FreshState(t *testing.T) {
	state := ephemeralState{PromptCount: 0}
	loadedNeedsClear := true // Missing file returns needsClear=true

	needsClear, newState := determineClearNeeded(state, loadedNeedsClear)

	if !needsClear {
		t.Error("Expected needsClear=true for fresh state")
	}
	if newState.PromptCount != 0 {
		t.Errorf("Expected PromptCount=0, got %d", newState.PromptCount)
	}
}

func TestDetermineClearNeeded_BelowThreshold(t *testing.T) {
	state := ephemeralState{PromptCount: 50}
	loadedNeedsClear := false

	needsClear, newState := determineClearNeeded(state, loadedNeedsClear)

	if needsClear {
		t.Error("Expected needsClear=false for count below threshold")
	}
	if newState.PromptCount != 50 {
		t.Errorf("Expected PromptCount=50, got %d", newState.PromptCount)
	}
}

func TestDetermineClearNeeded_AtThreshold(t *testing.T) {
	state := ephemeralState{PromptCount: clearThreshold}
	loadedNeedsClear := false

	needsClear, newState := determineClearNeeded(state, loadedNeedsClear)

	if !needsClear {
		t.Error("Expected needsClear=true at threshold")
	}
	// Count should be reset to 0 when threshold is reached
	if newState.PromptCount != 0 {
		t.Errorf("Expected PromptCount=0 after threshold, got %d", newState.PromptCount)
	}
}

func TestDetermineClearNeeded_AboveThreshold(t *testing.T) {
	state := ephemeralState{PromptCount: clearThreshold + 50}
	loadedNeedsClear := false

	needsClear, newState := determineClearNeeded(state, loadedNeedsClear)

	if !needsClear {
		t.Error("Expected needsClear=true above threshold")
	}
	if newState.PromptCount != 0 {
		t.Errorf("Expected PromptCount=0 after threshold, got %d", newState.PromptCount)
	}
}

func TestDetermineClearNeeded_LoadedNeedsClearOverrides(t *testing.T) {
	// Even if count is 0, if loadedNeedsClear is true (e.g., corrupted file),
	// we should clear
	state := ephemeralState{PromptCount: 0}
	loadedNeedsClear := true

	needsClear, _ := determineClearNeeded(state, loadedNeedsClear)

	if !needsClear {
		t.Error("Expected needsClear=true when loadedNeedsClear is true")
	}
}

// =============================================================================
// Tests for QueryWithExecutor - using mock executor
// =============================================================================

func TestQueryWithExecutor_PassesCorrectArgs(t *testing.T) {
	tempDir := t.TempDir()
	origOverride := stateFilePathOverride
	stateFilePathOverride = filepath.Join(tempDir, "state.json")
	defer func() { stateFilePathOverride = origOverride }()

	var capturedArgs []string
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		capturedArgs = args
		return "Mock Title", nil
	}

	result, err := QueryWithExecutor(context.Background(), "test prompt", nil, mockExecutor)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result != "Mock Title" {
		t.Errorf("Expected result 'Mock Title', got %q", result)
	}

	// With --no-session-persistence, prompt is unchanged (no /clear needed)
	if len(capturedArgs) < 2 {
		t.Fatalf("Expected at least 2 args, got %d", len(capturedArgs))
	}

	prompt := capturedArgs[1] // -p is at index 0, prompt at index 1
	if prompt != "test prompt" {
		t.Errorf("Expected prompt 'test prompt', got %q", prompt)
	}

	// Verify --no-session-persistence is in args
	found := false
	for _, arg := range capturedArgs {
		if arg == "--no-session-persistence" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected --no-session-persistence in args, got %v", capturedArgs)
	}
}

func TestQueryWithExecutor_ConsecutiveQueriesPreservePrompt(t *testing.T) {
	tempDir := t.TempDir()
	origOverride := stateFilePathOverride
	stateFilePathOverride = filepath.Join(tempDir, "state.json")
	defer func() { stateFilePathOverride = origOverride }()

	var capturedPrompts []string
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		if len(args) >= 2 {
			capturedPrompts = append(capturedPrompts, args[1])
		}
		return "Mock Title", nil
	}

	// Multiple queries - with --no-session-persistence, prompts are unchanged
	_, _ = QueryWithExecutor(context.Background(), "prompt 1", nil, mockExecutor)
	_, _ = QueryWithExecutor(context.Background(), "prompt 2", nil, mockExecutor)

	if len(capturedPrompts) != 2 {
		t.Fatalf("Expected 2 prompts, got %d", len(capturedPrompts))
	}

	// Both prompts should be unchanged (no /clear with --no-session-persistence)
	if capturedPrompts[0] != "prompt 1" {
		t.Errorf("First prompt should be 'prompt 1', got %q", capturedPrompts[0])
	}

	if capturedPrompts[1] != "prompt 2" {
		t.Errorf("Second prompt should be 'prompt 2', got %q", capturedPrompts[1])
	}
}

func TestQueryWithExecutor_StateTrackingWorks(t *testing.T) {
	tempDir := t.TempDir()
	origOverride := stateFilePathOverride
	stateFilePathOverride = filepath.Join(tempDir, "state.json")
	defer func() { stateFilePathOverride = origOverride }()

	// Pre-create state file with count below threshold
	os.WriteFile(stateFilePathOverride, []byte(`{"prompt_count":50}`), 0644)

	var capturedPrompts []string
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		if len(args) >= 2 {
			capturedPrompts = append(capturedPrompts, args[1])
		}
		return "Mock Title", nil
	}

	// Run two queries
	_, _ = QueryWithExecutor(context.Background(), "prompt 1", nil, mockExecutor)
	_, _ = QueryWithExecutor(context.Background(), "prompt 2", nil, mockExecutor)

	if len(capturedPrompts) != 2 {
		t.Fatalf("Expected 2 prompts, got %d", len(capturedPrompts))
	}

	// With --no-session-persistence, prompts are always unchanged
	if capturedPrompts[0] != "prompt 1" {
		t.Errorf("Expected 'prompt 1', got %q", capturedPrompts[0])
	}

	if capturedPrompts[1] != "prompt 2" {
		t.Errorf("Expected 'prompt 2', got %q", capturedPrompts[1])
	}

	// State should still be tracked (for future use if we switch back to persistent sessions)
	state, _ := loadState()
	if state.PromptCount != 52 {
		t.Errorf("Expected PromptCount=52, got %d", state.PromptCount)
	}
}

func TestQueryWithExecutor_IncrementsPromptCount(t *testing.T) {
	tempDir := t.TempDir()
	origOverride := stateFilePathOverride
	stateFilePathOverride = filepath.Join(tempDir, "state.json")
	defer func() { stateFilePathOverride = origOverride }()

	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		return "Mock Title", nil
	}

	// Run 3 queries
	for i := 0; i < 3; i++ {
		_, _ = QueryWithExecutor(context.Background(), "test", nil, mockExecutor)
	}

	// Check state file
	state, needsClear := loadState()
	if needsClear {
		t.Error("Expected needsClear=false after queries")
	}
	// After 3 queries, count should be 3
	if state.PromptCount != 3 {
		t.Errorf("Expected PromptCount=3, got %d", state.PromptCount)
	}
}

func TestQueryWithExecutor_ResetsCountAfterClear(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	// Pre-create state file at threshold
	stateDir := filepath.Join(tempDir, ".claude-cells")
	os.MkdirAll(stateDir, 0755)
	stateFile := filepath.Join(stateDir, "ephemeral-query-state.json")
	os.WriteFile(stateFile, []byte(`{"prompt_count":100}`), 0644)

	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		return "Mock Title", nil
	}

	// This query should trigger a clear and reset count
	_, _ = QueryWithExecutor(context.Background(), "test", nil, mockExecutor)

	// Check state file - count should be 1 (reset to 0, then incremented)
	state, _ := loadState()
	if state.PromptCount != 1 {
		t.Errorf("Expected PromptCount=1 after clear, got %d", state.PromptCount)
	}
}

func TestQueryWithExecutor_PropagatesError(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	expectedErr := context.DeadlineExceeded
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		return "", expectedErr
	}

	_, err := QueryWithExecutor(context.Background(), "test", nil, mockExecutor)
	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

func TestQueryWithExecutor_DoesNotIncrementOnError(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	// Pre-create state file
	stateDir := filepath.Join(tempDir, ".claude-cells")
	os.MkdirAll(stateDir, 0755)
	stateFile := filepath.Join(stateDir, "ephemeral-query-state.json")
	os.WriteFile(stateFile, []byte(`{"prompt_count":5}`), 0644)

	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		return "", context.DeadlineExceeded
	}

	_, _ = QueryWithExecutor(context.Background(), "test", nil, mockExecutor)

	// Count should NOT have incremented on error
	state, _ := loadState()
	if state.PromptCount != 5 {
		t.Errorf("Expected PromptCount=5 (unchanged after error), got %d", state.PromptCount)
	}
}

func TestQueryWithExecutor_UsesTimeout(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	var capturedCtx context.Context
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		capturedCtx = ctx
		return "ok", nil
	}

	opts := &QueryOptions{Timeout: 5 * time.Second}
	_, _ = QueryWithExecutor(context.Background(), "test", opts, mockExecutor)

	// Context should have a deadline
	if _, ok := capturedCtx.Deadline(); !ok {
		t.Error("Expected context to have a deadline")
	}
}

func TestQueryWithExecutor_OutputFormat(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	var capturedArgs []string
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		capturedArgs = args
		return "ok", nil
	}

	opts := &QueryOptions{OutputFormat: "json"}
	_, _ = QueryWithExecutor(context.Background(), "test", opts, mockExecutor)

	// Check that --output-format json is in args
	foundFormat := false
	for i, arg := range capturedArgs {
		if arg == "--output-format" && i+1 < len(capturedArgs) && capturedArgs[i+1] == "json" {
			foundFormat = true
			break
		}
	}
	if !foundFormat {
		t.Errorf("Expected --output-format json in args, got %v", capturedArgs)
	}
}

func TestQueryWithExecutor_TrimsOutput(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		return "  Mock Title  \n\n", nil
	}

	result, _ := QueryWithExecutor(context.Background(), "test", nil, mockExecutor)

	if result != "Mock Title" {
		t.Errorf("Expected trimmed result 'Mock Title', got %q", result)
	}
}

func TestQueryWithExecutor_SetsEnvironmentVariables(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	var capturedEnv []string
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		capturedEnv = env
		return "ok", nil
	}

	_, _ = QueryWithExecutor(context.Background(), "test", nil, mockExecutor)

	// Check for expected env vars
	envMap := make(map[string]bool)
	for _, e := range capturedEnv {
		envMap[e] = true
	}

	expectedEnvs := []string{"DISABLE_TELEMETRY=1", "DISABLE_ERROR_REPORTING=1"}
	for _, expected := range expectedEnvs {
		if !envMap[expected] {
			t.Errorf("Expected env var %q, not found in %v", expected, capturedEnv)
		}
	}
}

// =============================================================================
// Concurrency tests
// =============================================================================

func TestQueryWithExecutor_SerializesConcurrentCalls(t *testing.T) {
	tempDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	var concurrent int32
	var maxConcurrent int32

	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		current := atomic.AddInt32(&concurrent, 1)
		// Track max concurrent
		for {
			max := atomic.LoadInt32(&maxConcurrent)
			if current <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, current) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond) // Simulate work
		atomic.AddInt32(&concurrent, -1)
		return "ok", nil
	}

	// Launch concurrent queries
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = QueryWithExecutor(context.Background(), "test", nil, mockExecutor)
		}()
	}
	wg.Wait()

	// Due to mutex, max concurrent should be 1
	if maxConcurrent != 1 {
		t.Errorf("Expected max concurrent=1 (serialized), got %d", maxConcurrent)
	}
}

// =============================================================================
// Edge case tests
// =============================================================================

func TestQueryWithExecutor_EmptyPrompt(t *testing.T) {
	tempDir := t.TempDir()
	origOverride := stateFilePathOverride
	stateFilePathOverride = filepath.Join(tempDir, "state.json")
	defer func() { stateFilePathOverride = origOverride }()

	var capturedArgs []string
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		capturedArgs = args
		return "ok", nil
	}

	_, _ = QueryWithExecutor(context.Background(), "", nil, mockExecutor)

	// Even empty prompt should work
	if len(capturedArgs) < 2 {
		t.Fatalf("Expected at least 2 args, got %d", len(capturedArgs))
	}
	// With --no-session-persistence, prompt is passed unchanged (even if empty)
	if capturedArgs[1] != "" {
		t.Errorf("Expected empty prompt, got %q", capturedArgs[1])
	}
}

func TestQueryWithExecutor_PromptWithNewlines(t *testing.T) {
	tempDir := t.TempDir()
	origOverride := stateFilePathOverride
	stateFilePathOverride = filepath.Join(tempDir, "state.json")
	defer func() { stateFilePathOverride = origOverride }()

	var capturedArgs []string
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		capturedArgs = args
		return "ok", nil
	}

	multilinePrompt := "Line 1\nLine 2\nLine 3"
	_, _ = QueryWithExecutor(context.Background(), multilinePrompt, nil, mockExecutor)

	if len(capturedArgs) < 2 {
		t.Fatalf("Expected at least 2 args, got %d", len(capturedArgs))
	}
	if capturedArgs[1] != multilinePrompt {
		t.Errorf("Expected prompt to be preserved with newlines, got %q", capturedArgs[1])
	}
}

func TestQueryWithExecutor_PromptWithSpecialChars(t *testing.T) {
	tempDir := t.TempDir()
	origOverride := stateFilePathOverride
	stateFilePathOverride = filepath.Join(tempDir, "state.json")
	defer func() { stateFilePathOverride = origOverride }()

	var capturedArgs []string
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		capturedArgs = args
		return "ok", nil
	}

	specialPrompt := `Quote: "test" and 'test' and $var and \backslash`
	_, _ = QueryWithExecutor(context.Background(), specialPrompt, nil, mockExecutor)

	if len(capturedArgs) < 2 {
		t.Fatalf("Expected at least 2 args, got %d", len(capturedArgs))
	}
	if capturedArgs[1] != specialPrompt {
		t.Errorf("Expected prompt with special chars to be preserved, got %q", capturedArgs[1])
	}
}

// =============================================================================
// Helper functions
// =============================================================================

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
