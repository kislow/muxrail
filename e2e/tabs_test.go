//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

var escapes = regexp.MustCompile(`\x1b\[[0-9;:?]*[A-Za-z]|\x1b\(B|\x1b[=>]`)

const (
	claudeLogo = ""
	openaiLogo = ""
	train      = "\U0001F686"
)

func tabCmd() string {
	if cmd := os.Getenv("MUXRAIL_TAB_CMD"); cmd != "" {
		return cmd
	}
	bin := os.Getenv("MUXRAIL_BIN")
	if bin == "" {
		bin = "muxrail"
	}
	return bin + " tab"
}

// TestTabs reads the status line back from the client's output: a plain shell, a fake `claude` process and a fake
// `codex` process each get their own icon in front of the window name.
func TestTabs(t *testing.T) {
	s := newServer(t, "tabs")
	bin := filepath.Join(s.dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"claude", "codex"} {
		// A copy would be killed by macOS code signing; the process name is the link's.
		if err := os.Symlink("/bin/sleep", filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	cmd := tabCmd()
	s.start("set -g status on\nset -g status-interval 1\nset -g status-left ''\nset -g status-right ''\n"+
		"set -g window-status-separator ''\n"+
		fmt.Sprintf("set -g window-status-format \" #I #(%s #{window_id})#W \"\n", cmd)+
		fmt.Sprintf("set -g window-status-current-format \" #I #(%s #{window_id} plain)#W \"\n", cmd), 140, 30, "plainwin")
	s.tm("new-window", "-t", "t:", "-n", "fakeclaude", "-d", bin+"/claude 300")
	s.tm("new-window", "-t", "t:", "-n", "fakecodex", "-d", bin+"/codex 300")
	time.Sleep(time.Second)

	c := s.attach(30, 140)
	wanted := []struct{ name, window, glyph string }{
		{"plain window shows a train", "plainwin", train},
		{"claude window shows the Claude logo", "fakeclaude", claudeLogo},
		{"codex window shows the OpenAI logo", "fakecodex", openaiLogo},
	}
	found := make([]bool, len(wanted))
	seen := ""
	for deadline := time.Now().Add(25 * time.Second); time.Now().Before(deadline); time.Sleep(300 * time.Millisecond) {
		seen = escapes.ReplaceAllString(c.output(), "")
		done := true
		for i, w := range wanted {
			found[i] = regexp.MustCompile(regexp.QuoteMeta(w.glyph) + `\s*` + w.window).MatchString(seen)
			done = done && found[i]
		}
		if done {
			break
		}
	}
	for i, w := range wanted {
		t.Run(w.name, func(t *testing.T) {
			if !found[i] {
				t.Errorf("no %q before %q in the client output: %q", w.glyph, w.window, tail(seen, 600))
			}
		})
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
