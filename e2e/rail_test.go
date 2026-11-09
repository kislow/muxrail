//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestRailTabs loads the plugin with `@muxrail_tabs rail` and reads the status line back: the current window is the
// bold green stop, a waiting Claude and a working Codex take their states' stations, and the right of the bar counts
// the agents with the longest wait. The states come from `muxrail hook`, the way an agent's hooks would report them.
func TestRailTabs(t *testing.T) {
	s := newServer(t, "rail")
	bin := filepath.Join(s.dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"claude", "codex"} {
		if err := os.Symlink("/bin/sleep", filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(muxrailBin(t), filepath.Join(bin, "muxrail")); err != nil {
		t.Fatal(err)
	}
	s.env = append(s.env, "PATH="+bin+":"+os.Getenv("PATH"))

	s.start("set -g status on\nset -g status-interval 1\nset -g status-left ''\nset -g status-right '#(muxrail status)'\n"+
		"set -g @muxrail_tabs rail\nrun-shell "+pluginPath(t)+"\n", 140, 30, "plainwin")
	s.tm("new-window", "-t", "t:", "-n", "fakeclaude", "-d", bin+"/claude 300")
	s.tm("new-window", "-t", "t:", "-n", "fakecodex", "-d", bin+"/codex 300")

	panes := map[string]string{}
	for _, line := range strings.Split(s.tm("list-panes", "-a", "-F", "#{window_name} #{pane_id}"), "\n") {
		if name, id, ok := strings.Cut(line, " "); ok {
			panes[name] = id
		}
	}
	s.hook(panes["fakeclaude"], "start")
	s.hook(panes["fakeclaude"], "waiting")
	s.hook(panes["fakecodex"], "--kind", "codex", "working")
	time.Sleep(time.Second)

	c := s.attach(30, 140)
	wanted := []struct{ name, pattern string }{
		{"the current window is the green stop on a heavy rail", `━━\s*◉\s*0\s*` + train + `\s*plainwin\s*━━`},
		{"a waiting Claude sits on a yellow diamond", `◆\s*1\s*` + claudeLogo + `\s*fakeclaude`},
		{"a working Codex sits on a filled dot", `●\s*2\s*` + openaiLogo + `\s*fakecodex`},
		{"stops are joined by a light track", `──\s*◆`},
		{"the right of the bar counts waiting with the longest wait first", `◆ 1 now\s+● 1`},
	}
	found := make([]bool, len(wanted))
	seen := ""
	for deadline := time.Now().Add(25 * time.Second); time.Now().Before(deadline); time.Sleep(300 * time.Millisecond) {
		seen = escapes.ReplaceAllString(c.output(), "")
		done := true
		for i, w := range wanted {
			found[i] = regexp.MustCompile(w.pattern).MatchString(seen)
			done = done && found[i]
		}
		if done {
			break
		}
	}
	for i, w := range wanted {
		t.Run(w.name, func(t *testing.T) {
			if !found[i] {
				t.Errorf("no match for %q in the client output: %q", w.pattern, tail(seen, 700))
			}
		})
	}
}
