package ui

import (
	"testing"
	"time"

	"github.com/kislow/muxrail/internal/agents"
	"github.com/kislow/muxrail/internal/windows"
)

func TestRailTab(t *testing.T) {
	claude := TabIcon("claude", "claude", false)
	tests := []struct {
		name    string
		tab     windows.Tab
		current bool
		want    string
	}{
		{"working", windows.Tab{Kind: "claude", Command: "claude", Status: "working"}, false,
			"#[fg=cyan]● #[fg=brightblack]2#[default] " + claude},
		{"waiting", windows.Tab{Kind: "claude", Command: "claude", Status: "waiting"}, false,
			"#[fg=yellow]◆ #[fg=brightblack]2#[default] " + claude},
		{"done", windows.Tab{Kind: "claude", Command: "claude", Status: "done"}, false,
			"#[fg=green]✔ #[fg=brightblack]2#[default] " + claude},
		{"an agent without a state is idle", windows.Tab{Kind: "claude", Command: "claude"}, false,
			"#[fg=brightblack]○ #[fg=brightblack]2#[default] " + claude},
		{"a plain window is idle", windows.Tab{Command: "zsh"}, false,
			"#[fg=brightblack]○ #[fg=brightblack]2#[default] " + TabIcon("", "zsh", false) + " "},
		{"the current tab is bold and green when idle", windows.Tab{Command: "zsh"}, true,
			"#[fg=green,bold]◉ #[fg=default,bold]2#[default] " + TabIcon("", "zsh", false) + " "},
		{"the current tab keeps its agent's state colour", windows.Tab{Kind: "claude", Command: "claude", Status: "waiting"}, true,
			"#[fg=yellow,bold]◆ #[fg=default,bold]2#[default] " + claude},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RailTab(tt.tab, "2", tt.current); got != tt.want {
				t.Errorf("RailTab = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatusLine(t *testing.T) {
	now := time.Unix(10_000, 0)
	tests := []struct {
		name string
		in   agents.Summary
		want string
	}{
		{"nothing to report", agents.Summary{}, ""},
		{"waiting leads, with the longest wait", agents.Summary{Working: 2, Waiting: 1, Done: 3, WaitingSince: 10_000 - 180},
			"#[fg=yellow]◆ 1 3m#[default]  #[fg=cyan]● 2#[default]  #[fg=green]✔ 3#[default]  "},
		{"zero counts are left out", agents.Summary{Working: 1},
			"#[fg=cyan]● 1#[default]  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StatusLine(tt.in, now); got != tt.want {
				t.Errorf("StatusLine = %q, want %q", got, tt.want)
			}
		})
	}
}
