//go:build integration

package claude

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Integration tests for the Claude query functionality.
// These tests require the claude CLI to be installed and authenticated.
// Run with: go test -tags=integration ./internal/claude/...

// setupTestState creates a temp state file and sets the override.
// Returns a cleanup function that restores the original state.
func setupTestState(t *testing.T, promptCount int) func() {
	t.Helper()

	// Create temp dir for state file
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "ephemeral-query-state.json")

	// Write initial state if count >= 0
	if promptCount >= 0 {
		content := fmt.Sprintf(`{"prompt_count":%d}`, promptCount)
		if err := os.WriteFile(stateFile, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write state file: %v", err)
		}
	}

	// Set the override (doesn't change HOME, preserves auth)
	oldOverride := stateFilePathOverride
	stateFilePathOverride = stateFile

	return func() {
		stateFilePathOverride = oldOverride
	}
}

func TestIntegration_ClaudeCLIAvailable(t *testing.T) {
	// Verify claude CLI is available
	cmd := exec.Command("claude", "--version")
	output, err := cmd.Output()
	if err != nil {
		t.Skipf("claude CLI not available: %v", err)
	}
	t.Logf("Claude CLI version: %s", strings.TrimSpace(string(output)))
}

func TestIntegration_ClaudeAuthenticated(t *testing.T) {
	// Skip if claude CLI is not available
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not available")
	}

	// Try a simple command to verify auth
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", "-p", "Reply with exactly: ok", "--no-session-persistence")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("Claude CLI not authenticated or API error: %v\nOutput: %s", err, output)
	}
	t.Logf("Auth check passed, response: %s", strings.TrimSpace(string(output)))
}

func TestIntegration_BasicQuery(t *testing.T) {
	// Skip if claude CLI is not available
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not available")
	}

	// Use a fresh state file (-1 means no state file, triggers /clear)
	cleanup := setupTestState(t, -1)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Simple query that should return a short response
	prompt := "Reply with exactly one word: hello"
	result, err := Query(ctx, prompt, nil)
	if err != nil {
		// Get more details about the error
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("Query failed: %v\nStderr: %s", err, exitErr.Stderr)
		}
		t.Fatalf("Query failed: %v", err)
	}

	t.Logf("Query result: %q", result)

	if result == "" {
		t.Error("Expected non-empty result")
	}
}

func TestIntegration_QueryWithClear(t *testing.T) {
	// Skip if claude CLI is not available
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not available")
	}

	// No state file - should trigger /clear
	cleanup := setupTestState(t, -1)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// First query - should have /clear prepended (missing state file)
	prompt := "Reply with exactly one word: test"
	result, err := Query(ctx, prompt, nil)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("First query failed: %v\nStderr: %s", err, exitErr.Stderr)
		}
		t.Fatalf("First query failed: %v", err)
	}

	t.Logf("First query result (with /clear): %q", result)

	if result == "" {
		t.Error("Expected non-empty result from first query")
	}

	// Verify state file was created
	stateFile := stateFilePathOverride
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		t.Error("State file was not created after query")
	}
}

func TestIntegration_QueryWithoutClear(t *testing.T) {
	// Skip if claude CLI is not available
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not available")
	}

	// Pre-create state file so /clear won't be prepended
	cleanup := setupTestState(t, 1)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Query without /clear - tests normal prompt handling
	prompt := "Reply with exactly one word: working"
	result, err := Query(ctx, prompt, nil)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("Query failed: %v\nStderr: %s", err, exitErr.Stderr)
		}
		t.Fatalf("Query failed: %v", err)
	}

	t.Logf("Query result (without /clear): %q", result)

	if result == "" {
		t.Error("Expected non-empty result")
	}
}

func TestIntegration_TitleGeneration(t *testing.T) {
	// Skip if claude CLI is not available
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not available")
	}

	cleanup := setupTestState(t, 1)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// This mimics how GenerateTitleCmd uses the Query function
	taskPrompt := "Implement user authentication with OAuth2"
	prompt := `Generate a 3-5 word title summarizing this task. Output ONLY the title text, no quotes or explanation.

Task: ` + taskPrompt

	result, err := Query(ctx, prompt, nil)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("Title generation failed: %v\nStderr: %s", err, exitErr.Stderr)
		}
		t.Fatalf("Title generation failed: %v", err)
	}

	t.Logf("Generated title: %q", result)

	if result == "" {
		t.Error("Expected non-empty title")
	}

	// Title should be reasonably short (not a full response)
	words := strings.Fields(result)
	if len(words) > 10 {
		t.Errorf("Title seems too long (%d words): %q", len(words), result)
	}
}

func TestIntegration_ConsecutiveQueries(t *testing.T) {
	// Skip if claude CLI is not available
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not available")
	}

	cleanup := setupTestState(t, -1) // Fresh start
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Run multiple queries to verify the session works correctly
	prompts := []string{
		"Reply with exactly: one",
		"Reply with exactly: two",
		"Reply with exactly: three",
	}

	for i, prompt := range prompts {
		result, err := Query(ctx, prompt, nil)
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				t.Fatalf("Query %d failed: %v\nStderr: %s", i+1, err, exitErr.Stderr)
			}
			t.Fatalf("Query %d failed: %v", i+1, err)
		}
		t.Logf("Query %d result: %q", i+1, result)

		if result == "" {
			t.Errorf("Query %d returned empty result", i+1)
		}
	}

	// Verify state file has correct count
	state, needsClear := loadState()
	if needsClear {
		t.Error("Expected needsClear=false after multiple queries")
	}
	if state.PromptCount != 3 {
		t.Errorf("Expected PromptCount=3, got %d", state.PromptCount)
	}
}

func TestIntegration_ClearCommandWorks(t *testing.T) {
	// Skip if claude CLI is not available
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not available")
	}

	// Start with state file at threshold - this should trigger /clear
	cleanup := setupTestState(t, 100)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// This query should have /clear prepended due to threshold
	prompt := "Reply with exactly: cleared"
	result, err := Query(ctx, prompt, nil)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("Query after threshold failed: %v\nStderr: %s", err, exitErr.Stderr)
		}
		t.Fatalf("Query after threshold failed: %v", err)
	}

	t.Logf("Query result after /clear: %q", result)

	if result == "" {
		t.Error("Expected non-empty result")
	}

	// Verify count was reset
	state, _ := loadState()
	if state.PromptCount != 1 {
		t.Errorf("Expected PromptCount=1 after clear, got %d", state.PromptCount)
	}
}

func TestIntegration_Timeout(t *testing.T) {
	// Skip if claude CLI is not available
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not available")
	}

	cleanup := setupTestState(t, 1)
	defer cleanup()

	// Very short timeout should fail
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, err := Query(ctx, "test", nil)
	if err == nil {
		t.Error("Expected timeout error with 1ms timeout")
	}
}

func TestIntegration_EphemeralSessionID(t *testing.T) {
	// Verify that the session ID is a valid UUID format
	// This is critical for Claude CLI compatibility

	// UUID format validation
	parts := strings.Split(EphemeralSessionID, "-")
	if len(parts) != 5 {
		t.Errorf("EphemeralSessionID should have 5 parts, got %d: %s", len(parts), EphemeralSessionID)
	}

	expectedLengths := []int{8, 4, 4, 4, 12}
	for i, part := range parts {
		if len(part) != expectedLengths[i] {
			t.Errorf("Part %d should be %d chars, got %d", i, expectedLengths[i], len(part))
		}
	}

	t.Logf("Using ephemeral session ID: %s", EphemeralSessionID)
}

func TestIntegration_VerifyNoSessionPersistence(t *testing.T) {
	// This test verifies that --no-session-persistence is used
	// by checking the actual args that would be passed

	args, finalPrompt := buildQueryArgs("test prompt", true, "")

	// Prompt should be unchanged (no /clear with --no-session-persistence)
	if finalPrompt != "test prompt" {
		t.Errorf("Expected prompt 'test prompt', got: %q", finalPrompt)
	}

	// Verify --no-session-persistence is in args
	found := false
	for _, arg := range args {
		if arg == "--no-session-persistence" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected --no-session-persistence in args, got: %v", args)
	}

	t.Logf("Args: %v", args)
}
