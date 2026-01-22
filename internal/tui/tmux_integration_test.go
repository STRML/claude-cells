//go:build integration

package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const (
	testSessionName = "ccells-test"
	testWidth       = 120
	testHeight      = 40
)

// tmuxAvailable checks if tmux is installed
func tmuxAvailable() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// buildBinary builds the ccells binary for testing
func buildBinary(t *testing.T) string {
	t.Helper()

	binPath := "/tmp/claude/ccells-test"
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/ccells")
	cmd.Dir = getProjectRoot(t)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build binary: %v\n%s", err, output)
	}

	return binPath
}

// getProjectRoot finds the project root directory
func getProjectRoot(t *testing.T) string {
	t.Helper()

	// Walk up from current directory to find go.mod
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(dir + "/go.mod"); err == nil {
			return dir
		}
		parent := dir[:strings.LastIndex(dir, "/")]
		if parent == dir {
			t.Fatal("Could not find project root (no go.mod found)")
		}
		dir = parent
	}
}

// cleanupTmuxSession kills the test session if it exists
func cleanupTmuxSession() {
	exec.Command("tmux", "kill-session", "-t", testSessionName).Run()
}

// captureTmuxPane captures the current pane content as text
func captureTmuxPane(t *testing.T) string {
	t.Helper()

	// Use -p -e to preserve empty lines, -J to join wrapped lines
	cmd := exec.Command("tmux", "capture-pane", "-t", testSessionName, "-p", "-e")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to capture tmux pane: %v", err)
	}

	return string(output)
}

// countLines counts lines in captured output, preserving empty lines
func countLines(output string) int {
	// tmux capture-pane -p adds a trailing newline, so we count newlines
	// An empty 40-line terminal would have 40 newlines
	if output == "" {
		return 0
	}
	return strings.Count(output, "\n")
}

// TestTmuxViewportConsistency verifies that the TUI maintains consistent
// viewport height when rendered in an actual terminal via tmux.
func TestTmuxViewportConsistency(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not available")
	}

	// Clean up any existing session
	cleanupTmuxSession()
	defer cleanupTmuxSession()

	// Build the binary
	binPath := buildBinary(t)
	defer os.Remove(binPath)

	// Start tmux session with fixed dimensions
	startCmd := exec.Command("tmux", "new-session",
		"-d",                  // detached
		"-s", testSessionName, // session name
		"-x", fmt.Sprintf("%d", testWidth), // width
		"-y", fmt.Sprintf("%d", testHeight), // height
		binPath, // command to run
	)

	if err := startCmd.Run(); err != nil {
		t.Fatalf("Failed to start tmux session: %v", err)
	}

	// Wait for app to initialize (may need to build image first time)
	time.Sleep(2 * time.Second)

	// Capture initial frame
	frame1 := captureTmuxPane(t)
	lines1 := countLines(frame1)

	t.Logf("Initial frame has %d lines (expected %d)", lines1, testHeight)

	// The captured output should match the terminal height
	// tmux capture-pane includes all lines up to the terminal height
	if lines1 != testHeight {
		t.Errorf("Frame 1: got %d lines, want %d", lines1, testHeight)
		t.Logf("Frame content:\n%s", frame1)
	}

	// Wait a moment and capture again to verify consistency
	time.Sleep(500 * time.Millisecond)

	frame2 := captureTmuxPane(t)
	lines2 := countLines(frame2)

	if lines2 != testHeight {
		t.Errorf("Frame 2: got %d lines, want %d", lines2, testHeight)
	}

	// Verify the app actually rendered something (not just empty lines)
	// Look for expected UI elements (or building message which is also valid)
	combinedFrames := frame1 + frame2
	hasExpectedContent := strings.Contains(combinedFrames, "Claude") ||
		strings.Contains(combinedFrames, "workstream") ||
		strings.Contains(combinedFrames, "docker") ||
		strings.Contains(combinedFrames, "Building") ||
		strings.Contains(combinedFrames, "ccells")
	if !hasExpectedContent {
		t.Logf("Warning: Frame may not contain expected UI elements:\n%s", frame1)
	}
}

// TestTmuxResizeConsistency verifies that resizing the terminal maintains
// consistent viewport height.
func TestTmuxResizeConsistency(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not available")
	}

	cleanupTmuxSession()
	defer cleanupTmuxSession()

	binPath := buildBinary(t)
	defer os.Remove(binPath)

	// Start with initial size
	startCmd := exec.Command("tmux", "new-session",
		"-d",
		"-s", testSessionName,
		"-x", "100",
		"-y", "30",
		binPath,
	)

	if err := startCmd.Run(); err != nil {
		t.Fatalf("Failed to start tmux session: %v", err)
	}

	// Wait for app to initialize
	time.Sleep(2 * time.Second)

	// Test different resize dimensions
	sizes := []struct{ w, h int }{
		{100, 30},
		{120, 40},
		{80, 24},
		{140, 50},
	}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.w, size.h), func(t *testing.T) {
			// Resize the tmux pane
			resizeCmd := exec.Command("tmux", "resize-window",
				"-t", testSessionName,
				"-x", fmt.Sprintf("%d", size.w),
				"-y", fmt.Sprintf("%d", size.h),
			)
			if err := resizeCmd.Run(); err != nil {
				t.Fatalf("Failed to resize: %v", err)
			}

			// Wait for resize to propagate
			time.Sleep(300 * time.Millisecond)

			frame := captureTmuxPane(t)
			lines := countLines(frame)

			if lines != size.h {
				t.Errorf("After resize to %dx%d: got %d lines, want %d",
					size.w, size.h, lines, size.h)
			}
		})
	}
}
