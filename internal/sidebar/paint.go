package sidebar

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/kislow/muxrail/internal/ui"
)

// painter turns successive screens into the bytes that change the terminal from one to the next. Writing only the rows
// that differ keeps a drag cheap: a moving card touches a handful of rows, not the whole pane.
type painter struct {
	prev      []string
	lastWrite time.Time
}

// diff returns what to write for lines. The whole screen goes out on the first call, after a resize and every
// repaintEvery, in case tmux disturbed the pane; nothing goes out when nothing changed.
func (p *painter) diff(lines []string, now time.Time) string {
	full := len(p.prev) != len(lines) || now.Sub(p.lastWrite) > repaintEvery
	if full {
		p.remember(lines, now)
		return ui.JoinFrame(lines)
	}
	var b strings.Builder
	for i, line := range lines {
		if line != p.prev[i] {
			fmt.Fprintf(&b, "\x1b[%d;1H%s\x1b[K", i+1, line)
		}
	}
	if b.Len() > 0 {
		p.remember(lines, now)
	}
	return b.String()
}

func (p *painter) remember(lines []string, now time.Time) {
	p.prev, p.lastWrite = slices.Clone(lines), now
}

// animate moves every card a step towards its target line and returns how far each (by real index) still has to go.
// Cards only slide while a drag is on or just ended; any other change of the list snaps, so a window opening or a git
// row appearing never smears across the pane.
func (l *loop) animate(targets []int, now time.Time) map[int]int {
	if l.drag != nil {
		l.animUntil = now.Add(settleTime)
	}
	live := now.Before(l.animUntil)
	next := make(map[string]int, len(targets))
	slide := map[int]int{}
	for i, w := range l.wins {
		at, seen := l.shown[w.ID]
		if live && seen {
			at = approach(at, targets[i])
		} else {
			at = targets[i]
		}
		next[w.ID] = at
		if at != targets[i] {
			slide[i] = at - targets[i]
		}
	}
	l.shown, l.sliding = next, len(slide) > 0
	return slide
}

// approach covers about a third of the distance each frame, at least one row, so a card eases in over four or five frames.
func approach(at, target int) int {
	d := target - at
	step := (max(d, -d) + 2) / 3
	if d < 0 {
		step = -step
	}
	return at + step
}
