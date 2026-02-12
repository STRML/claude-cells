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
	if !strings.Contains(line, "^b") {
		t.Error("expected prefix hint ^b in keyhints")
	}
}

func TestFormatStatusLineMultiLine(t *testing.T) {
	workstreams := []StatusWorkstream{
		{Name: "auth", Status: "running"},
	}

	lines := FormatStatusLine(workstreams, "C-b", true)
	// Multi-line format has two lines separated by \n
	if !strings.Contains(lines, "\n") {
		t.Error("expected multi-line format to contain newline")
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
}

func TestFormatPaneBorderPaused(t *testing.T) {
	border := FormatPaneBorder("fix-bug", "paused", 0, "")
	if !strings.Contains(border, "paused") {
		t.Error("expected paused status in border")
	}
	if strings.Contains(border, "PR#") {
		t.Error("should not contain PR# when PR number is 0")
	}
}

func TestFormatPaneBorderWithSynopsis(t *testing.T) {
	border := FormatPaneBorder("auth", "running", 0, "Adding OAuth2 support")
	if !strings.Contains(border, "Adding OAuth2 support") {
		t.Error("expected synopsis in border")
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
