package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"
)

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
