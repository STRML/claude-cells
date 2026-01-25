package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRuntime_FlagOverridesConfig(t *testing.T) {
	// Create temp directory with project config
	tmpDir := t.TempDir()
	projectConfigDir := filepath.Join(tmpDir, ".claude-cells")
	if err := os.MkdirAll(projectConfigDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write project config with runtime: claudesp
	projectConfig := []byte("runtime: claudesp\n")
	if err := os.WriteFile(filepath.Join(projectConfigDir, "config.yaml"), projectConfig, 0644); err != nil {
		t.Fatal(err)
	}

	// CLI flag "claude" should override config "claudesp"
	runtime, err := ResolveRuntime("claude", tmpDir)
	if err != nil {
		t.Errorf("ResolveRuntime failed: %v", err)
	}
	if runtime != "claude" {
		t.Errorf("runtime = %q, want 'claude' (flag should override config)", runtime)
	}
}

func TestResolveRuntime_ConfigUsedWhenFlagEmpty(t *testing.T) {
	// Create temp directory with project config
	tmpDir := t.TempDir()
	projectConfigDir := filepath.Join(tmpDir, ".claude-cells")
	if err := os.MkdirAll(projectConfigDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write project config with runtime: claudesp
	projectConfig := []byte("runtime: claudesp\n")
	if err := os.WriteFile(filepath.Join(projectConfigDir, "config.yaml"), projectConfig, 0644); err != nil {
		t.Fatal(err)
	}

	// Empty flag should use config value
	runtime, err := ResolveRuntime("", tmpDir)
	if err != nil {
		t.Errorf("ResolveRuntime failed: %v", err)
	}
	if runtime != "claudesp" {
		t.Errorf("runtime = %q, want 'claudesp' (from config)", runtime)
	}
}

func TestResolveRuntime_InvalidRuntimeError(t *testing.T) {
	tmpDir := t.TempDir()

	// Invalid runtime should return error
	_, err := ResolveRuntime("invalid-runtime", tmpDir)
	if err == nil {
		t.Error("ResolveRuntime should return error for invalid runtime")
	}
}

func TestResolveRuntime_NormalizesRuntime(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"uppercase", "CLAUDE", "claude"},
		{"mixed case", "ClAuDe", "claude"},
		{"with spaces", "  claude  ", "claude"},
		{"claudesp uppercase", "CLAUDESP", "claudesp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime, err := ResolveRuntime(tt.input, tmpDir)
			if err != nil {
				t.Errorf("ResolveRuntime failed: %v", err)
			}
			if runtime != tt.expected {
				t.Errorf("runtime = %q, want %q", runtime, tt.expected)
			}
		})
	}
}

func TestResolveRuntime_DefaultBehavior(t *testing.T) {
	tmpDir := t.TempDir()

	// Empty flag and no config should default to "claude"
	runtime, err := ResolveRuntime("", tmpDir)
	if err != nil {
		t.Errorf("ResolveRuntime failed: %v", err)
	}
	if runtime != "claude" {
		t.Errorf("runtime = %q, want 'claude' (default)", runtime)
	}
}
