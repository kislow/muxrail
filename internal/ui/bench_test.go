package ui

import "testing"

// BenchmarkTick is what the sidebar does on every animation frame: the card heights, then the screen.
func BenchmarkTick(b *testing.B) {
	e, wins := testEnv(), fixtures()
	v := View{Width: 30, Rows: 59, Layout: LayoutTiles}
	b.ReportAllocs()
	for b.Loop() {
		e.Heights(wins, v.Width, v.Layout)
		e.Lines(wins, v)
	}
}
