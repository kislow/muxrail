package agents

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kislow/muxrail/internal/jsoncache"
)

const stateExt = ".json"

// State is what an agent's hooks last reported for one pane.
type State struct {
	Status    string  `json:"status"`
	UpdatedTS float64 `json:"updated_ts"`
	Kind      string  `json:"kind"`
}

// stateDir is where the hooks of one tmux server keep one file per pane: $XDG_STATE_HOME/muxrail/agents/<socket name>,
// default ~/.local/state/muxrail/agents/<socket name>. Pane ids restart at %0 on every server, so servers (the default
// socket, `tmux -L name`) each get their own folder; the name is the base name of the socket path.
func stateDir(socketPath string) (string, error) {
	name := filepath.Base(socketPath)
	if !validSocket(name) {
		return "", fmt.Errorf("unsafe tmux socket name %q", name)
	}
	if base := os.Getenv("XDG_STATE_HOME"); filepath.IsAbs(base) {
		return filepath.Join(base, "muxrail", "agents", name), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("%w: home directory", err)
	}
	return filepath.Join(home, ".local", "state", "muxrail", "agents", name), nil
}

// validSocket accepts a tmux socket's base name that is a plain file name, so it can never steer the folder elsewhere.
func validSocket(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsRune(name, '/')
}

// stateFile names the pane's file after its id without the %, which would only be noise in a file name.
func stateFile(dir, pane string) string {
	return filepath.Join(dir, strings.TrimPrefix(pane, "%")+stateExt)
}

// validPane accepts only what tmux puts in $TMUX_PANE (%12), so the id can never steer the file path elsewhere.
func validPane(pane string) bool {
	digits, ok := strings.CutPrefix(pane, "%")
	if !ok || digits == "" {
		return false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func writeState(dir, pane string, s State) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("%w: state directory", err)
	}
	return jsoncache.Write(stateFile(dir, pane), s)
}

func readState(dir, pane string) (State, bool) {
	var s State
	return s, jsoncache.Read(stateFile(dir, pane), 0, &s)
}

// readStates returns every readable state by pane id; files that are not a pane's state (a half-written temp file, a
// stray note) are skipped.
func readStates(dir string) map[string]State {
	entries, _ := os.ReadDir(dir) // a missing directory just means no hook has run yet
	states := map[string]State{}
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), stateExt)
		if !ok {
			continue
		}
		pane := "%" + id
		if !validPane(pane) {
			continue
		}
		if s, ok := readState(dir, pane); ok {
			states[pane] = s
		}
	}
	return states
}

// dropDead deletes the files of panes tmux no longer lists, so closed panes do not linger, and forgets them in states.
func dropDead(dir string, states map[string]State, live map[string]bool) error {
	var errs []error
	for pane := range states {
		if live[pane] {
			continue
		}
		delete(states, pane)
		if err := os.Remove(stateFile(dir, pane)); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// mergeStates adds status and timing by pane id; the process-detected kind wins, the file's kind fills the gaps.
func mergeStates(snap Snapshot, states map[string]State) {
	for pane, s := range states {
		info := snap[pane]
		if info == nil {
			info = &Info{}
			snap[pane] = info
		}
		info.Status, info.Since = s.Status, s.UpdatedTS
		if info.Kind == "" {
			info.Kind = s.Kind
		}
	}
}
