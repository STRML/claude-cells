package tui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/STRML/claude-cells/internal/workstream"
)

func TestGetWorktreePath(t *testing.T) {
	tests := []struct {
		name       string
		branchName string
		expected   string
	}{
		{
			name:       "simple branch name",
			branchName: "feature-test",
			expected:   "/tmp/ccells/worktrees/feature-test",
		},
		{
			name:       "branch with slash",
			branchName: "feature/add-tests",
			expected:   "/tmp/ccells/worktrees/feature-add-tests",
		},
		{
			name:       "branch with multiple slashes",
			branchName: "user/feature/sub-feature",
			expected:   "/tmp/ccells/worktrees/user-feature-sub-feature",
		},
		{
			name:       "branch with space",
			branchName: "feature test",
			expected:   "/tmp/ccells/worktrees/feature-test",
		},
		{
			name:       "branch with slash and space",
			branchName: "feature/test branch",
			expected:   "/tmp/ccells/worktrees/feature-test-branch",
		},
		{
			name:       "ccells prefixed branch",
			branchName: "ccells/test-feature",
			expected:   "/tmp/ccells/worktrees/ccells-test-feature",
		},
		{
			name:       "empty branch name",
			branchName: "",
			expected:   "/tmp/ccells/worktrees/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getWorktreePath(tt.branchName)
			if result != tt.expected {
				t.Errorf("getWorktreePath(%q) = %q, want %q", tt.branchName, result, tt.expected)
			}
		})
	}
}

func TestContainerStartedMsg(t *testing.T) {
	tests := []struct {
		name    string
		msg     ContainerStartedMsg
		wantID  string
		wantCID string
		resume  bool
	}{
		{
			name: "basic message",
			msg: ContainerStartedMsg{
				WorkstreamID: "ws-1",
				ContainerID:  "container-abc123",
			},
			wantID:  "ws-1",
			wantCID: "container-abc123",
			resume:  false,
		},
		{
			name: "resume message",
			msg: ContainerStartedMsg{
				WorkstreamID: "ws-2",
				ContainerID:  "container-def456",
				IsResume:     true,
			},
			wantID:  "ws-2",
			wantCID: "container-def456",
			resume:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.msg.WorkstreamID != tt.wantID {
				t.Errorf("WorkstreamID = %q, want %q", tt.msg.WorkstreamID, tt.wantID)
			}
			if tt.msg.ContainerID != tt.wantCID {
				t.Errorf("ContainerID = %q, want %q", tt.msg.ContainerID, tt.wantCID)
			}
			if tt.msg.IsResume != tt.resume {
				t.Errorf("IsResume = %v, want %v", tt.msg.IsResume, tt.resume)
			}
		})
	}
}

func TestContainerErrorMsg(t *testing.T) {
	err := errors.New("test error")
	msg := ContainerErrorMsg{
		WorkstreamID: "ws-1",
		Error:        err,
	}

	if msg.WorkstreamID != "ws-1" {
		t.Errorf("WorkstreamID = %q, want %q", msg.WorkstreamID, "ws-1")
	}
	if msg.Error != err {
		t.Errorf("Error = %v, want %v", msg.Error, err)
	}
}

func TestContainerOutputMsg(t *testing.T) {
	output := []byte("hello world")
	msg := ContainerOutputMsg{
		WorkstreamID: "ws-1",
		Output:       output,
	}

	if msg.WorkstreamID != "ws-1" {
		t.Errorf("WorkstreamID = %q, want %q", msg.WorkstreamID, "ws-1")
	}
	if string(msg.Output) != "hello world" {
		t.Errorf("Output = %q, want %q", string(msg.Output), "hello world")
	}
}

func TestContainerStoppedMsg(t *testing.T) {
	msg := ContainerStoppedMsg{
		WorkstreamID: "ws-1",
	}

	if msg.WorkstreamID != "ws-1" {
		t.Errorf("WorkstreamID = %q, want %q", msg.WorkstreamID, "ws-1")
	}
}

func TestContainerNotFoundMsg(t *testing.T) {
	msg := ContainerNotFoundMsg{
		WorkstreamID: "ws-1",
	}

	if msg.WorkstreamID != "ws-1" {
		t.Errorf("WorkstreamID = %q, want %q", msg.WorkstreamID, "ws-1")
	}
}

func TestPTYReadyMsg(t *testing.T) {
	session := &PTYSession{
		workstreamID: "test",
	}
	msg := PTYReadyMsg{
		WorkstreamID: "ws-1",
		Session:      session,
	}

	if msg.WorkstreamID != "ws-1" {
		t.Errorf("WorkstreamID = %q, want %q", msg.WorkstreamID, "ws-1")
	}
	if msg.Session != session {
		t.Error("Session should match")
	}
}

func TestContainerLogsMsg(t *testing.T) {
	tests := []struct {
		name     string
		msg      ContainerLogsMsg
		wantLogs string
		wantErr  bool
	}{
		{
			name: "with logs",
			msg: ContainerLogsMsg{
				WorkstreamID: "ws-1",
				Logs:         "container output\nmore output",
				Error:        nil,
			},
			wantLogs: "container output\nmore output",
			wantErr:  false,
		},
		{
			name: "with error",
			msg: ContainerLogsMsg{
				WorkstreamID: "ws-1",
				Logs:         "",
				Error:        errors.New("failed to get logs"),
			},
			wantLogs: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.msg.Logs != tt.wantLogs {
				t.Errorf("Logs = %q, want %q", tt.msg.Logs, tt.wantLogs)
			}
			if (tt.msg.Error != nil) != tt.wantErr {
				t.Errorf("Error = %v, wantErr = %v", tt.msg.Error, tt.wantErr)
			}
		})
	}
}

func TestBranchConflictMsg(t *testing.T) {
	msg := BranchConflictMsg{
		WorkstreamID: "ws-1",
		BranchName:   "feature/test",
		RepoPath:     "/home/user/repo",
		BranchInfo:   "3 commits ahead of main",
	}

	if msg.WorkstreamID != "ws-1" {
		t.Errorf("WorkstreamID = %q, want %q", msg.WorkstreamID, "ws-1")
	}
	if msg.BranchName != "feature/test" {
		t.Errorf("BranchName = %q, want %q", msg.BranchName, "feature/test")
	}
	if msg.RepoPath != "/home/user/repo" {
		t.Errorf("RepoPath = %q, want %q", msg.RepoPath, "/home/user/repo")
	}
	if msg.BranchInfo != "3 commits ahead of main" {
		t.Errorf("BranchInfo = %q, want %q", msg.BranchInfo, "3 commits ahead of main")
	}
}

func TestTitleGeneratedMsg(t *testing.T) {
	tests := []struct {
		name      string
		msg       TitleGeneratedMsg
		wantTitle string
		wantErr   bool
	}{
		{
			name: "successful title",
			msg: TitleGeneratedMsg{
				WorkstreamID: "ws-1",
				Title:        "Add user authentication",
				Error:        nil,
			},
			wantTitle: "Add user authentication",
			wantErr:   false,
		},
		{
			name: "with error",
			msg: TitleGeneratedMsg{
				WorkstreamID: "ws-1",
				Title:        "",
				Error:        errors.New("claude not available"),
			},
			wantTitle: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.msg.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", tt.msg.Title, tt.wantTitle)
			}
			if (tt.msg.Error != nil) != tt.wantErr {
				t.Errorf("Error = %v, wantErr = %v", tt.msg.Error, tt.wantErr)
			}
		})
	}
}

func TestUncommittedChangesMsg(t *testing.T) {
	tests := []struct {
		name       string
		msg        UncommittedChangesMsg
		hasChanges bool
		wantErr    bool
	}{
		{
			name: "has changes",
			msg: UncommittedChangesMsg{
				WorkstreamID: "ws-1",
				HasChanges:   true,
				Error:        nil,
			},
			hasChanges: true,
			wantErr:    false,
		},
		{
			name: "no changes",
			msg: UncommittedChangesMsg{
				WorkstreamID: "ws-1",
				HasChanges:   false,
				Error:        nil,
			},
			hasChanges: false,
			wantErr:    false,
		},
		{
			name: "with error",
			msg: UncommittedChangesMsg{
				WorkstreamID: "ws-1",
				HasChanges:   false,
				Error:        errors.New("git error"),
			},
			hasChanges: false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.msg.HasChanges != tt.hasChanges {
				t.Errorf("HasChanges = %v, want %v", tt.msg.HasChanges, tt.hasChanges)
			}
			if (tt.msg.Error != nil) != tt.wantErr {
				t.Errorf("Error = %v, wantErr = %v", tt.msg.Error, tt.wantErr)
			}
		})
	}
}

func TestPushBranchResultMsg(t *testing.T) {
	tests := []struct {
		name              string
		msg               PushBranchResultMsg
		wantErr           bool
		wantCommitsPushed int
	}{
		{
			name: "successful push",
			msg: PushBranchResultMsg{
				WorkstreamID: "ws-1",
				Error:        nil,
			},
			wantErr:           false,
			wantCommitsPushed: 0,
		},
		{
			name: "failed push",
			msg: PushBranchResultMsg{
				WorkstreamID: "ws-1",
				Error:        errors.New("push rejected"),
			},
			wantErr:           true,
			wantCommitsPushed: 0,
		},
		{
			name: "push with commit count",
			msg: PushBranchResultMsg{
				WorkstreamID:  "ws-1",
				Error:         nil,
				CommitsPushed: 3,
			},
			wantErr:           false,
			wantCommitsPushed: 3,
		},
		{
			name: "force push with commit count",
			msg: PushBranchResultMsg{
				WorkstreamID:  "ws-1",
				Error:         nil,
				ForcePush:     true,
				CommitsPushed: 5,
			},
			wantErr:           false,
			wantCommitsPushed: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if (tt.msg.Error != nil) != tt.wantErr {
				t.Errorf("Error = %v, wantErr = %v", tt.msg.Error, tt.wantErr)
			}
			if tt.msg.CommitsPushed != tt.wantCommitsPushed {
				t.Errorf("CommitsPushed = %d, want = %d", tt.msg.CommitsPushed, tt.wantCommitsPushed)
			}
		})
	}
}

func TestPRCreatedMsg(t *testing.T) {
	tests := []struct {
		name       string
		msg        PRCreatedMsg
		wantURL    string
		wantNumber int
		wantErr    bool
	}{
		{
			name: "successful PR",
			msg: PRCreatedMsg{
				WorkstreamID: "ws-1",
				PRURL:        "https://github.com/user/repo/pull/123",
				PRNumber:     123,
				Error:        nil,
			},
			wantURL:    "https://github.com/user/repo/pull/123",
			wantNumber: 123,
			wantErr:    false,
		},
		{
			name: "failed PR",
			msg: PRCreatedMsg{
				WorkstreamID: "ws-1",
				PRURL:        "",
				PRNumber:     0,
				Error:        errors.New("no remote"),
			},
			wantURL:    "",
			wantNumber: 0,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.msg.PRURL != tt.wantURL {
				t.Errorf("PRURL = %q, want %q", tt.msg.PRURL, tt.wantURL)
			}
			if tt.msg.PRNumber != tt.wantNumber {
				t.Errorf("PRNumber = %d, want %d", tt.msg.PRNumber, tt.wantNumber)
			}
			if (tt.msg.Error != nil) != tt.wantErr {
				t.Errorf("Error = %v, wantErr = %v", tt.msg.Error, tt.wantErr)
			}
		})
	}
}

func TestMergeBranchMsg(t *testing.T) {
	tests := []struct {
		name                string
		msg                 MergeBranchMsg
		wantErr             bool
		wantContainerRebase bool
		wantConflictFiles   int
	}{
		{
			name: "successful merge",
			msg: MergeBranchMsg{
				WorkstreamID: "ws-1",
				Error:        nil,
			},
			wantErr:             false,
			wantContainerRebase: false,
		},
		{
			name: "merge conflict",
			msg: MergeBranchMsg{
				WorkstreamID:  "ws-1",
				Error:         errors.New("merge conflict"),
				ConflictFiles: []string{"file1.go", "file2.go"},
			},
			wantErr:           true,
			wantConflictFiles: 2,
		},
		{
			name: "needs container rebase (worktree conflict)",
			msg: MergeBranchMsg{
				WorkstreamID:         "ws-1",
				Error:                errors.New("cannot auto-rebase: branch checked out in worktree"),
				NeedsContainerRebase: true,
			},
			wantErr:             true,
			wantContainerRebase: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if (tt.msg.Error != nil) != tt.wantErr {
				t.Errorf("Error = %v, wantErr = %v", tt.msg.Error, tt.wantErr)
			}
			if tt.msg.NeedsContainerRebase != tt.wantContainerRebase {
				t.Errorf("NeedsContainerRebase = %v, want %v", tt.msg.NeedsContainerRebase, tt.wantContainerRebase)
			}
			if len(tt.msg.ConflictFiles) != tt.wantConflictFiles {
				t.Errorf("ConflictFiles count = %d, want %d", len(tt.msg.ConflictFiles), tt.wantConflictFiles)
			}
		})
	}
}

func TestPairingEnabledMsg(t *testing.T) {
	tests := []struct {
		name           string
		msg            PairingEnabledMsg
		stashedChanges bool
		wantErr        bool
	}{
		{
			name: "enabled without stash",
			msg: PairingEnabledMsg{
				WorkstreamID:   "ws-1",
				StashedChanges: false,
				Error:          nil,
			},
			stashedChanges: false,
			wantErr:        false,
		},
		{
			name: "enabled with stash",
			msg: PairingEnabledMsg{
				WorkstreamID:   "ws-1",
				StashedChanges: true,
				Error:          nil,
			},
			stashedChanges: true,
			wantErr:        false,
		},
		{
			name: "failed to enable",
			msg: PairingEnabledMsg{
				WorkstreamID:   "ws-1",
				StashedChanges: false,
				Error:          errors.New("mutagen error"),
			},
			stashedChanges: false,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.msg.StashedChanges != tt.stashedChanges {
				t.Errorf("StashedChanges = %v, want %v", tt.msg.StashedChanges, tt.stashedChanges)
			}
			if (tt.msg.Error != nil) != tt.wantErr {
				t.Errorf("Error = %v, wantErr = %v", tt.msg.Error, tt.wantErr)
			}
		})
	}
}

func TestPairingDisabledMsg(t *testing.T) {
	tests := []struct {
		name           string
		msg            PairingDisabledMsg
		stashedChanges bool
		wantErr        bool
	}{
		{
			name: "disabled cleanly",
			msg: PairingDisabledMsg{
				WorkstreamID:   "ws-1",
				StashedChanges: false,
				Error:          nil,
			},
			stashedChanges: false,
			wantErr:        false,
		},
		{
			name: "disabled with stash reminder",
			msg: PairingDisabledMsg{
				WorkstreamID:   "ws-1",
				StashedChanges: true,
				Error:          nil,
			},
			stashedChanges: true,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.msg.StashedChanges != tt.stashedChanges {
				t.Errorf("StashedChanges = %v, want %v", tt.msg.StashedChanges, tt.stashedChanges)
			}
			if (tt.msg.Error != nil) != tt.wantErr {
				t.Errorf("Error = %v, wantErr = %v", tt.msg.Error, tt.wantErr)
			}
		})
	}
}

func TestResourceStatsMsg(t *testing.T) {
	msg := ResourceStatsMsg{
		TotalCPU:    150.5,
		TotalMemory: 1024 * 1024 * 1024, // 1GB
		IsGlobal:    true,
		Error:       nil,
	}

	if msg.TotalCPU != 150.5 {
		t.Errorf("TotalCPU = %f, want %f", msg.TotalCPU, 150.5)
	}
	if msg.TotalMemory != 1024*1024*1024 {
		t.Errorf("TotalMemory = %d, want %d", msg.TotalMemory, 1024*1024*1024)
	}
	if !msg.IsGlobal {
		t.Error("IsGlobal should be true")
	}
}

func TestPruneResultMsg(t *testing.T) {
	msg := PruneResultMsg{
		PrunedCount: 5,
		Error:       nil,
	}

	if msg.PrunedCount != 5 {
		t.Errorf("PrunedCount = %d, want 5", msg.PrunedCount)
	}
}

func TestPruneAllResultMsg(t *testing.T) {
	msg := PruneAllResultMsg{
		ContainersPruned: 3,
		BranchesPruned:   2,
		Error:            nil,
	}

	if msg.ContainersPruned != 3 {
		t.Errorf("ContainersPruned = %d, want 3", msg.ContainersPruned)
	}
	if msg.BranchesPruned != 2 {
		t.Errorf("BranchesPruned = %d, want 2", msg.BranchesPruned)
	}
}

func TestContainerCountMsg(t *testing.T) {
	msg := ContainerCountMsg{
		Count: 10,
		Error: nil,
	}

	if msg.Count != 10 {
		t.Errorf("Count = %d, want 10", msg.Count)
	}
}

func TestTrackContainer(t *testing.T) {
	// Save original tracker
	original := services.tracker
	defer func() {
		services.tracker = original
	}()

	// Test with nil tracker (should not panic)
	services.tracker = nil
	trackContainer("container-1", "ws-1", "branch-1", "/repo/path")
}

func TestUntrackContainer(t *testing.T) {
	// Save original tracker
	original := services.tracker
	defer func() {
		services.tracker = original
	}()

	// Test with nil tracker (should not panic)
	services.tracker = nil
	untrackContainer("container-1")
}

func TestSetContainerTracker(t *testing.T) {
	// Save original
	original := services.tracker
	defer func() {
		services.tracker = original
	}()

	// Test setting nil
	SetContainerTracker(nil)
	if services.tracker != nil {
		t.Error("services.tracker should be nil")
	}
}

func TestGetContainerTracker(t *testing.T) {
	// Save original
	original := services.tracker
	defer func() {
		services.tracker = original
	}()

	// Set to nil
	services.tracker = nil
	if GetContainerTracker() != nil {
		t.Error("GetContainerTracker should return nil")
	}
}

func TestGetCredentialRefresher(t *testing.T) {
	// Save original
	original := services.refresher
	defer func() {
		services.refresher = original
	}()

	// Set to nil
	services.refresher = nil
	if GetCredentialRefresher() != nil {
		t.Error("GetCredentialRefresher should return nil")
	}
}

func TestCheckUncommittedChangesCmd_NoWorktree(t *testing.T) {
	ws := workstream.New("test prompt")
	// No worktree path or branch name set

	cmd := CheckUncommittedChangesCmd(ws)
	msg := cmd()

	switch m := msg.(type) {
	case UncommittedChangesMsg:
		if m.WorkstreamID != ws.ID {
			t.Errorf("WorkstreamID = %q, want %q", m.WorkstreamID, ws.ID)
		}
		if m.Error == nil {
			t.Error("Expected error for workstream without worktree")
		}
	default:
		t.Errorf("Expected UncommittedChangesMsg, got %T", msg)
	}
}

func TestFetchContainerLogsCmd_EmptyContainerID(t *testing.T) {
	ws := workstream.New("test prompt")
	// ContainerID is empty

	cmd := FetchContainerLogsCmd(ws)
	msg := cmd()

	switch m := msg.(type) {
	case ContainerLogsMsg:
		if m.WorkstreamID != ws.ID {
			t.Errorf("WorkstreamID = %q, want %q", m.WorkstreamID, ws.ID)
		}
		if m.Logs != "" {
			t.Errorf("Logs should be empty, got %q", m.Logs)
		}
		if m.Error != nil {
			t.Errorf("Error should be nil, got %v", m.Error)
		}
	default:
		t.Errorf("Expected ContainerLogsMsg, got %T", msg)
	}
}

func TestPauseContainerCmd_EmptyContainerID(t *testing.T) {
	cmd := PauseContainerCmd("")
	msg := cmd()

	// Should return nil for empty container ID
	if msg != nil {
		t.Errorf("Expected nil, got %v", msg)
	}
}

func TestResumeContainerCmd_EmptyContainerID(t *testing.T) {
	ws := workstream.New("test prompt")
	// ContainerID is empty

	cmd := ResumeContainerCmd(ws, 80, 24)
	msg := cmd()

	switch m := msg.(type) {
	case ContainerErrorMsg:
		if m.WorkstreamID != ws.ID {
			t.Errorf("WorkstreamID = %q, want %q", m.WorkstreamID, ws.ID)
		}
		if m.Error == nil {
			t.Error("Expected error for empty container ID")
		}
	default:
		t.Errorf("Expected ContainerErrorMsg, got %T", msg)
	}
}

func TestPushBranchCmd_NoWorktree(t *testing.T) {
	ws := workstream.New("test prompt")
	// No worktree path or branch name

	cmd := PushBranchCmd(ws)
	msg := cmd()

	switch m := msg.(type) {
	case PushBranchResultMsg:
		if m.WorkstreamID != ws.ID {
			t.Errorf("WorkstreamID = %q, want %q", m.WorkstreamID, ws.ID)
		}
		if m.Error == nil {
			t.Error("Expected error for workstream without worktree")
		}
	default:
		t.Errorf("Expected PushBranchResultMsg, got %T", msg)
	}
}

func TestCreatePRCmd_NoWorktree(t *testing.T) {
	ws := workstream.New("test prompt")
	// No worktree path or branch name

	cmd := CreatePRCmd(ws)
	msg := cmd()

	switch m := msg.(type) {
	case PRCreatedMsg:
		if m.WorkstreamID != ws.ID {
			t.Errorf("WorkstreamID = %q, want %q", m.WorkstreamID, ws.ID)
		}
		if m.Error == nil {
			t.Error("Expected error for workstream without worktree")
		}
	default:
		t.Errorf("Expected PRCreatedMsg, got %T", msg)
	}
}

// BenchmarkGetWorktreePath benchmarks the path computation
func BenchmarkGetWorktreePath(b *testing.B) {
	branchName := "feature/add-some-feature"
	for i := 0; i < b.N; i++ {
		getWorktreePath(branchName)
	}
}

func TestUntrackedFilesPromptMsg(t *testing.T) {
	msg := UntrackedFilesPromptMsg{
		WorkstreamID:   "ws-1",
		UntrackedFiles: []string{"file1.txt", "dir/file2.txt"},
	}

	if msg.WorkstreamID != "ws-1" {
		t.Errorf("WorkstreamID = %q, want %q", msg.WorkstreamID, "ws-1")
	}
	if len(msg.UntrackedFiles) != 2 {
		t.Errorf("UntrackedFiles length = %d, want 2", len(msg.UntrackedFiles))
	}
	if msg.UntrackedFiles[0] != "file1.txt" {
		t.Errorf("UntrackedFiles[0] = %q, want %q", msg.UntrackedFiles[0], "file1.txt")
	}
	if msg.UntrackedFiles[1] != "dir/file2.txt" {
		t.Errorf("UntrackedFiles[1] = %q, want %q", msg.UntrackedFiles[1], "dir/file2.txt")
	}
}

func TestCopyUntrackedFilesToWorktree(t *testing.T) {
	// Create temp source directory
	srcDir, err := os.MkdirTemp("", "copy-test-src-*")
	if err != nil {
		t.Fatalf("Failed to create source temp dir: %v", err)
	}
	defer os.RemoveAll(srcDir)

	// Create temp destination directory
	dstDir, err := os.MkdirTemp("", "copy-test-dst-*")
	if err != nil {
		t.Fatalf("Failed to create dest temp dir: %v", err)
	}
	defer os.RemoveAll(dstDir)

	// Create test files in source
	testContent := []byte("test content")
	nestedContent := []byte("nested content")

	// Create a simple file
	if err := os.WriteFile(filepath.Join(srcDir, "simple.txt"), testContent, 0644); err != nil {
		t.Fatalf("Failed to create simple.txt: %v", err)
	}

	// Create a nested file
	nestedDir := filepath.Join(srcDir, "subdir", "deep")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("Failed to create nested directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "nested.txt"), nestedContent, 0755); err != nil {
		t.Fatalf("Failed to create nested.txt: %v", err)
	}

	// Files to copy
	files := []string{
		"simple.txt",
		"subdir/deep/nested.txt",
	}

	// Run the copy function
	err = copyUntrackedFilesToWorktree(srcDir, dstDir, files)
	if err != nil {
		t.Errorf("copyUntrackedFilesToWorktree returned error: %v", err)
	}

	// Verify simple file was copied
	copiedContent, err := os.ReadFile(filepath.Join(dstDir, "simple.txt"))
	if err != nil {
		t.Errorf("Failed to read copied simple.txt: %v", err)
	}
	if string(copiedContent) != string(testContent) {
		t.Errorf("simple.txt content = %q, want %q", string(copiedContent), string(testContent))
	}

	// Verify nested file was copied
	copiedNested, err := os.ReadFile(filepath.Join(dstDir, "subdir", "deep", "nested.txt"))
	if err != nil {
		t.Errorf("Failed to read copied nested.txt: %v", err)
	}
	if string(copiedNested) != string(nestedContent) {
		t.Errorf("nested.txt content = %q, want %q", string(copiedNested), string(nestedContent))
	}

	// Verify permissions are preserved
	info, err := os.Stat(filepath.Join(dstDir, "subdir", "deep", "nested.txt"))
	if err != nil {
		t.Errorf("Failed to stat nested.txt: %v", err)
	}
	// Check that the file is executable (0755)
	if info.Mode().Perm()&0100 == 0 {
		t.Errorf("nested.txt should be executable, mode = %o", info.Mode().Perm())
	}
}

func TestCopyUntrackedFilesToWorktree_NonexistentFile(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "copy-test-src-*")
	if err != nil {
		t.Fatalf("Failed to create source temp dir: %v", err)
	}
	defer os.RemoveAll(srcDir)

	dstDir, err := os.MkdirTemp("", "copy-test-dst-*")
	if err != nil {
		t.Fatalf("Failed to create dest temp dir: %v", err)
	}
	defer os.RemoveAll(dstDir)

	// Try to copy a non-existent file
	files := []string{"nonexistent.txt"}

	err = copyUntrackedFilesToWorktree(srcDir, dstDir, files)
	// Should return an error but not panic
	if err == nil {
		t.Error("Expected error for non-existent file, got nil")
	}
}

func TestCopyUntrackedFilesToWorktree_EmptyList(t *testing.T) {
	srcDir, err := os.MkdirTemp("", "copy-test-src-*")
	if err != nil {
		t.Fatalf("Failed to create source temp dir: %v", err)
	}
	defer os.RemoveAll(srcDir)

	dstDir, err := os.MkdirTemp("", "copy-test-dst-*")
	if err != nil {
		t.Fatalf("Failed to create dest temp dir: %v", err)
	}
	defer os.RemoveAll(dstDir)

	// Empty file list
	var files []string

	err = copyUntrackedFilesToWorktree(srcDir, dstDir, files)
	if err != nil {
		t.Errorf("Expected no error for empty file list, got: %v", err)
	}
}
