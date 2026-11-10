package sidebar

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kislow/muxrail/internal/term"
	"github.com/kislow/muxrail/internal/testutil"
	"github.com/kislow/muxrail/internal/ui"
	"github.com/kislow/muxrail/internal/windows"
)

func wins(activeID string, ids ...string) []*windows.Win {
	var out []*windows.Win
	for i, id := range ids {
		out = append(out, &windows.Win{ID: id, Index: i, Active: id == activeID})
	}
	return out
}

func TestFollowActiveMovesTheCursorOnlyWhenTheActiveWindowChanges(t *testing.T) {
	s := &state{}
	s.wins = wins("@b", "@a", "@b", "@c")
	s.followActive()
	if s.selected != 1 {
		t.Fatalf("first sight: cursor = %d, want the active window 1", s.selected)
	}
	s.selected = 2 // the user walked the cursor down
	s.followActive()
	if s.selected != 2 {
		t.Errorf("same active window: cursor = %d, want it left at 2", s.selected)
	}
	s.wins = wins("@b", "@b", "@a", "@c") // a reorder moves the active card but not the window
	s.followActive()
	if s.selected != 2 {
		t.Errorf("reorder: cursor = %d, want it left at 2", s.selected)
	}
	s.wins = wins("@c", "@b", "@a", "@d", "@c")
	s.followActive()
	if s.selected != 3 {
		t.Errorf("another window became active: cursor = %d, want 3", s.selected)
	}
}

func TestFollowActiveClampsTheCursor(t *testing.T) {
	s := &state{selected: 9, lastActive: "@a", wins: wins("@a", "@a", "@b")}
	s.followActive()
	if s.selected != 1 {
		t.Errorf("cursor = %d, want it clamped to the last window", s.selected)
	}
}

func TestWorking(t *testing.T) {
	s := &state{wins: wins("", "@a", "@b")}
	if s.working() {
		t.Error("no window is working")
	}
	s.wins[1].Status = "working"
	if !s.working() {
		t.Error("a working window should speed up the refresh")
	}
}

func stubSession(t *testing.T, layout string) { stubSessionOnScreen(t, layout, "1", "1") }

func stubSessionOnScreen(t *testing.T, layout, active, attached string) {
	t.Helper()
	stubSessionPanes(t, layout, active, attached, "2")
}

// stubSessionPanes is stubSession with the answers tmux gives for window_active, session_attached and window_panes.
func stubSessionPanes(t *testing.T, layout, active, attached, panes string) {
	t.Helper()
	testutil.IsolatedTmp(t)
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "%0")
	testutil.Stub(t, "tmux", `case "$1" in
display-message)
  case "$5" in
  '#{session_id}') echo '$1' ;;
  *) printf '(\037)\037%s\037\037\037%s\037%s\037%s\n' '`+layout+`' '`+active+`' '`+attached+`' '`+panes+`' ;;
  esac ;;
list-panes)
  if [ "$2" = "-a" ]; then exit 0; fi
  printf '@1\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\n' 0 1 %1 1 '' zsh /tmp one ''
  printf '@2\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\037%s\n' 1 0 %2 1 '' zsh /tmp two '' ;;
esac`)
	testutil.Stub(t, "ps", `exit 0`)
}

func TestOnceDrawsTilesWhateverTheLayoutOption(t *testing.T) {
	stubSession(t, "compact")
	var out bytes.Buffer
	if err := Once(context.Background(), &out, 34); err != nil {
		t.Fatal(err)
	}
	frame := out.String()
	if !strings.HasPrefix(frame, "\x1b[H") || !strings.HasSuffix(frame, "\n") {
		t.Errorf("frame = %q", frame)
	}
	plain := term.StripANSI(frame)
	for _, want := range []string{"WINDOWS  2", "╭─⠿─", "(0)", "one", "two", "(1)"} {
		if !strings.Contains(plain, want) {
			t.Errorf("frame lacks %q:\n%s", want, plain)
		}
	}
	if got := strings.Count(frame, "\r\n"); got != onceRows-1 {
		t.Errorf("frame has %d line breaks, want %d", got, onceRows-1)
	}
}

func TestLoadReadsTheLiveOptions(t *testing.T) {
	stubSession(t, "compact")
	s, err := newState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.layout != ui.LayoutCompact || s.env.CapL != "(" || s.env.CapR != ")" || len(s.wins) != 2 {
		t.Errorf("layout=%q caps=%q %q wins=%d", s.layout, s.env.CapL, s.env.CapR, len(s.wins))
	}
}

func TestNewStateNeedsATmuxPane(t *testing.T) {
	t.Setenv("TMUX_PANE", "")
	if _, err := newState(context.Background()); err == nil {
		t.Error("want an error outside tmux")
	}
}

func TestReadInputDropsInvalidUTF8(t *testing.T) {
	ch := readInput(strings.NewReader("a\xffb"))
	if got := <-ch; got != "ab" {
		t.Errorf("chunk = %q, want %q", got, "ab")
	}
	if _, open := <-ch; open {
		t.Error("channel should close at EOF")
	}
}

func TestFetchSaysWhetherTheWindowIsOnScreen(t *testing.T) {
	tests := []struct {
		name             string
		active, attached string
		want             bool
	}{
		{"the active window of an attached session", "1", "1", true},
		{"a window that is not the active one", "0", "1", false},
		{"a session nobody is attached to", "1", "0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubSessionOnScreen(t, "tiles", tt.active, tt.attached)
			s, err := newState(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			snap, err := s.fetch(context.Background(), false)
			if err != nil || snap.visible != tt.want {
				t.Errorf("visible = %v, %v; want %v", snap.visible, err, tt.want)
			}
		})
	}
}

func TestAHiddenSidebarReadsNothingButWhetherItIsOnScreen(t *testing.T) {
	stubSessionOnScreen(t, "tiles", "0", "1")
	s, err := newState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snap, err := s.fetch(context.Background(), true)
	if err != nil || !snap.idle || snap.visible || snap.wins != nil {
		t.Fatalf("idle=%v visible=%v wins=%d err=%v; want an idle read with no windows", snap.idle, snap.visible, len(snap.wins), err)
	}
	s.wins, s.visible = wins("@a", "@a"), true
	s.apply(snap)
	if s.visible || len(s.wins) != 1 {
		t.Errorf("visible=%v wins=%d; want hidden, and the windows it already had kept", s.visible, len(s.wins))
	}
}

func TestAnOnScreenSidebarReadsEverythingEvenWhenAskedToSkipHidden(t *testing.T) {
	stubSessionOnScreen(t, "tiles", "1", "1")
	s, err := newState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snap, err := s.fetch(context.Background(), true)
	if err != nil || snap.idle || len(snap.wins) != 2 {
		t.Errorf("idle=%v wins=%d err=%v; want a full read of 2 windows", snap.idle, len(snap.wins), err)
	}
}

func TestAHiddenSidebarRefreshesRarely(t *testing.T) {
	l := &loop{state: &state{}}
	if got := l.refreshInterval(); got != hiddenEvery {
		t.Errorf("hidden interval = %v, want %v", got, hiddenEvery)
	}
	l.visible = true
	if got := l.refreshInterval(); got != refreshEvery {
		t.Errorf("visible interval = %v, want %v", got, refreshEvery)
	}
}

func TestTheWakeSequenceShowsTheSidebarAtOnce(t *testing.T) {
	l := &loop{state: &state{}, refreshed: time.Now()}
	l.paint.prev = []string{"stale"}
	if err := l.handle(context.Background(), "x"+wakeSeq+"y"); err != nil {
		t.Fatal(err)
	}
	if !l.visible || !l.refreshed.IsZero() || l.paint.prev != nil {
		t.Errorf("visible=%v refreshed zero=%v prev=%v; want visible, due for a fetch, and a full repaint ahead",
			l.visible, l.refreshed.IsZero(), l.paint.prev)
	}
}

func TestFetchSaysWhetherTheSidebarIsAlone(t *testing.T) {
	tests := []struct {
		panes string
		want  bool
	}{{"1", true}, {"2", false}, {"3", false}}
	for _, tt := range tests {
		t.Run(tt.panes+" panes", func(t *testing.T) {
			stubSessionPanes(t, "tiles", "0", "1", tt.panes) // hidden: the answer must not depend on being on screen
			s, err := newState(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			snap, err := s.fetch(context.Background(), true)
			if err != nil || snap.alone != tt.want {
				t.Errorf("alone = %v, %v; want %v", snap.alone, err, tt.want)
			}
		})
	}
}

func TestTheSidebarExitsOnlyAfterTwoReadsAlone(t *testing.T) {
	l := &loop{state: &state{env: &ui.Env{}}, refreshed: time.Now()}
	read := func(alone bool) { l.finishFetch(fetchResult{snap: &snapshot{alone: alone}}) }

	read(true)
	if l.alone != 1 || !l.refreshed.IsZero() {
		t.Errorf("after one read alone = %d, refreshed zero = %v; want 1 and an immediate re-read", l.alone, l.refreshed.IsZero())
	}
	read(false)
	if l.alone != 0 {
		t.Errorf("a read with another pane left alone = %d, want the count reset", l.alone)
	}
	read(true)
	read(true)
	if l.alone != 2 {
		t.Errorf("alone = %d, want 2: the exit condition", l.alone)
	}
	l.fetched, l.fetching = make(chan fetchResult, 1), true // no real read may start
	l.input, l.winch = make(chan string), make(chan os.Signal)
	l.fetched <- fetchResult{snap: &snapshot{alone: true}}
	if err := l.iterate(context.Background()); !errors.Is(err, errAlone) {
		t.Errorf("iterate = %v, want errAlone", err)
	}
}

func TestAFailedReadDoesNotCountTowardsLeaving(t *testing.T) {
	testutil.IsolatedTmp(t)
	l := &loop{state: &state{}, alone: 1}
	l.finishFetch(fetchResult{err: errors.New("tmux gone")})
	if l.alone != 1 {
		t.Errorf("alone = %d, want an error read to leave the count as it was", l.alone)
	}
}
