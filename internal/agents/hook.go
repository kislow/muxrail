package agents

import (
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"time"
)

// hookStatus maps a hook action to the status it records; start registers the pane before any prompt, so it has none.
var hookStatus = map[string]string{"start": "", "working": "working", "waiting": "waiting", "done": "done"}

// Hook records what an agent's hook reports for the pane it runs in: args are `[--kind NAME] <start|working|waiting|done>`.
// It returns nothing and prints nothing on purpose: a failing hook would disturb every agent session, so a bad argument,
// no tmux pane or server ($TMUX_PANE, $TMUX) or an unwritable directory are all silent no-ops.
func Hook(args []string) {
	_ = record(args, os.Getenv("TMUX"), os.Getenv("TMUX_PANE"), time.Now())
}

// socketPath is the first field of $TMUX, which tmux sets to <socket path>,<server pid>,<session id>.
func socketPath(tmuxEnv string) string {
	path, _, _ := strings.Cut(tmuxEnv, ",")
	return path
}

func record(args []string, tmuxEnv, pane string, now time.Time) error {
	status, kind, err := parseHook(args)
	if err != nil || !validPane(pane) {
		return err
	}
	dir, err := stateDir(socketPath(tmuxEnv))
	if err != nil {
		return err
	}
	if prev, ok := readState(dir, pane); ok && status != "" && prev.Status == status && prev.Kind == kind {
		return nil // Since is when the status began, so a hook that repeats it (every tool call) changes nothing
	}
	if err := writeState(dir, pane, State{Status: status, UpdatedTS: float64(now.UnixMilli()) / 1000, Kind: kind}); err != nil {
		return err
	}
	return InvalidateCache()
}

func parseHook(args []string) (status, kind string, err error) {
	fs := flag.NewFlagSet("hook", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&kind, "kind", Claude, "agent kind")
	if err := fs.Parse(args); err != nil {
		return "", "", err
	}
	status, ok := hookStatus[fs.Arg(0)]
	if !ok || fs.NArg() != 1 || kind == "" {
		return "", "", errors.New("usage: muxrail hook [--kind NAME] start|working|waiting|done")
	}
	return status, kind, nil
}
