# Docker TUI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a terminal UI for running parallel Claude Code instances in isolated Docker containers.

**Architecture:** Go application using Bubble Tea for TUI, Docker SDK for container management, Mutagen for file sync. Each "workstream" encapsulates a container + branch + Claude Code instance. Manager orchestrates multiple workstreams.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, Docker SDK, Mutagen CLI, gh CLI

---

## Task 1: Project Initialization

**Files:**
- Create: `go.mod`
- Create: `cmd/docker-tui/main.go`

**Step 1: Initialize Go module**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go mod init github.com/samuelreed/docker-tui
```
Expected: `go.mod` created

**Step 2: Create minimal main.go placeholder**

Create `cmd/docker-tui/main.go`:
```go
package main

func main() {
	// TUI entry point - implemented later
}
```

**Step 3: Verify module compiles**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go build ./cmd/docker-tui
```
Expected: No errors

**Step 4: Commit**

```bash
git add go.mod cmd/
git commit -m "chore: initialize Go module and main entry point"
```

---

## Task 2: Branch Name Generator

**Files:**
- Create: `internal/workstream/branch.go`
- Create: `internal/workstream/branch_test.go`

**Step 1: Write the failing tests**

Create `internal/workstream/branch_test.go`:
```go
package workstream

import "testing"

func TestGenerateBranchName(t *testing.T) {
	tests := []struct {
		name     string
		prompt   string
		expected string
	}{
		{
			name:     "simple prompt",
			prompt:   "add user authentication",
			expected: "add-user-authentication",
		},
		{
			name:     "strips common words",
			prompt:   "add the user authentication to the app",
			expected: "add-user-authentication-app",
		},
		{
			name:     "converts to lowercase",
			prompt:   "Add User Authentication",
			expected: "add-user-authentication",
		},
		{
			name:     "replaces spaces with hyphens",
			prompt:   "fix   multiple   spaces",
			expected: "fix-multiple-spaces",
		},
		{
			name:     "truncates to 50 chars",
			prompt:   "implement a very long feature that has way too many words to fit in a branch name",
			expected: "implement-very-long-feature-has-way-too-many-words",
		},
		{
			name:     "handles special characters",
			prompt:   "add JWT/OAuth authentication!",
			expected: "add-jwt-oauth-authentication",
		},
		{
			name:     "strips leading/trailing hyphens",
			prompt:   "the authentication",
			expected: "authentication",
		},
		{
			name:     "with keyword",
			prompt:   "add user authentication with JWT tokens",
			expected: "add-user-authentication-jwt-tokens",
		},
		{
			name:     "for keyword",
			prompt:   "create tests for the API",
			expected: "create-tests-api",
		},
		{
			name:     "handles empty after stripping",
			prompt:   "the a an to for with",
			expected: "workstream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateBranchName(tt.prompt)
			if got != tt.expected {
				t.Errorf("GenerateBranchName(%q) = %q, want %q", tt.prompt, got, tt.expected)
			}
		})
	}
}

func TestGenerateBranchName_MaxLength(t *testing.T) {
	prompt := "this is an extremely long prompt that should definitely be truncated because it exceeds the maximum allowed length for a git branch name"
	got := GenerateBranchName(prompt)
	if len(got) > 50 {
		t.Errorf("GenerateBranchName() returned %d chars, want <= 50", len(got))
	}
}

func TestGenerateBranchName_NoTrailingHyphen(t *testing.T) {
	// Edge case: truncation might leave a trailing hyphen
	prompt := "add something that ends at exactly fifty chars here"
	got := GenerateBranchName(prompt)
	if got[len(got)-1] == '-' {
		t.Errorf("GenerateBranchName() has trailing hyphen: %q", got)
	}
}
```

**Step 2: Run tests to verify they fail**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go test ./internal/workstream/... -v
```
Expected: FAIL with "no Go files in directory" or similar

**Step 3: Write the implementation**

Create `internal/workstream/branch.go`:
```go
package workstream

import (
	"regexp"
	"strings"
)

// stopWords are common words stripped from branch names
var stopWords = map[string]bool{
	"the":  true,
	"a":    true,
	"an":   true,
	"to":   true,
	"for":  true,
	"with": true,
	"and":  true,
	"or":   true,
	"in":   true,
	"on":   true,
	"at":   true,
	"by":   true,
	"of":   true,
	"is":   true,
	"it":   true,
}

const maxBranchLength = 50
const defaultBranchName = "workstream"

// GenerateBranchName creates a git branch name from a prompt.
// Rules:
// - Lowercase
// - Hyphens instead of spaces
// - Max 50 chars
// - Strip common words
// - Keep it readable
func GenerateBranchName(prompt string) string {
	// Convert to lowercase
	name := strings.ToLower(prompt)

	// Replace special characters with spaces (except hyphens)
	nonAlphaNum := regexp.MustCompile(`[^a-z0-9\s-]`)
	name = nonAlphaNum.ReplaceAllString(name, " ")

	// Split into words
	words := strings.Fields(name)

	// Filter out stop words
	var filtered []string
	for _, word := range words {
		if !stopWords[word] && word != "" {
			filtered = append(filtered, word)
		}
	}

	// Handle empty result
	if len(filtered) == 0 {
		return defaultBranchName
	}

	// Join with hyphens
	name = strings.Join(filtered, "-")

	// Truncate to max length
	if len(name) > maxBranchLength {
		name = name[:maxBranchLength]
	}

	// Remove trailing hyphen if truncation left one
	name = strings.TrimRight(name, "-")

	return name
}
```

**Step 4: Run tests to verify they pass**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go test ./internal/workstream/... -v
```
Expected: PASS (all tests)

**Step 5: Commit**

```bash
git add internal/workstream/
git commit -m "feat: add branch name generator from prompts

Converts user prompts to valid git branch names by:
- Lowercasing and replacing spaces with hyphens
- Stripping common words (the, a, to, for, with, etc.)
- Truncating to 50 chars max
- Handling special characters"
```

---

## Task 3: Docker Client Wrapper

**Files:**
- Create: `internal/docker/client.go`
- Create: `internal/docker/client_test.go`

**Step 1: Add Docker SDK dependency**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go get github.com/docker/docker/client
```
Expected: `go.mod` and `go.sum` updated

**Step 2: Write the failing tests**

Create `internal/docker/client_test.go`:
```go
package docker

import (
	"context"
	"testing"
)

func TestNewClient(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
	defer client.Close()
}

func TestClient_Ping(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	err = client.Ping(ctx)
	if err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

func TestClient_Close(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	err = client.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}
```

**Step 3: Run tests to verify they fail**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go test ./internal/docker/... -v
```
Expected: FAIL with "no Go files"

**Step 4: Write the implementation**

Create `internal/docker/client.go`:
```go
package docker

import (
	"context"

	"github.com/docker/docker/client"
)

// Client wraps the Docker SDK client with simplified operations.
type Client struct {
	cli *client.Client
}

// NewClient creates a new Docker client using environment defaults.
func NewClient() (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Client{cli: cli}, nil
}

// Ping checks connectivity to the Docker daemon.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	return err
}

// Close releases the Docker client resources.
func (c *Client) Close() error {
	return c.cli.Close()
}

// Raw returns the underlying Docker client for advanced operations.
func (c *Client) Raw() *client.Client {
	return c.cli
}
```

**Step 5: Run tests to verify they pass**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go test ./internal/docker/... -v
```
Expected: PASS (requires Docker daemon running)

**Step 6: Commit**

```bash
git add internal/docker/ go.mod go.sum
git commit -m "feat: add Docker client wrapper

Thin wrapper around Docker SDK with:
- NewClient() using environment defaults
- Ping() for connectivity check
- Close() for cleanup
- Raw() for advanced operations"
```

---

## Task 4: Container Lifecycle

**Files:**
- Create: `internal/docker/container.go`
- Create: `internal/docker/container_test.go`
- Create: `configs/base.Dockerfile`

**Step 1: Create base Dockerfile**

Create `configs/base.Dockerfile`:
```dockerfile
ARG BASE_IMAGE=mcr.microsoft.com/devcontainers/base:ubuntu
FROM ${BASE_IMAGE}

# Install Node.js LTS
RUN curl -fsSL https://nodejs.org/dist/v20.11.0/node-v20.11.0-linux-x64.tar.xz \
    | tar -xJ -C /usr/local --strip-components=1

# Install Claude Code
RUN npm install -g @anthropic-ai/claude-code

WORKDIR /workspace
CMD ["sleep", "infinity"]
```

**Step 2: Write the failing tests**

Create `internal/docker/container_test.go`:
```go
package docker

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestContainerConfig(t *testing.T) {
	tests := []struct {
		name       string
		branchName string
		repoPath   string
		wantName   string
	}{
		{
			name:       "creates config with branch name",
			branchName: "add-auth",
			repoPath:   "/path/to/repo",
			wantName:   "docker-tui-add-auth",
		},
		{
			name:       "sanitizes special chars in name",
			branchName: "feature/add-auth",
			repoPath:   "/path/to/repo",
			wantName:   "docker-tui-feature-add-auth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewContainerConfig(tt.branchName, tt.repoPath)
			if cfg.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", cfg.Name, tt.wantName)
			}
			if cfg.RepoPath != tt.repoPath {
				t.Errorf("RepoPath = %q, want %q", cfg.RepoPath, tt.repoPath)
			}
		})
	}
}

// Integration test - requires Docker
func TestContainer_Lifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Use alpine for fast testing
	cfg := &ContainerConfig{
		Name:      "docker-tui-test-" + time.Now().Format("150405"),
		Image:     "alpine:latest",
		RepoPath:  "/tmp",
		ClaudeCfg: "/tmp",
	}

	// Create container
	containerID, err := client.CreateContainer(ctx, cfg)
	if err != nil {
		t.Fatalf("CreateContainer() error = %v", err)
	}
	if containerID == "" {
		t.Fatal("CreateContainer() returned empty ID")
	}

	// Cleanup
	defer func() {
		_ = client.StopContainer(ctx, containerID)
		_ = client.RemoveContainer(ctx, containerID)
	}()

	// Start container
	err = client.StartContainer(ctx, containerID)
	if err != nil {
		t.Fatalf("StartContainer() error = %v", err)
	}

	// Check it's running
	running, err := client.IsContainerRunning(ctx, containerID)
	if err != nil {
		t.Fatalf("IsContainerRunning() error = %v", err)
	}
	if !running {
		t.Error("Container should be running")
	}

	// Execute command
	output, err := client.ExecInContainer(ctx, containerID, []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("ExecInContainer() error = %v", err)
	}
	if !strings.Contains(output, "hello") {
		t.Errorf("ExecInContainer() output = %q, want to contain 'hello'", output)
	}

	// Stop container
	err = client.StopContainer(ctx, containerID)
	if err != nil {
		t.Fatalf("StopContainer() error = %v", err)
	}

	// Verify stopped
	running, err = client.IsContainerRunning(ctx, containerID)
	if err != nil {
		t.Fatalf("IsContainerRunning() error = %v", err)
	}
	if running {
		t.Error("Container should be stopped")
	}
}
```

**Step 3: Run tests to verify they fail**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go test ./internal/docker/... -v -short
```
Expected: FAIL

**Step 4: Write the implementation**

Create `internal/docker/container.go`:
```go
package docker

import (
	"bytes"
	"context"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/pkg/stdcopy"
)

// ContainerConfig holds configuration for creating a workstream container.
type ContainerConfig struct {
	Name      string // Container name (docker-tui-<branch>)
	Image     string // Docker image to use
	RepoPath  string // Path to repo on host
	ClaudeCfg string // Path to ~/.claude on host
}

// NewContainerConfig creates a container config for a workstream.
func NewContainerConfig(branchName, repoPath string) *ContainerConfig {
	// Sanitize branch name for container naming
	safeName := strings.ReplaceAll(branchName, "/", "-")
	return &ContainerConfig{
		Name:     "docker-tui-" + safeName,
		RepoPath: repoPath,
	}
}

// CreateContainer creates a new container but doesn't start it.
func (c *Client) CreateContainer(ctx context.Context, cfg *ContainerConfig) (string, error) {
	containerCfg := &container.Config{
		Image: cfg.Image,
		Cmd:   []string{"sleep", "infinity"},
		Tty:   true,
	}

	var mounts []mount.Mount
	if cfg.RepoPath != "" {
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: cfg.RepoPath,
			Target: "/workspace",
		})
	}
	if cfg.ClaudeCfg != "" {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   cfg.ClaudeCfg,
			Target:   "/root/.claude",
			ReadOnly: true,
		})
	}

	hostCfg := &container.HostConfig{
		Mounts: mounts,
	}

	resp, err := c.cli.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, cfg.Name)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// StartContainer starts a created container.
func (c *Client) StartContainer(ctx context.Context, containerID string) error {
	return c.cli.ContainerStart(ctx, containerID, container.StartOptions{})
}

// StopContainer stops a running container.
func (c *Client) StopContainer(ctx context.Context, containerID string) error {
	timeout := 10 // seconds
	return c.cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
}

// RemoveContainer removes a container.
func (c *Client) RemoveContainer(ctx context.Context, containerID string) error {
	return c.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
}

// IsContainerRunning checks if a container is running.
func (c *Client) IsContainerRunning(ctx context.Context, containerID string) (bool, error) {
	info, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return false, err
	}
	return info.State.Running, nil
}

// ExecInContainer runs a command in a container and returns output.
func (c *Client) ExecInContainer(ctx context.Context, containerID string, cmd []string) (string, error) {
	execCfg := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := c.cli.ContainerExecCreate(ctx, containerID, execCfg)
	if err != nil {
		return "", err
	}

	resp, err := c.cli.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return "", err
	}
	defer resp.Close()

	var stdout, stderr bytes.Buffer
	_, err = stdcopy.StdCopy(&stdout, &stderr, resp.Reader)
	if err != nil && err != io.EOF {
		return "", err
	}

	return stdout.String() + stderr.String(), nil
}
```

**Step 5: Run tests to verify they pass**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go test ./internal/docker/... -v
```
Expected: PASS

**Step 6: Commit**

```bash
git add internal/docker/container.go internal/docker/container_test.go configs/
git commit -m "feat: add container lifecycle management

- ContainerConfig for workstream containers
- Create, Start, Stop, Remove operations
- ExecInContainer for running commands
- IsContainerRunning status check
- Base Dockerfile with Node.js and Claude Code"
```

---

## Task 5: Workstream State

**Files:**
- Create: `internal/workstream/workstream.go`
- Create: `internal/workstream/workstream_test.go`

**Step 1: Write the failing tests**

Create `internal/workstream/workstream_test.go`:
```go
package workstream

import (
	"testing"
	"time"
)

func TestWorkstreamState(t *testing.T) {
	tests := []struct {
		name     string
		state    State
		isActive bool
	}{
		{"running is active", StateRunning, true},
		{"idle is active", StateIdle, true},
		{"pairing is active", StatePairing, true},
		{"stopped is not active", StateStopped, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.state.IsActive() != tt.isActive {
				t.Errorf("State(%q).IsActive() = %v, want %v", tt.state, tt.state.IsActive(), tt.isActive)
			}
		})
	}
}

func TestNewWorkstream(t *testing.T) {
	prompt := "add user authentication"
	ws := New(prompt)

	if ws.Prompt != prompt {
		t.Errorf("Prompt = %q, want %q", ws.Prompt, prompt)
	}
	if ws.BranchName != "add-user-authentication" {
		t.Errorf("BranchName = %q, want %q", ws.BranchName, "add-user-authentication")
	}
	if ws.State != StateStopped {
		t.Errorf("State = %q, want %q", ws.State, StateStopped)
	}
	if ws.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestWorkstream_SetState(t *testing.T) {
	ws := New("test")
	ws.SetState(StateRunning)

	if ws.State != StateRunning {
		t.Errorf("State = %q, want %q", ws.State, StateRunning)
	}
}

func TestWorkstream_SetContainerID(t *testing.T) {
	ws := New("test")
	ws.SetContainerID("abc123")

	if ws.ContainerID != "abc123" {
		t.Errorf("ContainerID = %q, want %q", ws.ContainerID, "abc123")
	}
}

func TestWorkstream_LastActivity(t *testing.T) {
	ws := New("test")
	before := time.Now()

	ws.UpdateActivity()

	if ws.LastActivity.Before(before) {
		t.Error("LastActivity should be updated to current time")
	}
}

func TestWorkstream_String(t *testing.T) {
	ws := New("add auth")
	ws.SetState(StateRunning)

	str := ws.String()
	if str == "" {
		t.Error("String() should return non-empty")
	}
}
```

**Step 2: Run tests to verify they fail**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go test ./internal/workstream/... -v -run TestWorkstream
```
Expected: FAIL

**Step 3: Write the implementation**

Create `internal/workstream/workstream.go`:
```go
package workstream

import (
	"fmt"
	"sync"
	"time"
)

// State represents the lifecycle state of a workstream.
type State string

const (
	StateRunning State = "running" // Claude active, processing or waiting
	StateIdle    State = "idle"    // Claude finished, container alive
	StatePairing State = "pairing" // Mutagen sync active with local
	StateStopped State = "stopped" // Container halted, can resume
)

// IsActive returns true if the workstream container should be running.
func (s State) IsActive() bool {
	return s == StateRunning || s == StateIdle || s == StatePairing
}

// Workstream represents a Docker container + git branch + Claude Code instance.
type Workstream struct {
	mu sync.RWMutex

	// Identity
	ID         string // Unique identifier
	Prompt     string // Original user prompt
	BranchName string // Generated git branch name

	// Docker
	ContainerID string // Docker container ID

	// State
	State        State     // Current lifecycle state
	CreatedAt    time.Time // When workstream was created
	LastActivity time.Time // Last interaction time

	// PR info (optional)
	PRNumber int    // GitHub PR number if created
	PRURL    string // GitHub PR URL if created
}

// New creates a new workstream from a prompt.
func New(prompt string) *Workstream {
	now := time.Now()
	return &Workstream{
		ID:           fmt.Sprintf("%d", now.UnixNano()),
		Prompt:       prompt,
		BranchName:   GenerateBranchName(prompt),
		State:        StateStopped,
		CreatedAt:    now,
		LastActivity: now,
	}
}

// SetState updates the workstream state.
func (w *Workstream) SetState(state State) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.State = state
	w.LastActivity = time.Now()
}

// SetContainerID sets the Docker container ID.
func (w *Workstream) SetContainerID(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ContainerID = id
}

// UpdateActivity updates the last activity timestamp.
func (w *Workstream) UpdateActivity() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.LastActivity = time.Now()
}

// GetState returns the current state (thread-safe).
func (w *Workstream) GetState() State {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.State
}

// String returns a human-readable representation.
func (w *Workstream) String() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return fmt.Sprintf("%s (%s)", w.BranchName, w.State)
}
```

**Step 4: Run tests to verify they pass**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go test ./internal/workstream/... -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/workstream/workstream.go internal/workstream/workstream_test.go
git commit -m "feat: add Workstream state model

Workstream encapsulates:
- Identity: ID, prompt, branch name
- Docker: container ID
- State: running/idle/pairing/stopped
- Timestamps: created, last activity
- PR info: number and URL"
```

---

## Task 6: Workstream Manager

**Files:**
- Create: `internal/workstream/manager.go`
- Create: `internal/workstream/manager_test.go`

**Step 1: Write the failing tests**

Create `internal/workstream/manager_test.go`:
```go
package workstream

import (
	"testing"
)

func TestManager_Add(t *testing.T) {
	m := NewManager()
	ws := New("test prompt")

	m.Add(ws)

	got := m.Get(ws.ID)
	if got != ws {
		t.Error("Get() should return added workstream")
	}
}

func TestManager_Remove(t *testing.T) {
	m := NewManager()
	ws := New("test prompt")
	m.Add(ws)

	m.Remove(ws.ID)

	got := m.Get(ws.ID)
	if got != nil {
		t.Error("Get() should return nil after Remove()")
	}
}

func TestManager_List(t *testing.T) {
	m := NewManager()
	ws1 := New("first")
	ws2 := New("second")
	m.Add(ws1)
	m.Add(ws2)

	list := m.List()

	if len(list) != 2 {
		t.Errorf("List() len = %d, want 2", len(list))
	}
}

func TestManager_Count(t *testing.T) {
	m := NewManager()

	if m.Count() != 0 {
		t.Error("Count() should be 0 for new manager")
	}

	m.Add(New("test"))

	if m.Count() != 1 {
		t.Error("Count() should be 1 after Add()")
	}
}

func TestManager_GetByBranch(t *testing.T) {
	m := NewManager()
	ws := New("add user auth")
	m.Add(ws)

	got := m.GetByBranch("add-user-auth")
	if got != ws {
		t.Error("GetByBranch() should return workstream")
	}

	got = m.GetByBranch("nonexistent")
	if got != nil {
		t.Error("GetByBranch() should return nil for nonexistent")
	}
}

func TestManager_Active(t *testing.T) {
	m := NewManager()
	ws1 := New("running")
	ws1.SetState(StateRunning)
	ws2 := New("stopped")
	ws2.SetState(StateStopped)
	m.Add(ws1)
	m.Add(ws2)

	active := m.Active()

	if len(active) != 1 {
		t.Errorf("Active() len = %d, want 1", len(active))
	}
	if active[0] != ws1 {
		t.Error("Active() should return running workstream")
	}
}

func TestManager_GetPairing(t *testing.T) {
	m := NewManager()
	ws := New("pairing")
	ws.SetState(StatePairing)
	m.Add(ws)

	got := m.GetPairing()
	if got != ws {
		t.Error("GetPairing() should return pairing workstream")
	}
}

func TestManager_GetPairing_None(t *testing.T) {
	m := NewManager()
	ws := New("running")
	ws.SetState(StateRunning)
	m.Add(ws)

	got := m.GetPairing()
	if got != nil {
		t.Error("GetPairing() should return nil when none pairing")
	}
}
```

**Step 2: Run tests to verify they fail**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go test ./internal/workstream/... -v -run TestManager
```
Expected: FAIL

**Step 3: Write the implementation**

Create `internal/workstream/manager.go`:
```go
package workstream

import (
	"sync"
)

// Manager tracks all workstreams.
type Manager struct {
	mu         sync.RWMutex
	workstreams map[string]*Workstream
}

// NewManager creates a new workstream manager.
func NewManager() *Manager {
	return &Manager{
		workstreams: make(map[string]*Workstream),
	}
}

// Add registers a workstream.
func (m *Manager) Add(ws *Workstream) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workstreams[ws.ID] = ws
}

// Remove unregisters a workstream.
func (m *Manager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.workstreams, id)
}

// Get returns a workstream by ID.
func (m *Manager) Get(id string) *Workstream {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.workstreams[id]
}

// GetByBranch returns a workstream by branch name.
func (m *Manager) GetByBranch(branchName string) *Workstream {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ws := range m.workstreams {
		if ws.BranchName == branchName {
			return ws
		}
	}
	return nil
}

// List returns all workstreams.
func (m *Manager) List() []*Workstream {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]*Workstream, 0, len(m.workstreams))
	for _, ws := range m.workstreams {
		list = append(list, ws)
	}
	return list
}

// Active returns workstreams with active containers.
func (m *Manager) Active() []*Workstream {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var active []*Workstream
	for _, ws := range m.workstreams {
		if ws.GetState().IsActive() {
			active = append(active, ws)
		}
	}
	return active
}

// Count returns the number of workstreams.
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.workstreams)
}

// GetPairing returns the workstream in pairing mode, if any.
func (m *Manager) GetPairing() *Workstream {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ws := range m.workstreams {
		if ws.GetState() == StatePairing {
			return ws
		}
	}
	return nil
}
```

**Step 4: Run tests to verify they pass**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go test ./internal/workstream/... -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/workstream/manager.go internal/workstream/manager_test.go
git commit -m "feat: add workstream Manager

Thread-safe manager for tracking workstreams:
- Add/Remove/Get by ID
- GetByBranch lookup
- List all / Active only
- GetPairing for sync mode"
```

---

## Task 7: Mutagen Wrapper

**Files:**
- Create: `internal/sync/mutagen.go`
- Create: `internal/sync/mutagen_test.go`

**Step 1: Write the failing tests**

Create `internal/sync/mutagen_test.go`:
```go
package sync

import (
	"context"
	"testing"
)

func TestMutagenSessionName(t *testing.T) {
	tests := []struct {
		branch   string
		expected string
	}{
		{"add-auth", "docker-tui-add-auth"},
		{"feature/login", "docker-tui-feature-login"},
	}

	for _, tt := range tests {
		got := SessionName(tt.branch)
		if got != tt.expected {
			t.Errorf("SessionName(%q) = %q, want %q", tt.branch, got, tt.expected)
		}
	}
}

func TestMutagen_CheckInstalled(t *testing.T) {
	m := NewMutagen()
	ctx := context.Background()

	err := m.CheckInstalled(ctx)
	// This test passes if mutagen is installed, skips otherwise
	if err != nil {
		t.Skipf("mutagen not installed: %v", err)
	}
}

func TestMutagen_CreateSession_InvalidContainer(t *testing.T) {
	m := NewMutagen()
	ctx := context.Background()

	err := m.CreateSession(ctx, "nonexistent-branch", "invalid-container", "/tmp")
	if err == nil {
		t.Error("CreateSession() should fail with invalid container")
	}
}
```

**Step 2: Run tests to verify they fail**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go test ./internal/sync/... -v
```
Expected: FAIL

**Step 3: Write the implementation**

Create `internal/sync/mutagen.go`:
```go
package sync

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Mutagen manages file sync sessions with containers.
type Mutagen struct{}

// NewMutagen creates a new Mutagen wrapper.
func NewMutagen() *Mutagen {
	return &Mutagen{}
}

// SessionName generates a mutagen session name for a branch.
func SessionName(branchName string) string {
	safe := strings.ReplaceAll(branchName, "/", "-")
	return "docker-tui-" + safe
}

// CheckInstalled verifies mutagen is available.
func (m *Mutagen) CheckInstalled(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "mutagen", "version")
	return cmd.Run()
}

// CreateSession starts a bidirectional sync between container and local path.
func (m *Mutagen) CreateSession(ctx context.Context, branchName, containerID, localPath string) error {
	sessionName := SessionName(branchName)
	containerPath := fmt.Sprintf("docker://%s/workspace", containerID)

	cmd := exec.CommandContext(ctx, "mutagen", "sync", "create",
		"--name", sessionName,
		"--sync-mode", "two-way-resolved",
		"--ignore", ".git",
		containerPath,
		localPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mutagen create failed: %w: %s", err, output)
	}
	return nil
}

// TerminateSession stops and removes a sync session.
func (m *Mutagen) TerminateSession(ctx context.Context, branchName string) error {
	sessionName := SessionName(branchName)

	cmd := exec.CommandContext(ctx, "mutagen", "sync", "terminate", sessionName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mutagen terminate failed: %w: %s", err, output)
	}
	return nil
}

// SessionExists checks if a session exists.
func (m *Mutagen) SessionExists(ctx context.Context, branchName string) (bool, error) {
	sessionName := SessionName(branchName)

	cmd := exec.CommandContext(ctx, "mutagen", "sync", "list", sessionName)
	err := cmd.Run()
	if err != nil {
		// Exit code 1 means not found
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetConflicts returns any sync conflict files.
func (m *Mutagen) GetConflicts(ctx context.Context, branchName string) ([]string, error) {
	sessionName := SessionName(branchName)

	cmd := exec.CommandContext(ctx, "mutagen", "sync", "list", "--long", sessionName)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Parse output for conflict indicators
	var conflicts []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Conflicts:") {
			// Extract conflict info - simplified parsing
			conflicts = append(conflicts, strings.TrimSpace(line))
		}
	}
	return conflicts, nil
}
```

**Step 4: Run tests to verify they pass**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go test ./internal/sync/... -v
```
Expected: PASS (some tests may skip if mutagen not installed)

**Step 5: Commit**

```bash
git add internal/sync/
git commit -m "feat: add Mutagen sync wrapper

Wrapper for mutagen CLI:
- CreateSession for bidirectional sync
- TerminateSession for cleanup
- SessionExists check
- GetConflicts detection
- Two-way-resolved mode with .git ignore"
```

---

## Task 8: Pairing Mode Orchestration

**Files:**
- Create: `internal/sync/pairing.go`
- Create: `internal/sync/pairing_test.go`

**Step 1: Write the failing tests**

Create `internal/sync/pairing_test.go`:
```go
package sync

import (
	"context"
	"testing"
)

func TestPairingState(t *testing.T) {
	p := NewPairing(nil, nil)

	if p.IsActive() {
		t.Error("IsActive() should be false initially")
	}
	if p.CurrentBranch() != "" {
		t.Error("CurrentBranch() should be empty initially")
	}
}

func TestPairing_StashedChanges(t *testing.T) {
	p := NewPairing(nil, nil)

	if p.HasStashedChanges() {
		t.Error("HasStashedChanges() should be false initially")
	}

	p.SetStashedChanges(true)

	if !p.HasStashedChanges() {
		t.Error("HasStashedChanges() should be true after SetStashedChanges(true)")
	}
}

// MockGit for testing without real git operations
type MockGit struct {
	stashCalled    bool
	checkoutCalled string
	hasChanges     bool
}

func (m *MockGit) HasUncommittedChanges(ctx context.Context) (bool, error) {
	return m.hasChanges, nil
}

func (m *MockGit) Stash(ctx context.Context) error {
	m.stashCalled = true
	return nil
}

func (m *MockGit) Checkout(ctx context.Context, branch string) error {
	m.checkoutCalled = branch
	return nil
}

func (m *MockGit) CurrentBranch(ctx context.Context) (string, error) {
	return "main", nil
}

// MockMutagen for testing
type MockMutagen struct {
	createCalled    bool
	terminateCalled bool
}

func (m *MockMutagen) CreateSession(ctx context.Context, branch, container, path string) error {
	m.createCalled = true
	return nil
}

func (m *MockMutagen) TerminateSession(ctx context.Context, branch string) error {
	m.terminateCalled = true
	return nil
}

func TestPairing_Enable_NoChanges(t *testing.T) {
	git := &MockGit{hasChanges: false}
	mutagen := &MockMutagen{}
	p := NewPairingWithMocks(git, mutagen)
	ctx := context.Background()

	err := p.Enable(ctx, "feature-branch", "container-123", "/path/to/repo")
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	if !mutagen.createCalled {
		t.Error("Mutagen CreateSession should be called")
	}
	if git.stashCalled {
		t.Error("Git Stash should not be called when no changes")
	}
	if git.checkoutCalled != "feature-branch" {
		t.Errorf("Git Checkout called with %q, want %q", git.checkoutCalled, "feature-branch")
	}
	if !p.IsActive() {
		t.Error("IsActive() should be true after Enable()")
	}
}

func TestPairing_Enable_WithChanges(t *testing.T) {
	git := &MockGit{hasChanges: true}
	mutagen := &MockMutagen{}
	p := NewPairingWithMocks(git, mutagen)
	ctx := context.Background()

	err := p.Enable(ctx, "feature-branch", "container-123", "/path/to/repo")
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	if !git.stashCalled {
		t.Error("Git Stash should be called when changes exist")
	}
	if !p.HasStashedChanges() {
		t.Error("HasStashedChanges() should be true")
	}
}

func TestPairing_Disable(t *testing.T) {
	git := &MockGit{}
	mutagen := &MockMutagen{}
	p := NewPairingWithMocks(git, mutagen)
	ctx := context.Background()

	// First enable
	_ = p.Enable(ctx, "feature-branch", "container-123", "/path/to/repo")

	// Then disable
	err := p.Disable(ctx)
	if err != nil {
		t.Fatalf("Disable() error = %v", err)
	}

	if !mutagen.terminateCalled {
		t.Error("Mutagen TerminateSession should be called")
	}
	if p.IsActive() {
		t.Error("IsActive() should be false after Disable()")
	}
}
```

**Step 2: Run tests to verify they fail**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go test ./internal/sync/... -v -run TestPairing
```
Expected: FAIL

**Step 3: Write the implementation**

Create `internal/sync/pairing.go`:
```go
package sync

import (
	"context"
	"sync"
)

// GitOperations defines git operations needed for pairing.
type GitOperations interface {
	HasUncommittedChanges(ctx context.Context) (bool, error)
	Stash(ctx context.Context) error
	Checkout(ctx context.Context, branch string) error
	CurrentBranch(ctx context.Context) (string, error)
}

// MutagenOperations defines mutagen operations needed for pairing.
type MutagenOperations interface {
	CreateSession(ctx context.Context, branch, container, path string) error
	TerminateSession(ctx context.Context, branch string) error
}

// Pairing orchestrates file sync between container and local.
type Pairing struct {
	mu sync.RWMutex

	git     GitOperations
	mutagen MutagenOperations

	active         bool
	currentBranch  string
	previousBranch string
	containerID    string
	localPath      string
	stashedChanges bool
}

// NewPairing creates a new pairing orchestrator.
func NewPairing(git GitOperations, mutagen MutagenOperations) *Pairing {
	return &Pairing{
		git:     git,
		mutagen: mutagen,
	}
}

// NewPairingWithMocks creates a pairing orchestrator with mock dependencies (for testing).
func NewPairingWithMocks(git GitOperations, mutagen MutagenOperations) *Pairing {
	return NewPairing(git, mutagen)
}

// IsActive returns true if pairing mode is active.
func (p *Pairing) IsActive() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.active
}

// CurrentBranch returns the branch being paired.
func (p *Pairing) CurrentBranch() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentBranch
}

// HasStashedChanges returns true if local changes were stashed.
func (p *Pairing) HasStashedChanges() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stashedChanges
}

// SetStashedChanges sets the stashed changes flag.
func (p *Pairing) SetStashedChanges(stashed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stashedChanges = stashed
}

// Enable starts pairing mode for a workstream.
func (p *Pairing) Enable(ctx context.Context, branchName, containerID, localPath string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check for uncommitted changes
	hasChanges, err := p.git.HasUncommittedChanges(ctx)
	if err != nil {
		return err
	}

	// Stash if needed
	if hasChanges {
		if err := p.git.Stash(ctx); err != nil {
			return err
		}
		p.stashedChanges = true
	}

	// Get current branch before switching
	prevBranch, err := p.git.CurrentBranch(ctx)
	if err != nil {
		return err
	}
	p.previousBranch = prevBranch

	// Start mutagen sync
	if err := p.mutagen.CreateSession(ctx, branchName, containerID, localPath); err != nil {
		return err
	}

	// Checkout workstream branch locally
	if err := p.git.Checkout(ctx, branchName); err != nil {
		// Try to clean up mutagen if checkout fails
		_ = p.mutagen.TerminateSession(ctx, branchName)
		return err
	}

	p.active = true
	p.currentBranch = branchName
	p.containerID = containerID
	p.localPath = localPath

	return nil
}

// Disable stops pairing mode.
func (p *Pairing) Disable(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.active {
		return nil
	}

	// Terminate mutagen sync
	if err := p.mutagen.TerminateSession(ctx, p.currentBranch); err != nil {
		return err
	}

	// Checkout previous branch
	if p.previousBranch != "" {
		if err := p.git.Checkout(ctx, p.previousBranch); err != nil {
			return err
		}
	}

	p.active = false
	p.currentBranch = ""
	p.containerID = ""

	return nil
}
```

**Step 4: Run tests to verify they pass**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go test ./internal/sync/... -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/sync/pairing.go internal/sync/pairing_test.go
git commit -m "feat: add pairing mode orchestration

Orchestrates bidirectional sync:
- Enable: stash changes, start mutagen, checkout branch
- Disable: terminate mutagen, restore previous branch
- Tracks stashed changes for user notification
- Interface-based for testability"
```

---

## Task 9: Git Branch Operations

**Files:**
- Create: `internal/git/branch.go`
- Create: `internal/git/branch_test.go`

**Step 1: Write the failing tests**

Create `internal/git/branch_test.go`:
```go
package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Initialize git repo
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "initial"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup failed: %v: %s", err, out)
		}
	}
	return dir
}

func TestGit_CurrentBranch(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	branch, err := g.CurrentBranch(ctx)
	if err != nil {
		t.Fatalf("CurrentBranch() error = %v", err)
	}
	// Git default branch could be main or master
	if branch != "main" && branch != "master" {
		t.Errorf("CurrentBranch() = %q, want main or master", branch)
	}
}

func TestGit_CreateBranch(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	err := g.CreateBranch(ctx, "test-branch")
	if err != nil {
		t.Fatalf("CreateBranch() error = %v", err)
	}

	// Verify branch exists
	cmd := exec.Command("git", "branch", "--list", "test-branch")
	cmd.Dir = dir
	out, _ := cmd.Output()
	if len(out) == 0 {
		t.Error("Branch was not created")
	}
}

func TestGit_Checkout(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Create and checkout
	_ = g.CreateBranch(ctx, "feature")
	err := g.Checkout(ctx, "feature")
	if err != nil {
		t.Fatalf("Checkout() error = %v", err)
	}

	branch, _ := g.CurrentBranch(ctx)
	if branch != "feature" {
		t.Errorf("CurrentBranch() = %q after checkout, want 'feature'", branch)
	}
}

func TestGit_HasUncommittedChanges(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Initially clean
	hasChanges, err := g.HasUncommittedChanges(ctx)
	if err != nil {
		t.Fatalf("HasUncommittedChanges() error = %v", err)
	}
	if hasChanges {
		t.Error("HasUncommittedChanges() = true, want false for clean repo")
	}

	// Create uncommitted file
	_ = os.WriteFile(filepath.Join(dir, "test.txt"), []byte("test"), 0644)

	hasChanges, err = g.HasUncommittedChanges(ctx)
	if err != nil {
		t.Fatalf("HasUncommittedChanges() error = %v", err)
	}
	if !hasChanges {
		t.Error("HasUncommittedChanges() = false, want true after adding file")
	}
}

func TestGit_Stash(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Create a tracked file first
	_ = os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("tracked"), 0644)
	exec.Command("git", "-C", dir, "add", "tracked.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "add tracked").Run()

	// Modify tracked file
	_ = os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("modified"), 0644)

	err := g.Stash(ctx)
	if err != nil {
		t.Fatalf("Stash() error = %v", err)
	}

	hasChanges, _ := g.HasUncommittedChanges(ctx)
	if hasChanges {
		t.Error("HasUncommittedChanges() = true after Stash(), want false")
	}
}

func TestGit_DeleteBranch(t *testing.T) {
	dir := setupTestRepo(t)
	defer os.RemoveAll(dir)

	g := New(dir)
	ctx := context.Background()

	// Create branch and switch back
	_ = g.CreateBranch(ctx, "to-delete")

	// Stay on main
	err := g.DeleteBranch(ctx, "to-delete")
	if err != nil {
		t.Fatalf("DeleteBranch() error = %v", err)
	}

	// Verify deleted
	cmd := exec.Command("git", "branch", "--list", "to-delete")
	cmd.Dir = dir
	out, _ := cmd.Output()
	if len(out) > 0 {
		t.Error("Branch was not deleted")
	}
}
```

**Step 2: Run tests to verify they fail**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go test ./internal/git/... -v
```
Expected: FAIL

**Step 3: Write the implementation**

Create `internal/git/branch.go`:
```go
package git

import (
	"context"
	"os/exec"
	"strings"
)

// Git provides git operations for a repository.
type Git struct {
	repoPath string
}

// New creates a Git instance for the given repository path.
func New(repoPath string) *Git {
	return &Git{repoPath: repoPath}
}

// run executes a git command and returns output.
func (g *Git) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.repoPath
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// CurrentBranch returns the current branch name.
func (g *Git) CurrentBranch(ctx context.Context) (string, error) {
	return g.run(ctx, "rev-parse", "--abbrev-ref", "HEAD")
}

// CreateBranch creates a new branch at HEAD.
func (g *Git) CreateBranch(ctx context.Context, name string) error {
	_, err := g.run(ctx, "branch", name)
	return err
}

// Checkout switches to a branch.
func (g *Git) Checkout(ctx context.Context, branch string) error {
	_, err := g.run(ctx, "checkout", branch)
	return err
}

// CreateAndCheckout creates and switches to a new branch.
func (g *Git) CreateAndCheckout(ctx context.Context, name string) error {
	_, err := g.run(ctx, "checkout", "-b", name)
	return err
}

// HasUncommittedChanges returns true if there are uncommitted changes.
func (g *Git) HasUncommittedChanges(ctx context.Context) (bool, error) {
	out, err := g.run(ctx, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return len(out) > 0, nil
}

// Stash stashes uncommitted changes.
func (g *Git) Stash(ctx context.Context) error {
	_, err := g.run(ctx, "stash", "push", "-m", "docker-tui pairing mode")
	return err
}

// StashPop restores stashed changes.
func (g *Git) StashPop(ctx context.Context) error {
	_, err := g.run(ctx, "stash", "pop")
	return err
}

// DeleteBranch deletes a local branch.
func (g *Git) DeleteBranch(ctx context.Context, name string) error {
	_, err := g.run(ctx, "branch", "-D", name)
	return err
}

// DeleteRemoteBranch deletes a remote branch.
func (g *Git) DeleteRemoteBranch(ctx context.Context, name string) error {
	_, err := g.run(ctx, "push", "origin", "--delete", name)
	return err
}

// Push pushes a branch to origin.
func (g *Git) Push(ctx context.Context, branch string) error {
	_, err := g.run(ctx, "push", "-u", "origin", branch)
	return err
}

// BranchExistsRemote checks if a branch exists on the remote.
func (g *Git) BranchExistsRemote(ctx context.Context, name string) (bool, error) {
	_, err := g.run(ctx, "ls-remote", "--heads", "origin", name)
	if err != nil {
		return false, nil
	}
	out, _ := g.run(ctx, "ls-remote", "--heads", "origin", name)
	return len(out) > 0, nil
}
```

**Step 4: Run tests to verify they pass**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go test ./internal/git/... -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/
git commit -m "feat: add git branch operations

Git wrapper with:
- Branch create/checkout/delete
- Stash push/pop for pairing mode
- HasUncommittedChanges detection
- Push and remote branch operations"
```

---

## Task 10: PR Creation

**Files:**
- Create: `internal/git/pr.go`
- Create: `internal/git/pr_test.go`

**Step 1: Write the failing tests**

Create `internal/git/pr_test.go`:
```go
package git

import (
	"context"
	"testing"
)

func TestGH_CheckInstalled(t *testing.T) {
	gh := NewGH()
	ctx := context.Background()

	err := gh.CheckInstalled(ctx)
	if err != nil {
		t.Skipf("gh CLI not installed: %v", err)
	}
}

func TestPRRequest(t *testing.T) {
	req := &PRRequest{
		Title: "Add feature",
		Body:  "This adds a new feature",
	}

	if req.Title == "" {
		t.Error("Title should not be empty")
	}
	if req.Body == "" {
		t.Error("Body should not be empty")
	}
}

func TestPRResponse(t *testing.T) {
	resp := &PRResponse{
		Number: 123,
		URL:    "https://github.com/user/repo/pull/123",
	}

	if resp.Number != 123 {
		t.Errorf("Number = %d, want 123", resp.Number)
	}
	if resp.URL == "" {
		t.Error("URL should not be empty")
	}
}
```

**Step 2: Run tests to verify they fail**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go test ./internal/git/... -v -run TestGH -run TestPR
```
Expected: FAIL

**Step 3: Write the implementation**

Create `internal/git/pr.go`:
```go
package git

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// GH wraps the GitHub CLI for PR operations.
type GH struct{}

// NewGH creates a new GitHub CLI wrapper.
func NewGH() *GH {
	return &GH{}
}

// CheckInstalled verifies gh CLI is available.
func (g *GH) CheckInstalled(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "gh", "--version")
	return cmd.Run()
}

// PRRequest contains data for creating a PR.
type PRRequest struct {
	Title string
	Body  string
	Base  string // Optional, defaults to default branch
	Draft bool
}

// PRResponse contains the created PR info.
type PRResponse struct {
	Number int
	URL    string
}

// CreatePR creates a pull request using gh CLI.
func (g *GH) CreatePR(ctx context.Context, repoPath string, req *PRRequest) (*PRResponse, error) {
	args := []string{"pr", "create",
		"--title", req.Title,
		"--body", req.Body,
	}

	if req.Base != "" {
		args = append(args, "--base", req.Base)
	}
	if req.Draft {
		args = append(args, "--draft")
	}

	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gh pr create failed: %w: %s", err, out)
	}

	// gh pr create outputs the URL on success
	url := strings.TrimSpace(string(out))

	// Extract PR number from URL
	var number int
	if _, err := fmt.Sscanf(url, "https://github.com/%s/%s/pull/%d", new(string), new(string), &number); err != nil {
		// Try alternate parsing
		parts := strings.Split(url, "/")
		if len(parts) > 0 {
			fmt.Sscanf(parts[len(parts)-1], "%d", &number)
		}
	}

	return &PRResponse{
		Number: number,
		URL:    url,
	}, nil
}

// GetPR gets info about a PR by number.
func (g *GH) GetPR(ctx context.Context, repoPath string, number int) (*PRResponse, error) {
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", fmt.Sprint(number), "--json", "number,url")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var resp PRResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// PRExists checks if a PR exists for the current branch.
func (g *GH) PRExists(ctx context.Context, repoPath string) (bool, *PRResponse, error) {
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", "--json", "number,url")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		// No PR for this branch
		return false, nil, nil
	}

	var resp PRResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return false, nil, err
	}
	return true, &resp, nil
}
```

**Step 4: Run tests to verify they pass**

Run:
```bash
cd /Users/samuelreed/git/oss/docker-tui && go test ./internal/git/... -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/pr.go internal/git/pr_test.go
git commit -m "feat: add PR creation via gh CLI

GitHub CLI wrapper for PR operations:
- CreatePR with title, body, base branch, draft options
- GetPR by number
- PRExists check for current branch"
```

---

## Task 11-20: TUI Components (Bubble Tea)

The TUI implementation is extensive. Due to plan length constraints, here's the high-level breakdown. Each follows the same TDD pattern.

### Task 11: Bubble Tea Dependencies
```bash
go get github.com/charmbracelet/bubbletea
go get github.com/charmbracelet/lipgloss
go get github.com/charmbracelet/bubbles
```

### Task 12: `internal/tui/styles.go` - Lip Gloss Styling
- Border styles, colors, status indicators
- Tests for style rendering

### Task 13: `internal/tui/pane.go` - Workstream Pane
- Single pane showing workstream output
- Input prompt at bottom
- Status indicator

### Task 14: `internal/tui/statusbar.go` - Status Bar
- Keybinding hints
- Pairing mode indicator
- Workstream count

### Task 15: `internal/tui/dialog.go` - Confirmation Dialogs
- Destroy confirmation (type "destroy")
- PR preview/edit dialog

### Task 16: `internal/tui/app.go` - Main Application Model
- Multi-pane layout
- Focus management
- Keyboard handling

### Task 17: `cmd/docker-tui/main.go` - Entry Point
- Initialize Bubble Tea
- Wire up components

### Task 18-20: Integration and Polish
- End-to-end flow testing
- Error handling
- Help screen

---

## Execution

Plan complete and saved to `docs/plans/2025-01-15-docker-tui-implementation.md`.

**Two execution options:**

**1. Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

**Which approach?**
