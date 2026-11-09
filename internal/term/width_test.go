package term

import "testing"

func TestWidth(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"ascii", "hello", 5},
		{"empty", "", 0},
		{"emoji train is two columns", "\U0001F686", 2},
		{"emoji then text", "\U0001F686ab", 4},
		{"combining acute adds nothing", "é", 1},
		{"cjk", "日本", 4},
		{"nerd font private use glyph is one column", "", 1},
		{"braille grip", "⠿", 1},
		{"ellipsis", "…", 1},
		{"box drawing", "╭─╮", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Width(tt.in); got != tt.want {
				t.Errorf("Width(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestClip(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{"fits", "abc", 3, "abc"},
		{"cut with ellipsis", "abcdef", 4, "abc…"},
		{"limit one", "abcdef", 1, "…"},
		{"wide characters are not split", "日本語", 4, "日…"},
		{"emoji at the edge", "a\U0001F686b", 3, "a…"},
		{"empty", "", 5, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Clip(tt.in, tt.limit)
			if got != tt.want {
				t.Errorf("Clip(%q, %d) = %q, want %q", tt.in, tt.limit, got, tt.want)
			}
			if Width(got) > max(tt.limit, 1) {
				t.Errorf("clipped text is %d wide, limit %d", Width(got), tt.limit)
			}
		})
	}
}

func TestFit(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{"fits untouched", "\x1b[31mabc\x1b[0m", 3, "\x1b[31mabc\x1b[0m"},
		{"cut keeps earlier escapes and resets", "\x1b[31mabcdef\x1b[0m", 3, "\x1b[31mabc\x1b[0m"},
		{"escape is never split", "ab\x1b[1;32mcd", 2, "ab\x1b[1;32m\x1b[0m"},
		{"wide character that does not fit is dropped", "a日", 2, "a\x1b[0m"},
		{"plain text", "abcdef", 4, "abcd\x1b[0m"},
		{"zero limit", "abc", 0, "\x1b[0m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Fit(tt.in, tt.limit)
			if got != tt.want {
				t.Errorf("Fit(%q, %d) = %q, want %q", tt.in, tt.limit, got, tt.want)
			}
			if VisibleWidth(got) > tt.limit {
				t.Errorf("fitted text is %d wide, limit %d", VisibleWidth(got), tt.limit)
			}
		})
	}
}

func TestPad(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{"plain", "ab", 4, "ab  "},
		{"escapes do not count", "\x1b[31mab\x1b[0m", 4, "\x1b[31mab\x1b[0m  "},
		{"already full", "abcd", 4, "abcd"},
		{"wider than limit", "abcdef", 4, "abcdef"},
		{"emoji counts two", "\U0001F686", 3, "\U0001F686 "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Pad(tt.in, tt.limit); got != tt.want {
				t.Errorf("Pad(%q, %d) = %q, want %q", tt.in, tt.limit, got, tt.want)
			}
		})
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct{ in, want string }{
		{"\x1b[1;31mred\x1b[0m", "red"},
		{"plain", "plain"},
		{"\x1b[Hhome", "\x1b[Hhome"},
		{"a\x1b[38;5;173mb", "ab"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := StripANSI(tt.in); got != tt.want {
				t.Errorf("StripANSI(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
