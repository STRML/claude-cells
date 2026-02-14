package tmux

import (
	"strings"
	"testing"
)

func TestFormatStatusLine(t *testing.T) {
	workstreams := []StatusWorkstream{
		{Name: "auth", Status: "running", HasPR: true, PRMerged: false},
		{Name: "fix-bug", Status: "paused", HasPR: false},
	}
	prefix := "C-b"

	line := FormatStatusLine(workstreams, prefix, false)

	if !strings.Contains(line, "auth") {
		t.Error("expected auth in status line")
	}
	if !strings.Contains(line, "fix-bug") {
		t.Error("expected fix-bug in status line")
	}
	// Should contain tmux color codes
	if !strings.Contains(line, "#[fg=") {
		t.Error("expected tmux color codes in status line")
	}
	// Running workstream should have green dot
	if !strings.Contains(line, "#[fg="+colorGreen+"]●") {
		t.Error("expected green dot for running workstream")
	}
	// Paused workstream should have yellow dot
	if !strings.Contains(line, "#[fg="+colorYellow+"]●") {
		t.Error("expected yellow dot for paused workstream")
	}
	// PR open should have cyan dot
	if !strings.Contains(line, "#[fg="+colorCyan+"]●") {
		t.Error("expected cyan dot for open PR")
	}
	// Workstream count
	if !strings.Contains(line, "2") {
		t.Error("expected workstream count of 2")
	}
}

func TestFormatStatusLineMultiLine(t *testing.T) {
	workstreams := []StatusWorkstream{
		{Name: "auth", Status: "running"},
	}

	line := FormatStatusLine(workstreams, "C-b", true)
	// Multi-line format should NOT contain keyhints (those go in status-right line 2)
	if strings.Contains(line, "new") {
		t.Error("multi-line format should not contain keyhints")
	}
	// Should have workstream count (with color codes around the number)
	if !strings.Contains(line, "1#[default] workstream") {
		t.Error("expected workstream count in multi-line format")
	}
}

func TestFormatStatusLineSingleWorkstream(t *testing.T) {
	workstreams := []StatusWorkstream{
		{Name: "auth", Status: "running"},
	}
	line := FormatStatusLine(workstreams, "C-b", false)
	// Should say "1 workstream" (singular)
	if !strings.Contains(line, "1#[default] workstream") {
		t.Error("expected singular 'workstream' for 1 workstream")
	}
	if strings.Contains(line, "workstreams") {
		t.Error("should use singular 'workstream' for 1 workstream")
	}
}

func TestFormatStatusLinePRMerged(t *testing.T) {
	workstreams := []StatusWorkstream{
		{Name: "feat", Status: "running", HasPR: true, PRMerged: true},
	}
	line := FormatStatusLine(workstreams, "C-b", false)
	if !strings.Contains(line, "#[fg="+colorGreen+"]✓") {
		t.Error("expected green check for merged PR")
	}
}

func TestFormatStatusLineExited(t *testing.T) {
	workstreams := []StatusWorkstream{
		{Name: "done", Status: "exited"},
	}
	line := FormatStatusLine(workstreams, "C-b", false)
	if !strings.Contains(line, "#[fg="+colorGray+"]●") {
		t.Error("expected gray dot for exited workstream")
	}
}

func TestFormatPaneBorder(t *testing.T) {
	border := FormatPaneBorder("auth-system", "running", 42, "")
	if !strings.Contains(border, "auth-system") {
		t.Error("expected workstream name in border")
	}
	if !strings.Contains(border, "PR#42") {
		t.Error("expected PR number in border")
	}
	// Name should be magenta
	if !strings.Contains(border, "#[fg="+colorMagenta+"]auth-system") {
		t.Error("expected magenta workstream name")
	}
	// PR should be cyan
	if !strings.Contains(border, "#[fg="+colorCyan+"]PR#42") {
		t.Error("expected cyan PR number")
	}
	// Running should have green dot
	if !strings.Contains(border, "#[fg="+colorGreen+"]●") {
		t.Error("expected green dot for running status")
	}
}

func TestFormatPaneBorderPaused(t *testing.T) {
	border := FormatPaneBorder("fix-bug", "paused", 0, "")
	// Should have yellow dot for paused
	if !strings.Contains(border, "#[fg="+colorYellow+"]●") {
		t.Error("expected yellow dot for paused status")
	}
	if strings.Contains(border, "PR#") {
		t.Error("should not contain PR# when PR number is 0")
	}
}

func TestFormatPaneBorderExited(t *testing.T) {
	border := FormatPaneBorder("old-task", "exited", 0, "")
	if !strings.Contains(border, "#[fg="+colorGray+"]●") {
		t.Error("expected gray dot for exited status")
	}
}

func TestFormatPaneBorderWithSynopsis(t *testing.T) {
	border := FormatPaneBorder("auth", "running", 0, "Adding OAuth2 support")
	if !strings.Contains(border, "Adding OAuth2 support") {
		t.Error("expected synopsis in border")
	}
	// Synopsis should be in hint gray
	if !strings.Contains(border, "#[fg="+colorHintGray+"]- Adding OAuth2 support") {
		t.Error("expected gray synopsis text")
	}
}

func TestFormatPrefixHint(t *testing.T) {
	hint := FormatPrefixHint("C-b")
	if !strings.Contains(hint, "^b") {
		t.Error("expected ^b in hint for C-b prefix")
	}

	hint = FormatPrefixHint("C-a")
	if !strings.Contains(hint, "^a") {
		t.Error("expected ^a in hint for C-a prefix")
	}
}

func TestFormatColoredKeyhints(t *testing.T) {
	hints := formatColoredKeyhints("^b")
	if !strings.Contains(hints, "^b+n") {
		t.Error("expected ^b+n in keyhints")
	}
	if !strings.Contains(hints, "new") {
		t.Error("expected 'new' in keyhints")
	}
	if !strings.Contains(hints, "#[fg="+colorMagenta+"]") {
		t.Error("expected magenta key color in keyhints")
	}
	if !strings.Contains(hints, "#[fg="+colorHintGray+"]") {
		t.Error("expected gray hint text color in keyhints")
	}
}

func TestFormatColoredKeyhints_DestroyUsesX(t *testing.T) {
	hints := formatColoredKeyhints("^b")
	// Verify destroy key is "x" (not "d" which conflicts with tmux detach)
	if !strings.Contains(hints, "^b+x") {
		t.Error("expected ^b+x for destroy keyhint")
	}
	if !strings.Contains(hints, "destroy") {
		t.Error("expected 'destroy' label in keyhints")
	}
	// Should NOT use "d" for destroy
	if strings.Contains(hints, "^b+d") {
		t.Error("should not use ^b+d for destroy (conflicts with tmux detach)")
	}
}

func TestFormatColoredKeyhints_AllKeys(t *testing.T) {
	hints := formatColoredKeyhints("^b")
	expectedKeys := []struct {
		key   string
		label string
	}{
		{"^b+n", "new"},
		{"^b+x", "destroy"},
		{"^b+m", "merge"},
		{"^b+?", "help"},
	}
	for _, expected := range expectedKeys {
		if !strings.Contains(hints, expected.key) {
			t.Errorf("expected key %q in keyhints", expected.key)
		}
		if !strings.Contains(hints, expected.label) {
			t.Errorf("expected label %q in keyhints", expected.label)
		}
	}
}

func TestEscapeShellArg(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "hello", "'hello'"},
		{"with spaces", "hello world", "'hello world'"},
		{"with single quote", "it's", "'it'\"'\"'s'"},
		{"with special chars", "hello;rm -rf /", "'hello;rm -rf /'"},
		{"empty", "", "''"},
		{"with newline", "hello\nworld", "'hello\nworld'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EscapeShellArg(tt.input)
			if got != tt.want {
				t.Errorf("EscapeShellArg(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatStatusLine_Empty(t *testing.T) {
	line := FormatStatusLine(nil, "C-b", false)
	// Should contain count 0
	if !strings.Contains(line, "0#[default] workstreams") {
		t.Errorf("empty workstreams should show count 0, got: %s", line)
	}
}

func TestFormatPaneBorder_Structure(t *testing.T) {
	border := FormatPaneBorder("test", "running", 0, "")
	// Should start with border chars
	if !strings.HasPrefix(border, "─── ") {
		t.Errorf("border should start with '─── ', got: %q", border)
	}
	// Should end with border chars
	if !strings.HasSuffix(border, " ───") {
		t.Errorf("border should end with ' ───', got: %q", border)
	}
}

func TestFormatPrefixHint_NonCtrl(t *testing.T) {
	// Non-Ctrl prefix should be returned as-is
	hint := FormatPrefixHint("M-a")
	if hint != "M-a" {
		t.Errorf("FormatPrefixHint('M-a') = %q, want 'M-a'", hint)
	}
}

func TestAbbreviatePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"single component", "claude-cells", "claude-cells"},
		{"root path", "/", "/"},
		{"absolute with dirs", "/usr/local/bin/tool", "/u/l/b/tool"},
		{"empty", "", ""},
		{"trailing slash dirs", "/a/b/c/last", "/a/b/c/last"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AbbreviatePath(tt.path)
			if got != tt.want {
				t.Errorf("AbbreviatePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestAbbreviatePath_HomeTilde(t *testing.T) {
	// Test that paths under $HOME get ~ substitution
	// We can't hardcode HOME, but we can verify the function doesn't panic
	path := "/some/deep/nested/project"
	got := AbbreviatePath(path)
	if got == "" {
		t.Error("AbbreviatePath should not return empty for non-empty path")
	}
	// All intermediate dirs should be abbreviated to 1 char
	parts := strings.Split(got, "/")
	for i := 1; i < len(parts)-1; i++ { // skip leading empty and last
		if parts[i] == "~" {
			continue
		}
		if len([]rune(parts[i])) > 1 {
			t.Errorf("intermediate component %q should be 1 char", parts[i])
		}
	}
}

func TestFormatPowerlineLeft(t *testing.T) {
	result := FormatPowerlineLeft("/Users/sam/git/oss/claude-cells", "main")

	// Should contain abbreviated path
	if !strings.Contains(result, "claude-cells") {
		t.Error("expected last path component in powerline left")
	}

	// Should contain branch name
	if !strings.Contains(result, "main") {
		t.Error("expected branch name in powerline left")
	}

	// Should contain powerline separator
	if !strings.Contains(result, powerlineSep) {
		t.Error("expected powerline separator character")
	}

	// Should contain branch icon
	if !strings.Contains(result, branchIcon) {
		t.Error("expected branch icon character")
	}

	// Should contain path background color
	if !strings.Contains(result, colorPathBg) {
		t.Error("expected path background color")
	}

	// Should contain branch background color
	if !strings.Contains(result, colorBranchBg) {
		t.Error("expected branch background color")
	}

	// Should end with #[default] reset
	if !strings.HasSuffix(result, "#[default]") {
		t.Error("expected #[default] reset at end")
	}
}

func TestFormatPowerlineLeft_LongBranch(t *testing.T) {
	result := FormatPowerlineLeft("/repo", "feature/very-long-branch-name-here")
	if !strings.Contains(result, "feature/very-long-branch-name-here") {
		t.Error("expected full branch name even when long")
	}
}

func TestAttachCommand(t *testing.T) {
	c := NewClient("test-socket")
	cmd := c.AttachCommand("my-session")

	// Should be a tmux command
	if !strings.HasSuffix(cmd.Path, "tmux") && cmd.Path != "tmux" {
		// Path could be absolute or just "tmux"
		args := cmd.Args
		if len(args) == 0 || args[0] != "tmux" {
			t.Errorf("expected tmux command, got path=%q args=%v", cmd.Path, cmd.Args)
		}
	}

	// Check args contain socket and session
	args := cmd.Args
	wantArgs := []string{"tmux", "-L", "test-socket", "attach-session", "-t", "my-session"}
	if len(args) != len(wantArgs) {
		t.Fatalf("Args length = %d, want %d: %v", len(args), len(wantArgs), args)
	}
	for i, want := range wantArgs {
		if args[i] != want {
			t.Errorf("Args[%d] = %q, want %q", i, args[i], want)
		}
	}
}

func TestAttachCommand_DifferentSockets(t *testing.T) {
	tests := []struct {
		socket  string
		session string
	}{
		{"ccells-abc123", "ccells"},
		{"default", "main-session"},
		{"my-socket", "session-with-dashes"},
	}
	for _, tt := range tests {
		t.Run(tt.socket+"/"+tt.session, func(t *testing.T) {
			c := NewClient(tt.socket)
			cmd := c.AttachCommand(tt.session)
			args := cmd.Args
			if args[2] != tt.socket {
				t.Errorf("socket arg = %q, want %q", args[2], tt.socket)
			}
			if args[5] != tt.session {
				t.Errorf("session arg = %q, want %q", args[5], tt.session)
			}
		})
	}
}

func TestAbbreviatePath_UTF8(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"unicode dirs", "/日本語/パス/project", "/日/パ/project"},
		{"emoji dir", "/🏠/code/project", "/🏠/c/project"},
		{"mixed", "/ascii/日本語/last", "/a/日/last"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AbbreviatePath(tt.path)
			if got != tt.want {
				t.Errorf("AbbreviatePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestAbbreviatePath_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"double slash", "//double//slash"},
		{"trailing slash", "/a/b/c/"},
		{"dot path", "./relative/path"},
		{"dotdot path", "../parent/path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify it doesn't panic
			got := AbbreviatePath(tt.path)
			if got == "" && tt.path != "" {
				t.Errorf("AbbreviatePath(%q) returned empty string", tt.path)
			}
		})
	}
}

func TestFormatPaneBorder_AllStatuses(t *testing.T) {
	statuses := []struct {
		status string
		color  string
	}{
		{"running", colorGreen},
		{"paused", colorYellow},
		{"exited", colorGray},
		{"unknown", colorGreen}, // default
	}
	for _, tt := range statuses {
		t.Run(tt.status, func(t *testing.T) {
			border := FormatPaneBorder("ws", tt.status, 0, "")
			if !strings.Contains(border, "#[fg="+tt.color+"]●") {
				t.Errorf("status %q should use color %s", tt.status, tt.color)
			}
		})
	}
}

func TestFormatStatusLine_Keyhints(t *testing.T) {
	line := FormatStatusLine([]StatusWorkstream{{Name: "a", Status: "running"}}, "C-a", false)
	// Single-line mode should include keyhints with the correct prefix
	if !strings.Contains(line, "^a+n") {
		t.Error("expected keyhints with ^a prefix in single-line mode")
	}
}

func TestColorConstants(t *testing.T) {
	// Verify color constants match expected tmux colour names
	tests := []struct {
		name     string
		constant string
		want     string
	}{
		{"green", colorGreen, "colour46"},
		{"yellow", colorYellow, "colour226"},
		{"gray", colorGray, "colour240"},
		{"magenta", colorMagenta, "colour201"},
		{"hint gray", colorHintGray, "colour244"},
		{"cyan", colorCyan, "colour38"},
		{"bar bg", colorBarBg, "colour236"},
		{"white", colorWhite, "colour255"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.constant, tt.want)
			}
		})
	}
}
