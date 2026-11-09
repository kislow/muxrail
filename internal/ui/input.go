package ui

import "github.com/kislow/muxrail/internal/term"

// Drag is a grabbed card: Source is where it started, Over the slot it would be dropped in, Y the pointer's 1-based screen
// row, Grab the row of the card the press landed on (so the card stays under the pointer where it was taken), and Moved
// whether the pointer has travelled since the press (a plain click never gets the lifted look).
type Drag struct {
	Source, Over, Y, Grab int
	Moved                 bool
}

// Move asks for the window at From to take the slot of the window at To.
type Move struct{ From, To int }

// Input is what one chunk of input decided: the new cursor and drag state, plus at most one action to carry out.
type Input struct {
	Selected     int
	Drag         *Drag
	Switch       *int
	Move         *Move
	ToggleLayout bool
}

// CardAt is the card under 1-based screen row y, given every card's real height; rows above the first card hit nothing.
func CardAt(y int, heights []int) (int, bool) {
	row := y - 1 - headerRows
	if row < 0 {
		return 0, false
	}
	for i, h := range heights {
		if row < h {
			return i, true
		}
		row -= h
	}
	return 0, false
}

// cardTop is the 1-based screen row of the first line of card pos.
func cardTop(pos int, heights []int) int {
	row := 1 + headerRows
	for _, h := range heights[:pos] {
		row += h
	}
	return row
}

// Apply interprets data (complete escape sequences only) against the current cursor and drag.
func Apply(data string, heights []int, selected int, drag *Drag) *Input {
	in := &Input{Selected: selected}
	if drag != nil {
		held := *drag
		in.Drag = &held
	}
	events, plain := term.ParseMouse(data)
	for _, m := range events {
		in.mouse(m, heights)
	}
	in.key(plain, len(heights))
	return in
}

// mouse: a press grabs a card, button-motion picks the drop slot, a release either clicks (same card) or moves.
func (in *Input) mouse(m term.Mouse, heights []int) {
	pos, hit := CardAt(m.Y, heights)
	switch {
	case m.Button == term.ButtonPress && !m.Release && hit:
		in.Selected, in.Drag = pos, &Drag{Source: pos, Over: pos, Y: m.Y, Grab: m.Y - cardTop(pos, heights)}
	case m.Button == term.ButtonMotion && in.Drag != nil:
		in.Drag.Y, in.Drag.Moved = m.Y, true
		if hit {
			in.Drag.Over = pos
		}
	case m.Button == term.ButtonPress && m.Release && in.Drag != nil:
		src, dst := in.Drag.Source, in.Drag.Over
		if hit {
			dst = pos
		}
		in.Drag = nil
		if src == dst {
			in.Switch = &src
			return
		}
		in.Move, in.Selected = &Move{From: src, To: dst}, dst
	}
}

func (in *Input) key(plain string, count int) {
	last, cur := count-1, in.Selected
	switch plain {
	case "j", "\x1b[B":
		in.Selected = min(cur+1, last)
	case "k", "\x1b[A":
		in.Selected = max(cur-1, 0)
	case "J", "K":
		dst := max(cur-1, 0)
		if plain == "J" {
			dst = min(cur+1, last)
		}
		if cur >= 0 && cur <= last && dst != cur {
			in.Move, in.Selected = &Move{From: cur, To: dst}, dst
		}
	case "\r", "\n":
		in.Switch = &cur
	case "v":
		in.ToggleLayout = true
	}
}
