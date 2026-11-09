package ui

import (
	"time"

	"github.com/kislow/muxrail/internal/ago"
	"github.com/kislow/muxrail/internal/term"
	"github.com/kislow/muxrail/internal/windows"
)

const (
	spinner = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
	// SpinFPS is how often the working spinner advances.
	SpinFPS = 8
)

type stateStyle struct{ glyph, sgr string }

var (
	states       = map[string]stateStyle{"working": {"●", cyan}, "waiting": {"◆", yellow}, "done": {"✔", green}}
	unknownState = stateStyle{"○", dim}
)

func spin(now time.Time) string {
	frames := []rune(spinner)
	return string(frames[int(float64(now.UnixNano())/1e9*SpinFPS)%len(frames)])
}

// stateChip is the state glyph (a spinner while working), the word when there is room, and how long the window has
// been in that state. Only agents have one.
func (e *Env) stateChip(w *windows.Win, width int) string {
	if !w.IsAgent() {
		return ""
	}
	st, known := states[w.Status]
	if !known {
		st = unknownState
	}
	glyph := st.glyph
	if w.Status == "working" {
		glyph = spin(e.Now)
	}
	word, since := "", ""
	if w.Status != "" && width >= chipWordWidth {
		word = " " + w.Status
	}
	if w.Since != 0 {
		since = " " + ago.Since(e.Now, w.Since)
	}
	return st.sgr + glyph + word + since + fgReset
}

func visible(s string) int { return term.VisibleWidth(s) }
