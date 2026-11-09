package ui

import (
	"strconv"
	"strings"

	"github.com/kislow/muxrail/internal/term"
	"github.com/kislow/muxrail/internal/windows"
)

// View is the state a frame is drawn from, besides the windows themselves.
type View struct {
	Width, Rows int
	Layout      string
	Selected    int
	Drag        *Drag       // nil when no card is grabbed
	Slide       map[int]int // rows each card (by real index) is still short of its place; a card in flight is drawn that far off
}

// DisplayOrder is the order cards are drawn in. While dragging, the grabbed card sits in the slot it would land in, so
// the list previews the result; the windows themselves do not move until the drop.
func DisplayOrder(count int, drag *Drag) []int {
	order := make([]int, count)
	for i := range order {
		order[i] = i
	}
	if drag == nil || drag.Source == drag.Over || drag.Source >= count {
		return order
	}
	grabbed := order[drag.Source]
	order = append(order[:drag.Source], order[drag.Source+1:]...)
	at := min(drag.Over, len(order))
	return append(order[:at], append([]int{grabbed}, order[at:]...)...)
}

// Targets is the first line of every card (by real index) in the order the list is drawn in; the animation heads there.
func Targets(heights []int, drag *Drag) []int {
	tops := make([]int, len(heights))
	line := headerRows
	for _, i := range DisplayOrder(len(heights), drag) {
		tops[i] = line
		line += heights[i]
	}
	return tops
}

// Heights are the real row counts of every card in the real window order; hit-testing must not follow the drag preview.
func (e *Env) Heights(wins []*windows.Win, width int, layout string) []int {
	heights := make([]int, len(wins))
	for i, w := range wins {
		heights[i] = 1
		if layout != LayoutCompact {
			heights[i] = len(e.bodyRows(w, width)) + 2
		}
	}
	return heights
}

// Frame is one full repaint: the cursor goes home and every line is overwritten in place, ending in clear-to-end-of-line.
// Clearing the screen first would flicker.
func (e *Env) Frame(wins []*windows.Win, v View) string { return JoinFrame(e.Lines(wins, v)) }

// JoinFrame turns Lines into the bytes of a full repaint.
func JoinFrame(lines []string) string {
	return "\x1b[H" + strings.Join(lines, clearEOL+"\r\n") + clearEOL
}

// Lines is the screen, one string per row, the last being the hint.
func (e *Env) Lines(wins []*windows.Win, v View) []string {
	hide := ""
	if v.Width >= hideHintWidth {
		hide = "   C-a b hides"
	}
	lines := make([]string, max(v.Rows-1, 0))
	header := []string{
		term.Fit(" "+bold+dim+"WINDOWS"+boldOff+"  "+strconv.Itoa(len(wins))+hide+fgReset, v.Width-1),
		" " + dim + strings.Repeat("─", max(v.Width-2, 0)) + fgReset,
	}
	copy(lines, header)
	cursor := headerRows
	for slot, i := range DisplayOrder(len(wins), v.Drag) {
		card := e.cardLines(wins[i], slot, i, v)
		top := cursor
		if v.Drag == nil || i != v.Drag.Source {
			top += v.Slide[i]
		}
		cursor += len(card)
		for k, line := range card {
			if y := top + k; y >= headerRows && y < len(lines) {
				lines[y] = line
			}
		}
	}
	lines = e.overlayLifted(lines, wins, v)
	hint := "click · drag · j/k · J/K · v"
	if v.Drag != nil {
		hint = "drop to place it there"
	}
	lines = append(lines, term.Fit(" "+dim+hint+normal, v.Width-1))
	return lines
}

// cardLines draws window i, which sits in display slot slot. While dragging, the pill shows the number the window
// would get.
func (e *Env) cardLines(w *windows.Win, slot, i int, v View) []string {
	grabbed := v.Drag != nil && i == v.Drag.Source
	if grabbed && v.Drag.Moved {
		return e.slot(w, v)
	}
	shown := w
	if v.Drag != nil {
		copied := *w
		copied.Index = slot
		shown = &copied
	}
	if v.Layout == LayoutCompact {
		return []string{e.compactCard(shown, v.Width, marker(shown, i == v.Selected, grabbed))}
	}
	label := ""
	if grabbed {
		label = "moving"
	}
	return e.card(shown, borderStyle(shown, i == v.Selected, grabbed), v.Width, label)
}

func marker(w *windows.Win, selected, grabbed bool) string {
	bar := ""
	switch {
	case grabbed:
		bar = magenta
	case w.Active:
		bar = fg(currentColour)
	case selected:
		bar = white
	}
	if bar == "" {
		return " "
	}
	return bar + "▌"
}

// lifted is the grabbed card as it looks in the hand: double border, numbered with the slot it would land in.
func (e *Env) lifted(w *windows.Win, v View) []string {
	held := *w
	held.Index = v.Drag.Over
	if v.Layout == LayoutCompact {
		return []string{e.compactCard(&held, v.Width, magenta+bold+"▶")}
	}
	return e.box(&held, e.bodyRows(&held, v.Width), liftedBox, magenta+bold, v.Width, grip, "moving")
}

// slot is the gap the grabbed card leaves in the list: a dashed outline of the same height, at the drop position.
func (e *Env) slot(w *windows.Win, v View) []string {
	if v.Layout == LayoutCompact {
		return []string{term.Fit(" "+magenta+"┄┄ drop here ┄┄"+normal, v.Width-1)}
	}
	rows := make([]string, len(e.bodyRows(w, v.Width)))
	return e.box(w, rows, slotBox, magenta, v.Width, "", "drop here")
}

// overlayLifted draws the grabbed card over the list, in the list's own columns, with the row it was grabbed by under the
// pointer, clamped to the list area.
func (e *Env) overlayLifted(lines []string, wins []*windows.Win, v View) []string {
	if v.Drag == nil || !v.Drag.Moved || v.Drag.Source >= len(wins) {
		return lines
	}
	card := e.lifted(wins[v.Drag.Source], v)
	top := min(v.Drag.Y-1-v.Drag.Grab, len(lines)-1-len(card)) // the last row is the hint
	top = max(top, headerRows)
	for i, line := range card {
		if top+i < len(lines)-1 {
			lines[top+i] = line
		}
	}
	return lines
}
