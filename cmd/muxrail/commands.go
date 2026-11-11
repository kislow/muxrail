package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kislow/muxrail/internal/agents"
	"github.com/kislow/muxrail/internal/picker"
	"github.com/kislow/muxrail/internal/sessions"
	"github.com/kislow/muxrail/internal/sidebar"
	"github.com/kislow/muxrail/internal/tmux"
	"github.com/kislow/muxrail/internal/ui"
	"github.com/kislow/muxrail/internal/windows"
)

const sidebarUsage = "usage: muxrail sidebar | sidebar toggle|ensure|wake|remember|apply <pane-id>"

// sidebarCmd runs the sidebar UI in this pane, or with an action and a pane id, the control path the tmux hooks call.
func sidebarCmd(ctx context.Context, args []string) error {
	switch len(args) {
	case 0:
		return sidebar.Run(ctx)
	case 2:
		bin, err := binaryPath()
		if err != nil {
			return err
		}
		return sidebar.NewController(bin).Do(ctx, args[0], args[1])
	}
	return errors.New(sidebarUsage)
}

// binaryPath is the muxrail that panes and popups run, by absolute path so the tmux server's PATH does not matter.
func binaryPath() (string, error) {
	if bin := os.Getenv("MUXRAIL_BIN"); bin != "" {
		return bin, nil
	}
	bin, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("%w: path of the muxrail binary", err)
	}
	return bin, nil
}

func onceCmd(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: muxrail --once <cols>")
	}
	cols, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("%w: columns", err)
	}
	return sidebar.Once(ctx, os.Stdout, cols)
}

const tabUsage = "usage: muxrail tab <window-id> [plain | rail|rail-current <index>]"

func tabCmd(ctx context.Context, args []string) error {
	rail := len(args) == 3 && (args[1] == "rail" || args[1] == "rail-current")
	if len(args) < 1 || (len(args) == 2 && args[1] != "plain") || (len(args) > 2 && !rail) || len(args) > 3 {
		return errors.New(tabUsage)
	}
	tab, ok, err := windows.TabOf(ctx, args[0])
	if err != nil || !ok {
		return err
	}
	set, err := iconSet(ctx)
	if err != nil {
		return err
	}
	if rail {
		_, err = fmt.Fprint(os.Stdout, ui.RailTab(set, tab, args[2], args[1] == "rail-current"))
		return err
	}
	_, err = fmt.Fprint(os.Stdout, ui.TabIcon(set, tab.Kind, tab.Command, len(args) == 2))
	return err
}

// iconSet reads @muxrail_icons from the tmux server; unset reads as the default Unicode set.
func iconSet(ctx context.Context) (ui.Icons, error) {
	out, err := tmux.Output(ctx, "show-options", "-gqv", "@muxrail_icons")
	if err != nil {
		return ui.IconsUnicode, err
	}
	return ui.ParseIcons(strings.TrimSpace(out)), nil
}

func statusCmd(ctx context.Context) error {
	snap, err := agents.Load(ctx)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(os.Stdout, ui.StatusLine(agents.Summarize(snap), time.Now()))
	return err
}

const searchUsage = "usage: muxrail search [--state FILE | --pick] | search cycle agent|sort FILE | search header FILE"

func searchCmd(ctx context.Context, args []string) error {
	switch {
	case len(args) == 0:
		return printList(ctx, nil, "")
	case len(args) == 1 && args[0] == "--pick":
		return pickCmd(ctx)
	case len(args) == 2 && args[0] == "--state":
		return printList(ctx, sessions.LoadView(args[1]), args[1])
	case len(args) == 3 && args[0] == "cycle":
		return sessions.Cycle(args[2], args[1])
	case len(args) == 2 && args[0] == "header":
		_, err := fmt.Println(sessions.LoadView(args[1]).Header())
		return err
	}
	return errors.New(searchUsage)
}

func pickCmd(ctx context.Context) error {
	bin, err := binaryPath()
	if err != nil {
		return err
	}
	home, err := userHome()
	if err != nil {
		return err
	}
	return picker.Run(ctx, bin, home)
}

// printList prints every session as fzf rows, narrowed and ordered by view (nil: newest first). With a state file it
// also records which agents have sessions, for the tab key.
func printList(ctx context.Context, view *sessions.View, statePath string) error {
	home, err := userHome()
	if err != nil {
		return err
	}
	list, listErr := sessions.List(ctx, home)
	if view != nil {
		view.Remember(list)
		if err := view.Save(statePath); err != nil {
			return err
		}
		list = view.Apply(list)
	}
	now := time.Now()
	for _, s := range list {
		fmt.Println(sessions.Row(s, now))
	}
	if listErr != nil { // a transcript that cannot be read must not fail the pipe into fzf
		fmt.Fprintf(os.Stderr, "muxrail search: warning: %v\n", listErr)
	}
	return nil
}

func planCmd(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("usage: muxrail plan <agent> <id> <cwd>")
	}
	home, err := userHome()
	if err != nil {
		return err
	}
	plan, err := sessions.Resume(home, args[0], args[1], args[2])
	if err != nil {
		return err
	}
	fmt.Println(plan.Command(args[2]))
	return nil
}

func showCmd(args []string) error {
	if len(args) != 3 {
		return errors.New("usage: muxrail show <agent> <id> <cwd>")
	}
	home, err := userHome()
	if err != nil {
		return err
	}
	text, err := sessions.Preview(home, args[0], args[1], args[2], time.Now())
	if err != nil {
		return err
	}
	_, err = fmt.Print(text)
	return err
}

func userHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("%w: home directory", err)
	}
	return home, nil
}
