package ui

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/kislow/muxrail/internal/agents"
	"github.com/kislow/muxrail/internal/term"
	"github.com/kislow/muxrail/internal/windows"
)

var shellTitleRE = regexp.MustCompile(`@.*:`)

// accent is the colour of a window's pill: green when active, otherwise cycled by index (yellow stays free for the bell).
func accent(w *windows.Win) int {
	if w.Active {
		return currentColour
	}
	return palette[w.Index%len(palette)]
}

func (e *Env) indexPill(w *windows.Win, rowBG string) string {
	c := accent(w)
	return fg(c) + e.CapL + bg(c) + fgReset + strconv.Itoa(w.Index) + normal + rowBG + fg(c) + e.CapR + fgReset
}

func activeBG(w *windows.Win) string {
	if w.Active {
		return greyBG
	}
	return ""
}

// titleOf is the agent's task title (Claude puts it in the pane title behind a status symbol); shell titles such as
// user@host:~ are not one.
func titleOf(w *windows.Win) string {
	title := strings.TrimSpace(w.Title)
	if !w.IsAgent() || title == "" || shellTitleRE.MatchString(title) {
		return ""
	}
	first := []rune(title)[0]
	if unicode.IsLetter(first) || unicode.IsNumber(first) {
		return title
	}
	if _, rest, ok := strings.Cut(title, " "); ok {
		return rest
	}
	return title
}

// shortPath is the path relative to home (~), ellipsised on the left to fit limit columns.
func (e *Env) shortPath(path string, limit int) string {
	p := path
	if e.Home != "" && strings.HasPrefix(path, e.Home) {
		p = "~" + path[len(e.Home):]
	}
	if term.Width(p) <= limit {
		return p
	}
	runes := []rune(p)
	keep := max(limit-1, 0)
	return "…" + string(runes[max(len(runes)-keep, 0):])
}

// displayName is the window name; an auto-named window (a bare version number) takes the task title when there is one.
func displayName(w *windows.Win, task string) string {
	if agents.IsVersion(w.Name) && task != "" {
		return task
	}
	return w.Name
}

// bodyRows are only the rows that have something to say: the name, then the task (or path), then the branch, so cards
// stay small.
func (e *Env) bodyRows(w *windows.Win, width int) []string {
	inner := max(width-4, 1) // two border columns plus a space on each side
	pill := e.indexPill(w, activeBG(w))
	indent := visible(pill) + 1
	chip := e.stateChip(w, width)
	chipWidth := visible(chip)
	task := titleOf(w)
	narrow := width < narrowWidth // the chip would leave the name a couple of characters, so it moves to the detail row
	nameRoom := inner - indent - 2
	if chip != "" && !narrow {
		nameRoom -= chipWidth + 1
	}
	head := pill + " " + agentIcon(w) + bold + term.Clip(displayName(w, task), max(nameRoom, 1)) + boldOff
	if !narrow {
		head = term.Pad(head, inner-chipWidth) + chip
	}
	rows := []string{head}
	if detail := e.detailRow(w, chip, task, inner-indent, narrow); detail != "" {
		rows = append(rows, strings.Repeat(" ", indent)+detail)
	}
	if act := activityRow(w, e, inner-indent); act != "" {
		rows = append(rows, strings.Repeat(" ", indent)+act)
	}
	if w.Git != nil {
		rows = append(rows, strings.Repeat(" ", indent)+gitRow(w.Git, inner-indent))
	}
	return rows
}

func (e *Env) detailRow(w *windows.Win, chip, task string, room int, narrow bool) string {
	path := dim + e.shortPath(w.Path, room) + fgReset
	if !w.IsAgent() {
		return path
	}
	text := ""
	if task != "" && !agents.IsVersion(w.Name) {
		text = dim + task + fgReset
	}
	detail := ""
	if narrow && chip != "" {
		detail = chip + " "
	}
	detail += text
	if text == "" && w.Git == nil {
		detail += path
	}
	return detail
}

// glyphs are the characters a box is drawn with.
type glyphs struct{ tl, tr, bl, br, h, v string }

var (
	roundBox  = glyphs{"╭", "╮", "╰", "╯", "─", "│"}
	liftedBox = glyphs{"╔", "╗", "╚", "╝", "═", "║"} // the card in the hand
	slotBox   = glyphs{"╭", "╮", "╰", "╯", "┄", "┊"} // where it will land
)

// edge is a top or bottom border; the optional grip and label sit in the line and are clipped on narrow panes.
func edge(left, right, fill string, width int, border, grip, label string) string {
	room := max(width-2, 0)
	var inline string
	if grip != "" {
		inline = fill + grip + fill
	}
	if label != "" {
		inline += " " + label + " "
	}
	text := term.Fit(inline, room)
	return border + left + text + border + strings.Repeat(fill, max(room-visible(text), 0)) + right + normal
}

func borderStyle(w *windows.Win, selected, grabbed bool) string {
	switch {
	case grabbed:
		return magenta
	case w.Active:
		return fg(currentColour)
	case selected:
		return white
	}
	return dim
}

// card is a rounded box around one window's rows.
func (e *Env) card(w *windows.Win, border string, width int, label string) []string {
	return e.box(w, e.bodyRows(w, width), roundBox, border, width, grip, label)
}

// box draws rows inside the given glyphs; the window only decides the row background.
func (e *Env) box(w *windows.Win, rows []string, g glyphs, border string, width int, grip, label string) []string {
	inner := max(width-4, 1)
	rowBG := activeBG(w)
	out := make([]string, 0, len(rows)+2)
	out = append(out, edge(g.tl, g.tr, g.h, width, border, grip, label))
	for _, r := range rows {
		out = append(out, border+g.v+normal+rowBG+" "+term.Pad(term.Fit(r, inner), inner)+" "+normal+border+g.v+normal)
	}
	return append(out, edge(g.bl, g.br, g.h, width, border, "", ""))
}

// compactCard is one line per window, no box: marker bar, pill, icon, name and state chip.
func (e *Env) compactCard(w *windows.Win, width int, marker string) string {
	pill := e.indexPill(w, activeBG(w))
	chip := e.stateChip(w, width)
	chipWidth := visible(chip)
	room := width - 1 - 4 - 2 - 1
	if chip != "" {
		room -= chipWidth + 1
	}
	name := displayName(w, titleOf(w))
	head := marker + normal + activeBG(w) + pill + " " + agentIcon(w) + bold + term.Clip(name, max(room, 1)) + boldOff
	return term.Fit(term.Pad(head, width-1-chipWidth)+chip, width-1) + normal
}
