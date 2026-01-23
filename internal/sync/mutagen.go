package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// SyncStatus represents the current state of a mutagen sync session.
type SyncStatus int

const (
	SyncStatusUnknown SyncStatus = iota
	SyncStatusDisconnected
	SyncStatusConnecting
	SyncStatusWatching   // Idle, watching for changes
	SyncStatusScanning   // Scanning for changes
	SyncStatusSyncing    // Staging/transferring files
	SyncStatusConflicted // Has conflicts
	SyncStatusError
)

// String returns a human-readable string for the status.
func (s SyncStatus) String() string {
	switch s {
	case SyncStatusDisconnected:
		return "Disconnected"
	case SyncStatusConnecting:
		return "Connecting"
	case SyncStatusWatching:
		return "Watching"
	case SyncStatusScanning:
		return "Scanning"
	case SyncStatusSyncing:
		return "Syncing"
	case SyncStatusConflicted:
		return "Conflicted"
	case SyncStatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// IsActive returns true if the status indicates active sync work (scanning/syncing).
func (s SyncStatus) IsActive() bool {
	return s == SyncStatusScanning || s == SyncStatusSyncing || s == SyncStatusConnecting
}

// SessionStatus contains detailed status information from a mutagen session.
type SessionStatus struct {
	Status     SyncStatus
	StatusText string
	Conflicts  []string
	Problems   []string
	LastUpdate time.Time
}

// Mutagen manages file sync sessions with containers.
type Mutagen struct{}

// NewMutagen creates a new Mutagen wrapper.
func NewMutagen() *Mutagen {
	return &Mutagen{}
}

// SessionName generates a mutagen session name for a branch.
// Uses a hash suffix to avoid collisions between similar branch names.
func SessionName(branchName string) string {
	h := sha256.Sum256([]byte(branchName))
	hashSuffix := hex.EncodeToString(h[:4]) // 8 hex chars

	safe := strings.ReplaceAll(branchName, "/", "-")
	if len(safe) > 20 {
		safe = safe[:20]
	}
	return "ccells-" + safe + "-" + hashSuffix
}

// CheckInstalled verifies mutagen is available.
func (m *Mutagen) CheckInstalled(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "mutagen", "version")
	return cmd.Run()
}

// CreateSession starts a bidirectional sync between container and local path.
func (m *Mutagen) CreateSession(ctx context.Context, branchName, containerID, localPath string) error {
	sessionName := SessionName(branchName)
	containerPath := fmt.Sprintf("docker://%s/workspace", containerID)

	cmd := exec.CommandContext(ctx, "mutagen", "sync", "create",
		"--name", sessionName,
		"--sync-mode", "two-way-resolved",
		"--ignore", ".git",
		containerPath,
		localPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mutagen create failed: %w: %s", err, output)
	}
	return nil
}

// TerminateSession stops and removes a sync session.
func (m *Mutagen) TerminateSession(ctx context.Context, branchName string) error {
	sessionName := SessionName(branchName)

	cmd := exec.CommandContext(ctx, "mutagen", "sync", "terminate", sessionName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mutagen terminate failed: %w: %s", err, output)
	}
	return nil
}

// SessionExists checks if a session exists.
func (m *Mutagen) SessionExists(ctx context.Context, branchName string) (bool, error) {
	sessionName := SessionName(branchName)

	cmd := exec.CommandContext(ctx, "mutagen", "sync", "list", sessionName)
	err := cmd.Run()
	if err != nil {
		// Exit code 1 means not found
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetConflicts returns any sync conflict files.
func (m *Mutagen) GetConflicts(ctx context.Context, branchName string) ([]string, error) {
	sessionName := SessionName(branchName)

	cmd := exec.CommandContext(ctx, "mutagen", "sync", "list", "--long", sessionName)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Parse output for conflict indicators
	var conflicts []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Conflicts:") {
			// Extract conflict info - simplified parsing
			conflicts = append(conflicts, strings.TrimSpace(line))
		}
	}
	return conflicts, nil
}

// GetSessionStatus returns detailed status information for a sync session.
// It parses the output of `mutagen sync list --long` to determine the current state.
func (m *Mutagen) GetSessionStatus(ctx context.Context, branchName string) (*SessionStatus, error) {
	sessionName := SessionName(branchName)

	cmd := exec.CommandContext(ctx, "mutagen", "sync", "list", "--long", sessionName)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// Session doesn't exist
			return &SessionStatus{
				Status:     SyncStatusDisconnected,
				StatusText: "Session not found",
				LastUpdate: time.Now(),
			}, nil
		}
		return &SessionStatus{
			Status:     SyncStatusError,
			StatusText: err.Error(),
			LastUpdate: time.Now(),
		}, err
	}

	return parseSessionStatus(string(output)), nil
}

// parseSessionStatus extracts SyncStatus from mutagen sync list output.
// Example output lines:
//
//	Status: Watching for changes
//	Status: Scanning (alpha)
//	Status: Staging files on beta
//	Status: Reconciling changes
//	Status: Connecting to beta
func parseSessionStatus(output string) *SessionStatus {
	status := &SessionStatus{
		Status:     SyncStatusUnknown,
		LastUpdate: time.Now(),
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Parse Status line
		if strings.HasPrefix(line, "Status:") {
			statusText := strings.TrimPrefix(line, "Status:")
			statusText = strings.TrimSpace(statusText)
			status.StatusText = statusText

			// Determine SyncStatus from status text
			lower := strings.ToLower(statusText)
			switch {
			case strings.Contains(lower, "watching"):
				status.Status = SyncStatusWatching
			case strings.Contains(lower, "scanning"):
				status.Status = SyncStatusScanning
			case strings.Contains(lower, "staging"),
				strings.Contains(lower, "reconciling"),
				strings.Contains(lower, "transferring"),
				strings.Contains(lower, "applying"):
				status.Status = SyncStatusSyncing
			case strings.Contains(lower, "connecting"):
				status.Status = SyncStatusConnecting
			case strings.Contains(lower, "disconnected"):
				status.Status = SyncStatusDisconnected
			case strings.Contains(lower, "conflict"):
				status.Status = SyncStatusConflicted
			case strings.Contains(lower, "error"),
				strings.Contains(lower, "problem"):
				status.Status = SyncStatusError
			}
		}

		// Parse Conflicts line
		if strings.HasPrefix(line, "Conflicts:") {
			conflictsText := strings.TrimPrefix(line, "Conflicts:")
			conflictsText = strings.TrimSpace(conflictsText)
			if conflictsText != "" && conflictsText != "0" && conflictsText != "None" {
				// May contain count or list
				status.Conflicts = append(status.Conflicts, conflictsText)
				if status.Status != SyncStatusError {
					status.Status = SyncStatusConflicted
				}
			}
		}

		// Parse Problems line
		if strings.HasPrefix(line, "Problems:") || strings.HasPrefix(line, "Problem:") {
			problemText := strings.TrimPrefix(line, "Problems:")
			problemText = strings.TrimPrefix(problemText, "Problem:")
			problemText = strings.TrimSpace(problemText)
			if problemText != "" && problemText != "None" {
				status.Problems = append(status.Problems, problemText)
			}
		}
	}

	return status
}
