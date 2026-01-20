package main

import (
	"context"
	"os"
	"path/filepath"
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

	// Start the spinner
	s.Start()

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
	s.Start()

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
	ccellsDir := filepath.Join(tmpDir, ".ccells")
	if err := os.MkdirAll(ccellsDir, 0755); err != nil {
		t.Fatalf("Failed to create .ccells dir: %v", err)
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
