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

// buildCommand constructs the command to execute.
func (e *Executor) buildCommand(op Operation, args []string) []string {
	switch op {
	case OpGitFetch:
		return append([]string{"git", "fetch"}, args...)
	case OpGitPull:
		return append([]string{"git", "pull"}, args...)
	case OpGitPush:
		return append([]string{"git", "push"}, args...)
	case OpGHPRView:
		return append([]string{"gh", "pr", "view"}, args...)
	case OpGHPRChecks:
		return append([]string{"gh", "pr", "checks"}, args...)
	case OpGHPRDiff:
		return append([]string{"gh", "pr", "diff"}, args...)
	case OpGHPRList:
		return append([]string{"gh", "pr", "list"}, args...)
	case OpGHPRCreate:
		return append([]string{"gh", "pr", "create"}, args...)
	case OpGHPRMerge:
		return append([]string{"gh", "pr", "merge"}, args...)
	case OpGHIssueView:
		return append([]string{"gh", "issue", "view"}, args...)
	case OpGHIssueList:
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
