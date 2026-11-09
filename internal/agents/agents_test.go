package agents

import (
	"context"
	"reflect"
	"testing"

	"github.com/kislow/muxrail/internal/testutil"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name       string
		comm, args string
		want       string
	}{
		{"claude by name", "claude", "claude --resume x", Claude},
		{"claude by version in comm", "2.1.319", "2.1.319", Claude},
		{"claude by version in args when comm is truncated", "/Users/jordan/.local/", "/Users/jordan/.local/share/claude/versions/2.1.319 --continue", Claude},
		{"claude path", "/opt/homebrew/bin/claude", "/opt/homebrew/bin/claude", Claude},
		{"codex", "codex", "codex", Codex},
		{"codex via node wrapper path in args", "/Users/jordan/.n", "/Users/jordan/.nvm/bin/codex --full-auto", Codex},
		{"agy is gemini", "agy", "agy", Gemini},
		{"gemini", "gemini", "gemini -m x", Gemini},
		{"cursor agent through node", "node", "agent /Users/x/cursor-agent/versions/1/index.js", Cursor},
		{"cursor-agent through node", "node", "cursor-agent /x/index.js --print", Cursor},
		{"agent without index.js is not cursor", "agent", "agent run", ""},
		{"node server", "node", "node server.js", ""},
		{"login shell", "-zsh", "-zsh", ""},
		{"version-like but four parts", "1.2.3.4", "1.2.3.4", ""},
		{"empty", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.comm, tt.args); got != tt.want {
				t.Errorf("Classify(%q, %q) = %q, want %q", tt.comm, tt.args, got, tt.want)
			}
		})
	}
}

func TestIsVersion(t *testing.T) {
	for in, want := range map[string]bool{"2.1.319": true, "10.0.1": true, "2.1": false, "v2.1.3": false, "": false, "a.b.c": false} {
		if got := IsVersion(in); got != want {
			t.Errorf("IsVersion(%q) = %v, want %v", in, got, want)
		}
	}
}

const psSample = `ttys001 Ss+  -zsh                /bin/zsh -l
ttys002 S+   claude              claude --continue
ttys003 S+   node                node /x/cursor-agent/index.js
ttys003 S+   agent               agent /x/cursor-agent/index.js
ttys004 S    codex               codex
ttys005 R+   /Users/jordan/.local/ /Users/jordan/.local/share/claude/versions/2.1.319 --x
?? Ss   /usr/sbin/syslogd        /usr/sbin/syslogd
ttys006 S+   node                node server.js
`

func TestForegroundKinds(t *testing.T) {
	want := map[string]string{"ttys002": Claude, "ttys003": Cursor, "ttys005": Claude}
	if got := ForegroundKinds(psSample); !reflect.DeepEqual(got, want) {
		t.Errorf("ForegroundKinds = %v, want %v", got, want)
	}
}

func TestMergeStates(t *testing.T) {
	snap := Snapshot{"%1": {Kind: Claude}}
	mergeStates(snap, map[string]State{
		"%1": {Status: "working", UpdatedTS: 100, Kind: Codex},
		"%2": {Status: "waiting", UpdatedTS: 200, Kind: Gemini},
	})
	if got := snap["%1"]; got.Kind != Claude || got.Status != "working" || got.Since != 100 {
		t.Errorf("%%1 = %+v: the process-detected kind must win and status must merge in", got)
	}
	if got := snap["%2"]; got.Kind != Gemini || got.Status != "waiting" {
		t.Errorf("%%2 = %+v: the file's kind must fill in for a pane process detection missed", got)
	}
}

const testSocket = "/tmp/tmux-501/default"

func stubWorld(t *testing.T) string {
	t.Helper()
	testutil.IsolatedTmp(t)
	testutil.NoTmux(t)
	testutil.Stub(t, "tmux", `case "$1" in
display-message) echo '`+testSocket+`' ;;
*) printf '%%1 /dev/ttys002\n%%2 /dev/ttys009\n' ;;
esac`)
	testutil.Stub(t, "ps", `cat <<'EOF'
ttys002 S+ claude claude
ttys009 S+ -zsh -zsh
EOF`)
	return serverDir(t, testSocket)
}

// serverDir is where the hooks of the tmux server on socket keep their files.
func serverDir(t *testing.T, socket string) string {
	t.Helper()
	dir, err := stateDir(socket)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadBuildsFromProcessesAndHookState(t *testing.T) {
	dir := stubWorld(t)
	if err := writeState(dir, "%2", State{Status: "done", UpdatedTS: 1700000000, Kind: Codex}); err != nil {
		t.Fatal(err)
	}
	snap, err := Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := Snapshot{"%1": {Kind: Claude}, "%2": {Kind: Codex, Status: "done", Since: 1700000000}}
	if !reflect.DeepEqual(snap, want) {
		t.Errorf("snapshot = %v, want %v", snap, want)
	}
}

func TestLoadCarriesOnWithoutHookState(t *testing.T) {
	stubWorld(t)
	snap, err := Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := (Snapshot{"%1": {Kind: Claude}}); !reflect.DeepEqual(snap, want) {
		t.Errorf("snapshot = %v, want %v", snap, want)
	}
}

func TestLoadDeletesStateOfClosedPanes(t *testing.T) {
	dir := stubWorld(t)
	for _, pane := range []string{"%2", "%7"} {
		if err := writeState(dir, pane, State{Status: "done", UpdatedTS: 1, Kind: Claude}); err != nil {
			t.Fatal(err)
		}
	}
	snap, err := Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap["%7"] != nil {
		t.Errorf("closed pane %%7 is in the snapshot: %+v", snap["%7"])
	}
	if _, ok := readState(dir, "%7"); ok {
		t.Error("the state file of the closed pane %7 is still there")
	}
	if _, ok := readState(dir, "%2"); !ok {
		t.Error("the state file of the live pane %2 was deleted")
	}
}

func TestLoadNeverTouchesAnotherServersState(t *testing.T) {
	a := stubWorld(t)
	b := serverDir(t, "/tmp/tmux-501/agents-hub")
	for _, dir := range []string{a, b} {
		if err := writeState(dir, "%7", State{Status: "done", UpdatedTS: 1, Kind: Claude}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := readState(a, "%7"); ok {
		t.Error("the closed pane %7 of the server being read kept its file")
	}
	if got, ok := readState(b, "%7"); !ok || got.Status != "done" {
		t.Errorf("another server's %%7 = %+v, %v, want it left alone: that server has its own %%7", got, ok)
	}
}

func TestLoadWithoutAServerSocketKeepsAllState(t *testing.T) {
	dir := stubWorld(t)
	if err := writeState(dir, "%7", State{Status: "done", UpdatedTS: 1, Kind: Claude}); err != nil {
		t.Fatal(err)
	}
	testutil.Stub(t, "tmux", `case "$1" in
display-message) exit 1 ;;
*) printf '%%1 /dev/ttys002\n' ;;
esac`)
	snap, err := Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap["%1"] == nil || snap["%7"] != nil {
		t.Errorf("snapshot = %v, want process detection alone", snap)
	}
	if _, ok := readState(dir, "%7"); !ok {
		t.Error("a file was deleted although the server could not be told apart")
	}
}

func TestHookShowsWithinTheCacheTTL(t *testing.T) {
	stubWorld(t)
	if _, err := Load(context.Background()); err != nil { // fills the cache
		t.Fatal(err)
	}
	t.Setenv("TMUX", testSocket+",4242,0")
	t.Setenv("TMUX_PANE", "%1")
	Hook([]string{"waiting"})
	snap, err := Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := snap["%1"]; got == nil || got.Status != "waiting" {
		t.Errorf("%%1 = %+v, want the waiting status the hook just wrote", got)
	}
}

func TestLoadServesTheCacheForAShortWhile(t *testing.T) {
	stubWorld(t)
	first, err := Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	testutil.Stub(t, "tmux", `echo boom >&2; exit 1`) // a rebuild would now fail
	second, err := Load(context.Background())
	if err != nil {
		t.Fatalf("second load should come from the cache: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("cached snapshot %v differs from %v", second, first)
	}
}

func TestLoadReportsTmuxFailure(t *testing.T) {
	stubWorld(t)
	testutil.Stub(t, "tmux", `exit 1`)
	if _, err := Load(context.Background()); err == nil {
		t.Error("want an error when tmux fails and there is no cache")
	}
}
