package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// configMu protects concurrent config file access
var configMu sync.Mutex

const (
	configDir  = ".ccells"
	configFile = "config.json"
)

// GlobalConfig represents the global ccells configuration
// stored in ~/.ccells/config.json
type GlobalConfig struct {
	Version           int  `json:"version"`
	IntroductionShown bool `json:"introduction_shown"`
}

// ConfigDir returns the path to the ccells config directory (~/.ccells)
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDir), nil
}

// ConfigPath returns the full path to the config file
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFile), nil
}

// Load loads the global configuration from disk
// Returns a default config if the file doesn't exist
func Load() (*GlobalConfig, error) {
	configMu.Lock()
	defer configMu.Unlock()

	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Return default config for first run
		return &GlobalConfig{Version: 1}, nil
	}
	if err != nil {
		return nil, err
	}

	var cfg GlobalConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Save saves the global configuration to disk
func Save(cfg *GlobalConfig) error {
	configMu.Lock()
	defer configMu.Unlock()

	dir, err := ConfigDir()
	if err != nil {
		return err
	}

	// Ensure config directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path := filepath.Join(dir, configFile)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// IsFirstRun returns true if this is the first time ccells is being run
func IsFirstRun() bool {
	cfg, err := Load()
	if err != nil {
		// If we can't read config, treat as first run
		return true
	}
	return !cfg.IntroductionShown
}

// MarkIntroductionShown marks the introduction as having been shown
func MarkIntroductionShown() error {
	cfg, err := Load()
	if err != nil {
		cfg = &GlobalConfig{Version: 1}
	}
	cfg.IntroductionShown = true
	return Save(cfg)
}
