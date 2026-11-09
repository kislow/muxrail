//go:build e2e

package e2e

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"
)

var nameRE = regexp.MustCompile(`[│▌ ]\s?(\d)\s+\S*?([A-E])(?:\s|│|$)`)

type compactState struct {
	Rows  int
	Tiles bool
}

// sidebarView reads the sidebar of the window the client is showing: after a click the client moves to another window.
type sidebarView struct{ s *server }

func (v sidebarView) pane() string {
	for _, line := range strings.Split(v.s.tm("list-panes", "-t", "t:", "-F", "#{pane_id} #{@sidebar}"), "\n") {
		if strings.HasSuffix(line, " 1") {
			return strings.Fields(line)[0]
		}
	}
	return ""
}

func (v sidebarView) screen() []string {
	pane := v.pane()
	if pane == "" {
		return nil
	}
	return strings.Split(v.s.tm("capture-pane", "-p", "-t", pane), "\n")
}

// names lists the window names in the order the sidebar shows them, with the number printed on each pill.
func (v sidebarView) names() (pills, names []string) {
	for _, line := range v.screen() {
		if m := nameRE.FindStringSubmatch(line); m != nil {
			pills, names = append(pills, m[1]), append(names, m[2])
		}
	}
	return pills, names
}

// tileRows is the screen row (1-based) of the first body line of every box, read from what the sidebar drew.
func (v sidebarView) tileRows() []int {
	var rows []int
	for i, line := range v.screen() {
		if strings.HasPrefix(line, "╭") {
			rows = append(rows, i+2)
		}
	}
	return rows
}

func (v sidebarView) compactRows() []int {
	var rows []int
	for i, line := range v.screen() {
		if nameRE.MatchString(line) {
			rows = append(rows, i+1)
		}
	}
	return rows
}

func (v sidebarView) compact() compactState {
	tiles := false
	for _, line := range v.screen() {
		tiles = tiles || strings.HasPrefix(line, "╭")
	}
	return compactState{len(v.compactRows()), tiles}
}

func (v sidebarView) says(word string) bool {
	for _, line := range v.screen() {
		if strings.Contains(line, word) {
			return true
		}
	}
	return false
}

func mouse(button, row int, kind string) string {
	return fmt.Sprintf("\x1b[<%d;10;%d%s", button, row, kind)
}

func needRows(t *testing.T, rows []int, n int) {
	t.Helper()
	if len(rows) < n {
		t.Fatalf("need %d sidebar rows, the screen shows %v", n, rows)
	}
}

// TestDrag clicks, drags with a live preview, drops, and switches to the compact layout, through a real tmux client
// with five windows A..E and the sidebar switched on by `muxrail sidebar toggle`.
func TestDrag(t *testing.T) {
	bin := muxrailBin(t)
	s := newServer(t, "drag")
	view := sidebarView{s}
	order := func() string {
		return strings.ReplaceAll(s.tm("list-windows", "-t", "t:", "-F", "#{window_index}:#{window_name}#{?window_active,*,}"), "\n", " ")
	}
	tileCount := func() int { return len(view.tileRows()) }
	pillNumbers := func() []string { p, _ := view.names(); return p }
	windowNames := func() []string { _, n := view.names(); return n }

	s.start("set -g mouse on\nset -g renumber-windows on\nrun-shell "+pluginPath(t)+"\n", 140, 44, "A")
	for _, name := range "BCDE" {
		s.tm("new-window", "-t", "t:", "-n", string(name), "-d")
	}
	s.tm("select-window", "-t", "t:0")
	s.tm("run-shell", bin+" sidebar toggle %0")

	c := s.attach(44, 140)
	time.Sleep(time.Second)
	send := func(data string) { c.send(t, data, 200*time.Millisecond) }
	const settle = 6 * time.Second

	expect(t, "sidebar is up in every window", tileCount, 5, 25*time.Second)
	expect(t, "start", order, "0:A* 1:B 2:C 3:D 4:E", settle)
	rows := view.tileRows()
	needRows(t, rows, 5)
	tall := false
	for i := 1; i < len(rows); i++ {
		tall = tall || rows[i]-rows[i-1] > 5
	}
	check(t, "boxes are 5 rows or less here (plain windows outside a repo)", !tall, fmt.Sprintf("rows=%v", rows))

	send(mouse(0, rows[2], "M"))
	send(mouse(0, rows[2], "m"))
	expect(t, "click switches window", order, "0:A 1:B 2:C* 3:D 4:E", settle)

	expect(t, "sidebar of the new window is up", tileCount, 5, settle)
	rows = view.tileRows()
	needRows(t, rows, 5)
	send(mouse(0, rows[1], "M"))
	send(mouse(32, rows[2], "M"))
	send(mouse(32, rows[3], "M"))
	expect(t, "mid-drag: list previews the new order", windowNames, []string{"A", "C", "D", "B", "E"}, settle)
	expect(t, "mid-drag: pills show the numbers the windows will get", pillNumbers, []string{"0", "1", "2", "3", "4"}, settle)
	expect(t, "mid-drag: grabbed box says moving", func() bool { return view.says("moving") }, true, settle)
	expect(t, "mid-drag: grabbed card is lifted under the pointer", func() bool { return view.says("╔") }, true, settle)
	check(t, "mid-drag: real windows have not moved yet", order() == "0:A 1:B 2:C* 3:D 4:E", order())
	send(mouse(0, rows[3], "m"))
	expect(t, "drop moves B after D and keeps C selected", order, "0:A 1:C* 2:D 3:B 4:E", settle)

	expect(t, "sidebar redrawn after the move", tileCount, 5, settle)
	rows = view.tileRows()
	needRows(t, rows, 5)
	send(mouse(0, rows[4], "M"))
	send(mouse(32, rows[0], "M"))
	send(mouse(0, rows[0], "m"))
	expect(t, "drag last box to the front", order, "0:E 1:A 2:C* 3:D 4:B", settle)

	pane := view.pane()
	if pane == "" {
		t.Fatal("no sidebar pane in the current window")
	}
	s.tm("send-keys", "-t", pane, "v")
	expect(t, "v switches to one line per window", view.compact, compactState{5, false}, settle)
	rows = view.compactRows()
	needRows(t, rows, 5)
	send(mouse(0, rows[3], "M"))
	send(mouse(32, rows[1], "M"))
	send(mouse(0, rows[1], "m"))
	expect(t, "compact layout: drag 4th line to 2nd", order, "0:E 1:D 2:A 3:C* 4:B", settle)

	if pane = view.pane(); pane == "" {
		t.Fatal("no sidebar pane in the current window")
	}
	s.tm("send-keys", "-t", pane, "v")
}
