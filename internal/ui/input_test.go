package ui

import (
	"reflect"
	"testing"
)

// press, motion and release at screen row y, column 10.
func press(y int) string   { return "\x1b[<0;10;" + itoaRow(y) + "M" }
func motion(y int) string  { return "\x1b[<32;10;" + itoaRow(y) + "M" }
func release(y int) string { return "\x1b[<0;10;" + itoaRow(y) + "m" }

func itoaRow(y int) string {
	digits := ""
	for ; y > 0; y /= 10 {
		digits = string(rune('0'+y%10)) + digits
	}
	if digits == "" {
		return "0"
	}
	return digits
}

var mixed = []int{3, 5, 4, 3} // card rows 3-5, 6-10, 11-14, 15-17

func TestApplyMouse(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		selected int
		drag     *Drag
		want     Input
	}{
		{"press grabs and selects", press(7), 0, nil, Input{Selected: 1, Drag: &Drag{Source: 1, Over: 1, Y: 7, Grab: 1}}},
		{"press on the header does nothing", press(2), 3, nil, Input{Selected: 3}},
		{"press below the cards does nothing", press(30), 2, nil, Input{Selected: 2}},
		{"motion sets the drop slot", motion(12), 1, &Drag{Source: 1, Over: 1, Y: 7}, Input{Selected: 1, Drag: &Drag{Source: 1, Over: 2, Y: 12, Moved: true}}},
		{"motion without a grab is ignored", motion(12), 1, nil, Input{Selected: 1}},
		{"motion off the list keeps the last slot", motion(30), 1, &Drag{Source: 1, Over: 2, Y: 12, Moved: true}, Input{Selected: 1, Drag: &Drag{Source: 1, Over: 2, Y: 30, Moved: true}}},
		{"release on the same card switches", release(8), 1, &Drag{Source: 1, Over: 1}, Input{Selected: 1, Switch: ptr(1)}},
		{"release on another card moves", release(12), 1, &Drag{Source: 1, Over: 2}, Input{Selected: 2, Move: &Move{1, 2}}},
		{"release off the list uses the last slot", release(40), 1, &Drag{Source: 1, Over: 3}, Input{Selected: 3, Move: &Move{1, 3}}},
		{"release without a grab is ignored", release(8), 1, nil, Input{Selected: 1}},
		{"whole drag in one chunk", press(7) + motion(11) + motion(16) + release(16), 0, nil, Input{Selected: 3, Move: &Move{1, 3}}},
		{"click in one chunk", press(4) + release(4), 2, nil, Input{Selected: 0, Switch: ptr(0)}},
		{"right button is ignored", "\x1b[<2;10;7M", 0, nil, Input{Selected: 0}},
		{"wheel is ignored", "\x1b[<64;10;7M", 0, nil, Input{Selected: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Apply(tt.data, mixed, tt.selected, tt.drag)
			if !reflect.DeepEqual(*got, tt.want) {
				t.Errorf("Apply = %+v (drag %v move %v switch %v), want %+v", *got, got.Drag, got.Move, derefInt(got.Switch), tt.want)
			}
		})
	}
}

func ptr(n int) *int { return &n }

func derefInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestApplyDoesNotMutateTheCallersDrag(t *testing.T) {
	drag := &Drag{Source: 1, Over: 1}
	Apply(motion(12), mixed, 1, drag)
	if drag.Over != 1 {
		t.Errorf("caller's drag changed to %+v", *drag)
	}
}

func TestApplyKeys(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		selected int
		want     Input
	}{
		{"j moves down", "j", 1, Input{Selected: 2}},
		{"down arrow", "\x1b[B", 1, Input{Selected: 2}},
		{"k moves up", "k", 1, Input{Selected: 0}},
		{"up arrow", "\x1b[A", 1, Input{Selected: 0}},
		{"j stops at the last card", "j", 3, Input{Selected: 3}},
		{"k stops at the first card", "k", 0, Input{Selected: 0}},
		{"J moves the window down", "J", 1, Input{Selected: 2, Move: &Move{1, 2}}},
		{"K moves the window up", "K", 1, Input{Selected: 0, Move: &Move{1, 0}}},
		{"J at the bottom does nothing", "J", 3, Input{Selected: 3}},
		{"K at the top does nothing", "K", 0, Input{Selected: 0}},
		{"enter switches", "\r", 2, Input{Selected: 2, Switch: ptr(2)}},
		{"newline switches", "\n", 2, Input{Selected: 2, Switch: ptr(2)}},
		{"v toggles the layout", "v", 1, Input{Selected: 1, ToggleLayout: true}},
		{"other keys do nothing", "x", 1, Input{Selected: 1}},
		{"a complete arrow for another direction does nothing", "\x1b[C", 1, Input{Selected: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Apply(tt.data, mixed, tt.selected, nil)
			if !reflect.DeepEqual(*got, tt.want) {
				t.Errorf("Apply(%q) = %+v, want %+v", tt.data, *got, tt.want)
			}
		})
	}
}

func TestKeysAreReadAfterMouseReportsAreRemoved(t *testing.T) {
	got := Apply(press(7)+"j", mixed, 0, nil)
	if got.Selected != 2 || got.Drag == nil {
		t.Errorf("got %+v: the report grabs card 1 and the key then moves the cursor to 2", *got)
	}
}
