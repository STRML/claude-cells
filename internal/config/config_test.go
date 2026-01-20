package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReturnsDefaultOnFirstRun(t *testing.T) {
	// Use a temp directory as home
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Version != 1 {
		t.Errorf("cfg.Version = %d, want 1", cfg.Version)
	}

	if cfg.IntroductionShown {
		t.Errorf("cfg.IntroductionShown = true, want false for first run")
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg := &GlobalConfig{
		Version:           1,
		IntroductionShown: true,
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file was created
	path := filepath.Join(tmpHome, configDir, configFile)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}

	// Load and verify
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.Version != cfg.Version {
		t.Errorf("loaded.Version = %d, want %d", loaded.Version, cfg.Version)
	}

	if loaded.IntroductionShown != cfg.IntroductionShown {
		t.Errorf("loaded.IntroductionShown = %v, want %v", loaded.IntroductionShown, cfg.IntroductionShown)
	}
}

func TestIsFirstRun(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Should be first run initially
	if !IsFirstRun() {
		t.Error("IsFirstRun() = false, want true for fresh config dir")
	}

	// Mark as shown
	if err := MarkIntroductionShown(); err != nil {
		t.Fatalf("MarkIntroductionShown() error = %v", err)
	}

	// Should no longer be first run
	if IsFirstRun() {
		t.Error("IsFirstRun() = true, want false after marking shown")
	}
}

func TestMarkIntroductionShown(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	if err := MarkIntroductionShown(); err != nil {
		t.Fatalf("MarkIntroductionShown() error = %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.IntroductionShown {
		t.Error("cfg.IntroductionShown = false after MarkIntroductionShown()")
	}
}

func TestConfigDirCreatedOnSave(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	configDirPath := filepath.Join(tmpHome, configDir)

	// Verify dir doesn't exist yet
	if _, err := os.Stat(configDirPath); !os.IsNotExist(err) {
		t.Fatal("Config dir should not exist before Save()")
	}

	cfg := &GlobalConfig{Version: 1}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify dir was created
	info, err := os.Stat(configDirPath)
	if os.IsNotExist(err) {
		t.Fatal("Config dir was not created")
	}
	if !info.IsDir() {
		t.Fatal("Config path is not a directory")
	}
}
