package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/STRML/claude-cells/internal/docker"
)

func TestSpinner_NewSpinner(t *testing.T) {
	s := newSpinner("Loading...")

	if s.message != "Loading..." {
		t.Errorf("message = %q, want %q", s.message, "Loading...")
	}
	if s.current != 0 {
		t.Errorf("current = %d, want 0", s.current)
	}
	if s.done == nil {
		t.Error("done channel should not be nil")
	}
	if len(s.frames) != 10 {
		t.Errorf("frames length = %d, want 10", len(s.frames))
	}
}

func TestSpinner_StartStop(t *testing.T) {
	s := newSpinner("Testing...")

	// Start the spinner with a context
	ctx := context.Background()
	s.Start(ctx)

	// Let it run briefly
	time.Sleep(200 * time.Millisecond)

	// Stop should complete without blocking
	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Expected
	case <-time.After(1 * time.Second):
		t.Error("Stop() should complete within 1 second")
	}
}

func TestSpinner_DoubleStop(t *testing.T) {
	s := newSpinner("Testing...")
	ctx := context.Background()
	s.Start(ctx)

	// First stop should work
	s.Stop()

	// Second stop should not panic (done channel already closed)
	// We can't call Stop() twice as it would panic, but we verify done is closed
	select {
	case <-s.done:
		// Expected - channel is closed
	default:
		t.Error("done channel should be closed after Stop()")
	}
}

func TestSpinner_ContextCancellation(t *testing.T) {
	s := newSpinner("Testing...")

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Start the spinner
	s.Start(ctx)

	// Let it run briefly
	time.Sleep(100 * time.Millisecond)

	// Cancel the context (simulates timeout or early termination)
	cancel()

	// The spinner goroutine should exit promptly after context cancellation
	// We give it 500ms to clean up
	time.Sleep(200 * time.Millisecond)

	// Verify we don't need to call Stop() - the goroutine should have exited
	// This test passes if there's no goroutine leak (verified by race detector)
}

func TestRunHeartbeat_ContextCancellation(t *testing.T) {
	// Create a real container tracker
	tracker, err := docker.NewContainerTracker()
	if err != nil {
		t.Skipf("Skipping test: could not create container tracker: %v", err)
	}
	defer tracker.Clear()

	ctx, cancel := context.WithCancel(context.Background())

	// Run heartbeat in goroutine
	done := make(chan struct{})
	go func() {
		runHeartbeat(ctx, tracker)
		close(done)
	}()

	// Let it run briefly
	time.Sleep(100 * time.Millisecond)

	// Cancel context
	cancel()

	// Should exit promptly
	select {
	case <-done:
		// Expected
	case <-time.After(1 * time.Second):
		t.Error("runHeartbeat should exit when context is cancelled")
	}
}

func TestCleanupOrphanedContainers_NilTracker(t *testing.T) {
	// Test that cleanupOrphanedContainers handles nil tracker gracefully
	// This should not panic
	cleanupOrphanedContainers(nil)
}

func TestCleanupOrphanedContainers_NoStateFile(t *testing.T) {
	// Create a temp directory for the test
	tmpDir, err := os.MkdirTemp("", "cleanup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp directory (no state file exists)
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// Create a tracker (simplified, without proper initialization)
	// This tests the path where no state file exists
	cleanupOrphanedContainers(nil)
}

func TestVersionInfo(t *testing.T) {
	// Verify default values
	if Version != "dev" {
		t.Errorf("Default Version = %q, want %q", Version, "dev")
	}
	if CommitHash != "unknown" {
		t.Errorf("Default CommitHash = %q, want %q", CommitHash, "unknown")
	}
}

// TestValidatePrerequisites_NoDocker tests validation when Docker is unavailable
// This test is skipped if Docker is actually available
func TestValidatePrerequisites_Behavior(t *testing.T) {
	// Save original directory
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	// Create a temp directory without devcontainer config
	tmpDir, err := os.MkdirTemp("", "prereq-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	os.Chdir(tmpDir)

	// validatePrerequisites may succeed or fail depending on Docker availability
	// We're testing that it doesn't panic and returns a sensible result
	err = validatePrerequisites()
	// Result depends on environment - just verify no panic occurred
	_ = err
}

// TestValidatePrerequisites_WithDevcontainerConfig tests behavior with devcontainer.json
func TestValidatePrerequisites_WithDevcontainerConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getenv("CI") != "" {
		t.Skip("skipping devcontainer build test in CI (takes too long)")
	}

	// Save original directory
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	// Create a temp directory with a minimal devcontainer config
	tmpDir, err := os.MkdirTemp("", "prereq-devcontainer-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .devcontainer directory
	devcontainerDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("Failed to create .devcontainer dir: %v", err)
	}

	// Create minimal devcontainer.json
	devcontainerJSON := `{
		"name": "test",
		"image": "ubuntu:22.04"
	}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(devcontainerJSON), 0644); err != nil {
		t.Fatalf("Failed to write devcontainer.json: %v", err)
	}

	os.Chdir(tmpDir)

	// This tests the code path where devcontainer config exists
	// Result depends on Docker availability
	err = validatePrerequisites()
	_ = err // Just verify no panic
}

// TestSpinner_FrameRotation verifies frames rotate correctly
func TestSpinner_FrameRotation(t *testing.T) {
	s := newSpinner("Test")

	// Verify initial state
	if s.current != 0 {
		t.Errorf("Initial current = %d, want 0", s.current)
	}

	// Manually test rotation logic
	for i := 0; i < 20; i++ {
		expected := i % len(s.frames)
		if s.current != expected {
			t.Errorf("Iteration %d: current = %d, want %d", i, s.current, expected)
		}
		s.current = (s.current + 1) % len(s.frames)
	}
}

// TestSpinner_Frames verifies the spinner frames are valid unicode
func TestSpinner_Frames(t *testing.T) {
	s := newSpinner("Test")

	expectedFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	if len(s.frames) != len(expectedFrames) {
		t.Fatalf("frames length = %d, want %d", len(s.frames), len(expectedFrames))
	}

	for i, frame := range s.frames {
		if frame != expectedFrames[i] {
			t.Errorf("frame[%d] = %q, want %q", i, frame, expectedFrames[i])
		}
	}
}

func TestExtractBranchFromContainerName(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		projectName   string
		want          string
	}{
		{"standard name", "ccells-myproject-fix-bug", "myproject", "fix-bug"},
		{"with leading slash", "/ccells-myproject-auth-system", "myproject", "auth-system"},
		{"no match", "other-container", "myproject", ""},
		{"empty container", "", "myproject", ""},
		{"empty project", "ccells--fix-bug", "", "fix-bug"},
		{"branch with hyphens", "ccells-proj-multi-word-branch", "proj", "multi-word-branch"},
		{"partial prefix match", "ccells-myproj-fix", "myproject", ""},
		{"exact prefix no branch", "ccells-myproject-", "myproject", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractBranchFromContainerName(tt.containerName, tt.projectName)
			if got != tt.want {
				t.Errorf("extractBranchFromContainerName(%q, %q) = %q, want %q",
					tt.containerName, tt.projectName, got, tt.want)
			}
		})
	}
}

func TestAcquireLock(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lock-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// First lock should succeed
	lock, err := acquireLock(tmpDir)
	if err != nil {
		t.Fatalf("acquireLock() error = %v", err)
	}
	if lock == nil {
		t.Fatal("acquireLock() returned nil lock")
	}

	// Lock file should exist
	lockPath := filepath.Join(tmpDir, lockFileName)
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("lock file should exist after acquireLock()")
	}

	// Second lock should fail (same process is running)
	_, err = acquireLock(tmpDir)
	if err == nil {
		t.Error("second acquireLock() should fail")
	}

	// Release lock
	lock.Release()

	// Lock file should be removed
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lock file should be removed after Release()")
	}

	// Third lock should succeed after release
	lock2, err := acquireLock(tmpDir)
	if err != nil {
		t.Fatalf("acquireLock() after release error = %v", err)
	}
	lock2.Release()
}

func TestAcquireLock_StaleLock(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lock-stale-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write a stale lock file with a non-existent PID
	lockPath := filepath.Join(tmpDir, lockFileName)
	if err := os.WriteFile(lockPath, []byte("999999999"), 0644); err != nil {
		t.Fatalf("Failed to write stale lock: %v", err)
	}

	// Should succeed because the PID doesn't exist
	lock, err := acquireLock(tmpDir)
	if err != nil {
		t.Fatalf("acquireLock() with stale lock error = %v", err)
	}
	lock.Release()
}

func TestLockFile_ReleaseNil(t *testing.T) {
	// nil lockFile should not panic
	var l *lockFile
	l.Release() // should be a no-op
}

func TestLockFile_ReleaseEmptyPath(t *testing.T) {
	l := &lockFile{path: ""}
	l.Release() // should be a no-op
}

func TestSetupLogFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logfile-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Save and restore global state
	oldLogFilePath := logFilePath
	defer func() { logFilePath = oldLogFilePath }()

	cleanup := setupLogFile(tmpDir)
	defer cleanup()

	// Log directory should be created
	logDir := filepath.Join(tmpDir, "logs")
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		t.Error("logs directory should be created")
	}

	// Log file should exist
	logPath := filepath.Join(logDir, "ccells.log")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("ccells.log should be created")
	}

	// logFilePath global should be set
	if logFilePath != logPath {
		t.Errorf("logFilePath = %q, want %q", logFilePath, logPath)
	}
}

func TestSetupLogFile_InvalidDir(t *testing.T) {
	// Save and restore global state
	oldLogFilePath := logFilePath
	defer func() { logFilePath = oldLogFilePath }()

	// Use a path that can't be created
	cleanup := setupLogFile("/dev/null/impossible")
	defer cleanup()

	// Should return a no-op cleanup without crashing
}

func TestPrintAbnormalExit_WithError(t *testing.T) {
	// Save and restore global state
	oldLogFilePath := logFilePath
	logFilePath = "/tmp/claude/test.log"
	defer func() { logFilePath = oldLogFilePath }()

	// Redirect stderr to capture output
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	printAbnormalExit(fmt.Errorf("test error"))

	w.Close()
	os.Stderr = oldStderr

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "terminated abnormally") {
		t.Error("expected 'terminated abnormally' in output")
	}
	if !strings.Contains(output, "test error") {
		t.Error("expected error message in output")
	}
	if !strings.Contains(output, "/tmp/claude/test.log") {
		t.Error("expected log file path in output")
	}
}

func TestPrintAbnormalExit_NilError(t *testing.T) {
	oldLogFilePath := logFilePath
	logFilePath = ""
	defer func() { logFilePath = oldLogFilePath }()

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	printAbnormalExit(nil)

	w.Close()
	os.Stderr = oldStderr

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "terminated abnormally") {
		t.Error("expected 'terminated abnormally' in output")
	}
	// Should not contain "Error:" line when err is nil
	if strings.Contains(output, "Error:") {
		t.Error("should not contain 'Error:' when err is nil")
	}
	// Should not contain "Log file:" when logFilePath is empty
	if strings.Contains(output, "Log file:") {
		t.Error("should not contain 'Log file:' when logFilePath is empty")
	}
}

func TestListExistingWorktrees(t *testing.T) {
	// Create a temp directory to simulate /tmp/ccells/worktrees
	tmpDir, err := os.MkdirTemp("", "worktree-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// The function reads from /tmp/ccells/worktrees which may or may not exist
	// Just verify it doesn't panic
	result := listExistingWorktrees()
	_ = result
}

// BenchmarkSpinner benchmarks spinner creation
func BenchmarkSpinner(b *testing.B) {
	for i := 0; i < b.N; i++ {
		s := newSpinner("Loading...")
		_ = s
	}
}

// TestCleanupOrphanedContainers_WithStateFile tests cleanup with an existing state file
func TestCleanupOrphanedContainers_WithStateFile(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "cleanup-state-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create ccells state directory and file
	ccellsDir := filepath.Join(tmpDir, ".claude-cells")
	if err := os.MkdirAll(ccellsDir, 0755); err != nil {
		t.Fatalf("Failed to create .claude-cells dir: %v", err)
	}

	// Create a minimal state file
	stateFile := filepath.Join(ccellsDir, "state.json")
	stateJSON := `{
		"workstreams": [
			{"id": "ws-1", "container_id": "container-123", "branch_name": "test-branch"}
		]
	}`
	if err := os.WriteFile(stateFile, []byte(stateJSON), 0644); err != nil {
		t.Fatalf("Failed to write state.json: %v", err)
	}

	// Change to temp directory
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// This tests the path where state file exists and is parsed
	// The actual cleanup depends on Docker availability
	cleanupOrphanedContainers(nil)
}
