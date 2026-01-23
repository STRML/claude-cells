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
	configDir    = ".claude-cells"
	appStateFile = "app-state.json"
	legacyConfig = "config.json" // Deprecated: migrated to app-state.json
)

// GlobalConfig represents the global ccells application state
// stored in ~/.claude-cells/app-state.json (internal, not user-editable)
type GlobalConfig struct {
	Version           int  `json:"version"`
	IntroductionShown bool `json:"introduction_shown"`
}

// ConfigDir returns the path to the ccells config directory (~/.claude-cells)
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDir), nil
}

// ConfigPath returns the full path to the app state file
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appStateFile), nil
}

// legacyConfigPath returns the path to the deprecated config.json
func legacyConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, legacyConfig), nil
}

// Load loads the global application state from disk
// Returns a default config if the file doesn't exist
// Automatically migrates from legacy config.json if needed
func Load() (*GlobalConfig, error) {
	configMu.Lock()
	defer configMu.Unlock()

	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Check for legacy config.json and migrate if found
		legacyPath, legacyErr := legacyConfigPath()
		if legacyErr == nil {
			legacyData, legacyReadErr := os.ReadFile(legacyPath)
			if legacyReadErr == nil {
				// Migrate legacy config
				var cfg GlobalConfig
				if jsonErr := json.Unmarshal(legacyData, &cfg); jsonErr == nil {
					// Write to new location (ignore save error, will retry on next save)
					saveData, _ := json.MarshalIndent(&cfg, "", "  ")
					_ = os.WriteFile(path, saveData, 0644)
					// Remove legacy file
					_ = os.Remove(legacyPath)
					return &cfg, nil
				}
			}
		}
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

	path := filepath.Join(dir, appStateFile)

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
