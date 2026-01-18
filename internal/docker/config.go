package docker

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const (
	// CellsDir is the directory where ccells stores its data
	CellsDir = ".claude-cells"
	// ClaudeConfigDir is the subdirectory for the copied claude config
	ClaudeConfigDir = "claude-config"
	// ClaudeJSONFile is the claude.json filename
	ClaudeJSONFile = ".claude.json"
	// ClaudeDir is the .claude directory name
	ClaudeDir = ".claude"
	// GitConfigFile is the .gitconfig filename
	GitConfigFile = ".gitconfig"
	// CredentialsFile is the credentials file for OAuth tokens
	CredentialsFile = ".claude-credentials"
)

// ConfigPaths holds paths to the isolated claude config for containers
type ConfigPaths struct {
	ClaudeDir   string // Path to copied .claude directory
	ClaudeJSON  string // Path to copied .claude.json file
	GitConfig   string // Path to copied .gitconfig file
	Credentials string // Path to credentials file (from keychain)
}

var (
	globalConfig     *ConfigPaths
	globalConfigOnce sync.Once
	globalConfigErr  error
)

// GetClaudeConfig returns the isolated claude config paths, initializing if needed.
// This is safe to call from multiple goroutines.
func GetClaudeConfig() (*ConfigPaths, error) {
	globalConfigOnce.Do(func() {
		globalConfig, globalConfigErr = InitClaudeConfig()
	})
	return globalConfig, globalConfigErr
}

// GetCellsDir returns the path to the ccells data directory
func GetCellsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, CellsDir), nil
}

// InitClaudeConfig copies the user's claude config to an isolated directory for container use.
// This is called on ccells startup to ensure containers have fresh config
// without being able to corrupt the user's original config.
func InitClaudeConfig() (*ConfigPaths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Source paths (user's original config)
	srcClaudeDir := filepath.Join(home, ClaudeDir)
	srcClaudeJSON := filepath.Join(home, ClaudeJSONFile)
	srcGitConfig := filepath.Join(home, GitConfigFile)

	// Destination paths (isolated copy for ccells)
	cellsDir, err := GetCellsDir()
	if err != nil {
		return nil, err
	}
	configDir := filepath.Join(cellsDir, ClaudeConfigDir)
	dstClaudeDir := filepath.Join(configDir, ClaudeDir)
	dstClaudeJSON := filepath.Join(configDir, ClaudeJSONFile)
	dstGitConfig := filepath.Join(configDir, GitConfigFile)

	// Try to remove old config directory, but don't fail if cleanup fails.
	// Some files may be locked by running processes, but we can still overwrite
	// the important files we need.
	_ = removeAllSafe(configDir)

	// Create config directory
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Copy .claude directory if it exists
	// First try to remove the destination dir to ensure a clean copy
	_ = removeAllSafe(dstClaudeDir)
	if _, err := os.Stat(srcClaudeDir); err == nil {
		if err := copyDir(srcClaudeDir, dstClaudeDir); err != nil {
			return nil, fmt.Errorf("failed to copy .claude directory: %w", err)
		}
	}

	// Copy .claude.json if it exists (using copyFileForce to ensure overwrite)
	if _, err := os.Stat(srcClaudeJSON); err == nil {
		if err := copyFileForce(srcClaudeJSON, dstClaudeJSON); err != nil {
			return nil, fmt.Errorf("failed to copy .claude.json: %w", err)
		}
	}

	// Copy .gitconfig if it exists (for git identity in containers)
	if _, err := os.Stat(srcGitConfig); err == nil {
		if err := copyFileForce(srcGitConfig, dstGitConfig); err != nil {
			return nil, fmt.Errorf("failed to copy .gitconfig: %w", err)
		}
	}

	// Extract and save OAuth credentials from keychain
	dstCredentials := filepath.Join(configDir, CredentialsFile)
	creds, err := GetClaudeCredentials()
	if err == nil && creds != nil && creds.Raw != "" {
		if err := os.WriteFile(dstCredentials, []byte(creds.Raw), 0600); err != nil {
			return nil, fmt.Errorf("failed to write credentials: %w", err)
		}
	}

	return &ConfigPaths{
		ClaudeDir:   dstClaudeDir,
		ClaudeJSON:  dstClaudeJSON,
		GitConfig:   dstGitConfig,
		Credentials: dstCredentials,
	}, nil
}

// removeAllSafe removes a directory and all its contents
func removeAllSafe(path string) error {
	// First check if path exists
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	}

	// Try os.RemoveAll - it handles most cases
	err := os.RemoveAll(path)
	if err == nil {
		return nil
	}

	// If RemoveAll fails, try walking and removing files manually
	// This handles edge cases with symlinks and permissions
	_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		// Remove files and symlinks first
		if !d.IsDir() {
			os.Remove(p)
		}
		return nil
	})

	// Walk again to remove empty directories (bottom-up would be ideal but Walk is top-down)
	// Just retry RemoveAll which should work now
	return os.RemoveAll(path)
}

// copyFile copies a single file from src to dst
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// copyFileForce removes the destination file before copying to ensure a fresh copy.
// This handles cases where the destination may have restrictive permissions or be locked.
func copyFileForce(src, dst string) error {
	// Try to remove destination first
	_ = os.Remove(dst)
	return copyFile(src, dst)
}

// copyDir recursively copies a directory from src to dst
func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Create destination directory
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		// Skip debug directory - it contains logs that aren't needed and can cause copy issues
		if entry.Name() == "debug" {
			continue
		}

		// Check if it's a symlink
		info, err := entry.Info()
		if err != nil {
			// Skip entries we can't stat (broken symlinks, etc)
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// Copy symlink
			linkTarget, err := os.Readlink(srcPath)
			if err != nil {
				continue // Skip broken symlinks
			}
			if err := os.Symlink(linkTarget, dstPath); err != nil {
				// Ignore symlink errors - they may point to relative paths that don't exist yet
				continue
			}
			continue
		}

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}
