package main

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRunExitCodes(t *testing.T) {
	t.Setenv("TMUX_PANE", "") // the hook cases must not write into the real state directory
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"no command prints usage", nil, 2},
		{"unknown command", []string{"frobnicate"}, 2},
		{"plan needs three arguments", []string{"plan", "claude"}, 1},
		{"tab needs a window id", []string{"tab"}, 1},
		{"tab only knows plain", []string{"tab", "@1", "fancy"}, 1},
		{"--once needs a number", []string{"--once", "wide"}, 1},
		{"--once needs the columns", []string{"--once"}, 1},
		{"show needs agent, id and folder", []string{"show", "claude", "x-1"}, 1},
		{"search takes a state file", []string{"search", "--state"}, 1},
		{"search cycle needs a state file", []string{"search", "cycle", "agent"}, 1},
		{"search header needs a state file", []string{"search", "header"}, 1},
		{"sidebar takes an action and a pane", []string{"sidebar", "toggle"}, 1},
		{"sidebar knows its actions", []string{"sidebar", "render", "%1"}, 1},
		{"hook with no arguments still succeeds", []string{"hook"}, 0},
		{"hook with a bad argument still succeeds", []string{"hook", "--nope", "bogus"}, 0},
		{"hook outside tmux succeeds", []string{"hook", "working"}, 0},
		{"version prints and succeeds", []string{"version"}, 0},
		{"--version too", []string{"--version"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := run(context.Background(), tt.args); got != tt.want {
				t.Errorf("run(%v) = %d, want %d", tt.args, got, tt.want)
			}
		})
	}
}

func TestBinaryPath(t *testing.T) {
	t.Setenv("MUXRAIL_BIN", "/opt/muxrail/bin/muxrail")
	if got, err := binaryPath(); err != nil || got != "/opt/muxrail/bin/muxrail" {
		t.Errorf("binaryPath = %q, %v; want MUXRAIL_BIN", got, err)
	}
	t.Setenv("MUXRAIL_BIN", "")
	got, err := binaryPath()
	if err != nil || !filepath.IsAbs(got) {
		t.Errorf("binaryPath = %q, %v; want the absolute path of the running binary", got, err)
	}
}
