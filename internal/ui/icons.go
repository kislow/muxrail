package ui

import (
	"strings"

	"github.com/kislow/muxrail/internal/agents"
	"github.com/kislow/muxrail/internal/term"
	"github.com/kislow/muxrail/internal/windows"
)

const (
	train      = "\U0001F686" // plain windows
	locomotive = "\U0001F682" // agents that match the version pattern but are not recognised
	grip       = "⠿"          // always on the card edge so it reads as draggable
	branchIcon = ""
)

// icon is how one kind of window is drawn: a glyph, its SGR colour in the sidebar and its tmux colour in the tab bar.
type icon struct{ glyph, sgr, tmuxColour string }

// Brand glyphs are Nerd Fonts 3.5+ codicons; emoji stand in for everything else so any terminal can draw them.
var (
	brands = map[string]icon{
		agents.Claude: {"", orange, "colour173"},
		agents.Codex:  {"", cyan, "cyan"},
		agents.Cursor: {"", white, "brightwhite"},
		agents.Gemini: {"", blue, "blue"},
	}
	unknownAgent = icon{locomotive, magenta, "magenta"}
	plainWindow  = icon{train, dim, "brightblack"}
)

func iconFor(kind string, isAgent bool) icon {
	if i, ok := brands[kind]; ok {
		return i
	}
	if isAgent {
		return unknownAgent
	}
	return plainWindow
}

// iconCell pads to two columns so one-column logos and two-column emoji start the name at the same place.
func iconCell(glyph string) string {
	return glyph + strings.Repeat(" ", max(2-term.Width(glyph), 0))
}

func agentIcon(w *windows.Win) string {
	i := iconFor(w.Kind, w.IsAgent())
	return i.sgr + iconCell(i.glyph) + fgReset
}

// TabIcon is the tab-bar icon for a window as tmux style codes; plain leaves the colour out for the current tab, whose
// coloured background the brand colours would clash with.
func TabIcon(kind, command string, plain bool) string {
	i := iconFor(kind, kind != "" || agents.IsVersion(command))
	pad := strings.Repeat(" ", max(2-term.Width(i.glyph), 0))
	if plain {
		return i.glyph + pad
	}
	return "#[fg=" + i.tmuxColour + "]" + i.glyph + "#[fg=default]" + pad
}
