package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoadDevcontainerConfig(t *testing.T) {
	tests := []struct {
		name           string
		setupFiles     map[string]string // path relative to project root -> content
		wantImage      string
		wantBuild      *DevcontainerBuild
		wantEnv        map[string]string
		wantErr        bool
		wantErrContain string
	}{
		{
			name: "image field only",
			setupFiles: map[string]string{
				".devcontainer/devcontainer.json": `{
					"name": "Test",
					"image": "golang:1.25"
				}`,
			},
			wantImage: "golang:1.25",
			wantBuild: nil,
			wantEnv:   nil,
		},
		{
			name: "devcontainer.json in root",
			setupFiles: map[string]string{
				".devcontainer.json": `{
					"name": "Test",
					"image": "node:20"
				}`,
			},
			wantImage: "node:20",
		},
		{
			name: "prefer .devcontainer folder over root",
			setupFiles: map[string]string{
				".devcontainer/devcontainer.json": `{"image": "golang:1.25"}`,
				".devcontainer.json":              `{"image": "node:20"}`,
			},
			wantImage: "golang:1.25",
		},
		{
			name: "build section with dockerfile",
			setupFiles: map[string]string{
				".devcontainer/devcontainer.json": `{
					"name": "Build Test",
					"build": {
						"dockerfile": "Dockerfile",
						"context": ".."
					}
				}`,
			},
			wantBuild: &DevcontainerBuild{
				Dockerfile: "Dockerfile",
				Context:    "..",
			},
		},
		{
			name: "build section with args",
			setupFiles: map[string]string{
				".devcontainer/devcontainer.json": `{
					"build": {
						"dockerfile": "Dockerfile",
						"args": {
							"GO_VERSION": "1.23",
							"NODE_VERSION": "20"
						}
					}
				}`,
			},
			wantBuild: &DevcontainerBuild{
				Dockerfile: "Dockerfile",
				Args: map[string]string{
					"GO_VERSION":   "1.23",
					"NODE_VERSION": "20",
				},
			},
		},
		{
			name: "containerEnv parsing",
			setupFiles: map[string]string{
				".devcontainer/devcontainer.json": `{
					"image": "golang:1.25",
					"containerEnv": {
						"GOPROXY": "https://proxy.golang.org",
						"CGO_ENABLED": "0"
					}
				}`,
			},
			wantImage: "golang:1.25",
			wantEnv: map[string]string{
				"GOPROXY":     "https://proxy.golang.org",
				"CGO_ENABLED": "0",
			},
		},
		{
			name:       "no devcontainer.json returns nil config",
			setupFiles: map[string]string{},
			wantErr:    false, // no error, just nil config
		},
		{
			name: "invalid JSON returns error",
			setupFiles: map[string]string{
				".devcontainer/devcontainer.json": `{invalid json}`,
			},
			wantErr:        true,
			wantErrContain: "failed to parse",
		},
		{
			name: "empty JSON object is valid",
			setupFiles: map[string]string{
				".devcontainer/devcontainer.json": `{}`,
			},
			wantErr: false,
		},
		{
			name: "comments in JSON (jsonc) should work",
			setupFiles: map[string]string{
				".devcontainer/devcontainer.json": `{
					// This is a comment
					"image": "ubuntu:22.04"
					/* multi
					   line */
				}`,
			},
			wantImage: "ubuntu:22.04",
		},
		{
			name: "trailing commas should work",
			setupFiles: map[string]string{
				".devcontainer/devcontainer.json": `{
					"image": "ubuntu:22.04",
					"containerEnv": {
						"FOO": "bar",
					},
				}`,
			},
			wantImage: "ubuntu:22.04",
			wantEnv: map[string]string{
				"FOO": "bar",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir, err := os.MkdirTemp("", "devcontainer-test-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			// Setup test files
			for relPath, content := range tt.setupFiles {
				fullPath := filepath.Join(tmpDir, relPath)
				if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
					t.Fatalf("failed to create dir for %s: %v", relPath, err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					t.Fatalf("failed to write %s: %v", relPath, err)
				}
			}

			// Call the function under test
			cfg, err := LoadDevcontainerConfig(tmpDir)

			// Check error expectations
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.wantErrContain != "" && !containsString(err.Error(), tt.wantErrContain) {
					t.Errorf("error %q should contain %q", err.Error(), tt.wantErrContain)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// No files = nil config
			if len(tt.setupFiles) == 0 {
				if cfg != nil {
					t.Errorf("expected nil config for no files, got %+v", cfg)
				}
				return
			}

			// Check config
			if cfg == nil {
				t.Fatal("expected non-nil config")
			}

			if tt.wantImage != "" && cfg.Image != tt.wantImage {
				t.Errorf("Image = %q, want %q", cfg.Image, tt.wantImage)
			}

			if tt.wantBuild != nil {
				if cfg.Build == nil {
					t.Fatal("expected Build to be non-nil")
				}
				if cfg.Build.Dockerfile != tt.wantBuild.Dockerfile {
					t.Errorf("Build.Dockerfile = %q, want %q", cfg.Build.Dockerfile, tt.wantBuild.Dockerfile)
				}
				if cfg.Build.Context != tt.wantBuild.Context {
					t.Errorf("Build.Context = %q, want %q", cfg.Build.Context, tt.wantBuild.Context)
				}
				for k, v := range tt.wantBuild.Args {
					if cfg.Build.Args[k] != v {
						t.Errorf("Build.Args[%q] = %q, want %q", k, cfg.Build.Args[k], v)
					}
				}
			}

			if tt.wantEnv != nil {
				if cfg.ContainerEnv == nil {
					t.Fatal("expected ContainerEnv to be non-nil")
				}
				for k, v := range tt.wantEnv {
					if cfg.ContainerEnv[k] != v {
						t.Errorf("ContainerEnv[%q] = %q, want %q", k, cfg.ContainerEnv[k], v)
					}
				}
			}
		})
	}
}

func TestGetProjectImage(t *testing.T) {
	tests := []struct {
		name       string
		setupFiles map[string]string
		wantImage  string
		wantBuild  bool
		wantErr    bool
	}{
		{
			name: "builds derived image when devcontainer.json has image",
			setupFiles: map[string]string{
				".devcontainer/devcontainer.json": `{"image": "golang:1.25"}`,
			},
			// When devcontainer.json has an image, we build a derived image with Claude Code
			wantBuild: true,
		},
		{
			name:       "fallback to default when no devcontainer.json",
			setupFiles: map[string]string{},
			wantImage:  GetBaseImageName(), // Hash-tagged name for content-based rebuilds
			wantBuild:  false,
		},
		{
			name: "build required when only dockerfile specified",
			setupFiles: map[string]string{
				".devcontainer/devcontainer.json": `{"build": {"dockerfile": "Dockerfile"}}`,
				".devcontainer/Dockerfile":        `FROM golang:1.25`,
			},
			wantBuild: true,
		},
		{
			name: "error when dockerfile not found",
			setupFiles: map[string]string{
				".devcontainer/devcontainer.json": `{"build": {"dockerfile": "NonExistent.Dockerfile"}}`,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "devcontainer-test-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			for relPath, content := range tt.setupFiles {
				fullPath := filepath.Join(tmpDir, relPath)
				if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
					t.Fatalf("failed to create dir for %s: %v", relPath, err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					t.Fatalf("failed to write %s: %v", relPath, err)
				}
			}

			image, needsBuild, err := GetProjectImage(tmpDir)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantBuild {
				if !needsBuild {
					t.Error("expected needsBuild=true")
				}
			} else {
				if image != tt.wantImage {
					t.Errorf("image = %q, want %q", image, tt.wantImage)
				}
				if needsBuild {
					t.Error("expected needsBuild=false")
				}
			}
		})
	}
}

func TestResolveDockerfilePath(t *testing.T) {
	tests := []struct {
		name       string
		setupFiles map[string]string
		dockerfile string
		context    string
		wantErr    bool
	}{
		{
			name: "dockerfile in .devcontainer",
			setupFiles: map[string]string{
				".devcontainer/Dockerfile": `FROM ubuntu:22.04`,
			},
			dockerfile: "Dockerfile",
			context:    "",
		},
		{
			name: "dockerfile with context",
			setupFiles: map[string]string{
				"Dockerfile": `FROM ubuntu:22.04`,
			},
			dockerfile: "Dockerfile",
			context:    "..",
		},
		{
			name:       "missing dockerfile",
			setupFiles: map[string]string{},
			dockerfile: "Dockerfile",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "devcontainer-test-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			for relPath, content := range tt.setupFiles {
				fullPath := filepath.Join(tmpDir, relPath)
				if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
					t.Fatalf("failed to create dir for %s: %v", relPath, err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					t.Fatalf("failed to write %s: %v", relPath, err)
				}
			}

			cfg := &DevcontainerConfig{
				Build: &DevcontainerBuild{
					Dockerfile: tt.dockerfile,
					Context:    tt.context,
				},
			}

			_, _, err = cfg.ResolveDockerfilePath(tmpDir)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestAcquireBuildLock(t *testing.T) {
	t.Run("same image serializes access", func(t *testing.T) {
		t.Parallel()

		const imageName = "test-lock-same-image"
		var sequence []int
		var mu sync.Mutex

		var wg sync.WaitGroup
		wg.Add(2)

		// First goroutine acquires lock, holds it briefly
		go func() {
			defer wg.Done()
			unlock := acquireBuildLock(imageName)
			mu.Lock()
			sequence = append(sequence, 1)
			mu.Unlock()
			time.Sleep(50 * time.Millisecond)
			mu.Lock()
			sequence = append(sequence, 2)
			mu.Unlock()
			unlock()
		}()

		// Give first goroutine time to acquire lock
		time.Sleep(10 * time.Millisecond)

		// Second goroutine should wait for first
		go func() {
			defer wg.Done()
			unlock := acquireBuildLock(imageName)
			mu.Lock()
			sequence = append(sequence, 3)
			mu.Unlock()
			unlock()
		}()

		wg.Wait()

		// Sequence should be 1, 2, 3 (not 1, 3, 2)
		if len(sequence) != 3 {
			t.Fatalf("expected 3 events, got %d", len(sequence))
		}
		if sequence[0] != 1 || sequence[1] != 2 || sequence[2] != 3 {
			t.Errorf("expected sequence [1,2,3], got %v", sequence)
		}
	})

	t.Run("different images allow parallel access", func(t *testing.T) {
		t.Parallel()

		var concurrent atomic.Int32
		var maxConcurrent atomic.Int32

		var wg sync.WaitGroup
		wg.Add(2)

		for i := 0; i < 2; i++ {
			imageName := fmt.Sprintf("test-lock-different-%d", i)
			go func() {
				defer wg.Done()
				unlock := acquireBuildLock(imageName)
				c := concurrent.Add(1)
				// Track max concurrent
				for {
					old := maxConcurrent.Load()
					if c <= old || maxConcurrent.CompareAndSwap(old, c) {
						break
					}
				}
				time.Sleep(50 * time.Millisecond)
				concurrent.Add(-1)
				unlock()
			}()
		}

		wg.Wait()

		// Both should have run concurrently
		if maxConcurrent.Load() != 2 {
			t.Errorf("expected max concurrent 2, got %d", maxConcurrent.Load())
		}
	})
}

func TestComputeConfigHash(t *testing.T) {
	tests := []struct {
		name           string
		setupFiles     map[string]string
		wantEmpty      bool
		expectSameHash []string // groups of configs that should produce same hash
	}{
		{
			name:       "no devcontainer.json returns empty",
			setupFiles: map[string]string{},
			wantEmpty:  true,
		},
		{
			name: "produces 12 char hex hash",
			setupFiles: map[string]string{
				".devcontainer/devcontainer.json": `{"image": "golang:1.25"}`,
			},
		},
		{
			name: "different content produces different hash",
			setupFiles: map[string]string{
				".devcontainer/devcontainer.json": `{"image": "golang:1.23"}`,
			},
		},
	}

	// Track hashes to verify different configs produce different hashes
	hashes := make(map[string]string) // config content -> hash

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "config-hash-test-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			for relPath, content := range tt.setupFiles {
				fullPath := filepath.Join(tmpDir, relPath)
				if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
					t.Fatalf("failed to create dir: %v", err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					t.Fatalf("failed to write file: %v", err)
				}
			}

			hash := computeConfigHash(tmpDir)

			if tt.wantEmpty {
				if hash != "" {
					t.Errorf("expected empty hash, got %q", hash)
				}
				return
			}

			// Verify hash format: 12 hex characters
			if len(hash) != 12 {
				t.Errorf("expected 12 char hash, got %d chars: %q", len(hash), hash)
			}
			for _, c := range hash {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
					t.Errorf("hash contains non-hex character: %q", hash)
					break
				}
			}

			// Track for uniqueness verification
			configContent := tt.setupFiles[".devcontainer/devcontainer.json"]
			if configContent != "" {
				if existingHash, ok := hashes[configContent]; ok {
					if existingHash != hash {
						t.Errorf("same content produced different hash")
					}
				} else {
					hashes[configContent] = hash
				}
			}
		})
	}

	// Verify different configs produced different hashes
	hashValues := make(map[string]bool)
	for _, h := range hashes {
		if hashValues[h] {
			t.Errorf("different configs produced same hash: %s", h)
		}
		hashValues[h] = true
	}
}

func TestComputeConfigHashNormalization(t *testing.T) {
	// These configs should produce the same hash despite formatting differences
	configs := []string{
		`{"image": "golang:1.25"}`,
		`{  "image":   "golang:1.25"  }`,
		`{"image":"golang:1.25"}`,
		`{
			"image": "golang:1.25"
		}`,
		`{
			// comment
			"image": "golang:1.25"
		}`,
	}

	var firstHash string
	for i, config := range configs {
		tmpDir, err := os.MkdirTemp("", "normalize-test-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		devcontainerDir := filepath.Join(tmpDir, ".devcontainer")
		if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
			t.Fatalf("failed to create .devcontainer: %v", err)
		}
		if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(config), 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		hash := computeConfigHash(tmpDir)
		if i == 0 {
			firstHash = hash
		} else if hash != firstHash {
			t.Errorf("config %d produced different hash:\nconfig: %s\nhash: %s\nexpected: %s", i, config, hash, firstHash)
		}
	}
}

func TestGenerateProjectImageNameWithHash(t *testing.T) {
	tests := []struct {
		name       string
		setupFiles map[string]string
		wantLatest bool // if true, expect :latest tag (no devcontainer.json)
	}{
		{
			name:       "no config uses latest tag",
			setupFiles: map[string]string{},
			wantLatest: true,
		},
		{
			name: "with config uses hash tag",
			setupFiles: map[string]string{
				".devcontainer/devcontainer.json": `{"image": "golang:1.25"}`,
			},
			wantLatest: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "imagename-test-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			for relPath, content := range tt.setupFiles {
				fullPath := filepath.Join(tmpDir, relPath)
				if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
					t.Fatalf("failed to create dir: %v", err)
				}
				if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
					t.Fatalf("failed to write file: %v", err)
				}
			}

			imageName := generateProjectImageName(tmpDir)

			if tt.wantLatest {
				if !containsString(imageName, ":latest") {
					t.Errorf("expected :latest tag, got %q", imageName)
				}
			} else {
				if containsString(imageName, ":latest") {
					t.Errorf("expected hash tag, got :latest in %q", imageName)
				}
				// Verify tag is 12 char hex
				parts := strings.Split(imageName, ":")
				if len(parts) != 2 {
					t.Fatalf("expected image:tag format, got %q", imageName)
				}
				tag := parts[1]
				if len(tag) != 12 {
					t.Errorf("expected 12 char hash tag, got %q", tag)
				}
			}
		})
	}
}

func TestConfigChangeTriggersNewImageName(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-change-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	devcontainerDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("failed to create .devcontainer: %v", err)
	}

	// Initial config
	config1 := `{"image": "golang:1.23"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(config1), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	imageName1 := generateProjectImageName(tmpDir)

	// Update config (simulating Go version bump)
	config2 := `{"image": "golang:1.25.5"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(config2), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	imageName2 := generateProjectImageName(tmpDir)

	// Image names should be different
	if imageName1 == imageName2 {
		t.Errorf("config change should produce different image name\nbefore: %s\nafter: %s", imageName1, imageName2)
	}

	// Both should have same prefix, different tags
	parts1 := strings.Split(imageName1, ":")
	parts2 := strings.Split(imageName2, ":")
	if parts1[0] != parts2[0] {
		t.Errorf("image name prefix should be same: %s vs %s", parts1[0], parts2[0])
	}
	if parts1[1] == parts2[1] {
		t.Errorf("image tags should be different: %s vs %s", parts1[1], parts2[1])
	}
}

func TestDockerfileChangeTriggersNewImageName(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dockerfile-change-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	devcontainerDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("failed to create .devcontainer: %v", err)
	}

	// Config with build section referencing Dockerfile
	config := `{"build": {"dockerfile": "Dockerfile"}}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(config), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Initial Dockerfile
	dockerfile1 := "FROM golang:1.23\nRUN go version"
	if err := os.WriteFile(filepath.Join(devcontainerDir, "Dockerfile"), []byte(dockerfile1), 0644); err != nil {
		t.Fatalf("failed to write Dockerfile: %v", err)
	}

	imageName1 := generateProjectImageName(tmpDir)
	hash1 := computeConfigHash(tmpDir)

	// Update only the Dockerfile (devcontainer.json stays the same)
	dockerfile2 := "FROM golang:1.25.5\nRUN go version"
	if err := os.WriteFile(filepath.Join(devcontainerDir, "Dockerfile"), []byte(dockerfile2), 0644); err != nil {
		t.Fatalf("failed to write Dockerfile: %v", err)
	}

	imageName2 := generateProjectImageName(tmpDir)
	hash2 := computeConfigHash(tmpDir)

	// Hashes should be different
	if hash1 == hash2 {
		t.Errorf("Dockerfile change should produce different hash\nbefore: %s\nafter: %s", hash1, hash2)
	}

	// Image names should be different
	if imageName1 == imageName2 {
		t.Errorf("Dockerfile change should produce different image name\nbefore: %s\nafter: %s", imageName1, imageName2)
	}

	// Both should have same prefix, different tags
	parts1 := strings.Split(imageName1, ":")
	parts2 := strings.Split(imageName2, ":")
	if parts1[0] != parts2[0] {
		t.Errorf("image name prefix should be same: %s vs %s", parts1[0], parts2[0])
	}
}

func TestDockerfileHashWithContext(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dockerfile-context-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	devcontainerDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("failed to create .devcontainer: %v", err)
	}

	// Config with build section using context ".." (project root)
	config := `{"build": {"dockerfile": "Dockerfile", "context": ".."}}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(config), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Dockerfile in project root (context is "..")
	dockerfile1 := "FROM node:20\nRUN npm --version"
	if err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(dockerfile1), 0644); err != nil {
		t.Fatalf("failed to write Dockerfile: %v", err)
	}

	hash1 := computeConfigHash(tmpDir)

	// Update Dockerfile in project root
	dockerfile2 := "FROM node:22\nRUN npm --version"
	if err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(dockerfile2), 0644); err != nil {
		t.Fatalf("failed to write Dockerfile: %v", err)
	}

	hash2 := computeConfigHash(tmpDir)

	// Hashes should be different
	if hash1 == hash2 {
		t.Errorf("Dockerfile change with context should produce different hash\nbefore: %s\nafter: %s", hash1, hash2)
	}
}

func TestImageOnlyConfigIgnoresDockerfile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "image-only-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	devcontainerDir := filepath.Join(tmpDir, ".devcontainer")
	if err := os.MkdirAll(devcontainerDir, 0755); err != nil {
		t.Fatalf("failed to create .devcontainer: %v", err)
	}

	// Config with image only (no build section)
	config := `{"image": "golang:1.25"}`
	if err := os.WriteFile(filepath.Join(devcontainerDir, "devcontainer.json"), []byte(config), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Random Dockerfile that shouldn't affect hash
	dockerfile := "FROM alpine:latest"
	if err := os.WriteFile(filepath.Join(devcontainerDir, "Dockerfile"), []byte(dockerfile), 0644); err != nil {
		t.Fatalf("failed to write Dockerfile: %v", err)
	}

	hash1 := computeConfigHash(tmpDir)

	// Update the Dockerfile
	dockerfile2 := "FROM ubuntu:22.04"
	if err := os.WriteFile(filepath.Join(devcontainerDir, "Dockerfile"), []byte(dockerfile2), 0644); err != nil {
		t.Fatalf("failed to write Dockerfile: %v", err)
	}

	hash2 := computeConfigHash(tmpDir)

	// Hashes should be the same (Dockerfile not referenced in config)
	if hash1 != hash2 {
		t.Errorf("Dockerfile change without build section should not affect hash\nbefore: %s\nafter: %s", hash1, hash2)
	}
}
