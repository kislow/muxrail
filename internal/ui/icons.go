package ui

import (
	"strings"

	"github.com/kislow/muxrail/internal/agents"
	"github.com/kislow/muxrail/internal/term"
	"github.com/kislow/muxrail/internal/windows"
)

const (
	train         = "\U0001F686" // plain windows
	locomotive    = "\U0001F682" // agents that match the version pattern but are not recognised
	grip          = "⠿"          // always on the card edge so it reads as draggable
	branchNerd    = ""
	branchUnicode = "⎇"
)

// icon is how one kind of window is drawn: a glyph, its SGR colour in the sidebar and its tmux colour in the tab bar.
type icon struct{ glyph, sgr, tmuxColour string }

// Icons picks the glyphs the agent logos and the branch mark are drawn with.
type Icons string

const (
	// IconsUnicode draws plain Unicode symbols that any monospaced font, or macOS's symbol fallback, has: nothing to install.
	IconsUnicode Icons = "unicode"
	// IconsNerd draws the brand logos, which need a Nerd Fonts 3.5+ font selected in the terminal.
	IconsNerd Icons = "nerd"
)

// ParseIcons reads @muxrail_icons: "nerd" selects the Nerd Font logos, anything else, unset included, is IconsUnicode.
func ParseIcons(v string) Icons {
	if v == "nerd" {
		return IconsNerd
	}
	return IconsUnicode
}

func (i Icons) branch() string {
	if i == IconsNerd {
		return branchNerd
	}
	return branchUnicode
}

func (i Icons) brands() map[string]icon {
	if i == IconsNerd {
		return brandsNerd
	}
	return brandsUnicode
}

// The Nerd logos are Nerd Fonts 3.5+ codicons; the Unicode set uses symbols the state marks (● ◆ ✔ ○) do not. Emoji stand
// in for everything else in both sets so any terminal can draw them.
var (
	brandsUnicode = map[string]icon{
		agents.Claude: {"✻", orange, "colour173"},
		agents.Codex:  {"⌘", cyan, "cyan"},
		agents.Cursor: {"➤", white, "brightwhite"},
		agents.Gemini: {"✦", blue, "blue"},
	}
	brandsNerd = map[string]icon{
		agents.Claude: {"", orange, "colour173"},
		agents.Codex:  {"", cyan, "cyan"},
		agents.Cursor: {"", white, "brightwhite"},
		agents.Gemini: {"", blue, "blue"},
	}
	unknownAgent = icon{locomotive, magenta, "magenta"}
	plainWindow  = icon{train, dim, "brightblack"}
)

func iconFor(set Icons, kind string, isAgent bool) icon {
	if i, ok := set.brands()[kind]; ok {
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

func agentIcon(set Icons, w *windows.Win) string {
	i := iconFor(set, w.Kind, w.IsAgent())
	return i.sgr + iconCell(i.glyph) + fgReset
}

// TabIcon is the tab-bar icon for a window as tmux style codes; plain leaves the colour out for the current tab, whose
// coloured background the brand colours would clash with.
func TabIcon(set Icons, kind, command string, plain bool) string {
	i := iconFor(set, kind, kind != "" || agents.IsVersion(command))
	pad := strings.Repeat(" ", max(2-term.Width(i.glyph), 0))
	if plain {
		return i.glyph + pad
	}
	return "#[fg=" + i.tmuxColour + "]" + i.glyph + "#[fg=default]" + pad
}
