# Claude Cells - TODO

## Planned Features

### 1. Post-Merge Verification & Container Destroy Prompt

**Status:** ✅ DONE

**Implementation:**
- Added `DialogPostMergeDestroy` dialog type in `internal/tui/dialog.go`
- Added `NewPostMergeDestroyDialog()` function to create the dialog
- Updated `MergeBranchMsg` handler in `internal/tui/app.go` to show dialog on success
- Added handler for `DialogPostMergeDestroy` in `DialogConfirmMsg` case

**User experience:**
- After successful merge, dialog appears: "Branch 'X' has been merged into main. Would you like to destroy this container?"
- Options: "Yes, destroy container" / "No, keep container"
- If yes, container and worktree are cleaned up

---

### 2. Merge Failure Handling with Rebase Option

**Status:** ✅ DONE

**Implementation:**
- Added `MergeConflictError` type in `internal/git/branch.go` to detect conflicts
- Updated `MergeBranch()` to detect conflicts and return them in structured error
- Added `RebaseBranch()` and `AbortRebase()` functions in `internal/git/branch.go`
- Updated `MergeBranchMsg` struct to include `ConflictFiles`
- Added `RebaseBranchMsg` and `RebaseBranchCmd` in `internal/tui/container.go`
- Added `DialogMergeConflict` dialog type with "Rebase onto main" / "Cancel" options
- Updated `MergeBranchMsg` handler to show conflict dialog when conflicts detected
- Added `RebaseBranchMsg` handler to notify user of rebase results

**User experience:**
- On merge conflict: dialog shows conflicting files and offers to rebase
- If rebase chosen: rebases branch onto main, notifies Claude of conflicts to resolve
- If conflicts during rebase: Claude is told which files need resolution
- If rebase succeeds: user can retry merge

---

### 3. Add `gh` CLI to Docker Image

**Status:** Not started

**Current behavior:** The base Dockerfile at `configs/base.Dockerfile` installs Node.js and Claude Code but not the GitHub CLI.

**Desired behavior:**
- Install `gh` CLI in the base image so Claude can create PRs, view issues, etc.

**Implementation:**
Add to `configs/base.Dockerfile` after the apt-get install block:

```dockerfile
# Install GitHub CLI
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
    && apt-get update \
    && apt-get install -y gh \
    && rm -rf /var/lib/apt/lists/*
```

**Also needed:** Mount gh auth credentials from host `~/.config/gh/` or pass `GH_TOKEN` env var.

---

### 4. OAuth Token Refresh for Running Containers

**Status:** ✅ DONE

**Implementation:**
- Added `CredentialRefresher` in `internal/docker/credentials.go`
- Background goroutine checks keychain every 15 minutes
- When credentials change, updates `.credentials.json` in all registered container config directories
- Containers see updates via bind mount (no restart needed)
- Containers registered on creation, unregistered on stop

**Key files changed:**
- `internal/docker/credentials.go` - Added CredentialRefresher struct and methods
- `internal/tui/container.go` - Added registration/unregistration calls
- `cmd/ccells/main.go` - Started credential refresher on app start

**Findings:**
- OAuth tokens last ~24 hours
- Claude Code on macOS auto-refreshes and updates keychain
- Tokens are picked up on next API call (no PTY restart needed)

---

### 5. Notify Claude Code UI on Merge/PR Creation

**Status:** ✅ DONE

**Implementation:**
- Added `SendToPTY()` method to `PaneModel` in `internal/tui/pane.go`
- Updated `PRCreatedMsg` and `MergeBranchMsg` handlers in `internal/tui/app.go`
- Sends notification to PTY session on successful operations

**Messages sent:**
```
[ccells] ✓ PR #42 created: https://github.com/user/repo/pull/42
[ccells] ✓ Branch 'ccells/feature-x' merged into main
```

---

## Implementation Order

1. ~~**OAuth token refresh**~~ ✅ DONE
2. ~~**Notify Claude on merge/PR**~~ ✅ DONE
3. ~~**Post-merge verification**~~ ✅ DONE
4. ~~**Merge failure handling**~~ ✅ DONE
5. **gh CLI** - Do inside ccells (deferred)
