package term

import (
	"reflect"
	"testing"
)

func TestParseMouse(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		wantEvents []Mouse
		wantRest   string
	}{
		{"press", "\x1b[<0;10;5M", []Mouse{{0, 10, 5, false}}, ""},
		{"motion while held", "\x1b[<32;11;6M", []Mouse{{32, 11, 6, false}}, ""},
		{"release", "\x1b[<0;10;7m", []Mouse{{0, 10, 7, true}}, ""},
		{"several in one chunk", "\x1b[<0;1;3M\x1b[<32;1;4M\x1b[<0;1;4m",
			[]Mouse{{0, 1, 3, false}, {32, 1, 4, false}, {0, 1, 4, true}}, ""},
		{"keys around a report stay", "j\x1b[<0;1;3Mk", []Mouse{{0, 1, 3, false}}, "jk"},
		{"plain keys", "v", nil, "v"},
		{"arrow is not a mouse report", "\x1b[B", nil, "\x1b[B"},
		{"malformed report stays", "\x1b[<0;1M", nil, "\x1b[<0;1M"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, rest := ParseMouse(tt.in)
			if !reflect.DeepEqual(events, tt.wantEvents) || rest != tt.wantRest {
				t.Errorf("ParseMouse(%q) = %v, %q; want %v, %q", tt.in, events, rest, tt.wantEvents, tt.wantRest)
			}
		})
	}
}

func TestTakeComplete(t *testing.T) {
	tests := []struct {
		name         string
		in           string
		wantComplete string
		wantPartial  string
	}{
		{"nothing pending", "j", "j", ""},
		{"complete report", "\x1b[<0;1;2M", "\x1b[<0;1;2M", ""},
		{"lone escape", "\x1b", "", "\x1b"},
		{"escape bracket", "\x1b[", "", "\x1b["},
		{"mouse prefix", "\x1b[<", "", "\x1b[<"},
		{"mouse half way", "\x1b[<0;12", "", "\x1b[<0;12"},
		{"text then half report", "j\x1b[<0;1;", "j", "\x1b[<0;1;"},
		{"complete report then half one", "\x1b[<0;1;2M\x1b[<32;1", "\x1b[<0;1;2M", "\x1b[<32;1"},
		{"finished arrow", "\x1b[B", "\x1b[B", ""},
		{"empty", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			complete, partial := TakeComplete(tt.in)
			if complete != tt.wantComplete || partial != tt.wantPartial {
				t.Errorf("TakeComplete(%q) = %q, %q; want %q, %q", tt.in, complete, partial, tt.wantComplete, tt.wantPartial)
			}
		})
	}
}

func TestPartialSequenceCompletes(t *testing.T) {
	complete, partial := TakeComplete("\x1b[<0;10")
	if complete != "" {
		t.Fatalf("complete = %q, want empty", complete)
	}
	complete, partial = TakeComplete(partial + ";5M")
	events, _ := ParseMouse(complete)
	if partial != "" || !reflect.DeepEqual(events, []Mouse{{0, 10, 5, false}}) {
		t.Errorf("after the rest arrived: partial=%q events=%v", partial, events)
	}
}
