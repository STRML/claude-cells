// Package claude provides utilities for interacting with the Claude CLI.
package claude

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultTimeout is the default timeout for ephemeral queries.
const DefaultTimeout = 30 * time.Second

// EphemeralSessionID is the fixed session ID used for all ephemeral queries.
// Using a fixed session keeps them grouped together and out of the main resume log.
// This is a valid UUID format as required by the Claude CLI.
// The UUID is deterministic (not random) so all ccells instances share the same session.
const EphemeralSessionID = "cccc0000-ce11-5000-0000-000000000001"

// clearThreshold is the number of prompts after which we run /clear.
// This keeps the session context from growing too large while avoiding
// the overhead of clearing on every query.
const clearThreshold = 100

// queryMutex prevents concurrent ephemeral queries from clobbering each other.
// Since we use a single session, concurrent access could cause race conditions.
var queryMutex sync.Mutex

// stateFilePathOverride allows tests to use a custom state file path.
// When empty (the default), getStateFilePath() uses the normal logic.
var stateFilePathOverride string

// ephemeralState tracks the state of the ephemeral query session.
type ephemeralState struct {
	PromptCount int `json:"prompt_count"`
}

// getStateFilePath returns the path to the ephemeral query state file.
func getStateFilePath() string {
	// Allow override for testing
	if stateFilePathOverride != "" {
		return stateFilePathOverride
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Fallback to /tmp if home dir is unavailable
		return "/tmp/claude-cells-ephemeral-state.json"
	}
	return filepath.Join(homeDir, ".claude-cells", "ephemeral-query-state.json")
}

// loadState loads the ephemeral query state from disk.
// If the file doesn't exist or is corrupted, returns a fresh state and needsClear=true.
func loadState() (state ephemeralState, needsClear bool) {
	stateFile := getStateFilePath()

	data, err := os.ReadFile(stateFile)
	if err != nil {
		// File doesn't exist or can't be read - start fresh, clear defensively
		return ephemeralState{PromptCount: 0}, true
	}

	if err := json.Unmarshal(data, &state); err != nil {
		// Corrupted/truncated file - start fresh, clear defensively
		return ephemeralState{PromptCount: 0}, true
	}

	// Sanity check: negative count means corruption
	if state.PromptCount < 0 {
		return ephemeralState{PromptCount: 0}, true
	}

	return state, false
}

// saveState persists the ephemeral query state to disk.
// Errors are logged but not returned - state persistence is best-effort.
func saveState(state ephemeralState) {
	stateFile := getStateFilePath()

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(stateFile), 0755); err != nil {
		return
	}

	data, err := json.Marshal(state)
	if err != nil {
		return
	}

	// Write atomically via temp file + rename
	tempFile := stateFile + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return
	}
	_ = os.Rename(tempFile, stateFile)
}

// QueryOptions configures an ephemeral Claude query.
type QueryOptions struct {
	// Timeout for the query. Defaults to DefaultTimeout if zero.
	Timeout time.Duration

	// OutputFormat specifies the output format (e.g., "json", "text").
	// Empty string uses Claude's default.
	OutputFormat string
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
		return "", err
	}
	return string(output), nil
}

// buildQueryArgs constructs the command-line arguments for a Claude query.
// Returns the args slice and the final prompt (with /clear prepended if needed).
// Note: We use --no-session-persistence for ephemeral queries to avoid polluting
// the resume log. This means each query is independent with no shared context,
// so needsClear is ignored (context is always fresh).
func buildQueryArgs(prompt string, needsClear bool, outputFormat string) ([]string, string) {
	// With --no-session-persistence, we don't need /clear since there's no
	// persistent context to clear. Each query starts fresh.
	// We keep the needsClear parameter for API compatibility and potential
	// future use if we switch back to persistent sessions.
	_ = needsClear

	args := []string{
		"-p", prompt,
		"--no-session-persistence",
	}
	if outputFormat != "" {
		args = append(args, "--output-format", outputFormat)
	}

	return args, prompt
}

// determineClearNeeded decides whether to prepend /clear to the prompt.
// It takes the loaded state and the needsClear flag from loadState.
// Returns whether to clear and the updated state (with count reset if clearing).
func determineClearNeeded(state ephemeralState, loadedNeedsClear bool) (needsClear bool, newState ephemeralState) {
	needsClear = loadedNeedsClear
	newState = state

	if state.PromptCount >= clearThreshold {
		needsClear = true
		newState.PromptCount = 0
	}

	return needsClear, newState
}

// QueryWithExecutor executes an ephemeral Claude CLI query using the provided executor.
// This is the testable version of Query that allows injecting a mock executor.
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

	// Acquire lock to prevent concurrent queries from interfering
	queryMutex.Lock()
	defer queryMutex.Unlock()

	// Load state and determine if we need to clear
	state, loadedNeedsClear := loadState()
	needsClear, state := determineClearNeeded(state, loadedNeedsClear)

	// Build the command arguments
	args, _ := buildQueryArgs(prompt, needsClear, opts.OutputFormat)

	// Build environment
	env := append(os.Environ(),
		"DISABLE_TELEMETRY=1",
		"DISABLE_ERROR_REPORTING=1",
	)

	// Execute the command
	output, err := executor(ctx, args, env)
	if err != nil {
		return "", err
	}

	// Update and save state (increment prompt count)
	state.PromptCount++
	saveState(state)

	return strings.TrimSpace(output), nil
}

// Query executes an ephemeral Claude CLI query that doesn't pollute the resume log.
// It uses a fixed session ID and runs /clear periodically (every 100 prompts) to
// keep the session context from growing too large.
//
// This is the preferred way to make one-off Claude queries for things like:
//   - Generating commit titles
//   - Generating workstream titles
//   - Any other short, stateless queries
//
// The function is thread-safe - concurrent calls will be serialized via a mutex.
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
