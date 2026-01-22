package claude

import (
	"context"
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
	if opts.OutputFormat != "" {
		t.Errorf("Expected empty output format, got %q", opts.OutputFormat)
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
	args := buildQueryArgs("test prompt", "", "")

	// Check required flags are present
	requiredFlags := []string{
		"-p", "test prompt",
		"--no-session-persistence",
		"--tools", "",
		"--disable-slash-commands",
		"--strict-mcp-config",
		"--system-prompt",
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
	args := buildQueryArgs("test prompt", "", "haiku")

	// Check --model haiku is present
	modelIdx := indexOf(args, "--model")
	if modelIdx == -1 || modelIdx+1 >= len(args) || args[modelIdx+1] != "haiku" {
		t.Errorf("Expected --model haiku in args, got %v", args)
	}
}

func TestBuildQueryArgs_WithOutputFormat(t *testing.T) {
	args := buildQueryArgs("test prompt", "json", "")

	// Check --output-format json is present
	formatIdx := indexOf(args, "--output-format")
	if formatIdx == -1 || formatIdx+1 >= len(args) || args[formatIdx+1] != "json" {
		t.Errorf("Expected --output-format json in args, got %v", args)
	}
}

func TestBuildQueryArgs_WithModelAndOutputFormat(t *testing.T) {
	args := buildQueryArgs("test prompt", "json", "sonnet")

	// Check both are present
	modelIdx := indexOf(args, "--model")
	formatIdx := indexOf(args, "--output-format")

	if modelIdx == -1 || modelIdx+1 >= len(args) || args[modelIdx+1] != "sonnet" {
		t.Errorf("Expected --model sonnet in args, got %v", args)
	}
	if formatIdx == -1 || formatIdx+1 >= len(args) || args[formatIdx+1] != "json" {
		t.Errorf("Expected --output-format json in args, got %v", args)
	}
}

func TestBuildQueryArgs_AlwaysIncludesNoSessionPersistence(t *testing.T) {
	args := buildQueryArgs("test", "", "")

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
		return "Mock Response", nil
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
		return "ok", nil
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
		return "ok", nil
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

func TestQueryWithExecutor_OutputFormat(t *testing.T) {
	var capturedArgs []string
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		capturedArgs = args
		return "ok", nil
	}

	opts := &QueryOptions{OutputFormat: "json"}
	_, _ = QueryWithExecutor(context.Background(), "test", opts, mockExecutor)

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
		return "  response with whitespace  \n", nil
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
		return "ok", nil
	}

	_, _ = QueryWithExecutor(context.Background(), "test", nil, mockExecutor)

	// Check for required env vars
	requiredVars := []string{
		"DISABLE_TELEMETRY=1",
		"DISABLE_ERROR_REPORTING=1",
		"CLAUDE_SKIP_SESSION_LEARNINGS=1",
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
		return "ok", nil
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
		return "ok", nil
	}

	_, _ = QueryWithExecutor(context.Background(), "test", nil, mockExecutor)
}

func TestQueryWithExecutor_EmptyPrompt(t *testing.T) {
	var capturedArgs []string
	mockExecutor := func(ctx context.Context, args []string, env []string) (string, error) {
		capturedArgs = args
		return "ok", nil
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
		return "ok", nil
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
		return "ok", nil
	}

	multilinePrompt := "Line 1\nLine 2\nLine 3"
	_, _ = QueryWithExecutor(context.Background(), multilinePrompt, nil, mockExecutor)

	if len(capturedArgs) < 2 || capturedArgs[1] != multilinePrompt {
		t.Errorf("Expected multiline prompt to be preserved, got %v", capturedArgs)
	}
}

// =============================================================================
// Tests for Query (integration with defaultExecutor - skipped in short mode)
// =============================================================================

func TestQuery_CanceledContext(t *testing.T) {
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

func indexOf(slice []string, item string) int {
	for i, v := range slice {
		if v == item {
			return i
		}
	}
	return -1
}
