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
