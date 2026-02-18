package main

import (
	"strings"
	"testing"
)

func TestFormatPS(t *testing.T) {
	workstreams := []PSEntry{
		{Name: "auth-system", Status: "running", Branch: "feat/auth", PR: "#42"},
		{Name: "fix-bug", Status: "paused", Branch: "fix/login", PR: ""},
	}
	out := formatPS(workstreams)
	if !strings.Contains(out, "auth-system") {
		t.Error("expected auth-system in output")
	}
	if !strings.Contains(out, "paused") {
		t.Error("expected paused in output")
	}
	if !strings.Contains(out, "#42") {
		t.Error("expected PR #42 in output")
	}
	if !strings.Contains(out, "fix/login") {
		t.Error("expected branch fix/login in output")
	}
}

func TestFormatPSEmpty(t *testing.T) {
	out := formatPS(nil)
	if !strings.Contains(out, "No workstreams") {
		t.Error("expected 'No workstreams' message for empty list")
	}
}

func TestFormatPSHeader(t *testing.T) {
	workstreams := []PSEntry{
		{Name: "test", Status: "running", Branch: "main", PR: ""},
	}
	out := formatPS(workstreams)
	if !strings.Contains(out, "NAME") {
		t.Error("expected header row with NAME")
	}
	if !strings.Contains(out, "STATUS") {
		t.Error("expected header row with STATUS")
	}
}
