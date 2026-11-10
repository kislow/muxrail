package ui

import (
	"strings"
	"testing"

	"github.com/kislow/muxrail/internal/activity"
	"github.com/kislow/muxrail/internal/term"
	"github.com/kislow/muxrail/internal/windows"
)

func TestTokens(t *testing.T) {
	for in, want := range map[int]string{0: "0", 850: "850", 8400: "8.4k", 10_000: "10k", 86_013: "86k", 999_999: "999k", 1_250_000: "1.2M"} {
		if got := tokens(in); got != want {
			t.Errorf("tokens(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestGauge(t *testing.T) {
	tests := []struct {
		name         string
		used, window int
		wantPlain    string
		wantColour   string
	}{
		{"nearly empty still shows one cell", 1_000, 200_000, "▰▱▱▱▱ 1.0k", green},
		{"half", 100_000, 200_000, "▰▰▰▱▱ 100k", green},
		{"past 60 percent warns", 130_000, 200_000, "▰▰▰▱▱ 130k", yellow},
		{"past 85 percent is critical", 180_000, 200_000, "▰▰▰▰▰ 180k", red},
		{"over the window is capped", 250_000, 200_000, "▰▰▰▰▰ 250k", red},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gauge(IconsNerd, tt.used, tt.window)
			if plain(got) != tt.wantPlain || !strings.HasPrefix(got, tt.wantColour) {
				t.Errorf("gauge = %q (colour prefix %q), want %q in %q", plain(got), got[:5], tt.wantPlain, tt.wantColour)
			}
		})
	}
}

func TestCtxWindow(t *testing.T) {
	e := testEnv()
	if got := e.ctxWindow(90_000, 0); got != defaultCtx {
		t.Errorf("a small context measures against %d, got %d", defaultCtx, got)
	}
	if got := e.ctxWindow(250_000, 0); got != extendedCtx {
		t.Errorf("a context past the default is a 1M window, got %d", got)
	}
	e.CtxWindow = 500_000
	if got := e.ctxWindow(90_000, 0); got != 500_000 {
		t.Errorf("the configured window wins, got %d", got)
	}
	e.CtxWindow = 0
	if got := e.ctxWindow(90_000, 258_400); got != 258_400 {
		t.Errorf("a window the agent reports beats the default, got %d", got)
	}
}

func claudeWin(status string, a *activity.Info) *windows.Win {
	return &windows.Win{Kind: "claude", Command: "claude", Status: status, Activity: a}
}

func TestAction(t *testing.T) {
	edit := &activity.Info{Action: "✎ Edit rail.go", Pending: true}
	tests := []struct {
		name string
		win  *windows.Win
		want string
	}{
		{"working shows the action", claudeWin("working", edit), "✎ Edit rail.go"},
		{"waiting with a call in flight is a permission prompt", claudeWin("waiting", edit), "? ✎ Edit rail.go"},
		{"waiting at the prompt says nothing", claudeWin("waiting", &activity.Info{Action: "thinking"}), ""},
		{"done says nothing", claudeWin("done", edit), ""},
		{"no activity", claudeWin("working", nil), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := plain(action(tt.win, 40)); got != tt.want {
				t.Errorf("action = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestActivityRowFillsExactlyItsRoom(t *testing.T) {
	e := testEnv()
	w := claudeWin("working", &activity.Info{Action: "$ go test ./... with a rather long description", Context: 86_000})
	for _, room := range []int{18, 24, 30, 36} {
		if got := term.VisibleWidth(activityRow(w, e, room)); got != room {
			t.Errorf("room %d: row is %d columns, want it to fill the room so the gauge sits at the right edge", room, got)
		}
	}
	if got := plain(activityRow(w, e, 12)); strings.Contains(got, "━") {
		t.Errorf("a 12 column row has no room for the gauge: %q", got)
	}
}

func TestActivityRowShowsTheGaugeAloneWhenIdle(t *testing.T) {
	got := plain(activityRow(claudeWin("done", &activity.Info{Context: 90_000}), testEnv(), 30))
	if strings.TrimSpace(got) != "━━─── 90k" || !strings.HasSuffix(got, "90k") {
		t.Errorf("row = %q, want the gauge at the right", got)
	}
	if got := activityRow(claudeWin("done", nil), testEnv(), 30); got != "" {
		t.Errorf("no activity should leave no row, got %q", got)
	}
}

func TestACardWithActivityGainsOneRow(t *testing.T) {
	e := testEnv()
	w := claudeWin("working", nil)
	w.Name, w.Path = "muxrail", "/Users/jordan/src/muxrail"
	without := len(e.bodyRows(w, 44))
	w.Activity = &activity.Info{Action: "✎ Edit rail.go", Context: 86_000}
	if with := len(e.bodyRows(w, 44)); with != without+1 {
		t.Errorf("rows = %d, want %d: the activity row", with, without+1)
	}
	assertWidthAtMost(t, 44, e.card(w, "", 44, ""))
}

func TestUnicodeGaugeUsesBoxDrawing(t *testing.T) {
	got := term.StripANSI(gauge(IconsUnicode, 86_000, 200_000))
	if strings.ContainsAny(got, "▰▱") || !strings.Contains(got, "━") || !strings.Contains(got, "─") {
		t.Errorf("gauge = %q, want box-drawing lines only", got)
	}
	if w := term.VisibleWidth(got); w != gaugeCells+1+len("86k") {
		t.Errorf("gauge is %d columns wide", w)
	}
}
