package sessions

import (
	"strings"
	"time"

	"github.com/kislow/muxrail/internal/ago"
)

const (
	labelWidth = 40
	cwdWidth   = 24
	ageWidth   = 4
	noPrompt   = "(no prompt)"

	sgrMagenta = "\x1b[35m"
	sgrGreen   = "\x1b[32m"
	sgrCyan    = "\x1b[36m"
	sgrDim     = "\x1b[90m"
	sgrReset   = "\x1b[0m"
)

var agentColours = map[string]string{"claude": sgrMagenta, "codex": sgrGreen, "cursor": sgrCyan}

// label is what names a session in the list: its title, else its first prompt.
func label(s *Session) string {
	if s.Title != "" {
		return s.Title
	}
	if len(s.Prompts) > 0 {
		return s.Prompts[0]
	}
	return noPrompt
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func runePrefix(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func padRight(s string, n int) string {
	return s + strings.Repeat(" ", max(n-len([]rune(s)), 0))
}

// Row is one line of the fzf list: agent, id and folder as the first tab fields, then the coloured display. The
// prompt tail is clipped on screen but fzf still searches all of it.
func Row(s *Session, now time.Time) string {
	mark := sgrDim + "○" + sgrReset
	if s.Renamed {
		mark = sgrGreen + "●" + sgrReset
	}
	label := label(s)
	prompts := make([]string, len(s.Prompts))
	for i, p := range s.Prompts {
		prompts[i] = oneLine(p)
	}
	age := ago.Since(now, s.TS)
	// The age sits beside the agent, not at the end: the preview pane halves the list and clips the right-hand columns.
	head := agentColours[s.Agent] + padRight(s.Agent, 6) + sgrReset + " " + sgrDim + strings.Repeat(" ", max(ageWidth-len(age), 0)) + age + sgrReset + " " + mark + " " +
		padRight(runePrefix(oneLine(label), labelWidth), labelWidth) + " " + sgrDim + padRight(runePrefix(basename(s.CWD), cwdWidth), cwdWidth)
	return s.Agent + "\t" + s.ID + "\t" + s.CWD + "\t" + head + "  " + strings.Join(prompts, " | ") + sgrReset
}
