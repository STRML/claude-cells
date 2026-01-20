package docker

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ClaudeCredentials holds the OAuth credentials from Claude Code
type ClaudeCredentials struct {
	Raw string // The raw JSON from keychain
}

// credentialsJSON represents the structure of Claude Code credentials
type credentialsJSON struct {
	ClaudeAiOauth *oauthData `json:"claudeAiOauth,omitempty"`
}

type oauthData struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    int64  `json:"expiresAt"`
}

// GetClaudeCredentials retrieves Claude Code OAuth credentials from the system keychain.
// Returns nil if credentials are not found or on non-macOS systems.
func GetClaudeCredentials() (*ClaudeCredentials, error) {
	// Only works on macOS
	if runtime.GOOS != "darwin" {
		return nil, nil
	}

	// Try to get credentials from keychain
	cmd := exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w")
	output, err := cmd.Output()
	if err != nil {
		// Credentials not found or access denied - not an error, just not available
		return nil, nil
	}

	raw := strings.TrimSpace(string(output))
	if raw == "" {
		return nil, nil
	}

	return &ClaudeCredentials{
		Raw: raw,
	}, nil
}

// ExpiresAt returns the expiry timestamp from the credentials, or 0 if not parseable
func (c *ClaudeCredentials) ExpiresAt() int64 {
	if c == nil || c.Raw == "" {
		return 0
	}
	var creds credentialsJSON
	if err := json.Unmarshal([]byte(c.Raw), &creds); err != nil {
		return 0
	}
	if creds.ClaudeAiOauth == nil {
		return 0
	}
	return creds.ClaudeAiOauth.ExpiresAt
}

// containerInfo holds info about a registered container
type containerInfo struct {
	name      string
	configDir string
}

// CredentialRefresher monitors keychain credentials and updates container configs
type CredentialRefresher struct {
	mu              sync.RWMutex
	containers      map[string]*containerInfo // containerID -> info
	lastCredentials string                    // cached raw credentials for comparison
	stopCh          chan struct{}
	interval        time.Duration
}

// NewCredentialRefresher creates a new credential refresher
func NewCredentialRefresher(interval time.Duration) *CredentialRefresher {
	if interval == 0 {
		interval = 15 * time.Minute // Default: check every 15 minutes
	}
	return &CredentialRefresher{
		containers: make(map[string]*containerInfo),
		stopCh:     make(chan struct{}),
		interval:   interval,
	}
}

// RegisterContainer adds a container to the refresh list
func (r *CredentialRefresher) RegisterContainer(containerID, containerName, configDir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.containers[containerID] = &containerInfo{name: containerName, configDir: configDir}
	log.Printf("[CredentialRefresher] Registered container %s (%s)", containerName, containerID[:12])
}

// UnregisterContainer removes a container from the refresh list
func (r *CredentialRefresher) UnregisterContainer(containerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if info, ok := r.containers[containerID]; ok {
		log.Printf("[CredentialRefresher] Unregistered container %s", info.name)
	}
	delete(r.containers, containerID)
}

// Start begins the background credential refresh loop
func (r *CredentialRefresher) Start() {
	// Get initial credentials
	creds, err := GetClaudeCredentials()
	if err == nil && creds != nil {
		r.mu.Lock()
		r.lastCredentials = creds.Raw
		r.mu.Unlock()
	}

	go r.refreshLoop()
	log.Printf("[CredentialRefresher] Started with %v interval", r.interval)
}

// Stop stops the background credential refresh loop
func (r *CredentialRefresher) Stop() {
	close(r.stopCh)
	log.Printf("[CredentialRefresher] Stopped")
}

func (r *CredentialRefresher) refreshLoop() {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.checkAndRefresh()
		}
	}
}

func (r *CredentialRefresher) checkAndRefresh() {
	creds, err := GetClaudeCredentials()
	if err != nil || creds == nil {
		return
	}

	r.mu.RLock()
	lastCreds := r.lastCredentials
	r.mu.RUnlock()

	// Compare credentials - if same, nothing to do
	if creds.Raw == lastCreds {
		return
	}

	log.Printf("[CredentialRefresher] Credentials changed, updating containers...")

	// Update cached credentials and copy container info
	r.mu.Lock()
	r.lastCredentials = creds.Raw
	containers := make(map[string]*containerInfo)
	for k, v := range r.containers {
		containers[k] = v
	}
	r.mu.Unlock()

	// Update all registered containers
	updated := 0
	for _, info := range containers {
		if err := r.updateContainerCredentials(info.configDir, creds.Raw); err != nil {
			log.Printf("[CredentialRefresher] Failed to update %s: %v", info.name, err)
		} else {
			updated++
		}
	}

	if updated > 0 {
		log.Printf("[CredentialRefresher] Updated credentials for %d containers", updated)
	}
}

func (r *CredentialRefresher) updateContainerCredentials(configDir, rawCreds string) error {
	// Update .credentials.json inside the .claude directory
	claudeDir := filepath.Join(configDir, ClaudeDir)
	credsPath := filepath.Join(claudeDir, ".credentials.json")

	// Ensure directory exists
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return err
	}

	// Write credentials with restrictive permissions
	if err := os.WriteFile(credsPath, []byte(rawCreds), 0600); err != nil {
		return err
	}

	return nil
}

// ForceRefresh immediately checks and updates credentials
func (r *CredentialRefresher) ForceRefresh() int {
	creds, err := GetClaudeCredentials()
	if err != nil || creds == nil {
		return 0
	}

	r.mu.Lock()
	r.lastCredentials = creds.Raw
	containers := make(map[string]*containerInfo)
	for k, v := range r.containers {
		containers[k] = v
	}
	r.mu.Unlock()

	updated := 0
	for _, info := range containers {
		if err := r.updateContainerCredentials(info.configDir, creds.Raw); err != nil {
			log.Printf("[CredentialRefresher] Failed to update %s: %v", info.name, err)
		} else {
			updated++
		}
	}

	return updated
}
