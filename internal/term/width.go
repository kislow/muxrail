// Package term holds terminal-facing primitives: display width, styled-text cutting, raw mode and input parsing.
package term

import (
	"sort"
	"strings"
	"unicode/utf8"
)

const reset = "\x1b[0m"

func inRanges(table [][2]rune, r rune) bool {
	i := sort.Search(len(table), func(i int) bool { return table[i][1] >= r })
	return i < len(table) && table[i][0] <= r
}

// RuneWidth is 0 for combining marks, 2 for East Asian wide and fullwidth (emoji) and 1 for the rest,
// private-use Nerd Font glyphs included.
func RuneWidth(r rune) int {
	switch {
	case r < 0x300:
		return 1
	case inRanges(combining, r):
		return 0
	case inRanges(wide, r):
		return 2
	}
	return 1
}

// Width is the display width of plain text; escape sequences must be removed first (see StripANSI).
func Width(s string) int {
	n := 0
	for _, r := range s {
		n += RuneWidth(r)
	}
	return n
}

// sgrLen is the length of an SGR sequence (ESC [ digits/semicolons m) starting at s[i], or 0.
func sgrLen(s string, i int) int {
	if s[i] != 0x1b || i+1 >= len(s) || s[i+1] != '[' {
		return 0
	}
	for j := i + 2; j < len(s); j++ {
		switch c := s[j]; {
		case c == 'm':
			return j + 1 - i
		case c >= '0' && c <= '9', c == ';':
		default:
			return 0
		}
	}
	return 0
}

func StripANSI(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if n := sgrLen(s, i); n > 0 {
			i += n
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// VisibleWidth is the display width of styled text.
func VisibleWidth(s string) int { return Width(StripANSI(s)) }

// Clip shortens plain text to limit columns, ending in an ellipsis when it had to cut.
func Clip(s string, limit int) string {
	if Width(s) <= limit {
		return s
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		w := RuneWidth(r)
		if used+w > limit-1 {
			break
		}
		b.WriteRune(r)
		used += w
	}
	b.WriteString("…")
	return b.String()
}

// Fit cuts styled text to limit columns without splitting an escape sequence, and resets the style when it cut,
// so no row can wrap and shift the ones below it.
func Fit(s string, limit int) string {
	var b strings.Builder
	used := 0
	for i := 0; i < len(s); {
		if n := sgrLen(s, i); n > 0 {
			b.WriteString(s[i : i+n])
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		w := RuneWidth(r)
		if used+w > limit {
			b.WriteString(reset)
			return b.String()
		}
		b.WriteString(s[i : i+size])
		used += w
		i += size
	}
	return b.String()
}

// Pad appends spaces until styled text fills limit columns.
func Pad(s string, limit int) string {
	return s + strings.Repeat(" ", max(0, limit-VisibleWidth(s)))
}
