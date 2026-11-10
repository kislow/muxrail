package ui

import (
	"fmt"
	"strings"

	"github.com/kislow/muxrail/internal/term"
	"github.com/kislow/muxrail/internal/windows"
)

const (
	gaugeCells   = 5
	defaultCtx   = 200_000
	extendedCtx  = 1_000_000
	gaugeMinRoom = 18 // the gauge only appears when the action still gets a few columns
	warnRatio    = 0.6
	critRatio    = 0.85
)

// ctxWindow is the size the gauge measures against: the configured one, else the one the agent reports (Codex does). Claude does not say which window a session runs with, so a
// context past the default one is taken to be a 1M window rather than shown as over 100 percent.
func (e *Env) ctxWindow(used, reported int) int {
	switch {
	case e.CtxWindow > 0:
		return e.CtxWindow
	case reported > 0:
		return reported
	case used > defaultCtx:
		return extendedCtx
	}
	return defaultCtx
}

// tokens abbreviates a token count: 850, 8.4k, 86k, 1.2M.
func tokens(n int) string {
	switch {
	case n < 1000:
		return fmt.Sprint(n)
	case n < 10_000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	case n < 1_000_000:
		return fmt.Sprintf("%dk", n/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1e6)
}

// gauge is a small bar of how full the context is, coloured by how close compaction is, with the token count. The
// Unicode set draws it with box-drawing lines, which terminals paint themselves in exactly one cell; ▰▱ are in few
// fonts, and the fallback glyph is wider than a cell and runs into the token count.
func gauge(set Icons, used, window int) string {
	ratio := min(float64(used)/float64(window), 1)
	filled := min(max(int(ratio*gaugeCells+0.5), 1), gaugeCells)
	colour := green
	switch {
	case ratio >= critRatio:
		colour = red
	case ratio >= warnRatio:
		colour = yellow
	}
	full, empty := "━", "─"
	if set == IconsNerd {
		full, empty = "▰", "▱"
	}
	return colour + strings.Repeat(full, filled) + dim + strings.Repeat(empty, gaugeCells-filled) + fgReset + " " + dim + tokens(used) + fgReset
}

// action is what to say about the agent right now: its tool call or thinking while it works, and a tool call it has
// not run yet, which is a permission prompt, while it waits.
func action(w *windows.Win, room int) string {
	a := w.Activity
	if a == nil || a.Action == "" || room < 1 {
		return ""
	}
	text := term.Clip(a.Action, room)
	switch {
	case w.Status == "working":
		return text
	case w.Status == "waiting" && a.Pending:
		return yellow + term.Clip("? "+a.Action, room) + fgReset
	}
	return ""
}

// activityRow is the action on the left and the context gauge on the right; empty when there is nothing to show.
func activityRow(w *windows.Win, e *Env, room int) string {
	if w.Activity == nil || room < 1 {
		return ""
	}
	if w.Activity.Context <= 0 || room < gaugeMinRoom {
		return action(w, room)
	}
	g := gauge(e.Icons, w.Activity.Context, e.ctxWindow(w.Activity.Context, w.Activity.Window))
	return term.Pad(action(w, room-visible(g)-1), room-visible(g)) + g
}
