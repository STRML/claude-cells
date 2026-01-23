// Package gitproxy provides a secure proxy for git and gh operations from containers.
// It validates operations against the workstream's branch to prevent destructive actions.
package gitproxy

import "time"

// Operation represents an allowed git/gh operation.
type Operation string

const (
	// Git operations
	OpGitFetch Operation = "git-fetch"
	OpGitPull  Operation = "git-pull"
	OpGitPush  Operation = "git-push"

	// gh CLI operations - read-only
	OpGHPRView    Operation = "gh-pr-view"
	OpGHPRChecks  Operation = "gh-pr-checks"
	OpGHPRDiff    Operation = "gh-pr-diff"
	OpGHPRList    Operation = "gh-pr-list"
	OpGHIssueView Operation = "gh-issue-view"
	OpGHIssueList Operation = "gh-issue-list"

	// gh CLI operations - mutating (require validation)
	OpGHPRCreate Operation = "gh-pr-create"
	OpGHPRMerge  Operation = "gh-pr-merge"
)

// Request is the JSON structure sent from container to host.
type Request struct {
	Operation Operation `json:"operation"`
	Args      []string  `json:"args,omitempty"`
}

// Response is the JSON structure sent from host to container.
type Response struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error,omitempty"` // Validation error message
}

// WorkstreamInfo contains the information needed to validate operations.
type WorkstreamInfo struct {
	ID           string // Workstream ID for logging
	Branch       string // The branch this container is working on
	PRNumber     int    // PR number if one has been created
	WorktreePath string // Path to the worktree
}

// PRCreateResult is returned when gh-pr-create succeeds, containing the PR details.
type PRCreateResult struct {
	Number int
	URL    string
}

// SocketConfig configures the socket server for a container.
type SocketConfig struct {
	ContainerID string
	SocketPath  string
	Workstream  WorkstreamInfo
}

// SocketInfo tracks an active socket for a container.
type SocketInfo struct {
	SocketPath  string
	ContainerID string
	Workstream  WorkstreamInfo
	CreatedAt   time.Time
}
