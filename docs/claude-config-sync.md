# Claude Code Configuration Sync

This document describes how Claude Code stores its configuration and how ccells syncs it to containers.

## Claude Code Configuration Files

Claude Code uses several files/directories for configuration:

### `~/.claude/` Directory
The main configuration directory containing:

| File/Dir | Purpose | Sync Required |
|----------|---------|---------------|
| `settings.json` | User settings (model, permissions, plugins, sandbox config) | **YES - Critical** |
| `.credentials.json` | OAuth tokens (access_token, refresh_token, expiresAt) - **NOTE: leading dot!** | **YES - Critical** |
| `CLAUDE.md` | User's global instructions | YES |
| `plugins/` | Installed plugins | YES |
| `commands/` | Custom slash commands (status line, etc.) | **YES - Critical for customizations** |
| `agents/` | Custom agents | **YES - Critical for customizations** |
| `history.jsonl` | Command history | Optional |
| `projects/` | Project-specific state | Optional |
| `debug/` | Debug logs | NO (skip) |
| `file-history/` | File edit history | NO (can skip) |
| `cache/` | Temporary cache | NO (skip) |

### `~/.claude.json` File
Session state and user preferences:

```json
{
  "userID": "...",          // User identifier
  "numStartups": N,         // Startup count
  "projects": { ... },      // Project-specific settings
  "cachedGrowthBookFeatures": { ... }  // Feature flags
}
```

**Note**: This file does NOT contain authentication. It contains session state.

## Authentication Flow

### macOS Keychain
On macOS, OAuth credentials are stored in the system keychain:
- Service: `Claude Code-credentials`
- Format: JSON with `claudeAiOauth` object containing tokens

### Credentials JSON Format
```json
{
  "claudeAiOauth": {
    "accessToken": "sk-ant-oat01-...",
    "refreshToken": "sk-ant-ort01-...",
    "expiresAt": 1768784418385,
    "scopes": ["user:inference", "user:profile", "user:sessions:claude_code"],
    "subscriptionType": "max",
    "rateLimitTier": "default_claude_max_20x"
  }
}
```

## ccells Sync Strategy

### Container Mounts
Containers run as root with `IS_SANDBOX=1` (which allows `--dangerously-skip-permissions`).
Configuration is mounted directly at `/root/`:

| Host Path | Container Path | Mode |
|-----------|----------------|------|
| `~/.claude-cells/containers/<name>/.claude` | `/root/.claude` | RW |
| `~/.claude-cells/containers/<name>/.claude.json` | `/root/.claude.json` | RW |
| `~/.claude-cells/containers/<name>/.gitconfig` | `/root/.gitconfig` | RO |

### PTY Startup
The PTY startup script creates ccells-specific commands in the mounted config directory:
- `commands/ccells-commit.md` - The `/ccells-commit` skill for committing changes

**IMPORTANT:** On Linux, Claude Code expects `.credentials.json` (with a leading dot), not `credentials.json`.

## Common Issues

### 1. "Invalid API key" Error
**Cause**: Credentials not synced or in wrong format
**Fix**:
- Verify `~/.claude-cells/containers/<name>/.claude/.credentials.json` exists and has valid tokens (note the leading dot!)
- Check token hasn't expired (`expiresAt` timestamp)

### 2. Wrong Model (e.g., "Sonnet 4.5" instead of configured model)
**Cause**: `settings.json` not synced or not mounted correctly
**Fix**:
- Verify `~/.claude-cells/containers/<name>/.claude/settings.json` exists
- Check it has the correct model configuration

### 3. Missing Plugins/Permissions
**Cause**: `settings.json` permissions or `plugins/` directory not synced
**Fix**:
- Re-run ccells to re-sync config
- Check `enabledPlugins` in settings.json

## Debug Checklist

1. Check source config exists:
   ```bash
   ls -la ~/.claude/settings.json
   ls -la ~/.claude/.credentials.json  # May not exist on macOS (uses keychain)
   cat ~/.claude.json | head -20
   ```

2. Check ccells copy:
   ```bash
   ls -la ~/.claude-cells/containers/<name>/
   ls -la ~/.claude-cells/containers/<name>/.claude/
   cat ~/.claude-cells/containers/<name>/.claude/settings.json | head -20
   cat ~/.claude-cells/containers/<name>/.claude/.credentials.json | head -5
   ```

3. Check keychain credentials:
   ```bash
   security find-generic-password -s "Claude Code-credentials" -w | head -c 100
   ```

4. Inside container:
   ```bash
   docker exec <container> ls -la /root/.claude/
   docker exec <container> cat /root/.claude/.credentials.json | head -5
   docker exec <container> cat /root/.claude/settings.json | head -20
   ```

## Key Points

- **Credentials on macOS come from keychain**, not a file
- **`.credentials.json` must be inside `~/.claude/`** for Claude Code to find it (note the leading dot!)
- **settings.json controls model choice** - must be synced
- **~/.claude.json is separate** from ~/.claude/ directory
- **Containers run as root** with `bypassPermissions` mode set in settings.json (skips workspace trust dialog)
