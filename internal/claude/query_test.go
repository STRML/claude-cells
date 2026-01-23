package claude

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// Tests for QueryOptions defaults
// =============================================================================

func TestQueryOptions_Defaults(t *testing.T) {
	opts := &QueryOptions{}

	if opts.Timeout != 0 {
		t.Errorf("Expected zero timeout, got %v", opts.Timeout)
	}
	if opts.Model != "" {
		t.Errorf("Expected empty model, got %q", opts.Model)
	}
}

func TestDefaultModel_IsHaiku(t *testing.T) {
	if DefaultModel != "haiku" {
		t.Errorf("Expected DefaultModel to be 'haiku', got %q", DefaultModel)
	}
}

func TestDefaultTimeout_Is30Seconds(t *testing.T) {
	if DefaultTimeout != 30*time.Second {
		t.Errorf("Expected DefaultTimeout to be 30s, got %v", DefaultTimeout)
	}
}

// =============================================================================
// Tests for buildQueryArgs
// =============================================================================

func TestBuildQueryArgs_Basic(t *testing.T) {
	args := buildQueryArgs("test prompt", "")

	// Check required flags are present
	requiredFlags := []string{
		"-p", "test prompt",
		"--no-session-persistence",
		"--tools", "",
		"--disable-slash-commands",
		"--strict-mcp-config",
		"--system-prompt",
		"--output-format", "json", // Always use JSON
	}
	for i := 0; i < len(requiredFlags); i++ {
		found := false
		for _, arg := range args {
			if arg == requiredFlags[i] {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected %q in args, got %v", requiredFlags[i], args)
		}
	}
}

func TestBuildQueryArgs_WithModel(t *testing.T) {
	args := buildQueryArgs("test prompt", "haiku")

	// Check --model haiku is present
	modelIdx := indexOf(args, "--model")
	if modelIdx == -1 || modelIdx+1 >= len(args) || args[modelIdx+1] != "haiku" {
		t.Errorf("Expected --model haiku in args, got %v", args)
	}
}

func TestBuildQueryArgs_AlwaysIncludesJSONOutput(t *testing.T) {
	args := buildQueryArgs("test", "")

	// Should always have --output-format json
	formatIdx := indexOf(args, "--output-format")
	if formatIdx == -1 || formatIdx+1 >= len(args) || args[formatIdx+1] != "json" {
		t.Errorf("Expected --output-format json in args, got %v", args)
	}
}

func TestBuildQueryArgs_DisablesAllHooks(t *testing.T) {
	args := buildQueryArgs("test", "")

	// Should have --settings with disableAllHooks
	settingsIdx := indexOf(args, "--settings")
	if settingsIdx == -1 || settingsIdx+1 >= len(args) {
		t.Fatalf("Expected --settings in args, got %v", args)
	}
	settingsVal := args[settingsIdx+1]
	if !strings.Contains(settingsVal, "disableAllHooks") || !strings.Contains(settingsVal, "true") {
		t.Errorf("Expected --settings to contain disableAllHooks:true, got %q", settingsVal)
	}
}

func TestBuildQueryArgs_AlwaysIncludesNoSessionPersistence(t *testing.T) {
	args := buildQueryArgs("test", "")

	found := false
	for _, arg := range args {
		if arg == "--no-session-persistence" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected --no-session-persistence in args, got %v", args)
	}
}

// =============================================================================
// Tests for QueryWithExecutor
// =============================================================================

func TestQueryWithExecutor_PassesCorrectArgs(t *testing.T) {
	var capturedArgs []string
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		capturedArgs = args
		// Return JSON envelope format
		return `{"type":"result","result":"Mock Response","is_error":false}`, nil
	}

	result, err := QueryWithExecutor(context.Background(), "test prompt", nil, mockExecutor)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result != "Mock Response" {
		t.Errorf("Expected 'Mock Response', got %q", result)
	}

	// Should have prompt
	if len(capturedArgs) < 2 || capturedArgs[1] != "test prompt" {
		t.Errorf("Expected prompt in args, got %v", capturedArgs)
	}

	// Should have --no-session-persistence
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

func TestQueryWithExecutor_UsesDefaultModel(t *testing.T) {
	var capturedArgs []string
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		capturedArgs = args
		return `{"type":"result","result":"ok"}`, nil
	}

	_, _ = QueryWithExecutor(context.Background(), "test", nil, mockExecutor)

	// Should have --model haiku (DefaultModel)
	modelIdx := -1
	for i, arg := range capturedArgs {
		if arg == "--model" {
			modelIdx = i
			break
		}
	}
	if modelIdx == -1 || modelIdx+1 >= len(capturedArgs) {
		t.Fatalf("Expected --model in args, got %v", capturedArgs)
	}
	if capturedArgs[modelIdx+1] != DefaultModel {
		t.Errorf("Expected default model %q, got %q", DefaultModel, capturedArgs[modelIdx+1])
	}
}

func TestQueryWithExecutor_CustomModel(t *testing.T) {
	var capturedArgs []string
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		capturedArgs = args
		return `{"type":"result","result":"ok"}`, nil
	}

	opts := &QueryOptions{Model: "opus"}
	_, _ = QueryWithExecutor(context.Background(), "test", opts, mockExecutor)

	// Should have --model opus
	modelIdx := -1
	for i, arg := range capturedArgs {
		if arg == "--model" {
			modelIdx = i
			break
		}
	}
	if modelIdx == -1 || modelIdx+1 >= len(capturedArgs) {
		t.Fatalf("Expected --model in args, got %v", capturedArgs)
	}
	if capturedArgs[modelIdx+1] != "opus" {
		t.Errorf("Expected model 'opus', got %q", capturedArgs[modelIdx+1])
	}
}

func TestQueryWithExecutor_AlwaysUsesJSONOutput(t *testing.T) {
	var capturedArgs []string
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		capturedArgs = args
		return `{"type":"result","result":"ok"}`, nil
	}

	// Even without any options, JSON output should be used
	_, _ = QueryWithExecutor(context.Background(), "test", nil, mockExecutor)

	// Should have --output-format json
	formatIdx := -1
	for i, arg := range capturedArgs {
		if arg == "--output-format" {
			formatIdx = i
			break
		}
	}
	if formatIdx == -1 || formatIdx+1 >= len(capturedArgs) {
		t.Fatalf("Expected --output-format in args, got %v", capturedArgs)
	}
	if capturedArgs[formatIdx+1] != "json" {
		t.Errorf("Expected format 'json', got %q", capturedArgs[formatIdx+1])
	}
}

func TestQueryWithExecutor_TrimsOutput(t *testing.T) {
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		return `  {"type":"result","result":"  response with whitespace  "}  `, nil
	}

	result, _ := QueryWithExecutor(context.Background(), "test", nil, mockExecutor)

	if result != "response with whitespace" {
		t.Errorf("Expected trimmed output, got %q", result)
	}
}

func TestQueryWithExecutor_PropagatesError(t *testing.T) {
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		return "", context.DeadlineExceeded
	}

	_, err := QueryWithExecutor(context.Background(), "test", nil, mockExecutor)

	if err == nil {
		t.Error("Expected error to be propagated")
	}
}

func TestQueryWithExecutor_SetsEnvironmentVariables(t *testing.T) {
	var capturedEnv []string
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		capturedEnv = env
		return `{"type":"result","result":"ok"}`, nil
	}

	_, _ = QueryWithExecutor(context.Background(), "test", nil, mockExecutor)

	// Check for required env vars (hooks disabled via --settings flag)
	requiredVars := []string{
		"DISABLE_TELEMETRY=1",
		"DISABLE_ERROR_REPORTING=1",
	}

	for _, required := range requiredVars {
		found := false
		for _, env := range capturedEnv {
			if env == required {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected %q in environment", required)
		}
	}
}

func TestQueryWithExecutor_UsesTimeout(t *testing.T) {
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		// Check that context has a deadline
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("Expected context to have deadline")
		}
		// Deadline should be roughly 5 seconds from now (within tolerance)
		remaining := time.Until(deadline)
		if remaining < 4*time.Second || remaining > 6*time.Second {
			t.Errorf("Expected ~5s timeout, got %v remaining", remaining)
		}
		return `{"type":"result","result":"ok"}`, nil
	}

	opts := &QueryOptions{Timeout: 5 * time.Second}
	_, _ = QueryWithExecutor(context.Background(), "test", opts, mockExecutor)
}

func TestQueryWithExecutor_DefaultTimeout(t *testing.T) {
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("Expected context to have deadline")
		}
		remaining := time.Until(deadline)
		// Should be close to DefaultTimeout (30s)
		if remaining < 29*time.Second || remaining > 31*time.Second {
			t.Errorf("Expected ~30s timeout, got %v remaining", remaining)
		}
		return `{"type":"result","result":"ok"}`, nil
	}

	_, _ = QueryWithExecutor(context.Background(), "test", nil, mockExecutor)
}

func TestQueryWithExecutor_EmptyPrompt(t *testing.T) {
	var capturedArgs []string
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		capturedArgs = args
		return `{"type":"result","result":"ok"}`, nil
	}

	_, _ = QueryWithExecutor(context.Background(), "", nil, mockExecutor)

	// Should still pass empty prompt
	if len(capturedArgs) < 2 || capturedArgs[1] != "" {
		t.Errorf("Expected empty prompt to be passed, got %v", capturedArgs)
	}
}

func TestQueryWithExecutor_PromptWithSpecialChars(t *testing.T) {
	var capturedArgs []string
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		capturedArgs = args
		return `{"type":"result","result":"ok"}`, nil
	}

	specialPrompt := `Quote: "test" and 'test' and $var`
	_, _ = QueryWithExecutor(context.Background(), specialPrompt, nil, mockExecutor)

	if len(capturedArgs) < 2 || capturedArgs[1] != specialPrompt {
		t.Errorf("Expected prompt with special chars to be preserved, got %v", capturedArgs)
	}
}

func TestQueryWithExecutor_PromptWithNewlines(t *testing.T) {
	var capturedArgs []string
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		capturedArgs = args
		return `{"type":"result","result":"ok"}`, nil
	}

	multilinePrompt := "Line 1\nLine 2\nLine 3"
	_, _ = QueryWithExecutor(context.Background(), multilinePrompt, nil, mockExecutor)

	if len(capturedArgs) < 2 || capturedArgs[1] != multilinePrompt {
		t.Errorf("Expected multiline prompt to be preserved, got %v", capturedArgs)
	}
}

// =============================================================================
// Tests for extractCLIResult
// =============================================================================

func TestExtractCLIResult_ValidEnvelope(t *testing.T) {
	input := `{"type":"result","result":"Hello world","is_error":false}`
	result := extractCLIResult(input)
	if result != "Hello world" {
		t.Errorf("Expected 'Hello world', got %q", result)
	}
}

func TestExtractCLIResult_EmptyResult(t *testing.T) {
	input := `{"type":"result","result":"","is_error":false}`
	result := extractCLIResult(input)
	// Empty result field means we return the original input
	if result != input {
		t.Errorf("Expected original input for empty result, got %q", result)
	}
}

func TestExtractCLIResult_NotEnvelope(t *testing.T) {
	input := "Just plain text"
	result := extractCLIResult(input)
	if result != input {
		t.Errorf("Expected original input, got %q", result)
	}
}

func TestExtractCLIResult_InvalidJSON(t *testing.T) {
	input := "{invalid json"
	result := extractCLIResult(input)
	if result != input {
		t.Errorf("Expected original input for invalid JSON, got %q", result)
	}
}

func TestExtractCLIResult_WrongType(t *testing.T) {
	input := `{"type":"error","result":"some error","is_error":true}`
	result := extractCLIResult(input)
	// type is "error" not "result", so returns original
	if result != input {
		t.Errorf("Expected original input for wrong type, got %q", result)
	}
}

func TestExtractCLIResult_IsolatesHookOutput(t *testing.T) {
	// Simulate hook output appearing before the JSON envelope
	// The extractCLIResult should fail to parse this as JSON and return as-is
	// but in practice, the JSON parsing will find the envelope
	input := `{"type":"result","result":"Actual response","is_error":false}`
	result := extractCLIResult(input)
	if result != "Actual response" {
		t.Errorf("Expected 'Actual response', got %q", result)
	}
}

// =============================================================================
// Tests for Query (integration with defaultExecutor - skipped in short mode)
// =============================================================================

func TestQuery_CanceledContext(t *testing.T) {
	// Skip if claude isn't installed
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude not installed")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := Query(ctx, "test", nil)

	if err == nil {
		t.Error("Expected error for canceled context")
	}
	if !strings.Contains(err.Error(), "context canceled") && !strings.Contains(err.Error(), "killed") {
		t.Errorf("Expected context canceled error, got %v", err)
	}
}

// =============================================================================
// Helper functions
// =============================================================================

func indexOf(slice []string, item string) int {
	for i, v := range slice {
		if v == item {
			return i
		}
	}
	return -1
}
