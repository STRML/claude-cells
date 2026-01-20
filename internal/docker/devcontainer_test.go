package docker

import (
	"fmt"
	"os"
	"path/filepath"
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
					"image": "golang:1.23"
				}`,
			},
			wantImage: "golang:1.23",
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
				".devcontainer/devcontainer.json": `{"image": "golang:1.23"}`,
				".devcontainer.json":              `{"image": "node:20"}`,
			},
			wantImage: "golang:1.23",
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
					"image": "golang:1.23",
					"containerEnv": {
						"GOPROXY": "https://proxy.golang.org",
						"CGO_ENABLED": "0"
					}
				}`,
			},
			wantImage: "golang:1.23",
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
				".devcontainer/devcontainer.json": `{"image": "golang:1.23"}`,
			},
			// When devcontainer.json has an image, we build a derived image with Claude Code
			wantBuild: true,
		},
		{
			name:       "fallback to default when no devcontainer.json",
			setupFiles: map[string]string{},
			wantImage:  DefaultImage,
			wantBuild:  false,
		},
		{
			name: "build required when only dockerfile specified",
			setupFiles: map[string]string{
				".devcontainer/devcontainer.json": `{"build": {"dockerfile": "Dockerfile"}}`,
				".devcontainer/Dockerfile":        `FROM golang:1.23`,
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
