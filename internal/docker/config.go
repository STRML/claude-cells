package docker

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	GitConfig   string // Path to copied .gitconfig file (empty if no .gitconfig exists)
	Credentials string // Path to credentials file (from keychain)
}

// GitIdentity holds the user's git identity
type GitIdentity struct {
	Name  string
	Email string
}

// GetHostTimezone returns the host's timezone name (e.g., "America/New_York").
// This is used to ensure container commits have the same timezone as the host.
// Returns empty string if timezone cannot be determined.
func GetHostTimezone() string {
	// First check TZ environment variable
	if tz := os.Getenv("TZ"); tz != "" {
		return tz
	}

	// On macOS/Linux, /etc/localtime is typically a symlink to a timezone file
	// like /var/db/timezone/zoneinfo/America/New_York (macOS)
	// or /usr/share/zoneinfo/America/New_York (Linux)
	target, err := filepath.EvalSymlinks("/etc/localtime")
	if err != nil {
		return ""
	}

	// Extract timezone name from path
	// Look for "zoneinfo/" and take everything after it
	const marker = "zoneinfo/"
	if idx := strings.Index(target, marker); idx != -1 {
		return target[idx+len(marker):]
	}

	return ""
}

// GetGitIdentity reads the user's git identity from the host system.
// It first tries git config --global, then falls back to git config (local).
// Returns nil if no identity is configured.
func GetGitIdentity() *GitIdentity {
	var name, email string

	// Try to get user.name from git config
	if out, err := exec.Command("git", "config", "--global", "user.name").Output(); err == nil {
		name = strings.TrimSpace(string(out))
	} else if out, err := exec.Command("git", "config", "user.name").Output(); err == nil {
		name = strings.TrimSpace(string(out))
	}

	// Try to get user.email from git config
	if out, err := exec.Command("git", "config", "--global", "user.email").Output(); err == nil {
		email = strings.TrimSpace(string(out))
	} else if out, err := exec.Command("git", "config", "user.email").Output(); err == nil {
		email = strings.TrimSpace(string(out))
	}

	// Return nil if neither name nor email is configured
	if name == "" && email == "" {
		return nil
	}

	return &GitIdentity{
		Name:  name,
		Email: email,
	}
}

var (
	globalConfig     *ConfigPaths
	globalConfigOnce sync.Once
	globalConfigErr  error
	configMutex      sync.Mutex // Protects per-container config creation
)

// GetClaudeConfig returns the isolated claude config paths, initializing if needed.
// This is safe to call from multiple goroutines.
// DEPRECATED: Use CreateContainerConfig for per-container isolation.
func GetClaudeConfig() (*ConfigPaths, error) {
	globalConfigOnce.Do(func() {
		globalConfig, globalConfigErr = InitClaudeConfig()
	})
	return globalConfig, globalConfigErr
}

// CCellsInstructions is the CLAUDE.md content for ccells containers
const CCellsInstructions = `# Claude Cells Session

You are in an isolated container with a dedicated git worktree. **Commit your work** - this is the most important thing. A dirty worktree means lost work.

## Constraints

- **No pushing** - remote access is disabled; attempts will fail
- **No branch switching** - you're locked to this worktree's branch
- **No merging** - the user handles integration across workstreams

## When Done

Commit all changes, then provide:

1. **Summary**: What you implemented/changed (2-3 sentences)
2. **Status** (required): ` + "`Ready for merge`" + ` | ` + "`Needs review`" + ` | ` + "`Incomplete`" + ` (and why)

The user is running multiple containers in parallel and relies on Status to triage.
`

// CreateContainerConfig creates an isolated config directory for a specific container.
// Each container gets its own copy to prevent race conditions when multiple
// Claude Code instances modify credentials simultaneously.
func CreateContainerConfig(containerName string) (*ConfigPaths, error) {
	configMutex.Lock()
	defer configMutex.Unlock()

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Source paths (user's original config)
	srcClaudeDir := filepath.Join(home, ClaudeDir)
	srcClaudeJSON := filepath.Join(home, ClaudeJSONFile)
	srcGitConfig := filepath.Join(home, GitConfigFile)

	// Per-container destination paths
	cellsDir, err := GetCellsDir()
	if err != nil {
		return nil, err
	}
	containerConfigDir := filepath.Join(cellsDir, "containers", containerName)
	dstClaudeDir := filepath.Join(containerConfigDir, ClaudeDir)
	dstClaudeJSON := filepath.Join(containerConfigDir, ClaudeJSONFile)
	dstGitConfig := filepath.Join(containerConfigDir, GitConfigFile)

	// Remove old container config if exists
	_ = removeAllSafe(containerConfigDir)

	// Create container config directory
	if err := os.MkdirAll(containerConfigDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create container config directory: %w", err)
	}

	// Copy .claude directory selectively (exclude projects/ to avoid copying
	// sessions from all other repos - we only need this container's sessions)
	if _, err := os.Stat(srcClaudeDir); err == nil {
		if err := copyClaudeDirSelective(srcClaudeDir, dstClaudeDir); err != nil {
			return nil, fmt.Errorf("failed to copy .claude directory: %w", err)
		}
	}

	// Copy .claude.json if it exists
	if _, err := os.Stat(srcClaudeJSON); err == nil {
		if err := copyFileForce(srcClaudeJSON, dstClaudeJSON); err != nil {
			return nil, fmt.Errorf("failed to copy .claude.json: %w", err)
		}
	}

	// Copy .gitconfig if it exists - track whether it was copied
	gitConfigCopied := false
	if _, err := os.Stat(srcGitConfig); err == nil {
		if err := copyFileForce(srcGitConfig, dstGitConfig); err != nil {
			return nil, fmt.Errorf("failed to copy .gitconfig: %w", err)
		}
		gitConfigCopied = true
	}

	// Extract and save OAuth credentials
	creds, err := GetClaudeCredentials()
	if err == nil && creds != nil && creds.Raw != "" {
		// Ensure .claude directory exists
		if err := os.MkdirAll(dstClaudeDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create .claude directory: %w", err)
		}
		// Write credentials inside .claude directory
		credsInClaudeDir := filepath.Join(dstClaudeDir, ".credentials.json")
		if err := os.WriteFile(credsInClaudeDir, []byte(creds.Raw), 0600); err != nil {
			return nil, fmt.Errorf("failed to write .credentials.json: %w", err)
		}
	}

	// Also write separate credentials file
	dstCredentials := filepath.Join(containerConfigDir, CredentialsFile)
	if creds != nil && creds.Raw != "" {
		if err := os.WriteFile(dstCredentials, []byte(creds.Raw), 0600); err != nil {
			return nil, fmt.Errorf("failed to write credentials: %w", err)
		}
	}

	// Write ccells-specific CLAUDE.md instructions
	// Ensure .claude directory exists first
	if err := os.MkdirAll(dstClaudeDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .claude directory: %w", err)
	}
	claudeMdPath := filepath.Join(dstClaudeDir, "CLAUDE.md")
	if err := os.WriteFile(claudeMdPath, []byte(CCellsInstructions), 0644); err != nil {
		return nil, fmt.Errorf("failed to write CLAUDE.md: %w", err)
	}

	// Only include GitConfig path if file was actually copied
	gitConfigPath := ""
	if gitConfigCopied {
		gitConfigPath = dstGitConfig
	}

	return &ConfigPaths{
		ClaudeDir:   dstClaudeDir,
		ClaudeJSON:  dstClaudeJSON,
		GitConfig:   gitConfigPath,
		Credentials: dstCredentials,
	}, nil
}

// CleanupContainerConfig removes the config directory for a container.
func CleanupContainerConfig(containerName string) error {
	cellsDir, err := GetCellsDir()
	if err != nil {
		return err
	}
	containerConfigDir := filepath.Join(cellsDir, "containers", containerName)
	return removeAllSafe(containerConfigDir)
}

// CleanupOrphanedContainerConfigs removes config directories for containers
// that no longer exist. Returns the number of configs cleaned up.
func CleanupOrphanedContainerConfigs(existingContainerNames map[string]bool) (int, error) {
	cellsDir, err := GetCellsDir()
	if err != nil {
		return 0, err
	}

	containersDir := filepath.Join(cellsDir, "containers")
	entries, err := os.ReadDir(containersDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil // No containers dir = nothing to clean up
		}
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		containerName := entry.Name()
		if !existingContainerNames[containerName] {
			configDir := filepath.Join(containersDir, containerName)
			if err := removeAllSafe(configDir); err == nil {
				count++
			}
		}
	}

	return count, nil
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

	// Copy .claude directory selectively (exclude projects/ to avoid copying
	// sessions from all other repos - we only need this container's sessions)
	// First try to remove the destination dir to ensure a clean copy
	_ = removeAllSafe(dstClaudeDir)
	if _, err := os.Stat(srcClaudeDir); err == nil {
		if err := copyClaudeDirSelective(srcClaudeDir, dstClaudeDir); err != nil {
			return nil, fmt.Errorf("failed to copy .claude directory: %w", err)
		}
	}

	// Copy .claude.json if it exists (using copyFileForce to ensure overwrite)
	if _, err := os.Stat(srcClaudeJSON); err == nil {
		if err := copyFileForce(srcClaudeJSON, dstClaudeJSON); err != nil {
			return nil, fmt.Errorf("failed to copy .claude.json: %w", err)
		}
	}

	// Copy .gitconfig if it exists (for git identity in containers) - track whether it was copied
	gitConfigCopied := false
	if _, err := os.Stat(srcGitConfig); err == nil {
		if err := copyFileForce(srcGitConfig, dstGitConfig); err != nil {
			return nil, fmt.Errorf("failed to copy .gitconfig: %w", err)
		}
		gitConfigCopied = true
	}

	// Extract and save OAuth credentials from keychain
	// Write to BOTH locations:
	// 1. Inside .claude/ directory (where Claude Code expects it on Linux)
	// 2. Separate file for backup/explicit mounting
	creds, err := GetClaudeCredentials()
	if err == nil && creds != nil && creds.Raw != "" {
		// Ensure .claude directory exists (might not if user has no ~/.claude/)
		if err := os.MkdirAll(dstClaudeDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create .claude directory: %w", err)
		}
		// Write inside .claude directory (primary location for Claude Code on Linux)
		// IMPORTANT: Linux expects .credentials.json (with leading dot), not credentials.json
		credsInClaudeDir := filepath.Join(dstClaudeDir, ".credentials.json")
		if err := os.WriteFile(credsInClaudeDir, []byte(creds.Raw), 0600); err != nil {
			return nil, fmt.Errorf("failed to write .credentials.json: %w", err)
		}
	}

	// Also write separate credentials file (for explicit mounting if needed)
	dstCredentials := filepath.Join(configDir, CredentialsFile)
	if creds != nil && creds.Raw != "" {
		if err := os.WriteFile(dstCredentials, []byte(creds.Raw), 0600); err != nil {
			return nil, fmt.Errorf("failed to write credentials: %w", err)
		}
	}

	// Only include GitConfig path if file was actually copied
	gitConfigPath := ""
	if gitConfigCopied {
		gitConfigPath = dstGitConfig
	}

	return &ConfigPaths{
		ClaudeDir:   dstClaudeDir,
		ClaudeJSON:  dstClaudeJSON,
		GitConfig:   gitConfigPath,
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

// copyClaudeDirSelective copies the .claude directory but excludes the projects/
// subdirectory. This prevents copying sessions from all other repos into each
// container, which can be very large and is unnecessary.
func copyClaudeDirSelective(src, dst string) error {
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

		// Skip projects directory - it contains sessions from ALL repos
		// which can be hundreds of MB. Each container will create its own
		// sessions in -workspace/ as it runs.
		if entry.Name() == "projects" {
			// Just create an empty projects directory
			_ = os.MkdirAll(dstPath, 0755)
			continue
		}

		// Skip other directories that aren't needed
		switch entry.Name() {
		case "debug", "cache", "image-cache":
			continue
		}

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				continue // Skip dirs with permission issues
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				continue // Skip files with permission issues
			}
		}
	}

	return nil
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

		// Skip directories that aren't needed or cause copy issues
		switch entry.Name() {
		case "debug":
			continue // Debug logs not needed
		case ".git":
			continue // Git repos in plugins can have permission issues
		case "cache":
			continue // Cache directories can be large and aren't needed
		case "image-cache":
			continue // Image cache not needed in container
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
				// Log but continue - some dirs may have permission issues (e.g., plugin .git)
				continue
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				// Log but continue - some files may have permission issues
				continue
			}
		}
	}

	return nil
}
