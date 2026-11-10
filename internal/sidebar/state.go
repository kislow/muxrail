// Package sidebar runs the interactive window list inside a tmux pane.
package sidebar

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/kislow/muxrail/internal/tmux"
	"github.com/kislow/muxrail/internal/ui"
	"github.com/kislow/muxrail/internal/windows"
)

// state is everything the loop remembers between iterations.
type state struct {
	sessionID  string
	pane       string
	env        *ui.Env
	wins       []*windows.Win
	layout     string
	selected   int
	lastActive string
	drag       *ui.Drag
	slide      map[int]int
	debug      bool
	visible    bool
}

func newState(ctx context.Context) (*state, error) {
	pane, err := windows.SidebarPane()
	if err != nil {
		return nil, err
	}
	sid, err := tmux.SessionID(ctx, pane)
	if err != nil {
		return nil, err
	}
	home, _ := os.UserHomeDir() // without a home directory paths are simply not shortened
	return &state{sessionID: sid, pane: pane, env: &ui.Env{Home: home}, layout: ui.LayoutTiles, visible: true}, nil
}

// snapshot is what one background read of tmux found.
type snapshot struct {
	wins       []*windows.Win
	capL, capR string
	icons      ui.Icons
	ctxWindow  int
	layout     string
	debug      bool
	// visible is whether this sidebar's window is the one on screen: a hidden sidebar need not refresh or animate.
	visible bool
	// alone is whether the sidebar is the only pane of its window.
	alone bool
	// idle marks a read that only asked whether the window is on screen: it carries no windows to apply.
	idle bool
}

// fetch reads the windows, their git stats and the options the sidebar follows live (pill caps, layout, debug). It
// touches no state, so it can run beside the input loop. With skipHidden a sidebar whose window is not on screen stops
// after that one tmux call: nothing is drawn for it, and the wake that brings it back reads everything again. Its git,
// ps calls were most of its cost.
func (s *state) fetch(ctx context.Context, skipHidden bool) (*snapshot, error) {
	v, err := tmux.Values(ctx, s.pane, "@cap_l", "@cap_r", "@sidebar_layout", "@sidebar_debug", "@muxrail_ctx_window",
		"window_active", "session_attached", "window_panes", "@muxrail_icons")
	if err != nil {
		return nil, err
	}
	snap := &snapshot{layout: ui.LayoutTiles, capL: v[0], capR: v[1], debug: v[3] == "1", visible: v[5] == "1" && v[6] != "0", alone: v[7] == "1",
		icons: ui.ParseIcons(v[8])}
	if v[2] == ui.LayoutCompact {
		snap.layout = ui.LayoutCompact
	}
	if n, convErr := strconv.Atoi(v[4]); convErr == nil { // unset or malformed: the default applies
		snap.ctxWindow = n
	}
	if skipHidden && !snap.visible {
		snap.idle = true
		return snap, nil
	}
	if snap.wins, err = windows.List(ctx, s.sessionID); err != nil {
		return nil, err
	}
	if err := windows.AttachGit(ctx, snap.wins); err != nil {
		return nil, err
	}
	if err := windows.AttachActivity(s.env.Home, snap.wins, time.Now()); err != nil {
		s.logError(err)
	}
	return snap, nil
}

func (s *state) apply(snap *snapshot) {
	if snap.idle {
		s.visible = false
		return
	}
	s.wins, s.env.CapL, s.env.CapR, s.debug, s.layout = snap.wins, snap.capL, snap.capR, snap.debug, snap.layout
	s.env.CtxWindow = snap.ctxWindow
	s.env.Icons = snap.icons
	s.visible = snap.visible
	s.followActive()
}

// load is fetch and apply in one step, for the first frame and the one-shot render.
func (s *state) load(ctx context.Context) error {
	snap, err := s.fetch(ctx, false)
	if err != nil {
		return err
	}
	s.apply(snap)
	return nil
}

// followActive snaps the cursor to the active window when the active window changes. It compares ids: a reorder
// moves the active card but must not move the cursor.
func (s *state) followActive() {
	id, index := "", 0
	for i, w := range s.wins {
		if w.Active {
			id, index = w.ID, i
			break
		}
	}
	if id != s.lastActive {
		s.lastActive, s.selected = id, index
	}
	s.selected = min(s.selected, max(len(s.wins)-1, 0))
}

func (s *state) view(cols, rows int) ui.View {
	return ui.View{Width: cols, Rows: rows, Layout: s.layout, Selected: s.selected, Drag: s.drag, Slide: s.slide}
}

func (s *state) working() bool {
	for _, w := range s.wins {
		if w.Status == "working" {
			return true
		}
	}
	return false
}

func (s *state) frame(cols, rows int, now time.Time) string {
	return ui.JoinFrame(s.lines(cols, rows, now))
}

func (s *state) lines(cols, rows int, now time.Time) []string {
	s.env.Now = now
	return s.env.Lines(s.wins, s.view(cols, rows))
}
