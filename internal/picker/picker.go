// Package picker is the session search popup: fzf over every agent session, then the pick resumed in a new tmux window.
package picker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kislow/muxrail/internal/sessions"
	"github.com/kislow/muxrail/internal/shell"
	"github.com/kislow/muxrail/internal/tmux"
)

// Pick is the session the user chose in fzf.
type Pick struct{ Agent, ID, CWD string }

// ParsePick reads the selected fzf row, `agent<TAB>id<TAB>cwd<TAB>display`.
func ParsePick(line string) (Pick, error) {
	fields := strings.SplitN(strings.TrimRight(line, "\r\n"), "\t", 4)
	if len(fields) < 3 || fields[0] == "" || fields[1] == "" {
		return Pick{}, fmt.Errorf("unexpected selection %q: want agent, id and cwd separated by tabs", line)
	}
	return Pick{Agent: fields[0], ID: fields[1], CWD: fields[2]}, nil
}

// ResumeCommand is the tmux shell command that runs cmd in shellPath's interactive shell and then stays in that shell,
// so the window outlives the agent. An empty shellPath means /bin/sh.
func ResumeCommand(shellPath, cmd string) string {
	if shellPath == "" {
		shellPath = "/bin/sh"
	}
	sh := shell.Quote(shellPath)
	return sh + " -ic " + shell.Quote(cmd+"; exec "+sh)
}

// fzfArgs are fzf's options. Its key bindings call bin, by absolute path, against the state file of this popup.
func fzfArgs(bin, state, header string) []string {
	run := func(args string) string { return shell.Quote(bin) + " " + args }
	st := shell.Quote(state)
	reload := fmt.Sprintf("reload(%s)+transform-header(%s)", run("search --state "+st), run("search header "+st))
	return []string{
		"--ansi", "--reverse", "--no-hscroll", "--delimiter=\t", "--with-nth=4", "--prompt=session > ",
		"--header=" + header,
		"--bind", fmt.Sprintf("tab:execute-silent(%s)+%s", run("search cycle agent "+st), reload),
		"--bind", fmt.Sprintf("ctrl-s:execute-silent(%s)+%s", run("search cycle sort "+st), reload),
		"--preview", run("show {1} {2} {3}"), "--preview-window=right,50%,wrap,border-left",
	}
}

// Run lets the user pick a session in fzf and opens it in a new window of the tmux server the environment points at.
// Cancelling fzf is not an error. bin is the muxrail binary fzf's bindings run.
func Run(ctx context.Context, bin, home string) error {
	fzf, err := exec.LookPath("fzf")
	if err != nil {
		return errors.New("fzf is not installed or not on PATH: install it to search sessions")
	}
	state, err := os.CreateTemp("", "muxrail-search.*")
	if err != nil {
		return fmt.Errorf("%w: state file", err)
	}
	defer func() { _ = os.Remove(state.Name()) }()
	if err := state.Close(); err != nil {
		return fmt.Errorf("%w: state file", err)
	}

	list := exec.CommandContext(ctx, bin, "search", "--state", state.Name())
	list.Stderr = os.Stderr
	rows, err := list.Output()
	if err != nil {
		return fmt.Errorf("%w: list sessions", err)
	}
	pick := exec.CommandContext(ctx, fzf, fzfArgs(bin, state.Name(), sessions.LoadView(state.Name()).Header())...)
	pick.Stdin, pick.Stderr = bytes.NewReader(rows), os.Stderr
	out, err := pick.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && (exit.ExitCode() == 1 || exit.ExitCode() == 130) { // nothing matched, or Esc
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: fzf", err)
	}
	chosen, err := ParsePick(string(out))
	if err != nil {
		return err
	}
	return resume(ctx, home, chosen)
}

func resume(ctx context.Context, home string, p Pick) error {
	plan, err := sessions.Resume(home, p.Agent, p.ID, p.CWD)
	if err != nil {
		return err
	}
	cwd, cmd := plan.Shell(p.CWD)
	args := []string{"new-window"}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	_, err = tmux.Output(ctx, append(args, ResumeCommand(os.Getenv("SHELL"), cmd))...)
	return err
}
