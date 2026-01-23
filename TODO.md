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

**Status:** ✅ DONE

**Implementation:**
- Added GitHub CLI installation to `configs/base.Dockerfile`
- Uses official GitHub apt repository for reliable installation
- Cleans up apt lists after installation to minimize image size

**Note:** For gh authentication in containers, either:
- Mount `~/.config/gh/` from host (not yet implemented)
- Pass `GH_TOKEN` environment variable
- Use `gh auth login` inside the container

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

### 6. Container Resource Limits

**Status:** ✅ DONE

**Problem:** Runaway processes in containers can consume all host CPU/memory.

**Implementation:**
- Added `DefaultCPULimit` (2.0 CPUs) and `DefaultMemoryLimit` (4GB) constants
- Added `CPULimit` and `MemoryLimit` fields to `ContainerConfig`
- Applied resource limits via `HostConfig.Resources` in `CreateContainer()`
- Memory swap disabled (set equal to memory limit)

**Defaults:**
- CPU: 2 CPUs per container
- Memory: 4GB per container
- Can be overridden by setting `CPULimit` and `MemoryLimit` on `ContainerConfig`

---

### 7. Export Container Logs

**Status:** ✅ DONE

**Problem:** Hard to debug issues or share conversations from ccells sessions.

**Implementation:**
- Added `ExportLogs()` method to `PaneModel` in `internal/tui/pane.go`
- Added keybinding `e` in nav mode to export logs
- Combines scrollback buffer with current vterm content
- Strips ANSI codes for clean text output
- Saves to `~/.claude-cells/logs/<branch>-<timestamp>.txt`
- Includes header with branch name, title, timestamp, and original prompt

---

### 8. Auto-Pull Main Before Branch Creation

**Status:** ✅ DONE

**Problem:** New branches may be based on stale main, causing merge conflicts later.

**Implementation:**
- Added `FetchMain()`, `PullMain()`, and `UpdateMainBranch()` to `internal/git/branch.go`
- `UpdateMainBranch()` uses `git fetch origin main:main` to update local ref without checkout
- Called automatically in `startContainerWithOptions()` before creating worktree
- Errors are non-fatal (e.g., no network, no remote, local changes)

**Key behavior:**
- Updates local main to match origin/main without changing current checkout
- Runs silently - no user intervention needed
- Gracefully handles offline/error scenarios

---

### 9. Pane Title from Claude's Summary

**Status:** ✅ DONE (already implemented)

**Problem:** Pane titles show branch names which can be cryptic (e.g., `ccells/abc123`).

**Implementation:**
- `workstream.GetTitle()` returns the generated summary title, falling back to branch name
- Pane `View()` uses `p.workstream.GetTitle()` for display
- Title is generated via `GenerateTitleCmd()` using Claude CLI before container starts
- Title is stored in `workstream.Title` field and persisted across sessions

---

## Implementation Order

1. ~~**OAuth token refresh**~~ ✅ DONE
2. ~~**Notify Claude on merge/PR**~~ ✅ DONE
3. ~~**Post-merge verification**~~ ✅ DONE
4. ~~**Merge failure handling**~~ ✅ DONE
5. ~~**gh CLI**~~ ✅ DONE - Added to Docker image
6. ~~**Auto-pull main**~~ ✅ DONE - Prevent stale branch issues
7. ~~**Pane title from summary**~~ ✅ DONE - Already implemented
8. ~~**Export logs**~~ ✅ DONE - Debugging/sharing (keybind: e)
9. ~~**Resource limits**~~ ✅ DONE - Safety (2 CPUs, 4GB RAM default)
