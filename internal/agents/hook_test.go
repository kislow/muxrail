package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kislow/muxrail/internal/testutil"
)

// hookWorld runs the hook as pane of the tmux server on testSocket and returns that server's state directory.
func hookWorld(t *testing.T, pane string) string {
	t.Helper()
	testutil.IsolatedTmp(t)
	t.Setenv("TMUX", testSocket+",4242,0")
	t.Setenv("TMUX_PANE", pane)
	return serverDir(t, testSocket)
}

func TestHookWritesWhatReadStatesReads(t *testing.T) {
	dir := hookWorld(t, "%12")
	before := float64(time.Now().Unix())
	Hook([]string{"working"})
	got := readStates(dir)["%12"]
	if got.Status != "working" || got.Kind != Claude || got.UpdatedTS < before {
		t.Errorf("state = %+v, want working for claude, stamped now", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "12.json")); err != nil {
		t.Errorf("the file is not named after the pane id without the %%: %v", err)
	}
}

func TestHookStartRegistersWithoutStatus(t *testing.T) {
	dir := hookWorld(t, "%3")
	Hook([]string{"--kind", "codex", "start"})
	got, ok := readState(dir, "%3")
	if !ok || got.Status != "" || got.Kind != Codex || got.UpdatedTS == 0 {
		t.Errorf("state = %+v, %v, want an empty status, kind codex and the start time", got, ok)
	}
}

func TestHookKeepsWhenTheStatusBeganWhileItRepeats(t *testing.T) {
	dir := hookWorld(t, "%3")
	if err := writeState(dir, "%3", State{Status: "working", UpdatedTS: 100, Kind: Claude}); err != nil {
		t.Fatal(err)
	}
	Hook([]string{"working"})
	if got, _ := readState(dir, "%3"); got.UpdatedTS != 100 {
		t.Errorf("updated_ts = %v, want 100: a repeated status must not restart the clock", got.UpdatedTS)
	}
	Hook([]string{"done"})
	if got, _ := readState(dir, "%3"); got.Status != "done" || got.UpdatedTS == 100 {
		t.Errorf("state = %+v, want a new status with a new time", got)
	}
}

func TestHookWithoutAPaneDoesNothing(t *testing.T) {
	for _, pane := range []string{"", "12", "%", "%../x", "%1/2"} {
		dir := hookWorld(t, pane)
		Hook([]string{"working"})
		if _, err := os.Stat(dir); err == nil {
			t.Errorf("TMUX_PANE=%q created %s", pane, dir)
		}
	}
}

func TestHookKeepsServersWithTheSamePaneIdApart(t *testing.T) {
	a := hookWorld(t, "%3")
	b := serverDir(t, "/tmp/tmux-501/agents-hub")
	Hook([]string{"working"})
	t.Setenv("TMUX", "/tmp/tmux-501/agents-hub,777,0")
	Hook([]string{"--kind", "codex", "done"})
	if got, ok := readState(a, "%3"); !ok || got.Status != "working" || got.Kind != Claude {
		t.Errorf("first server's %%3 = %+v, %v, want working for claude: the second server must not overwrite it", got, ok)
	}
	if got, ok := readState(b, "%3"); !ok || got.Status != "done" || got.Kind != Codex {
		t.Errorf("second server's %%3 = %+v, %v, want done for codex", got, ok)
	}
}

func TestHookWithoutATmuxServerDoesNothing(t *testing.T) {
	dir := hookWorld(t, "%3")
	for _, env := range []string{"", ",4242,0"} {
		t.Setenv("TMUX", env)
		Hook([]string{"working"})
	}
	if _, err := os.Stat(filepath.Dir(dir)); err == nil {
		t.Errorf("a hook without a usable $TMUX created %s", filepath.Dir(dir))
	}
}

func TestHookRefusesAnUnsafeSocketName(t *testing.T) {
	dir := hookWorld(t, "%3")
	root := filepath.Dir(dir)
	for _, env := range []string{"/tmp/tmux-501/..,1,0", "/tmp/tmux-501/.,1,0", "..,1,0", "/,1,0"} {
		t.Setenv("TMUX", env)
		Hook([]string{"working"})
		if _, err := os.Stat(root); err == nil {
			t.Fatalf("TMUX=%q created %s", env, root)
		}
	}
}

func TestValidSocket(t *testing.T) {
	for name, want := range map[string]bool{"default": true, "agents-hub": true, "a.b": true, "": false, ".": false, "..": false, "a/b": false, "/": false} {
		if got := validSocket(name); got != want {
			t.Errorf("validSocket(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestHookIgnoresBadArguments(t *testing.T) {
	dir := hookWorld(t, "%1")
	for _, args := range [][]string{nil, {"bogus"}, {"working", "done"}, {"--kind"}, {"--kind", "", "done"}, {"--nope", "done"}, {"-h"}} {
		Hook(args)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("a bad argument wrote state")
	}
}

func TestHookSurvivesAnUnwritableDirectory(t *testing.T) {
	hookWorld(t, "%1")
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", blocker) // a file where the directory should be
	Hook([]string{"working"})
}

func TestWriteStateIsAtomic(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agents")
	for _, status := range []string{"working", "waiting", "done"} {
		if err := writeState(dir, "%5", State{Status: status, UpdatedTS: 1, Kind: Claude}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "5.json" {
		t.Errorf("directory holds %v, want only 5.json: the temp file must be renamed away", entries)
	}
	if got, _ := readState(dir, "%5"); got.Status != "done" {
		t.Errorf("state = %+v, want the last write", got)
	}
}

func TestReadStatesSkipsWhatIsNotAPaneState(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"1.json":      `{"status":"working","updated_ts":1,"kind":"claude"}`,
		"1.json.4242": `{"status":"done"}`, // a write in flight
		"notes.json":  `{"status":"done"}`,
		"2.json":      `not json`,
		"README":      "x",
		"%3.json":     `{"status":"done"}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := readStates(dir)
	if len(got) != 1 || got["%1"].Status != "working" {
		t.Errorf("states = %v, want only %%1", got)
	}
}

func TestStateDirFollowsXDG(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/x/state")
	if got, _ := stateDir("/tmp/tmux-501/default"); got != "/x/state/muxrail/agents/default" {
		t.Errorf("stateDir = %q", got)
	}
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/home/u")
	if got, _ := stateDir("/tmp/tmux-501/default"); !strings.HasSuffix(got, "/home/u/.local/state/muxrail/agents/default") {
		t.Errorf("stateDir = %q, want the default under ~/.local/state", got)
	}
}
