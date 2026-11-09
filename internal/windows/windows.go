// Package windows lists the windows of a tmux session the way the sidebar shows them and acts on them.
package windows

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kislow/muxrail/internal/activity"
	"github.com/kislow/muxrail/internal/agents"
	"github.com/kislow/muxrail/internal/gitinfo"
	"github.com/kislow/muxrail/internal/tmux"
)

// Win is one window, represented by its most interesting pane.
type Win struct {
	ID      string
	Pane    string
	Index   int
	Active  bool
	Status  string
	Command string
	Path    string
	Name    string
	Title   string
	Kind    string
	Since   float64
	Git     *gitinfo.Info
	// Activity is what the agent is doing right now; nil unless it is a Claude session found by its pane.
	Activity *activity.Info
}

// IsAgent is true for a recognised agent and for a process that looks like one (a bare version number command).
func (w *Win) IsAgent() bool { return w.Kind != "" || agents.IsVersion(w.Command) }

var listFormat = tmux.Format("window_id", "window_index", "window_active", "pane_id", "pane_active", "@sidebar",
	"pane_current_command", "pane_current_path", "window_name", "pane_title")

// List returns the session's windows ordered by index, sidebars left out. A window with several panes is shown as
// one: an agent pane beats the active pane, which beats the rest.
func List(ctx context.Context, sessionID string) ([]*Win, error) {
	snap, err := agents.Load(ctx)
	if err != nil {
		return nil, err
	}
	out, err := tmux.Output(ctx, "list-panes", "-s", "-t", sessionID, "-F", listFormat)
	if err != nil {
		return nil, err
	}
	chosen, ranks := map[int]*Win{}, map[int]int{}
	for line := range strings.SplitSeq(out, "\n") {
		win, sidebar, rank, err := parseLine(line, snap)
		if err != nil {
			return nil, err
		}
		if sidebar {
			continue
		}
		if best, seen := ranks[win.Index]; !seen || rank > best {
			chosen[win.Index], ranks[win.Index] = win, rank
		}
	}
	return ordered(chosen), nil
}

func parseLine(line string, snap agents.Snapshot) (win *Win, sidebar bool, rank int, err error) {
	f := strings.SplitN(line, tmux.Sep, 10)
	if len(f) != 10 {
		return nil, false, 0, fmt.Errorf("pane line has %d fields, want 10: %q", len(f), line)
	}
	index, err := strconv.Atoi(f[1])
	if err != nil {
		return nil, false, 0, fmt.Errorf("%w: window index in %q", err, line)
	}
	win = &Win{ID: f[0], Pane: f[3], Index: index, Active: f[2] == "1", Command: f[6], Path: f[7], Name: f[8], Title: f[9]}
	if info := snap[f[3]]; info != nil {
		win.Kind, win.Since = info.Kind, info.Since
		if info.Status != "" {
			win.Status = info.Status
		}
	}
	return win, f[5] == "1", PaneRank(win.Kind != "", f[4] == "1"), nil
}

// PaneRank orders the panes of one window: an agent pane beats the active pane, which beats the rest.
func PaneRank(agent, active bool) int {
	rank := 0
	if agent {
		rank += 2
	}
	if active {
		rank++
	}
	return rank
}

func ordered(byIndex map[int]*Win) []*Win {
	wins := make([]*Win, 0, len(byIndex))
	for _, w := range byIndex {
		wins = append(wins, w)
	}
	sort.Slice(wins, func(i, j int) bool { return wins[i].Index < wins[j].Index })
	return wins
}

// AttachGit fills in Git for each window from the shared git cache.
func AttachGit(ctx context.Context, wins []*Win) error {
	paths := make([]string, len(wins))
	for i, w := range wins {
		paths[i] = w.Path
	}
	stats, err := gitinfo.Stats(ctx, paths)
	if err != nil {
		return err
	}
	for _, w := range wins {
		w.Git = stats[w.Path]
	}
	return nil
}

// AttachActivity fills in Activity for each agent window from its transcript. A window whose session is not found has
// none; a transcript that cannot be read is reported, and the other windows still get theirs.
func AttachActivity(home string, wins []*Win, now time.Time) error {
	var errs []error
	for _, w := range wins {
		info, err := activityOf(home, w, now)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		w.Activity = info
	}
	return errors.Join(errs...)
}

// activityOf finds the window's transcript. Claude records its tmux pane, so its session is exact; Codex and Cursor
// do not, so the newest recent transcript of the window's folder stands in.
func activityOf(home string, w *Win, now time.Time) (*activity.Info, error) {
	switch w.Kind {
	case agents.Claude:
		if id := activity.SessionID(home, w.ID, w.Pane); id != "" {
			return activity.Read(home, id)
		}
	case agents.Codex:
		if path := activity.CodexRollout(home, w.Path, now); path != "" {
			return activity.ReadCodex(path)
		}
	case agents.Cursor:
		if path := activity.CursorTranscript(home, w.Path, now); path != "" {
			return activity.ReadCursor(path)
		}
	}
	return nil, nil
}
