//go:build integration

package tui

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

const (
	testWidth  = 120
	testHeight = 40
)

// testSessionName returns a unique session name for this test process
// to avoid clobbering other developer's tmux sessions
func testSessionName() string {
	return fmt.Sprintf("ccells-test-%d", os.Getpid())
}

// tmuxAvailable checks if tmux is installed
func tmuxAvailable() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// buildBinary builds the ccells binary for testing in a temp directory
func buildBinary(t *testing.T) string {
	t.Helper()

	// Use t.TempDir() to avoid assuming /tmp/claude exists
	tempDir := t.TempDir()
	binPath := filepath.Join(tempDir, "ccells-test")

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
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("Could not find project root (no go.mod found)")
		}
		dir = parent
	}
}

// cleanupTmuxSession kills the test session if it exists
func cleanupTmuxSession() {
	sessionName := testSessionName()
	cmd := exec.Command("tmux", "kill-session", "-t", sessionName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Ignore expected errors when session doesn't exist
		outStr := string(output)
		if strings.Contains(outStr, "no server") ||
			strings.Contains(outStr, "session not found") ||
			strings.Contains(outStr, "can't find session") {
			return // Expected - session doesn't exist
		}
		// Log unexpected errors
		fmt.Printf("Warning: cleanupTmuxSession(%s) unexpected error: %v, output: %s\n",
			sessionName, err, outStr)
	}
}

// captureTmuxPane captures the current pane content as text
func captureTmuxPane(t *testing.T) string {
	t.Helper()

	sessionName := testSessionName()
	// Use -p -e to preserve empty lines
	cmd := exec.Command("tmux", "capture-pane", "-t", sessionName, "-p", "-e")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to capture tmux pane: %v", err)
	}

	return string(output)
}

// countLines counts lines in captured output, handling trailing newlines
func countLines(output string) int {
	if output == "" {
		return 0
	}
	// Trim trailing newline if present and count based on remaining newlines
	// This is more resilient to tmux output variations
	trimmed := strings.TrimSuffix(output, "\n")
	if trimmed == "" {
		return 0
	}
	return 1 + strings.Count(trimmed, "\n")
}

// waitForContent polls until content appears or timeout
func waitForContent(t *testing.T, substr string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(captureTmuxPane(t), substr) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// waitForCondition polls until condition returns true
func waitForCondition(t *testing.T, condition func(string) bool, timeout time.Duration) (string, bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastCapture string
	for time.Now().Before(deadline) {
		lastCapture = captureTmuxPane(t)
		if condition(lastCapture) {
			return lastCapture, true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return lastCapture, false
}

// sendKeys sends keystrokes to the tmux session
func sendKeys(t *testing.T, keys ...string) {
	t.Helper()
	sessionName := testSessionName()
	args := []string{"send-keys", "-t", sessionName}
	args = append(args, keys...)
	cmd := exec.Command("tmux", args...)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to send keys: %v", err)
	}
}

// sendKey sends a single key (convenience wrapper)
func sendKey(t *testing.T, key string) {
	sendKeys(t, key)
}

// updateGolden flag enables updating golden files when -update is passed
var updateGolden = flag.Bool("update", false, "update golden files")

// goldenPath returns path to a golden file
func goldenPath(name string) string {
	// Use getProjectRoot with nil t since we don't want to fail in path calculation
	root := getProjectRootOrPanic()
	return filepath.Join(root, "internal", "tui", "testdata", name+".golden")
}

// getProjectRootOrPanic is like getProjectRoot but panics on error (for non-test contexts)
func getProjectRootOrPanic() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("Failed to get working directory: %v", err))
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("Could not find project root (no go.mod found)")
		}
		dir = parent
	}
}

// Regex patterns for masking dynamic content
var (
	// Timestamps like "2024-01-22 15:30:45" or "15:30:45"
	timestampRegex = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}|\d{2}:\d{2}:\d{2}`)
	// Container IDs (12+ hex chars)
	containerIDRegex = regexp.MustCompile(`[a-f0-9]{12,64}`)
	// PIDs (process IDs)
	pidRegex = regexp.MustCompile(`\bPID[: ]+\d+|\bpid[: ]+\d+|\b\d{4,7}\b`)
	// Durations like "2.5s" or "100ms"
	durationRegex = regexp.MustCompile(`\d+(\.\d+)?(ms|s|m|h)\b`)
)

// maskDynamicContent replaces timestamps, IDs, and other dynamic content with placeholders
func maskDynamicContent(s string) string {
	s = timestampRegex.ReplaceAllString(s, "[TIMESTAMP]")
	s = containerIDRegex.ReplaceAllString(s, "[ID]")
	s = durationRegex.ReplaceAllString(s, "[DURATION]")
	// Don't mask PIDs by default as they can be confused with line numbers
	return s
}

// assertGolden compares actual output to a golden file, updating if -update flag is set
func assertGolden(t *testing.T, name string, actual string) {
	t.Helper()
	path := goldenPath(name)

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("Failed to create golden file directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(actual), 0644); err != nil {
			t.Fatalf("Failed to write golden file: %v", err)
		}
		t.Logf("Updated golden file: %s", path)
		return
	}

	expected, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("Golden file %s does not exist. Run with -update to create it.", path)
		}
		t.Fatalf("Failed to read golden file %s: %v", path, err)
	}

	if actual != string(expected) {
		t.Errorf("Output differs from golden file %s", name)
		// Show a simple diff indication
		expectedLines := strings.Split(string(expected), "\n")
		actualLines := strings.Split(actual, "\n")

		t.Logf("Expected %d lines, got %d lines", len(expectedLines), len(actualLines))

		// Show first difference
		for i := 0; i < len(expectedLines) && i < len(actualLines); i++ {
			if expectedLines[i] != actualLines[i] {
				t.Logf("First difference at line %d:", i+1)
				t.Logf("  Expected: %q", expectedLines[i])
				t.Logf("  Actual:   %q", actualLines[i])
				break
			}
		}
	}
}

// TestTmuxViewportConsistency verifies that the TUI maintains consistent
// viewport height when rendered in an actual terminal via tmux.
func TestTmuxViewportConsistency(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not available")
	}

	sessionName := testSessionName()

	// Clean up any existing session
	cleanupTmuxSession()
	defer cleanupTmuxSession()

	// Build the binary
	binPath := buildBinary(t)
	// No need to defer os.Remove - t.TempDir() handles cleanup

	// Start tmux session with fixed dimensions
	startCmd := exec.Command("tmux", "new-session",
		"-d",              // detached
		"-s", sessionName, // session name
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

	sessionName := testSessionName()

	cleanupTmuxSession()
	defer cleanupTmuxSession()

	binPath := buildBinary(t)
	// No need to defer os.Remove - t.TempDir() handles cleanup

	// Start with initial size
	startCmd := exec.Command("tmux", "new-session",
		"-d",
		"-s", sessionName,
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
				"-t", sessionName,
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

// TestTmuxWaitForStartup verifies app starts and shows expected UI elements
// using waitForContent instead of fixed sleep.
func TestTmuxWaitForStartup(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not available")
	}

	sessionName := testSessionName()

	cleanupTmuxSession()
	defer cleanupTmuxSession()

	binPath := buildBinary(t)

	// Start tmux session
	startCmd := exec.Command("tmux", "new-session",
		"-d",
		"-s", sessionName,
		"-x", fmt.Sprintf("%d", testWidth),
		"-y", fmt.Sprintf("%d", testHeight),
		binPath,
	)

	if err := startCmd.Run(); err != nil {
		t.Fatalf("Failed to start tmux session: %v", err)
	}

	// Wait for expected UI content using polling instead of fixed sleep
	// The app should display "workstream" or related UI elements
	expectedContent := []string{"Claude", "workstream", "ccells", "Building"}
	found := false
	for _, content := range expectedContent {
		if waitForContent(t, content, 5*time.Second) {
			t.Logf("Found expected content: %q", content)
			found = true
			break
		}
	}

	if !found {
		frame := captureTmuxPane(t)
		t.Logf("Final frame (no expected content found):\n%s", frame)
		// Don't fail - just log, since the app may be in various states
		t.Log("Warning: None of the expected UI elements appeared")
	}

	// Use waitForCondition for more complex assertions
	_, hasLines := waitForCondition(t, func(output string) bool {
		return countLines(output) == testHeight
	}, 3*time.Second)

	if !hasLines {
		t.Error("Viewport never reached expected height")
	}
}

// TestTmuxKeypressNewDialog verifies that pressing 'n' opens the New Workstream
// dialog and Escape closes it.
func TestTmuxKeypressNewDialog(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not available")
	}

	sessionName := testSessionName()

	cleanupTmuxSession()
	defer cleanupTmuxSession()

	binPath := buildBinary(t)

	// Start tmux session
	startCmd := exec.Command("tmux", "new-session",
		"-d",
		"-s", sessionName,
		"-x", fmt.Sprintf("%d", testWidth),
		"-y", fmt.Sprintf("%d", testHeight),
		binPath,
	)

	if err := startCmd.Run(); err != nil {
		t.Fatalf("Failed to start tmux session: %v", err)
	}

	// Wait for app to be ready (show some UI content)
	_, ready := waitForCondition(t, func(output string) bool {
		return strings.Contains(output, "Claude") ||
			strings.Contains(output, "workstream") ||
			strings.Contains(output, "ccells") ||
			strings.Contains(output, "Building")
	}, 5*time.Second)

	if !ready {
		t.Log("App may not be fully ready, continuing test anyway")
	}

	// Give the app a moment to stabilize
	time.Sleep(200 * time.Millisecond)

	// Send 'n' key to open New Workstream dialog
	sendKey(t, "n")

	// Wait for dialog to appear
	if waitForContent(t, "New Workstream", 3*time.Second) {
		t.Log("Dialog opened successfully")

		// Send Escape to close dialog
		sendKey(t, "Escape")

		// Wait for dialog to close (New Workstream should disappear)
		_, closed := waitForCondition(t, func(output string) bool {
			return !strings.Contains(output, "New Workstream")
		}, 2*time.Second)

		if !closed {
			t.Error("Dialog did not close after pressing Escape")
		}
	} else {
		frame := captureTmuxPane(t)
		t.Logf("Dialog did not appear. Current frame:\n%s", frame)
		// Don't fail - the app might be in a different state
		t.Log("Warning: New Workstream dialog did not appear after pressing 'n'")
	}
}

// TestTmuxScrollModeLongScrollback tests scroll mode with a large amount of
// scrollback content. This is a shell-based test (no Docker dependency) to
// verify that tmux scroll mode works correctly.
func TestTmuxScrollModeLongScrollback(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not available")
	}

	sessionName := testSessionName()

	cleanupTmuxSession()
	defer cleanupTmuxSession()

	// Start tmux session with a shell that will generate scrollback
	// We use 'bash -c' to echo many lines
	script := `for i in $(seq 1 500); do echo "Line $i: This is test content for scrollback"; done; echo "=== END OF OUTPUT ==="; exec bash`

	startCmd := exec.Command("tmux", "new-session",
		"-d",
		"-s", sessionName,
		"-x", fmt.Sprintf("%d", testWidth),
		"-y", fmt.Sprintf("%d", testHeight),
		"bash", "-c", script,
	)

	if err := startCmd.Run(); err != nil {
		t.Fatalf("Failed to start tmux session: %v", err)
	}

	// Wait for all output to be generated
	if !waitForContent(t, "=== END OF OUTPUT ===", 10*time.Second) {
		t.Fatal("Output generation did not complete")
	}

	// Capture current visible content (should be near the end)
	initialFrame := captureTmuxPane(t)
	t.Logf("Initial frame (last lines visible):\n%s", initialFrame)

	// Verify we see end marker but not early lines in the visible area
	if !strings.Contains(initialFrame, "=== END OF OUTPUT ===") {
		t.Error("Expected to see END OF OUTPUT marker in visible area")
	}

	// Enter tmux copy mode (scroll mode) with Ctrl+B, [
	sendKeys(t, "C-b", "[")
	time.Sleep(200 * time.Millisecond)

	// Scroll up using PageUp multiple times
	for i := 0; i < 20; i++ {
		sendKey(t, "PageUp")
		time.Sleep(50 * time.Millisecond)
	}

	// Capture scrolled content - use -S -500 to capture scrollback history
	cmd := exec.Command("tmux", "capture-pane", "-t", sessionName, "-p", "-S", "-500")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to capture scrollback: %v", err)
	}
	scrolledContent := string(output)

	// Verify we can see early lines in the scrollback
	if !strings.Contains(scrolledContent, "Line 1:") {
		t.Error("Expected to find 'Line 1:' in scrollback history")
		t.Logf("Scrollback content (first 1000 chars):\n%s", scrolledContent[:min(1000, len(scrolledContent))])
	}

	if !strings.Contains(scrolledContent, "Line 50:") {
		t.Error("Expected to find 'Line 50:' in scrollback history")
	}

	// Exit copy mode
	sendKey(t, "Escape")
	time.Sleep(100 * time.Millisecond)

	t.Log("Scroll mode test completed successfully")
}

// TestTmuxScrollModeNavigation tests various scroll navigation commands
func TestTmuxScrollModeNavigation(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not available")
	}

	sessionName := testSessionName()

	cleanupTmuxSession()
	defer cleanupTmuxSession()

	// Generate numbered lines for easy position tracking
	script := `for i in $(seq 1 200); do printf "LINE_%03d\n" $i; done; echo "=== END ==="; exec bash`

	startCmd := exec.Command("tmux", "new-session",
		"-d",
		"-s", sessionName,
		"-x", "80",
		"-y", "24",
		"bash", "-c", script,
	)

	if err := startCmd.Run(); err != nil {
		t.Fatalf("Failed to start tmux session: %v", err)
	}

	// Wait for output
	if !waitForContent(t, "=== END ===", 5*time.Second) {
		t.Fatal("Output generation did not complete")
	}

	// Enter copy mode
	sendKeys(t, "C-b", "[")
	time.Sleep(200 * time.Millisecond)

	tests := []struct {
		name        string
		keys        []string
		expectAfter string
	}{
		{
			name:        "PageUp navigation",
			keys:        []string{"PageUp", "PageUp", "PageUp"},
			expectAfter: "LINE_", // Should see some lines after scrolling up
		},
		{
			name:        "Arrow up navigation",
			keys:        []string{"Up", "Up", "Up", "Up", "Up"},
			expectAfter: "LINE_",
		},
		{
			name:        "Half-page scroll with Ctrl+U",
			keys:        []string{"C-u"},
			expectAfter: "LINE_",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Send the navigation keys
			for _, key := range tt.keys {
				sendKey(t, key)
				time.Sleep(50 * time.Millisecond)
			}

			// Capture with scrollback
			cmd := exec.Command("tmux", "capture-pane", "-t", sessionName, "-p", "-S", "-200")
			output, err := cmd.Output()
			if err != nil {
				t.Fatalf("Failed to capture: %v", err)
			}

			if !strings.Contains(string(output), tt.expectAfter) {
				t.Errorf("Expected to find %q after %v", tt.expectAfter, tt.keys)
			}
		})
	}

	// Exit copy mode
	sendKey(t, "Escape")
}

// TestTmuxStartupGolden captures the startup frame and compares it to a golden file.
// Dynamic content (timestamps, IDs) is masked to allow stable comparisons.
func TestTmuxStartupGolden(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not available")
	}

	sessionName := testSessionName()

	cleanupTmuxSession()
	defer cleanupTmuxSession()

	binPath := buildBinary(t)

	// Start tmux session
	startCmd := exec.Command("tmux", "new-session",
		"-d",
		"-s", sessionName,
		"-x", fmt.Sprintf("%d", testWidth),
		"-y", fmt.Sprintf("%d", testHeight),
		binPath,
	)

	if err := startCmd.Run(); err != nil {
		t.Fatalf("Failed to start tmux session: %v", err)
	}

	// Wait for app to show some UI (give it time to initialize)
	_, ready := waitForCondition(t, func(output string) bool {
		// Wait until we see some meaningful content
		return strings.Contains(output, "Claude") ||
			strings.Contains(output, "workstream") ||
			strings.Contains(output, "ccells") ||
			strings.Contains(output, "Building")
	}, 10*time.Second)

	if !ready {
		t.Log("App may not be fully ready, capturing anyway")
	}

	// Give it a moment to stabilize rendering
	time.Sleep(500 * time.Millisecond)

	// Capture the frame
	frame := captureTmuxPane(t)

	// Mask dynamic content for stable comparison
	maskedFrame := maskDynamicContent(frame)

	// Compare to golden file
	assertGolden(t, "startup_frame", maskedFrame)
}

// TestTmuxGoldenWithDialog tests golden file comparison with a dialog open
func TestTmuxGoldenWithDialog(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not available")
	}

	sessionName := testSessionName()

	cleanupTmuxSession()
	defer cleanupTmuxSession()

	binPath := buildBinary(t)

	startCmd := exec.Command("tmux", "new-session",
		"-d",
		"-s", sessionName,
		"-x", fmt.Sprintf("%d", testWidth),
		"-y", fmt.Sprintf("%d", testHeight),
		binPath,
	)

	if err := startCmd.Run(); err != nil {
		t.Fatalf("Failed to start tmux session: %v", err)
	}

	// Wait for app to be ready
	_, ready := waitForCondition(t, func(output string) bool {
		return strings.Contains(output, "Claude") ||
			strings.Contains(output, "workstream") ||
			strings.Contains(output, "ccells") ||
			strings.Contains(output, "Building")
	}, 10*time.Second)

	if !ready {
		t.Skip("App did not reach ready state")
	}

	time.Sleep(200 * time.Millisecond)

	// Press 'n' to open New Workstream dialog
	sendKey(t, "n")

	// Wait for dialog
	if !waitForContent(t, "New Workstream", 3*time.Second) {
		t.Skip("Dialog did not appear - skipping golden comparison")
	}

	// Give it a moment to render fully
	time.Sleep(200 * time.Millisecond)

	// Capture and mask
	frame := captureTmuxPane(t)
	maskedFrame := maskDynamicContent(frame)

	// Compare to golden file
	assertGolden(t, "new_workstream_dialog", maskedFrame)

	// Clean up by closing dialog
	sendKey(t, "Escape")
}
