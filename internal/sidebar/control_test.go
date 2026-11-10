package sidebar

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakeTmux answers tmux commands from a table keyed by the joined arguments, keeps global options as a real server
// would, and records every call.
type fakeTmux struct {
	options map[string]string
	replies map[string]string
	calls   []string
}

func newFake() *fakeTmux {
	return &fakeTmux{options: map[string]string{}, replies: map[string]string{}}
}

func (f *fakeTmux) run(_ context.Context, args ...string) (string, error) {
	call := strings.Join(args, " ")
	f.calls = append(f.calls, call)
	switch {
	case args[0] == "show-options":
		return f.options[args[len(args)-1]], nil
	case args[0] == "set-option" && args[1] == "-g":
		f.options[args[2]] = args[3]
		return "", nil
	}
	return f.replies[call], nil
}

func (f *fakeTmux) did(prefix string) []string {
	var out []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

func newController(t *testing.T, f *fakeTmux) *Controller {
	t.Helper()
	return &Controller{Tmux: f.run, Bin: "/opt/muxrail/bin/muxrail", LockDir: t.TempDir()}
}

const (
	listSidebars = "list-panes -s -t %5 -F #{pane_id} #{@sidebar}"
	listWindow   = "list-panes -t %5 -F #{@sidebar} #{pane_width} #{window_panes} #{window_width}"
)

func TestApplySharesTheDraggedWidth(t *testing.T) {
	f := newFake()
	f.replies[listWindow] = "0 100 2 200\n1 52 2 200"
	f.replies[listSidebars] = "%1 1\n%2 \n%3 1\n%4 0"
	f.replies["display-message -p -t %1 #{pane_width}"] = "34"
	f.replies["display-message -p -t %3 #{pane_width}"] = "52"
	if err := newController(t, f).Do(context.Background(), "apply", "%5"); err != nil {
		t.Fatal(err)
	}
	if got := f.options["@sidebar_width"]; got != "52" {
		t.Errorf("@sidebar_width = %q, want 52, the width of the sidebar next to the dragged pane", got)
	}
	if got, want := f.did("resize-pane"), []string{"resize-pane -t %1 -x 52"}; !slices.Equal(got, want) {
		t.Errorf("resizes = %q, want only the sidebar that is not 52 wide: %q", got, want)
	}
}

func TestRemember(t *testing.T) {
	tests := []struct {
		name, panes, want string
	}{
		{"normal", "0 100 2 200\n1 52 2 200", "52"},
		{"exactly half the window", "0 100 2 200\n1 100 2 200", "100"},
		{"over half the window", "0 40 2 200\n1 101 2 200", ""},
		{"alone in the window", "1 200 1 200", ""},
		{"no sidebar", "0 100 2 200\n0 40 2 200", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			f.replies[listWindow] = tt.panes
			if err := newController(t, f).Do(context.Background(), "remember", "%5"); err != nil {
				t.Fatal(err)
			}
			if got := f.options["@sidebar_width"]; got != tt.want {
				t.Errorf("@sidebar_width = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWidth(t *testing.T) {
	tests := []struct {
		name, stored string
		want         int
	}{
		{"unset", "", DefaultWidth},
		{"normal", "52", 52},
		{"at the cap", "80", MaxWidth},
		{"over the cap", "81", DefaultWidth},
		{"a window's full width", "200", DefaultWidth},
		{"not a number", "wide", DefaultWidth},
		{"zero", "0", DefaultWidth},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			f.options["@sidebar_width"] = tt.stored
			got, err := newController(t, f).width(context.Background())
			if err != nil || got != tt.want {
				t.Errorf("width = %d, %v; want %d", got, err, tt.want)
			}
		})
	}
}

func TestEnsureSplitsOnlyWideWindows(t *testing.T) {
	tests := []struct {
		name, cols string
		width      string
		wantSplit  string
	}{
		{"below the minimum", "89", "", ""},
		{"at the minimum", "90", "", "split-window -hbdf -l 34 -t %5 -P -F #{pane_id} /opt/muxrail/bin/muxrail sidebar"},
		{"shared width", "140", "41", "split-window -hbdf -l 41 -t %5 -P -F #{pane_id} /opt/muxrail/bin/muxrail sidebar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			f.options["@sidebar_on"], f.options["@sidebar_width"] = "1", tt.width
			f.replies["display-message -p -t %5 #{window_id}"] = "@3"
			f.replies["display-message -p -t %5 #{window_width}"] = tt.cols
			f.replies[tt.wantSplit] = "%9"
			if err := newController(t, f).Do(context.Background(), "ensure", "%5"); err != nil {
				t.Fatal(err)
			}
			got := f.did("split-window")
			if tt.wantSplit == "" && len(got) != 0 || tt.wantSplit != "" && !slices.Equal(got, []string{tt.wantSplit}) {
				t.Errorf("split = %q, want %q", got, tt.wantSplit)
			}
			if tt.wantSplit != "" && !slices.Contains(f.calls, "set-option -p -t %9 @sidebar 1") {
				t.Errorf("the new pane was not marked as a sidebar: %q", f.calls)
			}
		})
	}
}

func TestEnsureQuotesTheBinaryPath(t *testing.T) {
	f := newFake()
	f.options["@sidebar_on"] = "1"
	f.replies["display-message -p -t %5 #{window_width}"] = "120"
	c := newController(t, f)
	c.Bin = "/opt/my bin/muxrail"
	if err := c.Do(context.Background(), "ensure", "%5"); err != nil {
		t.Fatal(err)
	}
	got := f.did("split-window")
	if len(got) != 1 || !strings.HasSuffix(got[0], " '/opt/my bin/muxrail' sidebar") {
		t.Errorf("split = %q, want the path single-quoted", got)
	}
}

func TestEnsureResizesAnExistingSidebar(t *testing.T) {
	f := newFake()
	f.options["@sidebar_on"], f.options["@sidebar_width"] = "1", "40"
	f.replies["display-message -p -t %5 #{window_width}"] = "120"
	f.replies["list-panes -t %5 -F #{pane_id} #{@sidebar}"] = "%1 1\n%5 "
	f.replies["display-message -p -t %1 #{pane_width}"] = "34"
	if err := newController(t, f).Do(context.Background(), "ensure", "%5"); err != nil {
		t.Fatal(err)
	}
	if len(f.did("split-window")) != 0 {
		t.Errorf("split a window that has its sidebar: %q", f.calls)
	}
	if got := f.did("resize-pane"); !slices.Equal(got, []string{"resize-pane -t %1 -x 40"}) {
		t.Errorf("resizes = %q", got)
	}
}

func TestEnsureDoesNothingWhileSidebarsAreOff(t *testing.T) {
	f := newFake()
	f.replies["display-message -p -t %5 #{window_width}"] = "200"
	if err := newController(t, f).Do(context.Background(), "ensure", "%5"); err != nil {
		t.Fatal(err)
	}
	if len(f.did("split-window")) != 0 || len(f.did("display-message")) != 0 {
		t.Errorf("calls = %q, want only the option read", f.calls)
	}
}

func TestEnsureLock(t *testing.T) {
	f := newFake()
	f.options["@sidebar_on"] = "1"
	f.replies["display-message -p -t %5 #{window_id}"] = "@3"
	f.replies["display-message -p -t %5 #{window_width}"] = "120"
	c := newController(t, f)
	lock := filepath.Join(c.LockDir, "tmux-sidebar-@3.lock")

	if err := os.Mkdir(lock, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := c.Do(context.Background(), "ensure", "%5"); err != nil {
		t.Fatal(err)
	}
	if len(f.did("split-window")) != 0 {
		t.Errorf("a held lock must stop a second split: %q", f.calls)
	}
	if _, err := os.Stat(lock); err != nil {
		t.Errorf("the holder's lock was taken away: %v", err)
	}

	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if err := c.Do(context.Background(), "ensure", "%5"); err != nil {
		t.Fatal(err)
	}
	if len(f.did("split-window")) != 1 {
		t.Errorf("a free lock must allow the split: %q", f.calls)
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Errorf("the lock is still held after ensure: %v", err)
	}
}

func TestNewControllerLocksUnderTMPDIR(t *testing.T) {
	t.Setenv("TMPDIR", "/var/tmp/x/")
	if got := NewController("muxrail").LockDir; got != "/var/tmp/x/" {
		t.Errorf("LockDir = %q, want $TMPDIR", got)
	}
	t.Setenv("TMPDIR", "")
	if got := NewController("muxrail").LockDir; got != "/tmp" {
		t.Errorf("LockDir = %q, want /tmp", got)
	}
}

func TestToggleOnBuildsEveryWindowOfTheSession(t *testing.T) {
	f := newFake()
	f.replies["display-message -p -t %5 #{session_id}"] = "$1"
	f.replies["list-windows -t $1 -F #{window_id}"] = "@1\n@2"
	f.replies["display-message -p -t @1 #{window_id}"] = "@1"
	f.replies["display-message -p -t @2 #{window_id}"] = "@2"
	f.replies["display-message -p -t @1 #{window_width}"] = "120"
	f.replies["display-message -p -t @2 #{window_width}"] = "60"
	if err := newController(t, f).Do(context.Background(), "toggle", "%5"); err != nil {
		t.Fatal(err)
	}
	if f.options["@sidebar_on"] != "1" {
		t.Errorf("@sidebar_on = %q, want 1", f.options["@sidebar_on"])
	}
	if got := f.did("split-window"); len(got) != 1 || !strings.Contains(got[0], "-t @1 ") {
		t.Errorf("splits = %q, want one, in the wide window @1", got)
	}
}

func TestToggleOffKillsEverySidebar(t *testing.T) {
	f := newFake()
	f.options["@sidebar_on"] = "1"
	f.replies["list-panes -a -F #{pane_id} #{@sidebar}"] = "%1 1\n%2 \n%3 1"
	if err := newController(t, f).Do(context.Background(), "toggle", "%5"); err != nil {
		t.Fatal(err)
	}
	if f.options["@sidebar_on"] != "0" {
		t.Errorf("@sidebar_on = %q, want 0", f.options["@sidebar_on"])
	}
	if got, want := f.did("kill-pane"), []string{"kill-pane -t %1", "kill-pane -t %3"}; !slices.Equal(got, want) {
		t.Errorf("kills = %q, want %q", got, want)
	}
}

func TestWakeTypesTheWakeSequenceIntoEverySidebar(t *testing.T) {
	f := newFake()
	f.replies["list-panes -t %5 -F #{pane_id} #{@sidebar}"] = "%1 1\n%5 "
	if err := newController(t, f).Do(context.Background(), "wake", "%5"); err != nil {
		t.Fatal(err)
	}
	if got, want := f.did("send-keys"), []string{"send-keys -t %1 -H 1b 5b 39 39 39 39 7e"}; !slices.Equal(got, want) {
		t.Errorf("keys = %q, want %q: ESC [ 9 9 9 9 ~, the sequence the sidebar listens for", got, want)
	}
}

func TestUnknownAction(t *testing.T) {
	if err := newController(t, newFake()).Do(context.Background(), "render", "%5"); err == nil {
		t.Error("want an error for an action that does not exist")
	}
}
