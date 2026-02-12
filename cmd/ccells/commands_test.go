package main

import "testing"

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"bare ccells", []string{}, "up"},
		{"up", []string{"up"}, "up"},
		{"attach", []string{"attach"}, "attach"},
		{"down", []string{"down"}, "down"},
		{"ps", []string{"ps"}, "ps"},
		{"create", []string{"create"}, "create"},
		{"rm", []string{"rm"}, "rm"},
		{"pause", []string{"pause"}, "pause"},
		{"unpause", []string{"unpause"}, "unpause"},
		{"logs", []string{"logs"}, "logs"},
		{"pair", []string{"pair"}, "pair"},
		{"unpair", []string{"unpair"}, "unpair"},
		{"status", []string{"status"}, "status"},
		{"merge", []string{"merge"}, "merge"},
		{"version long", []string{"--version"}, "version"},
		{"version short", []string{"-v"}, "version"},
		{"help long", []string{"--help"}, "help"},
		{"help short", []string{"-h"}, "help"},
		{"runtime then command", []string{"--runtime", "claude", "create"}, "create"},
		{"runtime then nothing", []string{"--runtime", "claude"}, "up"},
		{"unknown", []string{"foobar"}, "help"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCommand(tt.args)
			if got != tt.want {
				t.Errorf("parseCommand(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantRuntime string
		wantRest    []string
	}{
		{"no flags", []string{"up"}, "", []string{"up"}},
		{"runtime flag", []string{"--runtime", "claude", "create"}, "claude", []string{"create"}},
		{"runtime flag only", []string{"--runtime", "claudesp"}, "claudesp", []string{}},
		{"runtime in middle", []string{"create", "--runtime", "claude"}, "claude", []string{"create"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime, rest := parseFlags(tt.args)
			if runtime != tt.wantRuntime {
				t.Errorf("runtime = %q, want %q", runtime, tt.wantRuntime)
			}
			if len(rest) != len(tt.wantRest) {
				t.Errorf("rest = %v, want %v", rest, tt.wantRest)
				return
			}
			for i, v := range rest {
				if v != tt.wantRest[i] {
					t.Errorf("rest[%d] = %q, want %q", i, v, tt.wantRest[i])
				}
			}
		})
	}
}
