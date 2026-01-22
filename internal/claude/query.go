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
// The name is intentionally verbose to make it clear to users (when they run
// `claude --resume` and see this in the list) that this is an internal session.
const EphemeralSessionID = "claude-cells-internal-do-not-use"

// clearThreshold is the number of prompts after which we run /clear.
// This keeps the session context from growing too large while avoiding
// the overhead of clearing on every query.
const clearThreshold = 100

// queryMutex prevents concurrent ephemeral queries from clobbering each other.
// Since we use a single session, concurrent access could cause race conditions.
var queryMutex sync.Mutex

// ephemeralState tracks the state of the ephemeral query session.
type ephemeralState struct {
	PromptCount int `json:"prompt_count"`
}

// getStateFilePath returns the path to the ephemeral query state file.
func getStateFilePath() string {
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

// Query executes an ephemeral Claude CLI query that doesn't pollute the resume log.
// It uses a fixed session ID and runs /clear periodically (every 50 prompts) to
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
	state, needsClear := loadState()
	if state.PromptCount >= clearThreshold {
		needsClear = true
		state.PromptCount = 0
	}

	// Build the prompt, optionally prepending /clear
	finalPrompt := prompt
	if needsClear {
		finalPrompt = "/clear\n" + prompt
	}

	// Build command arguments
	// Use --resume with our fixed session ID to keep queries grouped
	args := []string{
		"-p", finalPrompt,
		"--resume", EphemeralSessionID,
	}
	if opts.OutputFormat != "" {
		args = append(args, "--output-format", opts.OutputFormat)
	}

	cmd := exec.CommandContext(ctx, "claude", args...)

	// Inherit environment but add our overrides
	cmd.Env = append(os.Environ(),
		// Disable telemetry and error reporting for ephemeral queries
		"DISABLE_TELEMETRY=1",
		"DISABLE_ERROR_REPORTING=1",
	)

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Update and save state (increment prompt count)
	state.PromptCount++
	saveState(state)

	return strings.TrimSpace(string(output)), nil
}

// QueryWithTimeout is a convenience wrapper that creates a context with the given timeout.
func QueryWithTimeout(prompt string, timeout time.Duration) (string, error) {
	ctx := context.Background()
	return Query(ctx, prompt, &QueryOptions{Timeout: timeout})
}
