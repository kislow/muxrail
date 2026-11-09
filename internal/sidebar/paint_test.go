package sidebar

import (
	"strings"
	"testing"
	"time"

	"github.com/kislow/muxrail/internal/ui"
	"github.com/kislow/muxrail/internal/windows"
)

func TestPainterDiff(t *testing.T) {
	now := time.Now()
	p := &painter{}
	screen := []string{"a", "b", "c"}

	if got := p.diff(screen, now); got != ui.JoinFrame(screen) {
		t.Errorf("first paint = %q, want a full frame", got)
	}
	if got := p.diff(screen, now.Add(time.Second)); got != "" {
		t.Errorf("unchanged screen wrote %q", got)
	}
	got := p.diff([]string{"a", "B", "c"}, now.Add(2*time.Second))
	if got != "\x1b[2;1HB\x1b[K" {
		t.Errorf("one changed row = %q, want only row 2", got)
	}
	if got := p.diff([]string{"a", "B", "c", "d"}, now.Add(3*time.Second)); got != ui.JoinFrame([]string{"a", "B", "c", "d"}) {
		t.Errorf("resize = %q, want a full frame", got)
	}
	if got := p.diff([]string{"a", "B", "c", "d"}, now.Add(3*time.Second+repaintEvery+time.Millisecond)); !strings.HasPrefix(got, "\x1b[H") {
		t.Errorf("after %v of quiet = %q, want a forced full repaint", repaintEvery, got)
	}
}

func TestReorderedKeepsEverySlotsIndex(t *testing.T) {
	list := []*windows.Win{{ID: "@a", Index: 1}, {ID: "@b", Index: 2}, {ID: "@c", Index: 3}}
	got := reordered(list, 0, 2)
	ids, indexes := "", []int{}
	for _, w := range got {
		ids += w.ID
		indexes = append(indexes, w.Index)
	}
	if ids != "@b@c@a" || indexes[0] != 1 || indexes[1] != 2 || indexes[2] != 3 {
		t.Errorf("order %s indexes %v, want @b@c@a with 1 2 3", ids, indexes)
	}
	if list[0].ID != "@a" || list[0].Index != 1 {
		t.Error("the caller's windows must not change; the move still needs them")
	}
}

func TestFetchedDataWaitsForTheDropAndStaleDataIsDropped(t *testing.T) {
	l := &loop{state: &state{env: &ui.Env{}}}
	fresh := &snapshot{wins: wins("@a", "@a"), layout: ui.LayoutTiles}

	l.drag = &ui.Drag{}
	l.finishFetch(fetchResult{snap: fresh, gen: l.gen})
	l.applyPending()
	if len(l.wins) != 0 || l.pending == nil {
		t.Fatal("a refresh must be held while a card is grabbed")
	}
	l.drag = nil
	l.applyPending()
	if len(l.wins) != 1 || l.pending != nil {
		t.Errorf("after the drop the held refresh should apply (wins %d)", len(l.wins))
	}

	l.changed()
	l.finishFetch(fetchResult{snap: fresh, gen: l.gen - 1})
	if l.pending != nil || !l.refreshed.IsZero() {
		t.Error("a read that began before a move must be dropped and re-requested")
	}
}

func TestApproachEasesInAndArrives(t *testing.T) {
	at, steps := 0, 0
	for at != 5 {
		at = approach(at, 5)
		if steps++; steps > 6 {
			t.Fatalf("still at %d after %d frames", at, steps)
		}
	}
	if got := approach(9, 4); got != 7 {
		t.Errorf("moving up: approach(9, 4) = %d, want 7", got)
	}
	if got := approach(3, 3); got != 3 {
		t.Errorf("already there: %d", got)
	}
}

func TestCardsSlideOnlyDuringADrag(t *testing.T) {
	now := time.Now()
	l := &loop{state: &state{env: &ui.Env{}, wins: wins("", "@a", "@b")}}
	l.shown = map[string]int{"@a": 2, "@b": 7}

	slide := l.animate([]int{7, 2}, now) // not dragging: the list snaps
	if len(slide) != 0 || l.sliding || l.shown["@a"] != 7 {
		t.Errorf("idle list should snap: slide %v shown %v", slide, l.shown)
	}

	l.drag = &ui.Drag{}
	l.shown = map[string]int{"@a": 2, "@b": 7}
	slide = l.animate([]int{7, 2}, now)
	if !l.sliding || slide[0] >= 0 || slide[1] <= 0 || l.shown["@a"] == 7 {
		t.Errorf("dragging: cards should be mid-flight, slide %v shown %v", slide, l.shown)
	}

	l.drag = nil
	for i := 0; i < 6; i++ {
		slide = l.animate([]int{7, 2}, now.Add(time.Duration(i)*slideWait))
	}
	if len(slide) != 0 || l.sliding {
		t.Errorf("after the drop the cards should arrive: slide %v", slide)
	}
}
