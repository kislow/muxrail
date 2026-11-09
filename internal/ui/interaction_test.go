package ui

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/kislow/muxrail/internal/windows"
)

func TestCardAtUsesRealHeights(t *testing.T) {
	// header takes rows 1-2, so the first card starts on row 3
	heights := []int{3, 5, 4, 3}
	tests := []struct {
		name string
		y    int
		want int
		hit  bool
	}{
		{"title row hits nothing", 1, 0, false},
		{"rule row hits nothing", 2, 0, false},
		{"first card top edge", 3, 0, true},
		{"first card last row", 5, 0, true},
		{"second card first row", 6, 1, true},
		{"second card last row", 10, 1, true},
		{"third card", 11, 2, true},
		{"third card last row", 14, 2, true},
		{"fourth card", 15, 3, true},
		{"fourth card bottom", 17, 3, true},
		{"below the last card", 18, 0, false},
		{"zero row", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, hit := CardAt(tt.y, heights)
			if got != tt.want || hit != tt.hit {
				t.Errorf("CardAt(%d) = %d, %v; want %d, %v", tt.y, got, hit, tt.want, tt.hit)
			}
		})
	}
}

func TestCardAtCompactRows(t *testing.T) {
	heights := []int{1, 1, 1}
	for y, want := range map[int]int{3: 0, 4: 1, 5: 2} {
		if got, hit := CardAt(y, heights); !hit || got != want {
			t.Errorf("CardAt(%d) = %d, %v; want %d", y, got, hit, want)
		}
	}
	if _, hit := CardAt(6, heights); hit {
		t.Error("row past the last compact card must hit nothing")
	}
}

func TestHitTestingFollowsTheRealOrderNotThePreview(t *testing.T) {
	env := testEnv()
	wins := fixtures()
	heights := env.Heights(wins, 34, LayoutTiles)
	// the preview moves card 0 to the end; rows still belong to the cards where they really are
	if got, _ := CardAt(3, heights); got != 0 {
		t.Errorf("row 3 is card %d, want 0 whatever the preview shows", got)
	}
	frame := env.Frame(wins, View{Width: 34, Rows: 80, Layout: LayoutTiles, Drag: &Drag{Source: 0, Over: len(wins) - 1}})
	if heights2 := env.Heights(wins, 34, LayoutTiles); !reflect.DeepEqual(heights, heights2) {
		t.Errorf("drawing a preview must not change the heights: %v vs %v", heights, heights2)
	}
	if frame == "" {
		t.Error("empty frame")
	}
}

func TestDisplayOrder(t *testing.T) {
	tests := []struct {
		name  string
		count int
		drag  *Drag
		want  []int
	}{
		{"no drag", 5, nil, []int{0, 1, 2, 3, 4}},
		{"grab without moving", 5, &Drag{Source: 2, Over: 2}, []int{0, 1, 2, 3, 4}},
		{"move down", 5, &Drag{Source: 1, Over: 3}, []int{0, 2, 3, 1, 4}},
		{"move up", 5, &Drag{Source: 3, Over: 0}, []int{3, 0, 1, 2, 4}},
		{"to the end", 5, &Drag{Source: 0, Over: 4}, []int{1, 2, 3, 4, 0}},
		{"from the end to the front", 5, &Drag{Source: 4, Over: 0}, []int{4, 0, 1, 2, 3}},
		{"empty", 0, nil, []int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DisplayOrder(tt.count, tt.drag); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DisplayOrder = %v, want %v", got, tt.want)
			}
		})
	}
}

func fiveWindows() []*windows.Win {
	var wins []*windows.Win
	for i, name := range []string{"A", "B", "C", "D", "E"} {
		wins = append(wins, &windows.Win{ID: "@" + name, Index: i, Active: i == 2, Command: "zsh", Name: name, Path: "/tmp"})
	}
	return wins
}

var nameRE = regexp.MustCompile(`[│▌ ]\s?(\d)\s+\S*?([A-E])(?:\s|│|$)`)

// pillsAndNames reads the numbers on the pills and the window names in drawn order, as e2e/drag_test.go does.
func pillsAndNames(frame string) (pills, names []string) {
	for _, l := range rows(frame) {
		if m := nameRE.FindStringSubmatch(plain(l)); m != nil {
			pills, names = append(pills, m[1]), append(names, m[2])
		}
	}
	return pills, names
}

func TestDragPreviewOrderAndPills(t *testing.T) {
	env := testEnv()
	wins := fiveWindows()
	frame := env.Frame(wins, View{Width: 34, Rows: 50, Layout: LayoutTiles, Selected: 2, Drag: &Drag{Source: 1, Over: 3}})
	pills, names := pillsAndNames(frame)
	if want := []string{"A", "C", "D", "B", "E"}; !reflect.DeepEqual(names, want) {
		t.Errorf("preview order = %v, want %v", names, want)
	}
	if want := []string{"0", "1", "2", "3", "4"}; !reflect.DeepEqual(pills, want) {
		t.Errorf("pills = %v, want the numbers the windows will get %v", pills, want)
	}
	if !strings.Contains(plain(frame), "moving") {
		t.Error("grabbed card should say moving")
	}
	if !strings.Contains(frame, magenta+"╭") {
		t.Error("grabbed card should have a magenta border")
	}
	if !strings.Contains(plain(frame), "drop to place it there") {
		t.Error("hint should change while dragging")
	}
	for i, w := range wins {
		if w.Index != i {
			t.Errorf("preview must not touch the real windows: %s has index %d", w.Name, w.Index)
		}
	}
}

func TestNoPreviewWithoutDrag(t *testing.T) {
	frame := testEnv().Frame(fiveWindows(), View{Width: 34, Rows: 50, Layout: LayoutTiles, Selected: 2})
	pills, names := pillsAndNames(frame)
	if !reflect.DeepEqual(names, []string{"A", "B", "C", "D", "E"}) || !reflect.DeepEqual(pills, []string{"0", "1", "2", "3", "4"}) {
		t.Errorf("pills %v names %v", pills, names)
	}
	if strings.Contains(plain(frame), "moving") || !strings.Contains(plain(frame), "click · drag · j/k · J/K · v") {
		t.Error("idle frame should show the normal hint")
	}
}

func TestCompactDragMarker(t *testing.T) {
	frame := testEnv().Frame(fiveWindows(), View{Width: 34, Rows: 50, Layout: LayoutCompact, Selected: 2, Drag: &Drag{Source: 0, Over: 2}})
	lines := rows(frame)
	if !strings.HasPrefix(lines[4], magenta+"▌") { // two header rows, then the preview order B C A
		t.Errorf("grabbed marker line = %q", lines[4])
	}
	if strings.Contains(plain(frame), "moving") {
		t.Error("compact layout has no label")
	}
}

func TestFrameShape(t *testing.T) {
	env := testEnv()
	frame := env.Frame(fiveWindows(), View{Width: 34, Rows: 30, Layout: LayoutTiles, Selected: 2})
	if !strings.HasPrefix(frame, "\x1b[H") || !strings.HasSuffix(frame, clearEOL) {
		t.Error("a frame homes the cursor first and clears to end of line last")
	}
	lines := rows(frame)
	if len(lines) != 30 {
		t.Errorf("frame has %d rows, want 30", len(lines))
	}
	if !strings.Contains(plain(lines[0]), "WINDOWS  5   C-a b hides") {
		t.Errorf("header = %q", plain(lines[0]))
	}
	if strings.Contains(frame, "\x1b[2J") {
		t.Error("never clear the screen: it flickers")
	}
	if narrow := plain(rows(env.Frame(fiveWindows(), View{Width: 27, Rows: 30}))[0]); strings.Contains(narrow, "hides") {
		t.Errorf("the hide hint needs 28 columns, got %q", narrow)
	}
}

func TestFrameTruncatesToTheScreenAndKeepsTheHint(t *testing.T) {
	frame := testEnv().Frame(fixtures(), View{Width: 34, Rows: 10, Layout: LayoutTiles})
	lines := rows(frame)
	if len(lines) != 10 || !strings.Contains(plain(lines[9]), "click") {
		t.Errorf("%d rows, last = %q", len(lines), plain(lines[len(lines)-1]))
	}
}

func TestSelectedCardBorder(t *testing.T) {
	env := testEnv()
	wins := fiveWindows()
	frame := env.Frame(wins, View{Width: 34, Rows: 50, Layout: LayoutTiles, Selected: 0})
	if !strings.Contains(frame, white+"╭") {
		t.Error("the keyboard cursor card should have a white border")
	}
	if !strings.Contains(frame, fg(2)+"╭") {
		t.Error("the active card should have a green border")
	}
}

func TestMovedDragShowsSlotAndLiftedCard(t *testing.T) {
	env := testEnv()
	for _, width := range []int{18, 26, 34} {
		t.Run(fmt.Sprintf("tiles-%d", width), func(t *testing.T) {
			drag := &Drag{Source: 0, Over: 2, Y: 20, Grab: 1, Moved: true}
			frame := env.Frame(fiveWindows(), View{Width: width, Rows: 60, Layout: LayoutTiles, Drag: drag})
			lines := rows(frame)
			assertWidthAtMost(t, width, lines)
			text := plain(frame)
			for _, want := range []string{"╔", "╚", "┄", "drop here", "drop to place it there"} {
				if width >= 26 && !strings.Contains(text, want) {
					t.Errorf("moved drag frame lacks %q", want)
				}
			}
			if width >= 26 && !strings.HasPrefix(lines[18], magenta+bold+"╔") {
				t.Errorf("lifted card should start where it was grabbed (row 20, grabbed by its second row, puts its top on line 18), line = %q", lines[18])
			}
		})
	}
}

func TestLiftedCardStaysInsideTheList(t *testing.T) {
	env := testEnv()
	for _, y := range []int{1, 3, 200} {
		drag := &Drag{Source: 1, Over: 1, Y: y, Moved: true}
		frame := env.Frame(fiveWindows(), View{Width: 34, Rows: 40, Layout: LayoutTiles, Drag: drag})
		lines := rows(frame)
		if len(lines) != 40 || !strings.Contains(plain(lines[0]), "WINDOWS") || !strings.Contains(plain(lines[39]), "drop to place it there") {
			t.Errorf("y=%d: header or hint overwritten (%d lines)", y, len(lines))
		}
	}
}

func TestPressedButNotMovedHasNoGhost(t *testing.T) {
	frame := testEnv().Frame(fiveWindows(), View{Width: 34, Rows: 50, Layout: LayoutTiles, Drag: &Drag{Source: 1, Over: 1, Y: 8}})
	if text := plain(frame); strings.Contains(text, "╔") || strings.Contains(text, "drop here") {
		t.Error("a click must not flash the lifted card")
	}
}

func TestCompactMovedDrag(t *testing.T) {
	frame := testEnv().Frame(fiveWindows(), View{Width: 34, Rows: 30, Layout: LayoutCompact, Drag: &Drag{Source: 0, Over: 3, Y: 4, Moved: true}})
	text := plain(frame)
	if !strings.Contains(text, "drop here") || !strings.Contains(text, "▶") {
		t.Errorf("compact moved drag needs the slot line and the lifted marker:\n%s", text)
	}
}

func TestLiftedCardStaysUnderThePointerWhereItWasTaken(t *testing.T) {
	env := testEnv()
	for _, tt := range []struct{ y, grab, line int }{{7, 4, 2}, {8, 4, 3}, {12, 0, 11}, {12, 2, 9}} {
		drag := &Drag{Source: 0, Over: 0, Y: tt.y, Grab: tt.grab, Moved: true}
		lines := rows(env.Frame(fiveWindows(), View{Width: 34, Rows: 50, Layout: LayoutTiles, Drag: drag}))
		if !strings.HasPrefix(lines[tt.line], magenta+bold+"╔") {
			t.Errorf("pointer row %d grabbed at card row %d: card top should be line %d, got %q", tt.y, tt.grab, tt.line, lines[tt.line])
		}
	}
}

func TestPressRemembersTheRowGrabbed(t *testing.T) {
	got := Apply(press(9), mixed, 0, nil) // card 1 spans rows 6-10, so row 9 is its fourth line
	if got.Drag == nil || got.Drag.Grab != 3 {
		t.Errorf("drag = %+v, want Grab 3", got.Drag)
	}
}

func TestTargetsFollowThePreviewOrder(t *testing.T) {
	heights := []int{3, 5, 4}
	if got := Targets(heights, nil); !reflect.DeepEqual(got, []int{2, 5, 10}) {
		t.Errorf("idle targets = %v", got)
	}
	if got := Targets(heights, &Drag{Source: 0, Over: 2}); !reflect.DeepEqual(got, []int{11, 2, 7}) {
		t.Errorf("targets while dragging card 0 to the end = %v", got)
	}
}

func TestSlideDrawsACardOffItsPlace(t *testing.T) {
	env := testEnv()
	idle := rows(env.Frame(fiveWindows(), View{Width: 34, Rows: 50, Layout: LayoutTiles}))
	slid := rows(env.Frame(fiveWindows(), View{Width: 34, Rows: 50, Layout: LayoutTiles, Slide: map[int]int{1: 2}}))
	if plain(idle[7]) == plain(slid[7]) && plain(idle[8]) == plain(slid[8]) {
		t.Error("a card with Slide 2 should be drawn two rows lower")
	}
	assertWidthAtMost(t, 34, slid)
	if len(slid) != 50 {
		t.Errorf("frame has %d rows", len(slid))
	}
}
