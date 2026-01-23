package gitproxy

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultTimeout for command execution
	DefaultTimeout = 120 * time.Second
)

// Executor runs git/gh commands on the host.
type Executor struct {
	timeout time.Duration
}

// NewExecutor creates a new command executor.
func NewExecutor() *Executor {
	return &Executor{
		timeout: DefaultTimeout,
	}
}

// Execute runs the operation and returns the response.
// It also returns PRCreateResult if the operation was gh-pr-create.
func (e *Executor) Execute(ctx context.Context, op Operation, args []string, ws WorkstreamInfo) (*Response, *PRCreateResult) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	cmdArgs := e.buildCommand(op, args)
	if len(cmdArgs) == 0 {
		return &Response{
			ExitCode: 1,
			Error:    fmt.Sprintf("unknown operation: %s", op),
		}, nil
	}

	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Dir = ws.WorktreePath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	resp := &Response{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			resp.ExitCode = exitErr.ExitCode()
		} else {
			resp.ExitCode = 1
			resp.Error = err.Error()
		}
	}

	// If this was a PR create, extract PR number from output
	var prResult *PRCreateResult
	if op == OpGHPRCreate && resp.ExitCode == 0 {
		prResult = extractPRCreateResult(resp.Stdout)
	}

	return resp, prResult
}

// dangerousGitFlags are flags that could be used for command injection or arbitrary code execution.
var dangerousGitFlags = []string{
	"--upload-pack",
	"--receive-pack",
	"--exec",
	"-u", // short for --upload-pack in some contexts
}

// allowedGHFlags is a whitelist of safe gh CLI flags.
var allowedGHFlags = map[string]bool{
	// Common flags
	"--repo": true, "-R": true,
	"--json": true, "--jq": true, "--template": true,
	// PR flags
	"--title": true, "-t": true,
	"--body": true, "-b": true,
	"--head": true, "-H": true,
	"--base": true, "-B": true,
	"--assignee": true, "-a": true,
	"--label": true, "-l": true,
	"--milestone": true, "-m": true,
	"--project": true, "-p": true,
	"--reviewer": true, "-r": true,
	"--draft": true, "-d": true,
	"--fill": true, "-f": true,
	"--web": true, "-w": true,
	// Merge flags
	"--merge": true, "--squash": true, "--rebase": true,
	"--delete-branch": true, "--auto": true,
	"--admin": true, // needed for some merge operations
	// View/list flags
	"--comments": true, "-c": true,
	"--state": true, "-s": true,
	"--limit": true, "-L": true,
	"--author": true, "-A": true,
	"--search": true, "-S": true,
	// Issue flags
	"--body-file": true, "-F": true,
	// Output format
	"--color": true,
}

// validateGitArgs checks git arguments for dangerous flags.
// Returns an error if a dangerous flag is found.
func validateGitArgs(args []string) error {
	for _, arg := range args {
		argLower := strings.ToLower(arg)
		for _, dangerous := range dangerousGitFlags {
			if arg == dangerous || strings.HasPrefix(argLower, dangerous+"=") {
				return fmt.Errorf("dangerous flag not allowed: %s", arg)
			}
		}
	}
	return nil
}

// validateGHArgs checks gh arguments against the whitelist.
// Returns an error if an unknown flag is found.
func validateGHArgs(args []string) error {
	for _, arg := range args {
		// Skip non-flag arguments (positional args like PR numbers, URLs)
		if !strings.HasPrefix(arg, "-") {
			continue
		}

		// Check if it's a known flag
		// Handle --flag=value format
		flagName := arg
		if idx := strings.Index(arg, "="); idx != -1 {
			flagName = arg[:idx]
		}

		if !allowedGHFlags[flagName] {
			return fmt.Errorf("flag not allowed: %s", arg)
		}
	}
	return nil
}

// buildCommand constructs the command to execute.
// Returns nil if validation fails.
func (e *Executor) buildCommand(op Operation, args []string) []string {
	switch op {
	case OpGitFetch:
		if err := validateGitArgs(args); err != nil {
			return nil
		}
		return append([]string{"git", "fetch"}, args...)
	case OpGitPull:
		if err := validateGitArgs(args); err != nil {
			return nil
		}
		return append([]string{"git", "pull"}, args...)
	case OpGitPush:
		if err := validateGitArgs(args); err != nil {
			return nil
		}
		return append([]string{"git", "push"}, args...)
	case OpGHPRView:
		if err := validateGHArgs(args); err != nil {
			return nil
		}
		return append([]string{"gh", "pr", "view"}, args...)
	case OpGHPRChecks:
		if err := validateGHArgs(args); err != nil {
			return nil
		}
		return append([]string{"gh", "pr", "checks"}, args...)
	case OpGHPRDiff:
		if err := validateGHArgs(args); err != nil {
			return nil
		}
		return append([]string{"gh", "pr", "diff"}, args...)
	case OpGHPRList:
		if err := validateGHArgs(args); err != nil {
			return nil
		}
		return append([]string{"gh", "pr", "list"}, args...)
	case OpGHPRCreate:
		if err := validateGHArgs(args); err != nil {
			return nil
		}
		return append([]string{"gh", "pr", "create"}, args...)
	case OpGHPRMerge:
		if err := validateGHArgs(args); err != nil {
			return nil
		}
		return append([]string{"gh", "pr", "merge"}, args...)
	case OpGHIssueView:
		if err := validateGHArgs(args); err != nil {
			return nil
		}
		return append([]string{"gh", "issue", "view"}, args...)
	case OpGHIssueList:
		if err := validateGHArgs(args); err != nil {
			return nil
		}
		return append([]string{"gh", "issue", "list"}, args...)
	default:
		return nil
	}
}

// extractPRCreateResult parses the gh pr create output to get PR number and URL.
func extractPRCreateResult(output string) *PRCreateResult {
	// gh pr create outputs the PR URL on success, e.g.:
	// https://github.com/owner/repo/pull/123
	prURLRegex := regexp.MustCompile(`https://github\.com/[^/]+/[^/]+/pull/(\d+)`)

	match := prURLRegex.FindStringSubmatch(output)
	if match == nil {
		// Try finding in lines
		for _, line := range strings.Split(output, "\n") {
			match = prURLRegex.FindStringSubmatch(line)
			if match != nil {
				break
			}
		}
	}

	if match == nil {
		return nil
	}

	num, err := strconv.Atoi(match[1])
	if err != nil {
		return nil
	}

	return &PRCreateResult{
		Number: num,
		URL:    match[0],
	}
}
