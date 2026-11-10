//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

// TestSidebarControl drives `muxrail sidebar` the way the plugin's keys and hooks do: toggle builds a sidebar in every
// window wide enough, a drag on one border resizes the rest, and toggling again removes them all.
func TestSidebarControl(t *testing.T) {
	bin := muxrailBin(t)
	s := newServer(t, "control")
	s.start("run-shell "+pluginPath(t)+"\n", 140, 30, "A")
	s.tm("new-window", "-t", "t:", "-n", "B", "-d")
	s.tm("new-window", "-t", "t:", "-n", "narrow", "-d")
	s.tm("resize-window", "-t", "t:narrow", "-x", "80")

	sidebars := func() string {
		return strings.ReplaceAll(s.tm("list-panes", "-a", "-F", "#{window_name}:#{pane_width}:#{@sidebar}"), "\n", " ")
	}
	sidebarWidths := func() map[string]string {
		got := map[string]string{}
		for _, line := range strings.Fields(sidebars()) {
			if parts := strings.Split(line, ":"); len(parts) == 3 && parts[2] == "1" {
				got[parts[0]] = parts[1]
			}
		}
		return got
	}
	const settle = 20 * time.Second

	s.tm("run-shell", bin+" sidebar toggle %0")
	expect(t, "toggle builds a 34 column sidebar in every wide window, none in the narrow one", sidebarWidths,
		map[string]string{"A": "34", "B": "34"}, settle)

	var pane string
	for _, line := range strings.Split(s.tm("list-panes", "-t", "t:A", "-F", "#{pane_id} #{@sidebar}"), "\n") {
		if strings.HasSuffix(line, " 1") {
			pane = strings.Fields(line)[0]
		}
	}
	if pane == "" {
		t.Fatal("window A has no sidebar pane")
	}
	s.tm("resize-pane", "-t", pane, "-x", "45")
	s.tm("run-shell", bin+" sidebar apply "+pane)
	expect(t, "dragging one border sets the width of every sidebar", sidebarWidths,
		map[string]string{"A": "45", "B": "45"}, settle)
	expect(t, "the shared width is remembered", func() string { return s.tm("show-options", "-gqv", "@sidebar_width") }, "45", settle)

	s.tm("run-shell", bin+" sidebar toggle %0")
	expect(t, "a second toggle removes every sidebar", sidebarWidths, map[string]string{}, settle)
}

// TestLoneSidebarClosesItsWindow kills the only other pane of a window. The sidebar fills the window and fires
// after-resize-pane, but must neither store that width nor outlive the pane it sat next to.
func TestLoneSidebarClosesItsWindow(t *testing.T) {
	bin := muxrailBin(t)
	s := newServer(t, "lone")
	s.start("run-shell "+pluginPath(t)+"\n", 140, 30, "A")
	s.tm("new-window", "-t", "t:", "-n", "B", "-d")
	const settle = 30 * time.Second

	s.tm("run-shell", bin+" sidebar toggle %0")
	windowPanes := func(window string) string {
		return strings.ReplaceAll(s.tm("list-panes", "-t", "t:"+window, "-F", "#{pane_id}:#{@sidebar}"), "\n", " ")
	}
	expect(t, "toggle builds a sidebar next to the shell in window B", func() int { return strings.Count(windowPanes("B"), ":1") }, 1, settle)
	s.tm("set-option", "-g", "@sidebar_width", "40")

	var shell string
	for _, line := range strings.Fields(windowPanes("B")) {
		if strings.HasSuffix(line, ":") {
			shell = strings.TrimSuffix(line, ":")
		}
	}
	if shell == "" {
		t.Fatal("window B has no pane besides its sidebar")
	}
	s.tm("kill-pane", "-t", shell)

	windowNames := func() string { return strings.ReplaceAll(s.tm("list-windows", "-F", "#{window_name}"), "\n", " ") }
	expect(t, "the window whose last other pane exited is closed", windowNames, "A", settle)
	check(t, "the shared width is untouched", s.tm("show-options", "-gqv", "@sidebar_width") == "40",
		"@sidebar_width = "+s.tm("show-options", "-gqv", "@sidebar_width"))
}
