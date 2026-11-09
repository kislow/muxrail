package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kislow/muxrail/internal/jsoncache"
	"github.com/kislow/muxrail/internal/tmux"
)

const snapshotTTL = 3 * time.Second

// Info is what is known about one pane: the agent running in it, the status its hooks last reported and when that status began.
type Info struct {
	Kind   string  `json:"kind,omitempty"`
	Status string  `json:"status,omitempty"`
	Since  float64 `json:"since,omitempty"`
}

// Snapshot maps pane id to Info.
type Snapshot map[string]*Info

func cachePath() string { return filepath.Join(os.TempDir(), "tmux-sidebar-agents.json") }

// InvalidateCache drops the shared snapshot so the next Load rebuilds it, which is how a hook's write shows at once
// instead of after the cache's TTL.
func InvalidateCache() error {
	if err := os.Remove(cachePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: agent snapshot", err)
	}
	return nil
}

// Load returns the shared snapshot, rebuilt at most every few seconds however many sidebar processes ask.
func Load(ctx context.Context) (Snapshot, error) {
	var snap Snapshot
	if jsoncache.Read(cachePath(), snapshotTTL, &snap) && snap != nil {
		return snap, nil
	}
	snap, err := build(ctx)
	if err != nil {
		return nil, err
	}
	if err := jsoncache.Write(cachePath(), snap); err != nil {
		return nil, fmt.Errorf("%w: agent snapshot", err)
	}
	return snap, nil
}

// build combines process detection (kind) with the state files the hooks write (status and timing).
func build(ctx context.Context) (Snapshot, error) {
	ttys, err := paneTTYs(ctx)
	if err != nil {
		return nil, err
	}
	ps, err := exec.CommandContext(ctx, "ps", "-axo", "tty=,stat=,comm=,args=").Output()
	if err != nil {
		return nil, fmt.Errorf("%w: ps", err)
	}
	snap := Snapshot{}
	for tty, kind := range ForegroundKinds(string(ps)) {
		if pane, ok := ttys[tty]; ok {
			snap[pane] = &Info{Kind: kind}
		}
	}
	mergeHookState(ctx, snap, ttys)
	return snap, nil
}

// mergeHookState adds what the hooks of this tmux server reported and removes the files of its panes that no longer
// exist. The server is the one the pane list came from, asked for its socket rather than read from $TMUX, which a
// sidebar run from a plain shell with `tmux -L name` does not have. A server or directory that cannot be found leaves
// the sidebar on process detection alone, and touches no file.
func mergeHookState(ctx context.Context, snap Snapshot, ttys map[string]string) {
	socket, err := tmux.SocketPath(ctx)
	if err != nil {
		return
	}
	dir, err := stateDir(socket)
	if err != nil {
		return
	}
	states := readStates(dir)
	live := map[string]bool{}
	for _, pane := range ttys {
		live[pane] = true
	}
	_ = dropDead(dir, states, live) // cleanup is best effort: a file left behind is retried on the next build
	mergeStates(snap, states)
}

func paneTTYs(ctx context.Context) (map[string]string, error) {
	out, err := tmux.Output(ctx, "list-panes", "-a", "-F", "#{pane_id} #{pane_tty}")
	if err != nil {
		return nil, err
	}
	ttys := map[string]string{}
	for line := range strings.SplitSeq(out, "\n") {
		if pane, tty, ok := strings.Cut(line, " "); ok {
			ttys[strings.ReplaceAll(tty, "/dev/", "")] = pane
		}
	}
	return ttys, nil
}
