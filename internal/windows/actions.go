package windows

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/kislow/muxrail/internal/agents"
	"github.com/kislow/muxrail/internal/tmux"
)

// Switch selects win in one tmux call, then gives focus back to the pane used before the click focused the sidebar.
// The guard keeps last-pane from moving focus onto the sidebar when it was not the active pane.
func Switch(ctx context.Context, sessionID, sidebarPane string, win *Win) error {
	_, err := tmux.Output(ctx, "select-window", "-t", sessionID+":"+strconv.Itoa(win.Index), ";",
		"if-shell", "-F", "-t", sidebarPane, "#{pane_active}", "last-pane -t "+sidebarPane)
	return err
}

// Move puts wins[src] in the slot of wins[dst] (the others shift) and closes index gaps, which renumber-windows does
// not do on a move. -d on the active window would hand the selection to a neighbour, so only the others get it.
func Move(ctx context.Context, sessionID string, wins []*Win, src, dst int) error {
	if src < 0 || src >= len(wins) || dst < 0 || dst >= len(wins) {
		return fmt.Errorf("move %d to %d: only %d windows", src, dst, len(wins))
	}
	moved := wins[src]
	args := []string{"move-window"}
	if src < dst {
		args = append(args, "-a")
	} else {
		args = append(args, "-b")
	}
	if !moved.Active {
		args = append(args, "-d")
	}
	args = append(args, "-s", moved.ID, "-t", wins[dst].ID, ";", "move-window", "-r", "-t", sessionID)
	_, err := tmux.Output(ctx, args...)
	return err
}

// Tab is what the tab bar draws for a window: the agent kind of its best pane, or its command when that looks like
// an agent, plus the agent's state and when it began.
type Tab struct {
	Kind, Command, Status string
	Since                 float64
}

// TabOf finds the tab of a window. ok is false when the window has no pane besides sidebars.
func TabOf(ctx context.Context, windowID string) (tab Tab, ok bool, err error) {
	snap, err := agents.Load(ctx)
	if err != nil {
		return Tab{}, false, err
	}
	out, err := tmux.Output(ctx, "list-panes", "-t", windowID, "-F", "#{pane_id} #{pane_active} #{@sidebar} #{pane_current_command}")
	if err != nil {
		return Tab{}, false, err
	}
	best := -1
	for line := range strings.SplitSeq(out, "\n") {
		f := strings.SplitN(line, " ", 4)
		if len(f) != 4 || f[2] == "1" {
			continue
		}
		cand := Tab{Command: f[3]}
		if info := snap[f[0]]; info != nil {
			cand.Kind, cand.Status, cand.Since = info.Kind, info.Status, info.Since
		}
		if rank := PaneRank(cand.Kind != "", f[1] == "1"); rank > best {
			best, tab, ok = rank, cand, true
		}
	}
	return tab, ok, nil
}

// SidebarPane is the pane this process runs in.
func SidebarPane() (string, error) {
	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		return "", fmt.Errorf("TMUX_PANE is not set: run inside tmux")
	}
	return pane, nil
}
