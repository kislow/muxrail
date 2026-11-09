package ui

import (
	"testing"

	"github.com/kislow/muxrail/internal/term"
	"github.com/kislow/muxrail/internal/windows"
)

func TestTabIcon(t *testing.T) {
	tests := []struct {
		name          string
		kind, command string
		plain         bool
		want          string
	}{
		{"claude", "claude", "claude", false, "#[fg=colour173]#[fg=default] "},
		{"claude plain", "claude", "claude", true, " "},
		{"codex", "codex", "node", false, "#[fg=cyan]#[fg=default] "},
		{"cursor", "cursor", "node", false, "#[fg=brightwhite]#[fg=default] "},
		{"gemini", "gemini", "node", false, "#[fg=blue]#[fg=default] "},
		{"unrecognised agent shows a locomotive without padding", "", "9.9.9", false, "#[fg=magenta]\U0001F682#[fg=default]"},
		{"plain window shows a train", "", "zsh", false, "#[fg=brightblack]\U0001F686#[fg=default]"},
		{"plain window on the current tab", "", "zsh", true, "\U0001F686"},
		{"locomotive on the current tab", "", "1.2.3", true, "\U0001F682"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TabIcon(tt.kind, tt.command, tt.plain); got != tt.want {
				t.Errorf("TabIcon = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIconsOccupyTwoColumns(t *testing.T) {
	for _, w := range []*windows.Win{{Kind: "claude"}, {Kind: "codex"}, {Kind: "cursor"}, {Kind: "gemini"}, {Command: "1.1.1"}, {Command: "zsh"}} {
		if got := term.VisibleWidth(agentIcon(w)); got != 2 {
			t.Errorf("icon of %+v is %d columns, want 2", w, got)
		}
	}
}

func TestAgentIconColours(t *testing.T) {
	tests := []struct {
		win  windows.Win
		want string
	}{
		{windows.Win{Kind: "claude"}, "\x1b[38;5;173m \x1b[39m"},
		{windows.Win{Kind: "codex"}, cyan + " " + fgReset},
		{windows.Win{Command: "9.9.9"}, magenta + "\U0001F682" + fgReset},
		{windows.Win{Command: "zsh"}, dim + "\U0001F686" + fgReset},
	}
	for _, tt := range tests {
		if got := agentIcon(&tt.win); got != tt.want {
			t.Errorf("agentIcon(%+v) = %q, want %q", tt.win, got, tt.want)
		}
	}
}
