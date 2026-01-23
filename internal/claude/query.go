// Package claude provides utilities for interacting with the Claude CLI.
package claude

import (
	"context"
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

	// OutputFormat specifies the output format (e.g., "json", "text").
	// Empty string uses Claude's default.
	OutputFormat string

	// Model specifies which Claude model to use (e.g., "haiku", "sonnet", "opus").
	// Defaults to DefaultModel if empty.
	Model string
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
// We disable everything possible to minimize token usage:
// - No tools (just text generation)
// - No skills/slash commands
// - No MCP servers
// - Minimal system prompt
func buildQueryArgs(prompt string, outputFormat string, model string) []string {
	args := []string{
		"-p", prompt,
		"--no-session-persistence",
		"--tools", "", // Disable all tools
		"--disable-slash-commands",                                    // Disable skills
		"--strict-mcp-config",                                         // Only use MCP from --mcp-config (none)
		"--system-prompt", "You are a helpful assistant. Be concise.", // Minimal system prompt
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	if outputFormat != "" {
		args = append(args, "--output-format", outputFormat)
	}
	return args
}

// QueryWithExecutor executes an ephemeral Claude CLI query using the provided executor.
// This is the testable version of Query that allows injecting a mock executor.
//
// Each query is stateless (--no-session-persistence) so there's no session context
// to manage. This keeps the implementation simple and cost predictable.
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

	// Build the command arguments
	args := buildQueryArgs(prompt, opts.OutputFormat, model)

	// Build environment
	// CLAUDE_SKIP_SESSION_LEARNINGS=1 disables session-learnings hooks for ephemeral queries
	env := append(os.Environ(),
		"DISABLE_TELEMETRY=1",
		"DISABLE_ERROR_REPORTING=1",
		"CLAUDE_SKIP_SESSION_LEARNINGS=1",
	)

	// Execute the command
	output, err := executor(ctx, args, env)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(output), nil
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
