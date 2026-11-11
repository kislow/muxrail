package windows

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kislow/muxrail/internal/activity"
	"github.com/kislow/muxrail/internal/testutil"
)

// paneLines joins fields with the unit separator the way tmux prints them.
func paneLines(rows ...[]string) string {
	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = strings.Join(r, "\x1f")
	}
	return strings.Join(lines, "\n")
}

// row: window_id window_index window_active pane_id pane_active @sidebar command path name title
func row(id, idx, wactive, pane, pactive, sidebar, cmd, path, name, title string) []string {
	return []string{id, idx, wactive, pane, pactive, sidebar, cmd, path, name, title}
}

// writeAgentState leaves the file `muxrail hook` would write for the pane (its id without the %).
func writeAgentState(t *testing.T, pane, status string, ts int64) {
	t.Helper()
	dir := filepath.Join(os.Getenv("XDG_STATE_HOME"), "muxrail", "agents", "default")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	doc := fmt.Sprintf(`{"status":%q,"updated_ts":%d,"kind":"claude"}`, status, ts)
	if err := os.WriteFile(filepath.Join(dir, pane+".json"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
}

func stubTmux(t *testing.T, listOutput, paneTTYs string) {
	t.Helper()
	testutil.IsolatedTmp(t)
	testutil.NoTmux(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "list"), []byte(listOutput+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Stub(t, "tmux", `case "$1" in
display-message) echo /tmp/tmux-501/default ;;
list-panes) if [ "$2" = "-a" ]; then printf '%s\n' '`+paneTTYs+`'; else cat '`+filepath.Join(dir, "list")+`'; fi ;;
esac`)
	testutil.Stub(t, "ps", `echo "ttys002 S+ claude claude"`)
	writeAgentState(t, "3", "working", 1700000000)
}

func TestListChoosesOnePanePerWindowAndSkipsSidebars(t *testing.T) {
	stubTmux(t, paneLines(
		row("@1", "0", "1", "%0", "0", "1", "muxrail", "/", "a", ""), // sidebar
		row("@1", "0", "1", "%1", "1", "", "zsh", "/work", "a", "title"),
		row("@2", "1", "0", "%2", "1", "", "zsh", "/p/shell", "b", ""),
		row("@2", "1", "0", "%3", "0", "", "claude", "/p/agent", "b", "✳ fix it"), // agent beats the active pane
		row("@3", "2", "0", "%4", "0", "", "vim", "/p/one", "c", ""),
		row("@3", "2", "0", "%5", "0", "", "vim", "/p/two", "c", ""), // tie: first one stays
		row("@4", "3", "0", "%6", "1", "", "zsh", "/p/e", "d", ""),
	), "%1 /dev/ttys001\n%3 /dev/ttys002")
	wins, err := List(context.Background(), "$1")
	if err != nil {
		t.Fatal(err)
	}
	if len(wins) != 4 {
		t.Fatalf("got %d windows, want 4: %+v", len(wins), wins)
	}
	checks := []struct {
		i              int
		id, path, kind string
		status         string
		active         bool
	}{
		{0, "@1", "/work", "", "", true},
		{1, "@2", "/p/agent", "claude", "working", false},
		{2, "@3", "/p/one", "", "", false},
		{3, "@4", "/p/e", "", "", false},
	}
	for _, c := range checks {
		w := wins[c.i]
		if w.ID != c.id || w.Path != c.path || w.Kind != c.kind || w.Status != c.status || w.Active != c.active {
			t.Errorf("window %d = %+v, want id=%s path=%s kind=%q status=%q active=%v", c.i, w, c.id, c.path, c.kind, c.status, c.active)
		}
	}
	if wins[1].Since != 1700000000 || wins[1].Title != "✳ fix it" {
		t.Errorf("agent window since/title = %v / %q", wins[1].Since, wins[1].Title)
	}
}

func TestListKeepsALineWhosePaneTitleIsEmpty(t *testing.T) {
	stubTmux(t, paneLines(row("@1", "0", "1", "%1", "1", "", "zsh", "/w", "new", "")), "")
	wins, err := List(context.Background(), "$1")
	if err != nil {
		t.Fatal(err)
	}
	if len(wins) != 1 || wins[0].Name != "new" || wins[0].Title != "" {
		t.Errorf("got %+v", wins)
	}
}

func TestListRejectsAMalformedLine(t *testing.T) {
	stubTmux(t, "@1\x1f0\x1f1", "")
	if _, err := List(context.Background(), "$1"); err == nil {
		t.Error("want an error for a line with too few fields")
	}
}

func TestIsAgent(t *testing.T) {
	tests := []struct {
		name string
		win  Win
		want bool
	}{
		{"known kind", Win{Kind: "codex", Command: "node"}, true},
		{"version command", Win{Command: "2.1.319"}, true},
		{"shell", Win{Command: "zsh"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.win.IsAgent(); got != tt.want {
				t.Errorf("IsAgent = %v, want %v", got, tt.want)
			}
		})
	}
}

func recordTmux(t *testing.T) string {
	t.Helper()
	log := filepath.Join(t.TempDir(), "calls")
	testutil.NoTmux(t)
	testutil.Stub(t, "tmux", `echo "$@" >> '`+log+`'`)
	return log
}

func calls(t *testing.T, log string) string {
	t.Helper()
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

func TestSwitch(t *testing.T) {
	log := recordTmux(t)
	if err := Switch(context.Background(), "$2", "%9", &Win{Index: 3}); err != nil {
		t.Fatal(err)
	}
	want := "select-window -t $2:3 ; if-shell -F -t %9 #{pane_active} last-pane -t %9"
	if got := calls(t, log); got != want {
		t.Errorf("tmux call = %q, want %q", got, want)
	}
}

func TestMove(t *testing.T) {
	wins := []*Win{{ID: "@a", Index: 0, Active: true}, {ID: "@b", Index: 1}, {ID: "@c", Index: 2}}
	tests := []struct {
		name     string
		src, dst int
		want     string
	}{
		{"down moves after the target, -d keeps the selection", 1, 2, "move-window -a -d -s @b -t @c ; move-window -r -t $1"},
		{"up moves before the target", 2, 1, "move-window -b -d -s @c -t @b ; move-window -r -t $1"},
		{"the active window is moved without -d", 0, 2, "move-window -a -s @a -t @c ; move-window -r -t $1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := recordTmux(t)
			if err := Move(context.Background(), "$1", wins, tt.src, tt.dst); err != nil {
				t.Fatal(err)
			}
			if got := calls(t, log); got != tt.want {
				t.Errorf("tmux call = %q, want %q", got, tt.want)
			}
		})
	}
	t.Run("out of range", func(t *testing.T) {
		recordTmux(t)
		if err := Move(context.Background(), "$1", wins, 0, 7); err == nil {
			t.Error("want an error")
		}
	})
}

func TestTabKind(t *testing.T) {
	testutil.IsolatedTmp(t)
	testutil.NoTmux(t)
	testutil.Stub(t, "tmux", `if [ "$2" = "-a" ]; then echo '%2 /dev/ttys002'; exit 0; fi
printf '%%1 1  zsh\n%%2 0  claude\n%%9 0 1 muxrail\n'`)
	testutil.Stub(t, "ps", `echo "ttys002 S+ claude claude"`)
	tab, ok, err := TabOf(context.Background(), "@1")
	if err != nil || !ok || tab.Kind != "claude" || tab.Command != "claude" {
		t.Errorf("TabOf = %+v, %v, %v; want the agent pane to beat the active shell", tab, ok, err)
	}
}

func TestTabOfCarriesTheAgentState(t *testing.T) {
	testutil.IsolatedTmp(t)
	testutil.NoTmux(t)
	testutil.Stub(t, "tmux", `if [ "$1" = display-message ]; then echo /tmp/tmux-501/default; exit 0; fi
if [ "$2" = "-a" ]; then echo '%2 /dev/ttys002'; exit 0; fi
printf '%%2 1  claude\n'`)
	testutil.Stub(t, "ps", `echo "ttys002 S+ claude claude"`)
	writeAgentState(t, "2", "waiting", 1700000000)
	tab, ok, err := TabOf(context.Background(), "@1")
	if err != nil || !ok || tab.Status != "waiting" || tab.Since != 1700000000 {
		t.Errorf("TabOf = %+v, %v, %v; want the waiting state and its start", tab, ok, err)
	}
}

func TestTabOfWithOnlySidebars(t *testing.T) {
	testutil.IsolatedTmp(t)
	testutil.NoTmux(t)
	testutil.Stub(t, "tmux", `if [ "$2" = "-a" ]; then exit 0; fi
printf '%%9 1 1 muxrail\n'`)
	testutil.Stub(t, "ps", `exit 0`)
	if _, ok, err := TabOf(context.Background(), "@1"); err != nil || ok {
		t.Errorf("ok=%v err=%v, want no pane", ok, err)
	}
}

func TestAttachActivity(t *testing.T) {
	home := t.TempDir()
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(home, ".claude", "sessions", "1.json"), `{"sessionId":"abc","tmux":"main:@1.%3","startedAt":1}`)
	write(filepath.Join(home, ".claude", "projects", "-p", "abc.jsonl"),
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/x/a.go"}}],"usage":{"input_tokens":5,"cache_read_input_tokens":1000}}}`+"\n")
	wins := []*Win{
		{ID: "@1", Pane: "%3", Kind: "claude"},
		{ID: "@2", Pane: "%4", Kind: "claude"},
		{ID: "@1", Pane: "%3", Kind: "codex"},
	}
	if err := AttachActivity(home, wins, time.Now()); err != nil {
		t.Fatal(err)
	}
	if a := wins[0].Activity; a == nil || a.Action != "✎ Edit a.go" || a.Context != 1005 {
		t.Errorf("claude window Activity = %+v, want the Edit and 1005 tokens", a)
	}
	if wins[1].Activity != nil {
		t.Errorf("a pane without a session got %+v", wins[1].Activity)
	}
	if wins[2].Activity != nil {
		t.Errorf("only Claude windows read a transcript, got %+v", wins[2].Activity)
	}
}

func TestGitPathFollowsTheAgentNotThePane(t *testing.T) {
	tests := []struct {
		name string
		win  Win
		want string
	}{
		{"no activity", Win{Path: "/home/me"}, "/home/me"},
		{"activity without a folder", Win{Path: "/home/me", Activity: &activity.Info{}}, "/home/me"},
		{"the agent moved into a repo", Win{Path: "/home/me", Activity: &activity.Info{Cwd: "/home/me/src/api"}}, "/home/me/src/api"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gitPath(&tt.win); got != tt.want {
				t.Errorf("gitPath = %q, want %q", got, tt.want)
			}
		})
	}
}
