# Claude Sneakpeek Runtime Support

**Date:** 2026-01-25
**Status:** Approved

## Overview

Add support for running [claude-sneakpeek](https://github.com/mikekelly/claude-sneakpeek) as an alternative runtime to standard Claude Code. Claude-sneakpeek is an experimental build that unlocks feature-flagged capabilities like swarm mode, delegate mode, and team coordination.

## Design Decisions

- **Runtime applies session-wide**: `ccells --runtime claudesp` means ALL workstreams in that session use the experimental build
- **Isolated installation per container**: Each container gets its own claude-sneakpeek installation during image build (handles cross-platform compatibility)
- **Isolated config directory**: Similar to `.claude`, each container gets its own `.claude-sneakpeek` directory with fresh credentials
- **Config file + CLI override**: Default runtime set in `~/.claude-cells/config.yaml`, but `--runtime` flag can override per-session
- **Binary aliasing**: Setup script aliases `claudesp` → `claude` so PTY commands remain unchanged

## Architecture

### Configuration Schema

**`~/.claude-cells/config.yaml` (global):**
```yaml
runtime: claudesp  # Default runtime: "claude" (default) or "claudesp"
dockerfile:
  inject: [...]
security:
  tier: moderate
```

**`.claude-cells/config.yaml` (project-level):**
```yaml
runtime: claudesp  # Overrides global default for this project
```

**Runtime selection priority** (highest to lowest):
1. CLI flag: `--runtime claudesp`
2. Project config: `.claude-cells/config.yaml`
3. Global config: `~/.claude-cells/config.yaml`
4. Default: `"claude"` (standard Claude Code)

### Container Configuration

**Pattern (same as `.claude`):**
- **Host original**: `~/.claude-sneakpeek` (created by user if they've run it before, or empty)
- **Per-container copy**: `~/.claude-cells/containers/<name>/.claude-sneakpeek`
- **Container mount**: `/root/.claude-sneakpeek`

**Implementation in `CreateContainerConfig()` (internal/docker/config.go):**
```go
// If runtime is claudesp, also copy .claude-sneakpeek directory
if runtime == "claudesp" {
    srcSneakpeekDir := filepath.Join(home, ".claude-sneakpeek")
    dstSneakpeekDir := filepath.Join(containerConfigDir, ".claude-sneakpeek")

    if _, err := os.Stat(srcSneakpeekDir); err == nil {
        // Copy existing config (selective, like .claude)
        copyClaudeDirSelective(srcSneakpeekDir, dstSneakpeekDir)
    } else {
        // Create empty directory (will be populated by install)
        os.MkdirAll(dstSneakpeekDir, 0755)
    }

    // Copy credentials into .claude-sneakpeek/.credentials.json
    // (same pattern as .claude credentials)
}
```

**Mounts in `CreateContainer()` (internal/docker/container.go):**
```go
if runtime == "claudesp" {
    mounts = append(mounts, mount.Mount{
        Type:     mount.TypeBind,
        Source:   configPaths.SneakpeekDir,
        Target:   "/root/.claude-sneakpeek",
        ReadOnly: false,
    })
}
```

### Dockerfile Changes

**`configs/base.Dockerfile`:**
```dockerfile
# Install Claude Code (existing)
RUN curl -fsSL https://claude.ai/install.sh | bash

# Install claude-sneakpeek (experimental build with swarm mode)
RUN npx @realmikekelly/claude-sneakpeek quick --name claudesp
```

### PTY Runtime Selection

**Binary aliasing in `containerSetupScript` (internal/tui/pty.go):**

Add at beginning of setup script:
```bash
# Select runtime based on CLAUDE_RUNTIME env var
if [ "$CLAUDE_RUNTIME" = "claudesp" ]; then
  # Use claudesp binary (experimental with swarm mode)
  CLAUDE_BIN="claudesp"
else
  # Default to standard Claude Code
  CLAUDE_BIN="claude"
fi

# Ensure the selected binary is available as 'claude' for the rest of the script
mkdir -p "$HOME/.local/bin" 2>/dev/null
ln -sf "$(which $CLAUDE_BIN)" "$HOME/.local/bin/claude" 2>/dev/null || true
export PATH="$HOME/.local/bin:$PATH"
```

**Environment variable in `NewPTYSession()`:**
```go
type PTYOptions struct {
    Width           int
    Height          int
    EnvVars         []string
    IsResume        bool
    ClaudeSessionID string
    HostProjectPath string
    Runtime         string  // NEW: "claude" or "claudesp"
}

// In env setup:
if opts != nil && opts.Runtime == "claudesp" {
    env = append(env, "CLAUDE_RUNTIME=claudesp")
}
```

### Data Flow

1. **Startup** (`cmd/ccells/main.go`):
   - Parse `--runtime` flag
   - Load config with flag override
   - Pass runtime to `AppModel`

2. **AppModel** (`internal/tui/app.go`):
   - Store runtime choice
   - Pass to orchestrator for container creation

3. **Container Creation** (`internal/orchestrator/create.go`):
   - Create config with runtime: `CreateContainerConfig(containerName, runtime)`
   - Pass runtime to PTY options

4. **Container Setup** (inside container):
   - `CLAUDE_RUNTIME` env var → alias `claudesp`→`claude` → exec `claude`

### State Persistence

Runtime choice saved in `state.json` so resumed sessions use the same runtime:

```go
type WorkstreamState struct {
    // ... existing fields ...
    Runtime string `json:"runtime,omitempty"`
}
```

## Testing Strategy

### Unit Tests
- Config loading with runtime field (global, project, flag priority)
- `CreateContainerConfig()` with runtime="claudesp" creates sneakpeek directory
- PTYOptions runtime propagation

### Integration Tests
- Build image with both claude and claudesp installed
- Start container with `CLAUDE_RUNTIME=claudesp`, verify binary aliasing works
- Test config copying for both `.claude` and `.claude-sneakpeek`

### Manual Verification
- `ccells --runtime claudesp` launches experimental build
- Swarm mode features work (TeammateTool, delegate mode)
- Credentials isolated per-container
- Resume sessions preserve runtime choice

## Edge Cases

1. **claudesp not installed in container:**
   - Setup script fails gracefully with clear error
   - Add check: `which $CLAUDE_BIN || { echo "Runtime $CLAUDE_RUNTIME not found"; exit 1; }`

2. **Missing host .claude-sneakpeek:**
   - `CreateContainerConfig()` creates empty directory if missing
   - Container install populates it on first run

3. **Mixing runtimes across sessions:**
   - Not supported (runtime is session-wide)
   - If user tries to resume with different runtime, show warning

4. **Invalid runtime value:**
   - Validate in config loading: only "claude" or "claudesp" allowed
   - Default to "claude" if invalid

## Container Context

Each container's CLAUDE.md should include runtime information so Claude knows its execution environment:

```go
// In CreateContainerConfig(), when writing CLAUDE.md:
claudeMdContent := fmt.Sprintf(`# Claude Cells Session

**Runtime:** %s
%s

You are in an isolated container with a dedicated git worktree...
`,
    runtimeDescription(runtime),
    runtimeFeatures(runtime),
    // ... rest of CCellsInstructions
)
```

**Runtime descriptions:**
- `claude`: "Standard Claude Code"
- `claudesp`: "Claude Sneakpeek (experimental build with swarm mode, delegate mode, and team coordination)"

**Runtime-specific features section:**
```markdown
## Available Features (claudesp runtime)

This is an experimental build with unreleased features:

- **Swarm Mode**: Multi-agent orchestration via TeammateTool
- **Delegate Mode**: Spawn background agents for parallel tasks
- **Team Coordination**: Teammate messaging and task ownership

See https://github.com/mikekelly/claude-sneakpeek for details.
```

## Implementation Checklist

- [ ] Add `Runtime string` field to Config struct (internal/docker/config.go)
- [ ] Modify LoadConfig() to support runtime field with priority chain
- [ ] Update CreateContainerConfig() to copy .claude-sneakpeek directory
- [ ] Add .claude-sneakpeek mount to CreateContainer()
- [ ] Update base.Dockerfile to install claudesp
- [ ] Add runtime aliasing to containerSetupScript
- [ ] Add Runtime field to PTYOptions
- [ ] Pass runtime via CLAUDE_RUNTIME env var in NewPTYSession()
- [ ] Add runtime to WorkstreamState for persistence
- [ ] Add --runtime flag to main.go
- [ ] Update CCellsInstructions to include runtime context
- [ ] Write unit tests for config loading
- [ ] Write integration test for runtime selection
- [ ] Update README.md with --runtime flag documentation
- [ ] Update CLAUDE.md with runtime configuration
