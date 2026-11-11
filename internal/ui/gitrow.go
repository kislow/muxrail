package ui

import (
	"fmt"
	"strings"

	"github.com/kislow/muxrail/internal/gitinfo"
	"github.com/kislow/muxrail/internal/term"
)

const minBranchRoom = 6

// kfmt abbreviates a line count from 1000 on: 1.3k, 12k.
func kfmt(n int) string {
	if n < 1000 {
		return fmt.Sprint(n)
	}
	return strings.ReplaceAll(fmt.Sprintf("%.1fk", float64(n)/1000), ".0k", "k")
}

// diffChip is "+added -deleted" in green and red, with an optional pen glyph in front; zero parts are left out.
func diffChip(l gitinfo.Lines, pen string) string {
	var parts []string
	if l.Added != 0 {
		parts = append(parts, green+"+"+kfmt(l.Added))
	}
	if l.Deleted != 0 {
		parts = append(parts, red+"-"+kfmt(l.Deleted))
	}
	if len(parts) == 0 {
		return ""
	}
	prefix := ""
	if pen != "" {
		prefix = dim + pen + fgReset + " "
	}
	return prefix + strings.Join(parts, " ") + fgReset
}

// gitRow is the branch, then committed and uncommitted changes (pen glyph); the branch name gives way first.
func gitRow(set Icons, info *gitinfo.Info, room int) string {
	if info == nil {
		return ""
	}
	var chips []string
	for _, c := range []string{diffChip(info.Committed, ""), diffChip(info.Dirty, "✎")} {
		if c != "" {
			chips = append(chips, c)
		}
	}
	stats := strings.Join(chips, " ")
	taken := 0
	if stats != "" {
		taken = visible(stats) + 1
	}
	branchRoom := max(room-2-taken, minBranchRoom)
	row := blue + set.branch() + " " + term.Clip(info.Branch, branchRoom) + fgReset
	if stats != "" {
		row += " " + stats
	}
	return row
}
