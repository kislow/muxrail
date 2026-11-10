package sidebar

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kislow/muxrail/internal/shell"
	"github.com/kislow/muxrail/internal/tmux"
)

const (
	// DefaultWidth is the sidebar width until a border is dragged.
	DefaultWidth = 34
	// MaxWidth caps a stored @sidebar_width. A sidebar that fills its window reports the window's width, and nobody
	// drags a rail past this, so a larger stored value is ignored and the default applies.
	MaxWidth = 80
	// MinWindowWidth is the narrowest window that gets a sidebar.
	MinWindowWidth = 90
)

// Controller builds, sizes and wakes the sidebar panes of a tmux server. The tmux hooks and the toggle key call it
// through `muxrail sidebar <action> <pane-id>`.
type Controller struct {
	// Tmux runs one tmux command and returns its stdout.
	Tmux func(ctx context.Context, args ...string) (string, error)
	// Bin is the muxrail binary a new sidebar pane runs, by absolute path: the tmux server's PATH does not matter.
	Bin string
	// LockDir holds the per-window locks.
	LockDir string
}

// NewController controls the server the environment points at, starting sidebar panes with bin.
func NewController(bin string) *Controller {
	dir := os.Getenv("TMPDIR")
	if dir == "" {
		dir = "/tmp"
	}
	return &Controller{Tmux: tmux.Output, Bin: bin, LockDir: dir}
}

// Do runs one action on the pane or window target: toggle, ensure, wake, remember or apply.
func (c *Controller) Do(ctx context.Context, action, target string) error {
	switch action {
	case "toggle":
		return c.toggle(ctx, target)
	case "ensure":
		return c.ensure(ctx, target)
	case "wake":
		return c.wake(ctx, target)
	case "remember":
		return c.remember(ctx, target)
	case "apply":
		if err := c.remember(ctx, target); err != nil {
			return err
		}
		return c.resize(ctx, "-s", "-t", target)
	}
	return fmt.Errorf("unknown action %q: want toggle, ensure, wake, remember or apply", action)
}

// sidebarPanes lists the sidebar panes of the scope given as list-panes arguments (-a, or -s/-t with a target).
func (c *Controller) sidebarPanes(ctx context.Context, scope ...string) ([]string, error) {
	out, err := c.Tmux(ctx, append([]string{"list-panes"}, append(scope, "-F", "#{pane_id} #{@sidebar}")...)...)
	if err != nil {
		return nil, err
	}
	var panes []string
	for _, line := range strings.Split(out, "\n") {
		if id, flag, ok := strings.Cut(line, " "); ok && flag == "1" {
			panes = append(panes, id)
		}
	}
	return panes, nil
}

// width is the one width every window's sidebar shares: dragging any sidebar's border sets it, the rest follow.
func (c *Controller) width(ctx context.Context) (int, error) {
	out, err := c.Tmux(ctx, "show-options", "-gqv", "@sidebar_width")
	if err != nil {
		return 0, err
	}
	if w, err := strconv.Atoi(strings.TrimSpace(out)); err == nil && w > 0 && w <= MaxWidth {
		return w, nil
	}
	return DefaultWidth, nil
}

// resize gives every sidebar in scope the shared width.
func (c *Controller) resize(ctx context.Context, scope ...string) error {
	width, err := c.width(ctx)
	if err != nil {
		return err
	}
	panes, err := c.sidebarPanes(ctx, scope...)
	if err != nil {
		return err
	}
	for _, pane := range panes {
		have, err := c.Tmux(ctx, "display-message", "-p", "-t", pane, "#{pane_width}")
		if err != nil {
			return err
		}
		if strings.TrimSpace(have) == strconv.Itoa(width) {
			continue
		}
		if _, err := c.Tmux(ctx, "resize-pane", "-t", pane, "-x", strconv.Itoa(width)); err != nil {
			return err
		}
	}
	return nil
}

// remember stores the width of target's window's sidebar as the shared one. During a mouse drag tmux names the pane
// under the mouse, not necessarily the sidebar, so the sidebar is looked up in the window. A sidebar that is the only
// pane, or wider than half the window, got that size by filling the window rather than by a drag, so it is not stored.
func (c *Controller) remember(ctx context.Context, target string) error {
	out, err := c.Tmux(ctx, "list-panes", "-t", target, "-F", "#{@sidebar} #{pane_width} #{window_panes} #{window_width}")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 4 || fields[0] != "1" {
			continue
		}
		panes, _ := strconv.Atoi(fields[2])
		width, _ := strconv.Atoi(fields[1])
		window, _ := strconv.Atoi(fields[3])
		if panes < 2 || width*2 > window {
			return nil
		}
		_, err := c.Tmux(ctx, "set-option", "-g", "@sidebar_width", fields[1])
		return err
	}
	return nil
}

func (c *Controller) enabled(ctx context.Context) (bool, error) {
	out, err := c.Tmux(ctx, "show-options", "-gqv", "@sidebar_on")
	return strings.TrimSpace(out) == "1", err
}

// lock takes the per-window lock: the hooks and a toggle can fire together, and it stops them both splitting the same
// window. The directory name is the one the shell version used, so an old and a new version never race.
func (c *Controller) lock(window string) (release func(), ok bool) {
	dir := filepath.Join(c.LockDir, "tmux-sidebar-"+window+".lock")
	if os.Mkdir(dir, 0o700) != nil {
		return nil, false
	}
	return func() { _ = os.Remove(dir) }, true
}

// ensure gives target's window its sidebar when sidebars are on and the window is wide enough, and sizes the one it has.
func (c *Controller) ensure(ctx context.Context, target string) error {
	on, err := c.enabled(ctx)
	if err != nil || !on {
		return err
	}
	window, err := c.Tmux(ctx, "display-message", "-p", "-t", target, "#{window_id}")
	if err != nil {
		return err
	}
	release, ok := c.lock(window)
	if !ok {
		return nil
	}
	defer release()
	panes, err := c.sidebarPanes(ctx, "-t", target)
	if err != nil {
		return err
	}
	cols, err := c.Tmux(ctx, "display-message", "-p", "-t", target, "#{window_width}")
	if err != nil {
		return err
	}
	if n, _ := strconv.Atoi(strings.TrimSpace(cols)); len(panes) > 0 || n < MinWindowWidth {
		return c.resize(ctx, "-t", target)
	}
	return c.split(ctx, target)
}

func (c *Controller) split(ctx context.Context, target string) error {
	width, err := c.width(ctx)
	if err != nil {
		return err
	}
	pane, err := c.Tmux(ctx, "split-window", "-hbdf", "-l", strconv.Itoa(width), "-t", target,
		"-P", "-F", "#{pane_id}", shell.Quote(c.Bin)+" sidebar")
	if err != nil {
		return err
	}
	_, err = c.Tmux(ctx, "set-option", "-p", "-t", strings.TrimSpace(pane), "@sidebar", "1")
	return err
}

// toggle turns the sidebars on in every window of the pane's session, or off everywhere.
func (c *Controller) toggle(ctx context.Context, pane string) error {
	on, err := c.enabled(ctx)
	if err != nil {
		return err
	}
	if on {
		return c.disableEverywhere(ctx)
	}
	return c.enableEverywhere(ctx, pane)
}

// enableEverywhere builds every window's sidebar up front, so switching windows never builds one mid-click.
func (c *Controller) enableEverywhere(ctx context.Context, pane string) error {
	if _, err := c.Tmux(ctx, "set-option", "-g", "@sidebar_on", "1"); err != nil {
		return err
	}
	session, err := c.Tmux(ctx, "display-message", "-p", "-t", pane, "#{session_id}")
	if err != nil {
		return err
	}
	out, err := c.Tmux(ctx, "list-windows", "-t", session, "-F", "#{window_id}")
	if err != nil {
		return err
	}
	for _, window := range strings.Fields(out) {
		if err := c.ensure(ctx, window); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) disableEverywhere(ctx context.Context) error {
	if _, err := c.Tmux(ctx, "set-option", "-g", "@sidebar_on", "0"); err != nil {
		return err
	}
	panes, err := c.sidebarPanes(ctx, "-a")
	if err != nil {
		return err
	}
	for _, pane := range panes {
		if _, err := c.Tmux(ctx, "kill-pane", "-t", pane); err != nil {
			return err
		}
	}
	return nil
}

// wake types the wake sequence into the sidebars of the window that just came on screen. A sidebar whose window is
// hidden refreshes rarely and does not animate; this makes it show fresh data at once.
func (c *Controller) wake(ctx context.Context, target string) error {
	panes, err := c.sidebarPanes(ctx, "-t", target)
	if err != nil {
		return err
	}
	keys := []string{"send-keys", "-t", "", "-H"}
	for i := range len(wakeSeq) {
		keys = append(keys, strconv.FormatInt(int64(wakeSeq[i]), 16))
	}
	for _, pane := range panes {
		keys[2] = pane
		if _, err := c.Tmux(ctx, keys...); err != nil {
			return err
		}
	}
	return nil
}
