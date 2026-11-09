package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/kislow/muxrail/internal/gitinfo"
	"github.com/kislow/muxrail/internal/term"
	"github.com/kislow/muxrail/internal/windows"
)

var epoch = time.Unix(1_700_000_000, 0)

func testEnv() *Env {
	return &Env{Now: epoch, Home: "/Users/jordan", CapL: "", CapR: ""}
}

func plain(s string) string { return term.StripANSI(s) }

// fixtures covers every card shape: plain with and without git, agents with title, status, git, a version-named
// auto-titled window, a long name, and an unrecognised agent.
func fixtures() []*windows.Win {
	repo := &gitinfo.Info{Branch: "feat/a-rather-long-branch-name", Committed: gitinfo.Lines{Added: 1300, Deleted: 12}, Dirty: gitinfo.Lines{Added: 4, Deleted: 2}}
	return []*windows.Win{
		{ID: "@1", Index: 0, Active: true, Command: "zsh", Path: "/Users/jordan/src/muxrail", Name: "shell", Git: repo},
		{ID: "@2", Index: 1, Command: "zsh", Path: "/var/folders/some/very/deep/path/here", Name: "plain"},
		{ID: "@3", Index: 2, Command: "claude", Kind: "claude", Status: "working", Since: float64(epoch.Unix() - 125),
			Path: "/Users/jordan/repos/acme/platform-api", Name: "fix-the-flaky-test-in-ci", Title: "✳ Fix flaky CI", Git: repo},
		{ID: "@4", Index: 3, Command: "2.1.319", Kind: "claude", Status: "waiting", Since: float64(epoch.Unix() - 7300),
			Path: "/tmp/x", Name: "2.1.319", Title: "✳ Refactor the session picker"},
		{ID: "@5", Index: 4, Command: "codex", Kind: "codex", Status: "done", Since: float64(epoch.Unix() - 90000),
			Path: "/Users/jordan/p", Name: "codex-run", Title: "Add tests", Git: &gitinfo.Info{Branch: "main"}},
		{ID: "@6", Index: 5, Command: "9.9.9", Path: "/Users/jordan", Name: "mystery"},
		{ID: "@7", Index: 6, Command: "node", Kind: "cursor", Path: "/Users/jordan/日本語/プロジェクト", Name: "日本語のとても長い名前のウィンドウ", Title: "user@host:~/x"},
	}
}

func rows(frame string) []string {
	frame = strings.TrimPrefix(frame, "\x1b[H")
	return strings.Split(frame, clearEOL+"\r\n")
}

func assertWidthAtMost(t *testing.T, width int, lines []string) {
	t.Helper()
	for i, l := range lines {
		if w := term.VisibleWidth(strings.TrimSuffix(l, clearEOL)); w > width {
			t.Errorf("row %d is %d columns wide, pane is %d: %q", i, w, width, plain(l))
		}
	}
}
