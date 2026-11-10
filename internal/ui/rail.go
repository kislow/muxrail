package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/kislow/muxrail/internal/agents"
	"github.com/kislow/muxrail/internal/ago"
	"github.com/kislow/muxrail/internal/windows"
)

// station is a window's stop on the rail: the glyph and tmux colour of its agent's state.
type station struct{ glyph, colour string }

var (
	stations = map[string]station{
		"working": {"●", "cyan"},
		"waiting": {"◆", "yellow"},
		"done":    {"✔", "green"},
	}
	idleStation    = station{"○", "brightblack"}
	currentStation = station{"◉", "green"}
)

func stationFor(status string, current bool) station {
	if st, ok := stations[status]; ok {
		return st
	}
	if current {
		return currentStation
	}
	return idleStation
}

// RailTab is a window's stop on the tab rail as tmux style codes: the state station, the window index and the agent
// logo. The track between stops and the window name come from the tmux format around it.
func RailTab(set Icons, t windows.Tab, index string, current bool) string {
	st := stationFor(t.Status, current)
	idx, weight := "#[fg=brightblack]", ""
	if current {
		idx, weight = "#[fg=default,bold]", ",bold"
	}
	icon := TabIcon(set, t.Kind, t.Command, false)
	if !strings.HasSuffix(icon, " ") { // two-column emoji get no padding from TabIcon but still need room before the name
		icon += " "
	}
	return "#[fg=" + st.colour + weight + "]" + st.glyph + " " + idx + index + "#[default] " + icon
}

// StatusLine summarises the agents for the right of the status bar, what needs a person first: waiting (with the longest
// wait), then working, then done. Empty when no agent has a state.
func StatusLine(s agents.Summary, now time.Time) string {
	var out string
	add := func(count int, status, suffix string) {
		if count == 0 {
			return
		}
		st := stations[status]
		out += "#[fg=" + st.colour + "]" + st.glyph + " " + strconv.Itoa(count) + suffix + "#[default]  "
	}
	wait := ""
	if s.WaitingSince != 0 {
		wait = " " + ago.Since(now, s.WaitingSince)
	}
	add(s.Waiting, "waiting", wait)
	add(s.Working, "working", "")
	add(s.Done, "done", "")
	return out
}
