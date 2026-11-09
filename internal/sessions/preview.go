package sessions

import (
	"fmt"
	"strings"
	"time"

	"github.com/kislow/muxrail/internal/ago"
)

const previewTurns = 8

// Preview is the text of fzf's preview pane for one session: where it ran, how old it is and its latest messages.
func Preview(home, agent, id, cwd string, now time.Time) (string, error) {
	tr, err := Locate(home, agent, id)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s%s  %s\n%s%s%s\n", agentColours[agent], agent, sgrReset, id, sgrDim, cwd, sgrReset)
	if tr == nil {
		b.WriteString("\nNo readable transcript for this session.\n")
		return b.String(), nil
	}
	fmt.Fprintf(&b, "%slast active %s (%s)%s\n", sgrDim, activeAgo(now, tr.Modified), stamp(tr.Modified, now), sgrReset)
	turns, err := LastTurns(tr.Path, previewTurns)
	if err != nil {
		return "", err
	}
	for _, t := range turns {
		who, colour := "you", sgrCyan
		if t.Role == "assistant" {
			who, colour = agent, agentColours[agent]
		}
		fmt.Fprintf(&b, "\n%s%s%s\n%s\n", colour, who, sgrReset, clipText(t.Text, turnRunes))
	}
	return b.String(), nil
}

func clipText(s string, limit int) string {
	if r := []rune(s); len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return s
}

// activeAgo reads "just now" or "5m ago" for the preview header.
func activeAgo(now, modified time.Time) string {
	age := ago.Since(now, float64(modified.UnixNano())/1e9)
	if age == "now" {
		return "just now"
	}
	return age + " ago"
}

// stamp is the exact date and time, without the year when it is this year's.
func stamp(t, now time.Time) string {
	if t.Year() == now.Year() {
		return t.Format("Mon 2 Jan 15:04")
	}
	return t.Format("2 Jan 2006 15:04")
}
