package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SecurityTier represents the level of container hardening.
// Higher tiers are more restrictive but may break some images.
type SecurityTier string

const (
	// TierHardened is the most restrictive tier.
	// Drops SYS_ADMIN, SYS_MODULE, SYS_PTRACE, NET_ADMIN, NET_RAW.
	// May break: ping, some debuggers, ptrace-based tools.
	TierHardened SecurityTier = "hardened"

	// TierModerate drops only the most dangerous capabilities.
	// Drops SYS_ADMIN and SYS_MODULE only.
	// Very compatible - works with most dev images.
	TierModerate SecurityTier = "moderate"

	// TierCompat is the most compatible tier.
	// Only applies no-new-privileges and init.
	// No capability drops.
	TierCompat SecurityTier = "compat"
)

// SecurityConfig defines container security settings.
// These settings follow the principle of secure defaults that can be relaxed.
type SecurityConfig struct {
	// Tier sets the overall security level.
	// Options: "hardened", "moderate", "compat"
	// Default: "moderate" (good balance of security and compatibility)
	Tier SecurityTier `yaml:"tier,omitempty"`

	// NoNewPrivileges blocks setuid/setcap-based privilege escalation.
	// This is safe for most dev workflows and blocks a common attack vector.
	// Default: true (all tiers)
	NoNewPrivileges *bool `yaml:"no_new_privileges,omitempty"`

	// Init enables proper signal handling and zombie process reaping.
	// Uses tini or docker-init. Safe for all containers.
	// Default: true (all tiers)
	Init *bool `yaml:"init,omitempty"`

	// PidsLimit sets the maximum number of processes in the container.
	// Prevents fork bombs. 1024 is generous for typical dev work.
	// Set to 0 to disable the limit.
	// Default: 1024
	PidsLimit *int64 `yaml:"pids_limit,omitempty"`

	// CapDrop lists Linux capabilities to drop from the container.
	// If set, overrides the tier's default cap_drop list.
	CapDrop []string `yaml:"cap_drop,omitempty"`

	// CapAdd lists Linux capabilities to add to the container.
	// Use sparingly - most dev work doesn't need extra capabilities.
	CapAdd []string `yaml:"cap_add,omitempty"`

	// Privileged grants full host access. DANGEROUS - defeats all isolation.
	// Default: false. Only enable if explicitly required.
	Privileged *bool `yaml:"privileged,omitempty"`

	// HostNetwork uses the host's network namespace (--network=host).
	// Defeats network isolation. Default: false.
	HostNetwork *bool `yaml:"host_network,omitempty"`

	// HostPID shares the host's PID namespace (--pid=host).
	// Can see and signal host processes. Default: false.
	HostPID *bool `yaml:"host_pid,omitempty"`

	// HostIPC shares the host's IPC namespace (--ipc=host).
	// Can access host shared memory. Default: false.
	HostIPC *bool `yaml:"host_ipc,omitempty"`

	// DockerSocket mounts the Docker socket into the container.
	// Grants near-host-level access. Default: false.
	DockerSocket *bool `yaml:"docker_socket,omitempty"`

	// AutoRelax enables automatic tier relaxation on container start failure.
	// When enabled, ccells will retry with progressively less restrictive
	// settings and save the working configuration for future use.
	// Default: true
	AutoRelax *bool `yaml:"auto_relax,omitempty"`
}

// DockerfileConfig defines Dockerfile customization settings.
type DockerfileConfig struct {
	// Inject specifies RUN commands to inject into the Dockerfile.
	// These are appended after Claude Code installation.
	// Example: ["npm install -g ccstatusline", "apt-get install -y vim"]
	Inject []string `yaml:"inject,omitempty"`
}

// CellsConfig is the top-level configuration file structure.
type CellsConfig struct {
	Runtime    string           `yaml:"runtime,omitempty"`
	Security   SecurityConfig   `yaml:"security,omitempty"`
	Dockerfile DockerfileConfig `yaml:"dockerfile,omitempty"`
}

// Helper functions for pointer creation
func boolPtr(b bool) *bool    { return &b }
func int64Ptr(i int64) *int64 { return &i }

// TierCapDrops returns the default capabilities to drop for each tier.
func TierCapDrops(tier SecurityTier) []string {
	switch tier {
	case TierHardened:
		return []string{"SYS_ADMIN", "SYS_MODULE", "SYS_PTRACE", "NET_ADMIN", "NET_RAW"}
	case TierModerate:
		return []string{"SYS_ADMIN", "SYS_MODULE"}
	case TierCompat:
		return []string{}
	default:
		return []string{"SYS_ADMIN", "SYS_MODULE"} // Default to moderate
	}
}

// NextTier returns the next less restrictive tier, or empty string if at minimum.
func NextTier(tier SecurityTier) SecurityTier {
	switch tier {
	case TierHardened:
		return TierModerate
	case TierModerate:
		return TierCompat
	case TierCompat:
		return "" // No fallback from compat
	default:
		return TierCompat
	}
}

// DefaultSecurityConfig returns the default security configuration.
// Uses "moderate" tier which is a good balance of security and compatibility.
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		Tier:            TierModerate,
		NoNewPrivileges: boolPtr(true),
		Init:            boolPtr(true),
		PidsLimit:       int64Ptr(1024),
		CapDrop:         nil, // Use tier default
		CapAdd:          []string{},
		Privileged:      boolPtr(false),
		HostNetwork:     boolPtr(false),
		HostPID:         boolPtr(false),
		HostIPC:         boolPtr(false),
		DockerSocket:    boolPtr(false),
		AutoRelax:       boolPtr(true),
	}
}

// ConfigForTier returns a security config for the specified tier.
func ConfigForTier(tier SecurityTier) SecurityConfig {
	cfg := DefaultSecurityConfig()
	cfg.Tier = tier
	cfg.CapDrop = TierCapDrops(tier)
	return cfg
}

// LoadSecurityConfig loads and merges security configuration.
// Order of precedence (highest to lowest):
// 1. Project config (.claude-cells/config.yaml in projectPath)
// 2. Global config (~/.claude-cells/config.yaml)
// 3. Hardened defaults (moderate tier)
func LoadSecurityConfig(projectPath string) SecurityConfig {
	cfg := DefaultSecurityConfig()

	// Load global config
	globalCfg := loadGlobalCellsConfig()
	if globalCfg != nil {
		cfg = mergeSecurityConfig(cfg, globalCfg.Security)
	}

	// Load project config (takes precedence)
	if projectPath != "" {
		projectCfg := loadProjectCellsConfig(projectPath)
		if projectCfg != nil {
			cfg = mergeSecurityConfig(cfg, projectCfg.Security)
		}
	}

	// Apply tier defaults if CapDrop wasn't explicitly set
	if cfg.CapDrop == nil {
		cfg.CapDrop = TierCapDrops(cfg.Tier)
	}

	return cfg
}

// loadGlobalCellsConfig loads ~/.claude-cells/config.yaml
func loadGlobalCellsConfig() *CellsConfig {
	cellsDir, err := GetCellsDir()
	if err != nil {
		return nil
	}

	configPath := filepath.Join(cellsDir, "config.yaml")
	return loadCellsConfigFile(configPath)
}

// LoadDockerfileConfig loads and merges Dockerfile configuration.
// Order of precedence (highest to lowest):
// 1. Project config (.claude-cells/config.yaml in projectPath)
// 2. Global config (~/.claude-cells/config.yaml)
// 3. Default (empty - no injections)
func LoadDockerfileConfig(projectPath string) DockerfileConfig {
	// Default: no injections
	cfg := DockerfileConfig{
		Inject: []string{},
	}

	// Load global config
	globalCfg := loadGlobalCellsConfig()
	if globalCfg != nil && len(globalCfg.Dockerfile.Inject) > 0 {
		cfg.Inject = globalCfg.Dockerfile.Inject
	}

	// Load project config (takes precedence)
	if projectPath != "" {
		projectCfg := loadProjectCellsConfig(projectPath)
		if projectCfg != nil && len(projectCfg.Dockerfile.Inject) > 0 {
			cfg.Inject = projectCfg.Dockerfile.Inject
		}
	}

	return cfg
}

// normalizeRuntime normalizes and validates a runtime value.
// Returns normalized value or falls back to "claude" for invalid/empty input.
func normalizeRuntime(runtime string) string {
	runtime = strings.ToLower(strings.TrimSpace(runtime))
	// Only return if valid, otherwise fall back to default
	if runtime == "claude" || runtime == "claudesp" {
		return runtime
	}
	return "claude" // Default fallback
}

// LoadConfig loads and merges the full CellsConfig (runtime, security, dockerfile).
// Order of precedence (highest to lowest):
// 1. Project config (.claude-cells/config.yaml in projectPath)
// 2. Global config (~/.claude-cells/config.yaml)
// 3. Default (runtime="claude")
func LoadConfig(projectPath string) CellsConfig {
	cfg := CellsConfig{
		Runtime: "claude", // Default runtime
	}

	// Load global config
	globalCfg := loadGlobalCellsConfig()
	if globalCfg != nil {
		if globalCfg.Runtime != "" {
			cfg.Runtime = normalizeRuntime(globalCfg.Runtime)
		}
		cfg.Security = mergeSecurityConfig(DefaultSecurityConfig(), globalCfg.Security)
		if len(globalCfg.Dockerfile.Inject) > 0 {
			cfg.Dockerfile.Inject = globalCfg.Dockerfile.Inject
		}
	} else {
		cfg.Security = DefaultSecurityConfig()
	}

	// Load project config (takes precedence)
	if projectPath != "" {
		projectCfg := loadProjectCellsConfig(projectPath)
		if projectCfg != nil {
			if projectCfg.Runtime != "" {
				cfg.Runtime = normalizeRuntime(projectCfg.Runtime)
			}
			cfg.Security = mergeSecurityConfig(cfg.Security, projectCfg.Security)
			if len(projectCfg.Dockerfile.Inject) > 0 {
				cfg.Dockerfile.Inject = projectCfg.Dockerfile.Inject
			}
		}
	}

	// Apply tier defaults if CapDrop wasn't explicitly set
	if cfg.Security.CapDrop == nil {
		cfg.Security.CapDrop = TierCapDrops(cfg.Security.Tier)
	}

	return cfg
}

// loadProjectCellsConfig loads .claude-cells/config.yaml from the project
func loadProjectCellsConfig(projectPath string) *CellsConfig {
	configPath := filepath.Join(projectPath, ".claude-cells", "config.yaml")
	return loadCellsConfigFile(configPath)
}

// loadCellsConfigFile loads a single config file
func loadCellsConfigFile(path string) *CellsConfig {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var cfg CellsConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil
	}

	return &cfg
}

// mergeSecurityConfig merges override values into base.
// Only non-nil/non-zero values in override replace base values.
// Arrays are replaced entirely (not appended) when non-nil.
func mergeSecurityConfig(base, override SecurityConfig) SecurityConfig {
	result := base

	if override.Tier != "" {
		result.Tier = override.Tier
	}
	if override.NoNewPrivileges != nil {
		result.NoNewPrivileges = override.NoNewPrivileges
	}
	if override.Init != nil {
		result.Init = override.Init
	}
	if override.PidsLimit != nil {
		result.PidsLimit = override.PidsLimit
	}
	if override.CapDrop != nil {
		result.CapDrop = override.CapDrop
	}
	if override.CapAdd != nil {
		result.CapAdd = override.CapAdd
	}
	if override.Privileged != nil {
		result.Privileged = override.Privileged
	}
	if override.HostNetwork != nil {
		result.HostNetwork = override.HostNetwork
	}
	if override.HostPID != nil {
		result.HostPID = override.HostPID
	}
	if override.HostIPC != nil {
		result.HostIPC = override.HostIPC
	}
	if override.DockerSocket != nil {
		result.DockerSocket = override.DockerSocket
	}
	if override.AutoRelax != nil {
		result.AutoRelax = override.AutoRelax
	}

	return result
}

// SaveProjectSecurityConfig saves a security config to the project's config file.
// This is used when auto-relaxation occurs to persist the working configuration.
func SaveProjectSecurityConfig(projectPath string, cfg SecurityConfig) error {
	configDir := filepath.Join(projectPath, ".claude-cells")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")

	// Load existing config to preserve other settings
	existing := loadCellsConfigFile(configPath)
	if existing == nil {
		existing = &CellsConfig{}
	}
	existing.Security = cfg

	data, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Add header comment
	header := []byte(`# Claude Cells Configuration
# This file was auto-generated after security tier relaxation.
# See: docs/CONTAINER-SECURITY.md for details on security tiers.
#
# To increase security, change tier to "hardened" or "moderate".
# To further relax, change tier to "compat" or set specific options.

`)
	data = append(header, data...)

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// Getter methods for safe access to pointer fields

// GetBool safely dereferences a bool pointer, returning the default if nil.
func (s *SecurityConfig) GetBool(ptr *bool, defaultVal bool) bool {
	if ptr == nil {
		return defaultVal
	}
	return *ptr
}

// GetNoNewPrivileges returns the no_new_privileges setting (default: true).
func (s *SecurityConfig) GetNoNewPrivileges() bool {
	return s.GetBool(s.NoNewPrivileges, true)
}

// GetInit returns the init setting (default: true).
func (s *SecurityConfig) GetInit() bool {
	return s.GetBool(s.Init, true)
}

// GetPidsLimit returns the pids_limit setting (default: 1024, 0 = unlimited).
func (s *SecurityConfig) GetPidsLimit() int64 {
	if s.PidsLimit == nil {
		return 1024
	}
	return *s.PidsLimit
}

// GetPrivileged returns the privileged setting (default: false).
func (s *SecurityConfig) GetPrivileged() bool {
	return s.GetBool(s.Privileged, false)
}

// GetHostNetwork returns the host_network setting (default: false).
func (s *SecurityConfig) GetHostNetwork() bool {
	return s.GetBool(s.HostNetwork, false)
}

// GetHostPID returns the host_pid setting (default: false).
func (s *SecurityConfig) GetHostPID() bool {
	return s.GetBool(s.HostPID, false)
}

// GetHostIPC returns the host_ipc setting (default: false).
func (s *SecurityConfig) GetHostIPC() bool {
	return s.GetBool(s.HostIPC, false)
}

// GetDockerSocket returns the docker_socket setting (default: false).
func (s *SecurityConfig) GetDockerSocket() bool {
	return s.GetBool(s.DockerSocket, false)
}

// GetAutoRelax returns the auto_relax setting (default: true).
func (s *SecurityConfig) GetAutoRelax() bool {
	return s.GetBool(s.AutoRelax, true)
}

// GetEffectiveCapDrop returns the capabilities to drop, using tier defaults if not set.
func (s *SecurityConfig) GetEffectiveCapDrop() []string {
	if s.CapDrop != nil {
		return s.CapDrop
	}
	return TierCapDrops(s.Tier)
}

// TierDescription returns a human-readable description of the tier.
func TierDescription(tier SecurityTier) string {
	switch tier {
	case TierHardened:
		return "Hardened (drops SYS_ADMIN, SYS_MODULE, SYS_PTRACE, NET_ADMIN, NET_RAW)"
	case TierModerate:
		return "Moderate (drops SYS_ADMIN, SYS_MODULE)"
	case TierCompat:
		return "Compatible (no capability drops, only no-new-privileges and init)"
	default:
		return string(tier)
	}
}

// DefaultConfigYAML is the default config.yaml content with commented documentation.
const DefaultConfigYAML = `# Claude Cells Configuration
# Documentation: https://github.com/anthropics/claude-cells/blob/main/docs/CONTAINER-SECURITY.md

# Dockerfile customization - commands injected after Claude Code installation
# Uncomment and customize as needed:
# dockerfile:
#   inject:
#     - "apt-get update && apt-get install -y vim"
#     - "pip install ipython"

security:
  # Security tier controls the default capability drops.
  # Options:
  #   - "hardened": Maximum security. Drops SYS_ADMIN, SYS_MODULE, SYS_PTRACE, NET_ADMIN, NET_RAW.
  #                 May break: ping, debuggers (gdb/strace), some profilers.
  #   - "moderate": Balanced (default). Drops SYS_ADMIN, SYS_MODULE.
  #                 Works with most dev images including debuggers.
  #   - "compat":   Maximum compatibility. No capability drops.
  #                 Only applies no-new-privileges and init.
  tier: moderate

  # Block setuid/setcap-based privilege escalation.
  # Safe for all dev workflows. Recommended: true
  no_new_privileges: true

  # Use init process (tini) for proper signal handling and zombie reaping.
  # Recommended: true
  init: true

  # Maximum number of processes in the container.
  # Prevents fork bombs. Set to 0 for unlimited.
  pids_limit: 1024

  # Capabilities to drop. Uncomment to override tier defaults.
  # cap_drop:
  #   - SYS_ADMIN
  #   - SYS_MODULE

  # Capabilities to add. Use sparingly.
  # cap_add:
  #   - NET_RAW       # Needed for ping
  #   - SYS_PTRACE    # Needed for some debuggers

  # Auto-relax security on container start failure.
  # When true, ccells automatically tries less restrictive tiers
  # and saves the working config for future use.
  auto_relax: true

  # ============================================================================
  # DANGEROUS OPTIONS - These defeat container isolation.
  # Only enable if you understand the security implications.
  # ============================================================================

  # Grant full host access. Container can trivially escape.
  # privileged: false

  # Share host network namespace. Defeats network isolation.
  # host_network: false

  # Share host PID namespace. Can see/signal host processes.
  # host_pid: false

  # Share host IPC namespace. Can access host shared memory.
  # host_ipc: false

  # Mount Docker socket. Grants near-host-level access.
  # docker_socket: false
`

// WriteDefaultGlobalConfig writes the default config.yaml to ~/.claude-cells/
// if it doesn't already exist. Called on first run.
func WriteDefaultGlobalConfig() error {
	cellsDir, err := GetCellsDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(cellsDir, "config.yaml")

	// Don't overwrite existing config
	if _, err := os.Stat(configPath); err == nil {
		return nil
	}

	// Ensure directory exists
	if err := os.MkdirAll(cellsDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write default config
	if err := os.WriteFile(configPath, []byte(DefaultConfigYAML), 0644); err != nil {
		return fmt.Errorf("failed to write default config: %w", err)
	}

	return nil
}
