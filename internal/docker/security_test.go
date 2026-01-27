package docker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_DefaultRuntime(t *testing.T) {
	// Test that default runtime is "claude"
	tmpDir := t.TempDir()

	// No config file exists
	cfg := LoadConfig(tmpDir)

	if cfg.Runtime != "claude" {
		t.Errorf("Default runtime should be 'claude', got '%s'", cfg.Runtime)
	}
}

func TestLoadConfig_RuntimeFromGlobalConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create global config with runtime=claudesp
	globalConfigPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	configContent := `runtime: claudesp
security:
  tier: moderate
`
	if err := os.WriteFile(globalConfigPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write global config: %v", err)
	}

	// Use SetTestCellsDir to override GetCellsDir for this test
	// (os.UserHomeDir() ignores HOME env var, so we use this test helper instead)
	SetTestCellsDir(tmpDir)
	defer SetTestCellsDir("") // Reset after test

	cfg := LoadConfig("") // Empty projectPath means global only

	if cfg.Runtime != "claudesp" {
		t.Errorf("Runtime should be 'claudesp' from global config, got '%s'", cfg.Runtime)
	}
}

func TestLoadConfig_ProjectConfigOverridesGlobal(t *testing.T) {
	tmpDir := t.TempDir()

	// Create global config with runtime=claude
	globalConfigPath := filepath.Join(tmpDir, ".claude-cells", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(globalConfigPath), 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	globalContent := `runtime: claude`
	if err := os.WriteFile(globalConfigPath, []byte(globalContent), 0644); err != nil {
		t.Fatalf("Failed to write global config: %v", err)
	}

	// Create project config with runtime=claudesp
	projectDir := filepath.Join(tmpDir, "myproject")
	projectConfigPath := filepath.Join(projectDir, ".claude-cells", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(projectConfigPath), 0755); err != nil {
		t.Fatalf("Failed to create project config dir: %v", err)
	}

	projectContent := `runtime: claudesp`
	if err := os.WriteFile(projectConfigPath, []byte(projectContent), 0644); err != nil {
		t.Fatalf("Failed to write project config: %v", err)
	}

	// Set HOME to tmpDir
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	cfg := LoadConfig(projectDir)

	if cfg.Runtime != "claudesp" {
		t.Errorf("Runtime should be 'claudesp' from project config, got '%s'", cfg.Runtime)
	}
}

func TestDefaultSecurityConfig(t *testing.T) {
	cfg := DefaultSecurityConfig()

	if cfg.Tier != TierModerate {
		t.Errorf("Default tier should be moderate, got %s", cfg.Tier)
	}

	if !cfg.GetNoNewPrivileges() {
		t.Error("Default no_new_privileges should be true")
	}

	if !cfg.GetInit() {
		t.Error("Default init should be true")
	}

	if cfg.GetPidsLimit() != 1024 {
		t.Errorf("Default pids_limit should be 1024, got %d", cfg.GetPidsLimit())
	}

	if cfg.GetPrivileged() {
		t.Error("Default privileged should be false")
	}

	if !cfg.GetAutoRelax() {
		t.Error("Default auto_relax should be true")
	}
}

func TestTierCapDrops(t *testing.T) {
	tests := []struct {
		tier     SecurityTier
		expected []string
	}{
		{TierHardened, []string{"SYS_ADMIN", "SYS_MODULE", "SYS_PTRACE", "NET_ADMIN", "NET_RAW"}},
		{TierModerate, []string{"SYS_ADMIN", "SYS_MODULE"}},
		{TierCompat, []string{}},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier), func(t *testing.T) {
			got := TierCapDrops(tt.tier)
			if len(got) != len(tt.expected) {
				t.Errorf("TierCapDrops(%s) = %v, want %v", tt.tier, got, tt.expected)
				return
			}
			for i, cap := range got {
				if cap != tt.expected[i] {
					t.Errorf("TierCapDrops(%s)[%d] = %s, want %s", tt.tier, i, cap, tt.expected[i])
				}
			}
		})
	}
}

func TestNextTier(t *testing.T) {
	tests := []struct {
		tier     SecurityTier
		expected SecurityTier
	}{
		{TierHardened, TierModerate},
		{TierModerate, TierCompat},
		{TierCompat, ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier), func(t *testing.T) {
			got := NextTier(tt.tier)
			if got != tt.expected {
				t.Errorf("NextTier(%s) = %s, want %s", tt.tier, got, tt.expected)
			}
		})
	}
}

func TestConfigForTier(t *testing.T) {
	cfg := ConfigForTier(TierHardened)

	if cfg.Tier != TierHardened {
		t.Errorf("ConfigForTier(hardened).Tier = %s, want hardened", cfg.Tier)
	}

	capDrop := cfg.GetEffectiveCapDrop()
	if len(capDrop) != 5 {
		t.Errorf("Hardened tier should drop 5 capabilities, got %d", len(capDrop))
	}
}

func TestMergeSecurityConfig(t *testing.T) {
	base := DefaultSecurityConfig()
	override := SecurityConfig{
		Tier:      TierCompat,
		PidsLimit: int64Ptr(2048),
	}

	merged := mergeSecurityConfig(base, override)

	// Override should change tier
	if merged.Tier != TierCompat {
		t.Errorf("Merged tier should be compat, got %s", merged.Tier)
	}

	// Override should change pids_limit
	if merged.GetPidsLimit() != 2048 {
		t.Errorf("Merged pids_limit should be 2048, got %d", merged.GetPidsLimit())
	}

	// Base values should be preserved when not overridden
	if !merged.GetNoNewPrivileges() {
		t.Error("Merged no_new_privileges should be preserved as true")
	}

	if !merged.GetInit() {
		t.Error("Merged init should be preserved as true")
	}
}

func TestLoadSecurityConfigWithFiles(t *testing.T) {
	// Create temp directories for testing
	tmpDir, err := os.MkdirTemp("", "ccells-security-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create project config directory
	projectConfigDir := filepath.Join(tmpDir, ".claude-cells")
	if err := os.MkdirAll(projectConfigDir, 0755); err != nil {
		t.Fatalf("Failed to create project config dir: %v", err)
	}

	// Write a project config file
	configContent := `security:
  tier: compat
  pids_limit: 512
`
	configPath := filepath.Join(projectConfigDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load security config for the project
	cfg := LoadSecurityConfig(tmpDir)

	// Project config should override defaults
	if cfg.Tier != TierCompat {
		t.Errorf("Loaded tier should be compat, got %s", cfg.Tier)
	}

	if cfg.GetPidsLimit() != 512 {
		t.Errorf("Loaded pids_limit should be 512, got %d", cfg.GetPidsLimit())
	}

	// Non-overridden values should use defaults
	if !cfg.GetNoNewPrivileges() {
		t.Error("Loaded no_new_privileges should be default true")
	}
}

func TestSaveProjectSecurityConfig(t *testing.T) {
	// Create temp directory for testing
	tmpDir, err := os.MkdirTemp("", "ccells-security-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Save a security config
	cfg := ConfigForTier(TierCompat)
	if err := SaveProjectSecurityConfig(tmpDir, cfg); err != nil {
		t.Fatalf("SaveProjectSecurityConfig failed: %v", err)
	}

	// Verify file was created
	configPath := filepath.Join(tmpDir, ".claude-cells", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Config file was not created at %s", configPath)
	}

	// Read it back
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	// Verify header comment exists
	if len(content) == 0 {
		t.Error("Config file should not be empty")
	}

	// Load and verify the saved config
	loadedCfg := LoadSecurityConfig(tmpDir)
	if loadedCfg.Tier != TierCompat {
		t.Errorf("Loaded tier should be compat, got %s", loadedCfg.Tier)
	}
}

func TestSecurityConfigGetters(t *testing.T) {
	// Test with nil pointers (should return defaults)
	cfg := SecurityConfig{}

	if !cfg.GetNoNewPrivileges() {
		t.Error("GetNoNewPrivileges with nil should return true")
	}

	if !cfg.GetInit() {
		t.Error("GetInit with nil should return true")
	}

	if cfg.GetPidsLimit() != 1024 {
		t.Errorf("GetPidsLimit with nil should return 1024, got %d", cfg.GetPidsLimit())
	}

	if cfg.GetPrivileged() {
		t.Error("GetPrivileged with nil should return false")
	}

	if cfg.GetHostNetwork() {
		t.Error("GetHostNetwork with nil should return false")
	}

	if cfg.GetHostPID() {
		t.Error("GetHostPID with nil should return false")
	}

	if cfg.GetHostIPC() {
		t.Error("GetHostIPC with nil should return false")
	}

	if cfg.GetDockerSocket() {
		t.Error("GetDockerSocket with nil should return false")
	}

	if !cfg.GetAutoRelax() {
		t.Error("GetAutoRelax with nil should return true")
	}
}

func TestGetEffectiveCapDrop(t *testing.T) {
	// Test with explicit CapDrop
	cfg := SecurityConfig{
		Tier:    TierModerate,
		CapDrop: []string{"NET_RAW"},
	}
	capDrop := cfg.GetEffectiveCapDrop()
	if len(capDrop) != 1 || capDrop[0] != "NET_RAW" {
		t.Errorf("GetEffectiveCapDrop with explicit CapDrop should return explicit list, got %v", capDrop)
	}

	// Test without explicit CapDrop (should use tier default)
	cfg2 := SecurityConfig{
		Tier: TierModerate,
	}
	capDrop2 := cfg2.GetEffectiveCapDrop()
	if len(capDrop2) != 2 {
		t.Errorf("GetEffectiveCapDrop without explicit CapDrop should return tier default, got %v", capDrop2)
	}
}

func TestTierDescription(t *testing.T) {
	tests := []struct {
		tier     SecurityTier
		contains string
	}{
		{TierHardened, "Hardened"},
		{TierModerate, "Moderate"},
		{TierCompat, "Compatible"},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier), func(t *testing.T) {
			desc := TierDescription(tt.tier)
			if desc == "" {
				t.Errorf("TierDescription(%s) should not be empty", tt.tier)
			}
		})
	}
}

func TestWriteDefaultGlobalConfig(t *testing.T) {
	// Save original HOME and restore after test
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	// Create temp directory for testing
	tmpDir, err := os.MkdirTemp("", "ccells-write-config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set HOME to temp directory
	os.Setenv("HOME", tmpDir)

	// Write default config
	if err := WriteDefaultGlobalConfig(); err != nil {
		t.Fatalf("WriteDefaultGlobalConfig() error = %v", err)
	}

	// Verify file was created
	configPath := filepath.Join(tmpDir, ".claude-cells", "config.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	// Verify content has expected structure
	contentStr := string(content)
	expectedStrings := []string{
		"security:",
		"tier: moderate",
		"no_new_privileges: true",
		"init: true",
		"pids_limit: 1024",
		"auto_relax: true",
		"DANGEROUS OPTIONS",
	}

	for _, expected := range expectedStrings {
		if !contains(contentStr, expected) {
			t.Errorf("Config file should contain %q", expected)
		}
	}

	// Verify calling again doesn't overwrite
	// First, modify the file
	modifiedContent := []byte("# Modified\nsecurity:\n  tier: compat\n")
	if err := os.WriteFile(configPath, modifiedContent, 0644); err != nil {
		t.Fatalf("Failed to modify config file: %v", err)
	}

	// Call WriteDefaultGlobalConfig again
	if err := WriteDefaultGlobalConfig(); err != nil {
		t.Fatalf("WriteDefaultGlobalConfig() second call error = %v", err)
	}

	// Verify file wasn't overwritten
	afterContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file after second call: %v", err)
	}

	if string(afterContent) != string(modifiedContent) {
		t.Error("WriteDefaultGlobalConfig should not overwrite existing config")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
