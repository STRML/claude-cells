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
| `commands/` | Custom slash commands | YES |
| `agents/` | Custom agents | YES |
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
| Host Path | Container Path | Mode |
|-----------|----------------|------|
| `~/.claude-cells/claude-config/.claude` | `/home/claude/.claude` | RW |
| `~/.claude-cells/claude-config/.claude.json` | `/home/claude/.claude.json` | RW |
| `~/.claude-cells/claude-config/.claude-credentials` | `/home/claude/.claude-credentials` | RO |
| `~/.claude-cells/claude-config/.gitconfig` | `/home/claude/.gitconfig` | RO |

### PTY Startup
The PTY startup command copies credentials to the expected location:
```bash
test -f /home/claude/.claude-credentials && \
  cp /home/claude/.claude-credentials /home/claude/.claude/.credentials.json
```

**IMPORTANT:** On Linux, Claude Code expects `.credentials.json` (with a leading dot), not `credentials.json`.

## Common Issues

### 1. "Invalid API key" Error
**Cause**: Credentials not synced or in wrong format
**Fix**:
- Verify `~/.claude-cells/claude-config/.claude/.credentials.json` exists and has valid tokens (note the leading dot!)
- Check token hasn't expired (`expiresAt` timestamp)

### 2. Wrong Model (e.g., "Sonnet 4.5" instead of configured model)
**Cause**: `settings.json` not synced or not mounted correctly
**Fix**:
- Verify `~/.claude-cells/claude-config/.claude/settings.json` exists
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
   ls -la ~/.claude-cells/claude-config/
   ls -la ~/.claude-cells/claude-config/.claude/
   cat ~/.claude-cells/claude-config/.claude/settings.json | head -20
   cat ~/.claude-cells/claude-config/.claude/.credentials.json | head -5
   ```

3. Check keychain credentials:
   ```bash
   security find-generic-password -s "Claude Code-credentials" -w | head -c 100
   ```

4. Inside container:
   ```bash
   docker exec <container> ls -la /home/claude/.claude/
   docker exec <container> cat /home/claude/.claude/.credentials.json | head -5
   docker exec <container> cat /home/claude/.claude/settings.json | head -20
   ```

## Key Points

- **Credentials on macOS come from keychain**, not a file
- **`.credentials.json` must be inside `~/.claude/`** for Claude Code to find it (note the leading dot!)
- **settings.json controls model choice** - must be synced
- **~/.claude.json is separate** from ~/.claude/ directory
