package gitproxy

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Validator validates an operation's arguments against workstream constraints.
type Validator func(args []string, ws WorkstreamInfo) error

// validators maps operations to their validation functions.
var validators = map[Operation]Validator{
	// Git operations
	OpGitFetch: validateUnrestricted,
	OpGitPull:  validateUnrestricted,
	OpGitPush:  validatePush,

	// gh CLI operations - read-only
	OpGHPRView:    validateUnrestricted,
	OpGHPRChecks:  validateUnrestricted,
	OpGHPRDiff:    validateUnrestricted,
	OpGHPRList:    validateUnrestricted,
	OpGHIssueView: validateUnrestricted,
	OpGHIssueList: validateUnrestricted,

	// gh CLI operations - mutating
	OpGHPRCreate: validateUnrestricted, // PR number captured from output
	OpGHPRMerge:  validateMerge,
}

// Validate checks if the operation is allowed with the given arguments.
func Validate(op Operation, args []string, ws WorkstreamInfo) error {
	validator, ok := validators[op]
	if !ok {
		return fmt.Errorf("operation not allowed: %s", op)
	}
	return validator(args, ws)
}

// IsAllowedOperation checks if an operation is in the allowlist.
func IsAllowedOperation(op Operation) bool {
	_, ok := validators[op]
	return ok
}

// validateUnrestricted allows any arguments.
func validateUnrestricted(args []string, ws WorkstreamInfo) error {
	return nil
}

// validatePush ensures push is only to the workstream's branch and no --force.
func validatePush(args []string, ws WorkstreamInfo) error {
	// Check for force flags
	for _, arg := range args {
		if arg == "--force" || arg == "-f" ||
			strings.HasPrefix(arg, "--force-with-lease") {
			return fmt.Errorf("force push not allowed from container")
		}
	}

	// Parse push arguments to find the branch
	// git push [remote] [branch]
	// git push origin feature-branch
	// git push origin HEAD:feature-branch
	// git push (uses current branch)

	branch := extractPushBranch(args)
	if branch == "" {
		// No explicit branch - will push current branch, which should be correct
		return nil
	}

	if branch != ws.Branch {
		return fmt.Errorf("can only push to branch %q, not %q", ws.Branch, branch)
	}

	return nil
}

// extractPushBranch extracts the target branch from git push arguments.
func extractPushBranch(args []string) string {
	// Skip flags and find positional args
	var positional []string
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		// Skip flags that take values (consume next arg)
		if arg == "-o" || arg == "--push-option" || arg == "--repo" {
			skipNext = true
			continue
		}
		// Skip boolean flags (don't consume next arg)
		if strings.HasPrefix(arg, "-") {
			continue
		}
		positional = append(positional, arg)
	}

	if len(positional) < 2 {
		return "" // No explicit branch
	}

	// Second positional arg is the refspec
	refspec := positional[1]

	// Handle HEAD:branch syntax
	if strings.Contains(refspec, ":") {
		parts := strings.SplitN(refspec, ":", 2)
		return parts[1]
	}

	return refspec
}

// validateMerge ensures merge is only for this workstream's PR.
func validateMerge(args []string, ws WorkstreamInfo) error {
	if ws.PRNumber == 0 {
		return fmt.Errorf("no PR associated with this workstream; create a PR first")
	}

	// Parse args to find PR number
	prNum := extractPRNumber(args)

	if prNum == 0 {
		// No explicit PR number - gh pr merge uses current branch's PR
		// This is fine as long as we're on the right branch (which we are in the worktree)
		return nil
	}

	if prNum != ws.PRNumber {
		return fmt.Errorf("can only merge PR #%d (this workstream's PR), not #%d", ws.PRNumber, prNum)
	}

	return nil
}

// extractPRNumber extracts the PR number from gh pr merge arguments.
func extractPRNumber(args []string) int {
	// gh pr merge [<number> | <url> | <branch>] [flags]
	prNumRegex := regexp.MustCompile(`^(\d+)$`)
	prURLRegex := regexp.MustCompile(`/pull/(\d+)`)

	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue // Skip flags
		}

		// Check if it's a number
		if match := prNumRegex.FindStringSubmatch(arg); match != nil {
			num, _ := strconv.Atoi(match[1])
			return num
		}

		// Check if it's a URL containing PR number
		if match := prURLRegex.FindStringSubmatch(arg); match != nil {
			num, _ := strconv.Atoi(match[1])
			return num
		}
	}

	return 0
}

// ParseOperation converts a command string to an Operation.
// Returns the operation and remaining args, or error if not recognized.
func ParseOperation(cmd string, args []string) (Operation, []string, error) {
	// Normalize command
	cmd = strings.TrimSpace(cmd)

	// Git commands
	if cmd == "git" && len(args) > 0 {
		switch args[0] {
		case "fetch":
			return OpGitFetch, args[1:], nil
		case "pull":
			return OpGitPull, args[1:], nil
		case "push":
			return OpGitPush, args[1:], nil
		case "remote":
			return "", nil, fmt.Errorf("git remote commands are blocked")
		}
		return "", nil, fmt.Errorf("git %s is not proxied", args[0])
	}

	// gh commands
	if cmd == "gh" && len(args) > 1 {
		subCmd := args[0] + " " + args[1]
		switch subCmd {
		case "pr view":
			return OpGHPRView, args[2:], nil
		case "pr checks":
			return OpGHPRChecks, args[2:], nil
		case "pr diff":
			return OpGHPRDiff, args[2:], nil
		case "pr list":
			return OpGHPRList, args[2:], nil
		case "pr create":
			return OpGHPRCreate, args[2:], nil
		case "pr merge":
			return OpGHPRMerge, args[2:], nil
		case "issue view":
			return OpGHIssueView, args[2:], nil
		case "issue list":
			return OpGHIssueList, args[2:], nil
		}
		return "", nil, fmt.Errorf("gh %s is not proxied", subCmd)
	}

	return "", nil, fmt.Errorf("command not recognized: %s", cmd)
}
