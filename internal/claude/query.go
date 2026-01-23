// Package claude provides utilities for interacting with the Claude CLI.
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DefaultTimeout is the default timeout for ephemeral queries.
const DefaultTimeout = 30 * time.Second

// DefaultModel is the default model for ephemeral queries.
// Haiku is fast and cheap, ideal for title generation and summarization.
const DefaultModel = "haiku"

// QueryOptions configures an ephemeral Claude query.
type QueryOptions struct {
	// Timeout for the query. Defaults to DefaultTimeout if zero.
	Timeout time.Duration

	// Model specifies which Claude model to use (e.g., "haiku", "sonnet", "opus").
	// Defaults to DefaultModel if empty.
	Model string
}

// cliEnvelope is the JSON envelope returned by claude CLI with --output-format json.
// Using JSON output isolates the actual response from any hook output that may
// be written to stdout.
type cliEnvelope struct {
	Type    string `json:"type"`
	Result  string `json:"result"`
	IsError bool   `json:"is_error"`
}

// extractCLIResult extracts the actual result from Claude CLI's JSON envelope.
// When using --output-format json, the CLI wraps responses in an envelope like:
// {"type":"result","result":"actual content here",...}
func extractCLIResult(output string) string {
	var envelope cliEnvelope
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		// Not a CLI envelope, return as-is
		return output
	}
	if envelope.Type == "result" && envelope.Result != "" {
		return envelope.Result
	}
	return output
}

// CommandExecutor is a function type that executes a command and returns its output.
// This allows for dependency injection in tests.
type CommandExecutor func(ctx context.Context, args []string, env []string) (string, error)

// defaultExecutor is the real command executor that runs the claude CLI.
func defaultExecutor(ctx context.Context, args []string, env []string) (string, error) {
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Env = env
	output, err := cmd.Output()
	if err != nil {
		// Extract stderr from ExitError for better diagnostics
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if stderr != "" {
				return "", fmt.Errorf("claude CLI failed (exit %d): %s", exitErr.ExitCode(), stderr)
			}
			return "", fmt.Errorf("claude CLI failed (exit %d): %w", exitErr.ExitCode(), err)
		}
		return "", err
	}
	return string(output), nil
}

// buildQueryArgs constructs the command-line arguments for a Claude query.
// We disable everything possible to minimize side effects and token usage:
// - No tools (just text generation)
// - No skills/slash commands
// - No MCP servers
// - Minimal system prompt
// - JSON output format to isolate response from any stray output
// - All hooks disabled via settings
func buildQueryArgs(prompt string, model string) []string {
	args := []string{
		"-p", prompt,
		"--no-session-persistence",
		"--tools", "", // Disable all tools
		"--disable-slash-commands",                                    // Disable skills
		"--strict-mcp-config",                                         // Only use MCP from --mcp-config (none)
		"--system-prompt", "You are a helpful assistant. Be concise.", // Minimal system prompt
		"--output-format", "json", // Always use JSON for clean parsing
		"--settings", `{"disableAllHooks":true}`, // Disable all hooks for ephemeral queries
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	return args
}

// QueryWithExecutor executes an ephemeral Claude CLI query using the provided executor.
// This is the testable version of Query that allows injecting a mock executor.
//
// Each query is stateless (--no-session-persistence) so there's no session context
// to manage. This keeps the implementation simple and cost predictable.
//
// The output is returned as JSON and parsed to extract just the result field,
// which isolates the response from any hook output that might be written to stdout.
func QueryWithExecutor(ctx context.Context, prompt string, opts *QueryOptions, executor CommandExecutor) (string, error) {
	if opts == nil {
		opts = &QueryOptions{}
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Use default model if not specified
	model := opts.Model
	if model == "" {
		model = DefaultModel
	}

	// Build the command arguments (always uses JSON output)
	args := buildQueryArgs(prompt, model)

	// Build environment to minimize side effects for ephemeral queries.
	// Hooks are disabled via --settings flag in buildQueryArgs.
	env := append(os.Environ(),
		"DISABLE_TELEMETRY=1",
		"DISABLE_ERROR_REPORTING=1",
	)

	// Execute the command
	output, err := executor(ctx, args, env)
	if err != nil {
		return "", err
	}

	// Extract the result from the JSON envelope
	// This isolates the actual response from any hook output
	result := extractCLIResult(strings.TrimSpace(output))
	return strings.TrimSpace(result), nil
}

// Query executes an ephemeral Claude CLI query that doesn't pollute the resume log.
// Each query is independent with no shared context (--no-session-persistence).
//
// This is the preferred way to make one-off Claude queries for things like:
//   - Generating commit titles
//   - Generating workstream titles
//   - Any other short, stateless queries
//
// Returns the trimmed output string, or an error if the query fails.
func Query(ctx context.Context, prompt string, opts *QueryOptions) (string, error) {
	return QueryWithExecutor(ctx, prompt, opts, defaultExecutor)
}

// QueryWithTimeout is a convenience wrapper that creates a context with the given timeout.
func QueryWithTimeout(prompt string, timeout time.Duration) (string, error) {
	ctx := context.Background()
	return Query(ctx, prompt, &QueryOptions{Timeout: timeout})
}
