// Package ui draws the sidebar: icons, state chips, window cards, the frame, and turns mouse and key input into actions.
package ui

import (
	"strconv"
	"time"
)

// SGR styles. Only the 16 colours and the one 256-colour Claude orange are used so the terminal's theme applies.
const (
	fgReset  = "\x1b[39m"
	bold     = "\x1b[1m"
	boldOff  = "\x1b[22m"
	dim      = "\x1b[90m"
	cyan     = "\x1b[36m"
	magenta  = "\x1b[35m"
	green    = "\x1b[32m"
	yellow   = "\x1b[33m"
	white    = "\x1b[97m"
	red      = "\x1b[31m"
	blue     = "\x1b[34m"
	orange   = "\x1b[38;5;173m"
	greyBG   = "\x1b[100m"
	normal   = "\x1b[0m"
	clearEOL = "\x1b[K"
)

const (
	currentColour = 2 // the active window is green; the others cycle through palette by index
	narrowWidth   = 30
	hideHintWidth = 28
	chipWordWidth = 34
	headerRows    = 2
)

var palette = [...]int{1, 2, 4, 5, 6}

const (
	LayoutTiles   = "tiles"
	LayoutCompact = "compact"
)

// Env is what a render depends on besides the windows: the clock, the home directory for path shortening and the
// pill end-caps shared with the tab bar.
type Env struct {
	Now        time.Time
	Home       string
	CapL, CapR string
	// CtxWindow is the context size the gauge measures against; zero means 200k, or 1M once a context outgrows that.
	CtxWindow int
}

func fg(colour int) string { return "\x1b[3" + strconv.Itoa(colour) + "m" }
func bg(colour int) string { return "\x1b[4" + strconv.Itoa(colour) + "m" }
