# Extract Orchestrator Layer Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extract container orchestration business logic from `internal/tui/container.go` into a new `internal/orchestrator` package, enabling testable, reusable workstream lifecycle management.

**Architecture:** Create a new `internal/orchestrator` package that owns all workstream lifecycle operations (create, pause, resume, destroy). The TUI layer becomes a thin wrapper that converts orchestrator results into Bubble Tea messages. This separates concerns: orchestrator handles "what happens", TUI handles "how it looks".

**Tech Stack:** Go, existing Docker/Git/Workstream packages, table-driven tests

---

## Background

### Current Problem

`internal/tui/container.go` (1653 lines) contains business logic that should be in a domain layer:
- `startContainerWithFullOptions()` - 254 lines of git + docker orchestration
- `RebuildContainerCmd()`, `pauseContainerCmd()`, `resumeContainerCmd()`, `destroyContainerCmd()`
- All return `tea.Cmd` / `tea.Msg`, coupling business logic to Bubble Tea

### Target State

```
internal/orchestrator/
  orchestrator.go      # WorkstreamOrchestrator struct + interface
  create.go            # CreateWorkstream() - worktree + container + PTY
  lifecycle.go         # Pause(), Resume(), Destroy(), Rebuild()
  orchestrator_test.go # Unit tests with mock Docker/Git

internal/tui/container.go  # Thin wrappers returning tea.Cmd
```

### Key Files to Reference

- `internal/tui/container.go` - Current implementation (extract from here)
- `internal/docker/interface.go` - DockerClient interface
- `internal/git/interface.go` - GitClient interface
- `internal/workstream/workstream.go` - Workstream domain model
- `internal/docker/mock_client.go` - Existing mock pattern to follow

---

## Task 1: Create Orchestrator Interface and Struct

**Files:**
- Create: `internal/orchestrator/orchestrator.go`
- Test: `internal/orchestrator/orchestrator_test.go`

**Step 1: Write the interface and struct skeleton**

```go
// internal/orchestrator/orchestrator.go
package orchestrator

import (
	"context"

	"github.com/anthropics/claude-cells/internal/docker"
	"github.com/anthropics/claude-cells/internal/git"
	"github.com/anthropics/claude-cells/internal/workstream"
)

// WorkstreamOrchestrator manages workstream lifecycle operations.
// It coordinates between Docker, Git, and Workstream packages.
type WorkstreamOrchestrator interface {
	// CreateWorkstream creates a new workstream with container and worktree.
	// Returns the container ID on success.
	CreateWorkstream(ctx context.Context, ws *workstream.Workstream, opts CreateOptions) (string, error)

	// PauseWorkstream pauses a running workstream's container.
	PauseWorkstream(ctx context.Context, ws *workstream.Workstream) error

	// ResumeWorkstream resumes a paused workstream's container.
	ResumeWorkstream(ctx context.Context, ws *workstream.Workstream) error

	// DestroyWorkstream removes container, worktree, and cleans up state.
	DestroyWorkstream(ctx context.Context, ws *workstream.Workstream, opts DestroyOptions) error

	// RebuildWorkstream destroys and recreates the container.
	RebuildWorkstream(ctx context.Context, ws *workstream.Workstream, opts CreateOptions) (string, error)
}

// CreateOptions configures workstream creation.
type CreateOptions struct {
	RepoPath        string
	CopyUntracked   bool
	UntrackedFiles  []string
	ImageName       string // Empty = auto-detect
	IsResume        bool   // Resuming existing session
}

// DestroyOptions configures workstream destruction.
type DestroyOptions struct {
	DeleteBranch    bool
	KeepWorktree    bool
	Force           bool // Force even if container is running
}

// Orchestrator implements WorkstreamOrchestrator.
type Orchestrator struct {
	dockerClient docker.DockerClient
	gitFactory   func(repoPath string) git.GitClient
	repoPath     string
}

// New creates a new Orchestrator.
func New(dockerClient docker.DockerClient, gitFactory func(string) git.GitClient, repoPath string) *Orchestrator {
	return &Orchestrator{
		dockerClient: dockerClient,
		gitFactory:   gitFactory,
		repoPath:     repoPath,
	}
}
```

**Step 2: Write a basic instantiation test**

```go
// internal/orchestrator/orchestrator_test.go
package orchestrator

import (
	"testing"

	"github.com/anthropics/claude-cells/internal/docker"
	"github.com/anthropics/claude-cells/internal/git"
)

func TestNew(t *testing.T) {
	mockDocker := docker.NewMockClient()
	gitFactory := func(path string) git.GitClient {
		return git.NewMockClient()
	}

	orch := New(mockDocker, gitFactory, "/test/repo")

	if orch == nil {
		t.Fatal("expected non-nil orchestrator")
	}
	if orch.repoPath != "/test/repo" {
		t.Errorf("expected repoPath /test/repo, got %s", orch.repoPath)
	}
}
```

**Step 3: Run test to verify it passes**

Run: `go test -v ./internal/orchestrator/...`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/orchestrator/
git commit -m "feat(orchestrator): add orchestrator interface and struct skeleton

Introduces WorkstreamOrchestrator interface for managing workstream
lifecycle operations. This is the foundation for extracting business
logic from internal/tui/container.go."
```

---

## Task 2: Extract Worktree Creation Logic

**Files:**
- Modify: `internal/orchestrator/orchestrator.go`
- Create: `internal/orchestrator/create.go`
- Test: `internal/orchestrator/orchestrator_test.go`

**Step 1: Write test for worktree creation**

```go
// Add to internal/orchestrator/orchestrator_test.go

func TestCreateWorkstream_CreatesWorktree(t *testing.T) {
	mockDocker := docker.NewMockClient()
	mockGit := git.NewMockClient()
	gitFactory := func(path string) git.GitClient {
		return mockGit
	}

	orch := New(mockDocker, gitFactory, "/test/repo")

	ws := &workstream.Workstream{
		ID:         "test-id",
		BranchName: "ccells/test-branch",
	}

	opts := CreateOptions{
		RepoPath: "/test/repo",
	}

	ctx := context.Background()
	_, err := orch.CreateWorkstream(ctx, ws, opts)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify worktree was created
	if !mockGit.WorktreeCreated {
		t.Error("expected worktree to be created")
	}

	expectedPath := "/tmp/ccells/worktrees/ccells/test-branch"
	if mockGit.LastWorktreePath != expectedPath {
		t.Errorf("expected worktree path %s, got %s", expectedPath, mockGit.LastWorktreePath)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./internal/orchestrator/... -run TestCreateWorkstream_CreatesWorktree`
Expected: FAIL (method not implemented)

**Step 3: Implement worktree creation in create.go**

```go
// internal/orchestrator/create.go
package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anthropics/claude-cells/internal/workstream"
)

const worktreeBaseDir = "/tmp/ccells/worktrees"

// CreateWorkstream creates a new workstream with container and worktree.
func (o *Orchestrator) CreateWorkstream(ctx context.Context, ws *workstream.Workstream, opts CreateOptions) (string, error) {
	// Step 1: Create git worktree
	worktreePath, err := o.createWorktree(ctx, ws.BranchName)
	if err != nil {
		return "", fmt.Errorf("create worktree: %w", err)
	}
	ws.WorktreePath = worktreePath

	// TODO: Steps 2-4 will be added in subsequent tasks
	// Step 2: Copy untracked files (if requested)
	// Step 3: Create container
	// Step 4: Start container

	return "", nil // Container ID will be returned once implemented
}

func (o *Orchestrator) createWorktree(ctx context.Context, branchName string) (string, error) {
	// Ensure base directory exists
	if err := os.MkdirAll(worktreeBaseDir, 0755); err != nil {
		return "", fmt.Errorf("create worktree base dir: %w", err)
	}

	worktreePath := filepath.Join(worktreeBaseDir, branchName)

	gitClient := o.gitFactory(o.repoPath)
	if err := gitClient.CreateWorktree(ctx, worktreePath, branchName); err != nil {
		return "", fmt.Errorf("git create worktree: %w", err)
	}

	return worktreePath, nil
}

// cleanupWorktree removes a worktree on error.
func (o *Orchestrator) cleanupWorktree(ctx context.Context, branchName string) {
	worktreePath := filepath.Join(worktreeBaseDir, branchName)
	gitClient := o.gitFactory(o.repoPath)
	_ = gitClient.RemoveWorktree(ctx, worktreePath)
}
```

**Step 4: Update git mock to track worktree creation**

Check if `internal/git/mock_client.go` already tracks this. If not, add:

```go
// Add fields to MockClient in internal/git/mock_client.go
type MockClient struct {
	// ... existing fields
	WorktreeCreated   bool
	LastWorktreePath  string
	LastWorktreeBranch string
}

func (m *MockClient) CreateWorktree(ctx context.Context, path, branch string) error {
	m.WorktreeCreated = true
	m.LastWorktreePath = path
	m.LastWorktreeBranch = branch
	return nil
}
```

**Step 5: Run test to verify it passes**

Run: `go test -v ./internal/orchestrator/... -run TestCreateWorkstream_CreatesWorktree`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/orchestrator/ internal/git/mock_client.go
git commit -m "feat(orchestrator): implement worktree creation

CreateWorkstream now creates isolated git worktrees for each
workstream at /tmp/ccells/worktrees/<branch>."
```

---

## Task 3: Extract Container Creation Logic

**Files:**
- Modify: `internal/orchestrator/create.go`
- Test: `internal/orchestrator/orchestrator_test.go`
- Reference: `internal/tui/container.go:598-852` (current implementation)

**Step 1: Write test for container creation**

```go
// Add to internal/orchestrator/orchestrator_test.go

func TestCreateWorkstream_CreatesContainer(t *testing.T) {
	mockDocker := docker.NewMockClient()
	mockGit := git.NewMockClient()
	gitFactory := func(path string) git.GitClient {
		return mockGit
	}

	orch := New(mockDocker, gitFactory, "/test/repo")

	ws := &workstream.Workstream{
		ID:         "test-id",
		BranchName: "ccells/test-branch",
	}

	opts := CreateOptions{
		RepoPath:  "/test/repo",
		ImageName: "ccells-test:latest",
	}

	ctx := context.Background()
	containerID, err := orch.CreateWorkstream(ctx, ws, opts)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if containerID == "" {
		t.Error("expected container ID to be returned")
	}

	// Verify container was created and started
	state, err := mockDocker.GetContainerState(ctx, containerID)
	if err != nil {
		t.Fatalf("failed to get container state: %v", err)
	}
	if state != "running" {
		t.Errorf("expected container state 'running', got '%s'", state)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./internal/orchestrator/... -run TestCreateWorkstream_CreatesContainer`
Expected: FAIL (container ID is empty)

**Step 3: Implement container creation**

```go
// Add to internal/orchestrator/create.go

func (o *Orchestrator) CreateWorkstream(ctx context.Context, ws *workstream.Workstream, opts CreateOptions) (string, error) {
	// Step 1: Create git worktree
	worktreePath, err := o.createWorktree(ctx, ws.BranchName)
	if err != nil {
		return "", fmt.Errorf("create worktree: %w", err)
	}
	ws.WorktreePath = worktreePath

	// Step 2: Copy untracked files (if requested)
	if opts.CopyUntracked && len(opts.UntrackedFiles) > 0 {
		if err := o.copyUntrackedFiles(opts.RepoPath, worktreePath, opts.UntrackedFiles); err != nil {
			o.cleanupWorktree(ctx, ws.BranchName)
			return "", fmt.Errorf("copy untracked files: %w", err)
		}
	}

	// Step 3: Create container config
	cfg := o.buildContainerConfig(ws, worktreePath, opts)

	// Step 4: Create and start container
	containerID, err := o.createAndStartContainer(ctx, cfg)
	if err != nil {
		o.cleanupWorktree(ctx, ws.BranchName)
		return "", fmt.Errorf("create container: %w", err)
	}

	ws.ContainerID = containerID
	return containerID, nil
}

func (o *Orchestrator) buildContainerConfig(ws *workstream.Workstream, worktreePath string, opts CreateOptions) *docker.ContainerConfig {
	cfg := docker.NewContainerConfig(ws.BranchName, worktreePath)
	cfg.HostGitDir = filepath.Join(o.repoPath, ".git")

	if opts.ImageName != "" {
		cfg.Image = opts.ImageName
	}

	return cfg
}

func (o *Orchestrator) createAndStartContainer(ctx context.Context, cfg *docker.ContainerConfig) (string, error) {
	containerID, err := o.dockerClient.CreateContainer(ctx, cfg)
	if err != nil {
		return "", fmt.Errorf("docker create: %w", err)
	}

	if err := o.dockerClient.StartContainer(ctx, containerID); err != nil {
		// Cleanup: remove container on start failure
		_ = o.dockerClient.RemoveContainer(ctx, containerID)
		return "", fmt.Errorf("docker start: %w", err)
	}

	return containerID, nil
}

func (o *Orchestrator) copyUntrackedFiles(srcRepo, dstWorktree string, files []string) error {
	for _, file := range files {
		src := filepath.Join(srcRepo, file)
		dst := filepath.Join(dstWorktree, file)

		// Ensure destination directory exists
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}

		// Copy file
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			return err
		}
	}
	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./internal/orchestrator/... -run TestCreateWorkstream_CreatesContainer`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/orchestrator/
git commit -m "feat(orchestrator): implement container creation

CreateWorkstream now creates Docker containers with proper config.
Includes cleanup on failure and untracked file copying."
```

---

## Task 4: Extract Pause/Resume/Destroy Logic

**Files:**
- Create: `internal/orchestrator/lifecycle.go`
- Test: `internal/orchestrator/orchestrator_test.go`
- Reference: `internal/tui/container.go` (pauseContainerCmd, resumeContainerCmd, destroyContainerCmd)

**Step 1: Write tests for lifecycle operations**

```go
// Add to internal/orchestrator/orchestrator_test.go

func TestPauseWorkstream(t *testing.T) {
	mockDocker := docker.NewMockClient()
	orch := New(mockDocker, nil, "/test/repo")

	// Create a "running" container in mock
	ctx := context.Background()
	cfg := &docker.ContainerConfig{Name: "test", Image: "test:latest"}
	containerID, _ := mockDocker.CreateContainer(ctx, cfg)
	_ = mockDocker.StartContainer(ctx, containerID)

	ws := &workstream.Workstream{
		ID:          "test-id",
		ContainerID: containerID,
	}

	err := orch.PauseWorkstream(ctx, ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	state, _ := mockDocker.GetContainerState(ctx, containerID)
	if state != "paused" {
		t.Errorf("expected state 'paused', got '%s'", state)
	}
}

func TestResumeWorkstream(t *testing.T) {
	mockDocker := docker.NewMockClient()
	orch := New(mockDocker, nil, "/test/repo")

	ctx := context.Background()
	cfg := &docker.ContainerConfig{Name: "test", Image: "test:latest"}
	containerID, _ := mockDocker.CreateContainer(ctx, cfg)
	_ = mockDocker.StartContainer(ctx, containerID)
	_ = mockDocker.PauseContainer(ctx, containerID)

	ws := &workstream.Workstream{
		ID:          "test-id",
		ContainerID: containerID,
	}

	err := orch.ResumeWorkstream(ctx, ws)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	state, _ := mockDocker.GetContainerState(ctx, containerID)
	if state != "running" {
		t.Errorf("expected state 'running', got '%s'", state)
	}
}

func TestDestroyWorkstream(t *testing.T) {
	mockDocker := docker.NewMockClient()
	mockGit := git.NewMockClient()
	gitFactory := func(path string) git.GitClient {
		return mockGit
	}
	orch := New(mockDocker, gitFactory, "/test/repo")

	ctx := context.Background()
	cfg := &docker.ContainerConfig{Name: "test", Image: "test:latest"}
	containerID, _ := mockDocker.CreateContainer(ctx, cfg)

	ws := &workstream.Workstream{
		ID:           "test-id",
		ContainerID:  containerID,
		BranchName:   "ccells/test",
		WorktreePath: "/tmp/ccells/worktrees/ccells/test",
	}

	err := orch.DestroyWorkstream(ctx, ws, DestroyOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Container should be removed
	_, err = mockDocker.GetContainerState(ctx, containerID)
	if err == nil {
		t.Error("expected container to be removed")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -v ./internal/orchestrator/... -run "TestPauseWorkstream|TestResumeWorkstream|TestDestroyWorkstream"`
Expected: FAIL (methods not implemented)

**Step 3: Implement lifecycle operations**

```go
// internal/orchestrator/lifecycle.go
package orchestrator

import (
	"context"
	"fmt"

	"github.com/anthropics/claude-cells/internal/workstream"
)

// PauseWorkstream pauses a running workstream's container.
func (o *Orchestrator) PauseWorkstream(ctx context.Context, ws *workstream.Workstream) error {
	if ws.ContainerID == "" {
		return fmt.Errorf("workstream has no container")
	}

	if err := o.dockerClient.PauseContainer(ctx, ws.ContainerID); err != nil {
		return fmt.Errorf("pause container: %w", err)
	}

	return nil
}

// ResumeWorkstream resumes a paused workstream's container.
func (o *Orchestrator) ResumeWorkstream(ctx context.Context, ws *workstream.Workstream) error {
	if ws.ContainerID == "" {
		return fmt.Errorf("workstream has no container")
	}

	if err := o.dockerClient.UnpauseContainer(ctx, ws.ContainerID); err != nil {
		return fmt.Errorf("resume container: %w", err)
	}

	return nil
}

// DestroyWorkstream removes container, worktree, and optionally the branch.
func (o *Orchestrator) DestroyWorkstream(ctx context.Context, ws *workstream.Workstream, opts DestroyOptions) error {
	var errs []error

	// Step 1: Stop and remove container
	if ws.ContainerID != "" {
		if err := o.dockerClient.StopContainer(ctx, ws.ContainerID); err != nil {
			// Container might already be stopped, continue
		}
		if err := o.dockerClient.RemoveContainer(ctx, ws.ContainerID); err != nil {
			errs = append(errs, fmt.Errorf("remove container: %w", err))
		}
	}

	// Step 2: Remove worktree (unless KeepWorktree is set)
	if !opts.KeepWorktree && ws.WorktreePath != "" {
		gitClient := o.gitFactory(o.repoPath)
		if err := gitClient.RemoveWorktree(ctx, ws.WorktreePath); err != nil {
			errs = append(errs, fmt.Errorf("remove worktree: %w", err))
		}
	}

	// Step 3: Delete branch (if requested)
	if opts.DeleteBranch && ws.BranchName != "" {
		gitClient := o.gitFactory(o.repoPath)
		if err := gitClient.DeleteBranch(ctx, ws.BranchName); err != nil {
			errs = append(errs, fmt.Errorf("delete branch: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("destroy had errors: %v", errs)
	}
	return nil
}

// RebuildWorkstream destroys and recreates the container with fresh state.
func (o *Orchestrator) RebuildWorkstream(ctx context.Context, ws *workstream.Workstream, opts CreateOptions) (string, error) {
	// Save branch and worktree info
	branchName := ws.BranchName
	worktreePath := ws.WorktreePath

	// Destroy container only (keep worktree and branch)
	destroyOpts := DestroyOptions{
		KeepWorktree: true,
		DeleteBranch: false,
	}
	if err := o.DestroyWorkstream(ctx, ws, destroyOpts); err != nil {
		return "", fmt.Errorf("destroy for rebuild: %w", err)
	}

	// Recreate with existing worktree
	ws.WorktreePath = worktreePath
	ws.BranchName = branchName

	cfg := o.buildContainerConfig(ws, worktreePath, opts)
	containerID, err := o.createAndStartContainer(ctx, cfg)
	if err != nil {
		return "", fmt.Errorf("recreate container: %w", err)
	}

	ws.ContainerID = containerID
	return containerID, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -v ./internal/orchestrator/...`
Expected: All PASS

**Step 5: Commit**

```bash
git add internal/orchestrator/
git commit -m "feat(orchestrator): implement pause, resume, destroy, rebuild

Complete lifecycle management for workstreams. Destroy cleans up
container, worktree, and optionally the git branch."
```

---

## Task 5: Wire Orchestrator into TUI (Thin Wrappers)

**Files:**
- Modify: `internal/tui/container.go`
- Modify: `internal/tui/app.go` (add orchestrator field)

**Step 1: Add orchestrator to AppModel**

```go
// In internal/tui/app.go, add to AppModel struct:
type AppModel struct {
    // ... existing fields
    orchestrator *orchestrator.Orchestrator
}

// In NewAppModel() or initialization:
func NewAppModel(ctx context.Context) AppModel {
    // ... existing code
    dockerClient, _ := docker.NewClient()
    orch := orchestrator.New(dockerClient, git.New, workingDir)

    return AppModel{
        // ... existing fields
        orchestrator: orch,
    }
}
```

**Step 2: Create thin wrapper in container.go**

```go
// Replace startContainerWithFullOptions with a thin wrapper:

func startContainerCmd(m *AppModel, ws *workstream.Workstream, opts orchestrator.CreateOptions) tea.Cmd {
    return func() tea.Msg {
        ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
        defer cancel()

        containerID, err := m.orchestrator.CreateWorkstream(ctx, ws, opts)
        if err != nil {
            return ContainerErrorMsg{
                WorkstreamID: ws.ID,
                Error:        err,
            }
        }

        return ContainerStartedMsg{
            WorkstreamID: ws.ID,
            ContainerID:  containerID,
            IsResume:     opts.IsResume,
        }
    }
}

// Similar thin wrappers for pause, resume, destroy
func pauseContainerCmd(m *AppModel, ws *workstream.Workstream) tea.Cmd {
    return func() tea.Msg {
        ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
        defer cancel()

        if err := m.orchestrator.PauseWorkstream(ctx, ws); err != nil {
            return ContainerPausedMsg{WorkstreamID: ws.ID, Error: err}
        }
        return ContainerPausedMsg{WorkstreamID: ws.ID}
    }
}
```

**Step 3: Run existing tests**

Run: `go test -v ./internal/tui/...`
Expected: PASS (behavior unchanged, just refactored)

**Step 4: Commit**

```bash
git add internal/tui/ internal/orchestrator/
git commit -m "refactor(tui): wire orchestrator, create thin command wrappers

TUI container.go now delegates to orchestrator for all business logic.
Commands are thin wrappers that convert results to tea.Msg."
```

---

## Task 6: Remove Old Business Logic from container.go

**Files:**
- Modify: `internal/tui/container.go` (delete extracted code)

**Step 1: Identify and remove extracted functions**

Delete these functions from `internal/tui/container.go` (they now live in orchestrator):
- `startContainerWithFullOptions()` (254 lines)
- `createWorktree()` helper
- `cleanupWorktree()` helper
- `copyUntrackedFiles()` helper
- Any other helpers that were moved

**Step 2: Verify no dead code remains**

Run: `go build ./...`
Expected: No unused function errors

**Step 3: Run full test suite**

Run: `go test -race ./...`
Expected: All PASS

**Step 4: Commit**

```bash
git add internal/tui/container.go
git commit -m "refactor(tui): remove extracted business logic from container.go

container.go is now ~500 lines (down from 1653), containing only
Bubble Tea command wrappers and message handling."
```

---

## Task 7: Update Codemaps and Documentation

**Files:**
- Modify: `codemaps/architecture.md`
- Modify: `codemaps/backend.md`
- Modify: `CLAUDE.md`

**Step 1: Update architecture.md**

Add `internal/orchestrator` to the layer diagram:
```
┌─────────────────────────────────────────────────────────────┐
│ Presentation Layer (internal/tui)                          │
│ - Thin wrappers, no business logic                         │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Orchestration Layer (internal/orchestrator)         [NEW]  │
│ - WorkstreamOrchestrator: create, pause, resume, destroy   │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Domain Layer (internal/workstream)                         │
└─────────────────────────────────────────────────────────────┘
```

Mark architectural smell as resolved.

**Step 2: Update backend.md**

Add new package documentation:
```markdown
## `internal/orchestrator/`

Workstream lifecycle orchestration (extracted from tui/container.go).

### Key Types

| Type | File | Description |
|------|------|-------------|
| `WorkstreamOrchestrator` | orchestrator.go | Interface for lifecycle ops |
| `Orchestrator` | orchestrator.go | Implementation |
| `CreateOptions` | orchestrator.go | Creation config |
| `DestroyOptions` | orchestrator.go | Destruction config |

### Key Functions

- `CreateWorkstream()` - Worktree + container + PTY setup
- `PauseWorkstream()` - Pause container
- `ResumeWorkstream()` - Resume container
- `DestroyWorkstream()` - Cleanup container, worktree, branch
- `RebuildWorkstream()` - Destroy and recreate
```

**Step 3: Update CLAUDE.md architecture section**

Add orchestrator to the package list and note the refactoring.

**Step 4: Commit**

```bash
git add codemaps/ CLAUDE.md
git commit -m "docs: update codemaps for orchestrator extraction

- Added internal/orchestrator package documentation
- Updated layer diagrams
- Marked 'business logic in TUI' smell as resolved"
```

---

## Verification Checklist

After completing all tasks:

- [ ] `go test -race ./...` passes
- [ ] `go vet ./...` clean
- [ ] `go build ./cmd/ccells` succeeds
- [ ] Manual test: create workstream, pause, resume, destroy
- [ ] `internal/tui/container.go` is < 600 lines
- [ ] `internal/orchestrator/` has > 80% test coverage
- [ ] Codemaps updated

---

## Rollback Plan

If issues arise:
1. All changes are in separate commits
2. Each task is independently revertable
3. Old code in `container.go` can be restored from git history
4. Orchestrator package can be deleted without affecting other code

---

## Future Work (Out of Scope)

- Extract PaneModel into smaller components (separate plan)
- Eliminate global state (separate plan)
- Add observability/metrics (separate plan)
