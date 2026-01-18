package docker

import (
	"runtime"
	"strings"
	"testing"
)

func TestGetClaudeCredentials(t *testing.T) {
	creds, err := GetClaudeCredentials()
	if err != nil {
		t.Fatalf("GetClaudeCredentials() error = %v", err)
	}

	// On non-macOS, should return nil
	if runtime.GOOS != "darwin" {
		if creds != nil {
			t.Error("Expected nil credentials on non-macOS")
		}
		return
	}

	// On macOS, credentials may or may not exist
	// Just verify no error and if credentials exist, they have content
	if creds != nil {
		if creds.Raw == "" {
			t.Error("Credentials returned but Raw is empty")
		}
		t.Logf("Found credentials (length: %d)", len(creds.Raw))

		// Verify credentials contain expected OAuth fields
		if !strings.Contains(creds.Raw, "claudeAiOauth") {
			t.Error("Credentials should contain 'claudeAiOauth' field")
		}
		if !strings.Contains(creds.Raw, "accessToken") {
			t.Error("Credentials should contain 'accessToken' field")
		}
		if !strings.Contains(creds.Raw, "refreshToken") {
			t.Error("Credentials should contain 'refreshToken' field")
		}
	} else {
		t.Log("No Claude credentials found in keychain (this is OK)")
	}
}

func TestCredentialsStructure(t *testing.T) {
	// Test that ClaudeCredentials struct works correctly
	creds := &ClaudeCredentials{
		Raw: `{"claudeAiOauth":{"accessToken":"test-access","refreshToken":"test-refresh"}}`,
	}

	if creds.Raw == "" {
		t.Error("ClaudeCredentials.Raw should not be empty")
	}

	if !strings.Contains(creds.Raw, "accessToken") {
		t.Error("Raw should contain accessToken")
	}
}
