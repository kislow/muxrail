package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kislow/muxrail/internal/agents"
)

const usage = `muxrail: tmux window sidebar and agent session search

usage:
  muxrail sidebar                      run the sidebar UI in the current pane
  muxrail tab <window-id> [plain]      print the tab-bar icon for a window
  muxrail tab <window-id> rail|rail-current <index>
                                       print a window's stop on the tab rail: state, index and logo
  muxrail status                       print the agents by state (waiting, working, done) for the status bar
  muxrail hook [--kind NAME] start|working|waiting|done
                                       record an agent's state for its tmux pane; called from agent hooks, never fails
  muxrail search [--state FILE]        list sessions as TSV for fzf, narrowed and ordered by the state file
  muxrail search cycle agent|sort FILE  advance the agent filter or the sort order saved in FILE
  muxrail search header FILE           print the header line for the state in FILE
  muxrail show <agent> <id> <cwd>      print the latest messages of a session, for the fzf preview
  muxrail plan <agent> <id> <cwd>      print the command that resumes a session
  muxrail version                      print the version
`

// version is set at build time (-X main.version=...); a plain go build reports dev.
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	code := run(ctx, os.Args[1:])
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	var err error
	switch args[0] {
	case "sidebar":
		err = sidebarCmd(ctx)
	case "--once":
		err = onceCmd(ctx, args[1:])
	case "tab":
		err = tabCmd(ctx, args[1:])
	case "status":
		err = statusCmd(ctx)
	case "hook":
		agents.Hook(args[1:])
	case "search":
		err = searchCmd(ctx, args[1:])
	case "show":
		err = showCmd(args[1:])
	case "plan":
		err = planCmd(args[1:])
	case "version", "--version":
		fmt.Println("muxrail", version)
	default:
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "muxrail %s: %v\n", args[0], err)
		return 1
	}
	return 0
}
