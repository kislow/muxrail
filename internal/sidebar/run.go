package sidebar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kislow/muxrail/internal/term"
	"github.com/kislow/muxrail/internal/tmux"
	"github.com/kislow/muxrail/internal/ui"
	"github.com/kislow/muxrail/internal/windows"
)

const (
	refreshEvery = time.Second      // window data
	repaintEvery = 5 * time.Second  // forced repaint in case tmux disturbed the pane
	hiddenEvery  = 10 * time.Second // window data for a sidebar whose window is not on screen

	// wakeSeq is typed into a sidebar's pane by Controller.wake when its window comes on screen. No key sends it, and
	// a sidebar that does not know it ignores it as an unknown escape sequence.
	wakeSeq = "\x1b[9999~"
	minWait = 50 * time.Millisecond
	// A little slower than the spinner's frame rate, enough to animate it.
	spinWait = time.Second / (ui.SpinFPS - 3)
	errWait  = time.Second

	slideWait  = 16 * time.Millisecond  // one animation frame
	settleTime = 150 * time.Millisecond // how long cards may keep sliding after the pointer stops or the card drops

	enterTerminal = "\x1b[?25l\x1b[?1002h\x1b[?1006h\x1b[2J" // hide cursor, button-motion mouse reports, SGR encoding
	leaveTerminal = "\x1b[?1006l\x1b[?1002l\x1b[?25h"
)

// Run draws the sidebar in the current pane until ctx ends or stdin closes. It restores the terminal on the way out.
func Run(ctx context.Context) error {
	s, err := newState(ctx)
	if err != nil {
		return err
	}
	restore, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer func() {
		if _, err := io.WriteString(os.Stdout, leaveTerminal); err != nil {
			s.logError(err)
		}
		if err := restore(); err != nil {
			s.logError(err)
		}
	}()
	if _, err := io.WriteString(os.Stdout, enterTerminal); err != nil {
		return fmt.Errorf("%w: enter terminal mode", err)
	}
	if err := s.load(ctx); err != nil {
		return err
	}
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	loop := &loop{state: s, input: readInput(os.Stdin), fetched: make(chan fetchResult, 1), refreshed: time.Now(), winch: winch}
	for ctx.Err() == nil {
		err := loop.iterate(ctx)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			s.logError(err)
			sleep(ctx, errWait)
		}
	}
	return nil
}

type fetchResult struct {
	snap *snapshot
	err  error
	gen  int
}

type loop struct {
	*state
	input     <-chan string
	partial   string // a half-received escape sequence waiting for the rest
	heights   []int
	paint     painter
	refreshed time.Time

	// Cards displaced by a drag slide into place over a few frames: shown is where each window (by id) is drawn now.
	shown     map[string]int
	animUntil time.Time
	sliding   bool

	// Data is read beside the input loop, so a slow tmux, git or ps call never stalls the mouse. A result is held back
	// while a card is grabbed (the list must not change under the pointer) and dropped when a move or switch happened
	// after the read began (gen), because it would show the old order.
	// winch carries terminal resizes. tmux resizes a pane when its window comes on screen, which is the cue to wake even when
	// no hook sent the wake sequence.
	winch    <-chan os.Signal
	fetched  chan fetchResult
	fetching bool
	pending  *snapshot
	gen      int
}

func (l *loop) iterate(ctx context.Context) error {
	now := time.Now()
	l.startFetch(ctx, now)
	l.applyPending()
	if l.visible {
		if err := l.draw(now); err != nil {
			return err
		}
	}
	wait := max(minWait, l.refreshInterval()-time.Since(l.refreshed))
	if l.visible && l.working() {
		wait = min(wait, spinWait)
	}
	if l.sliding {
		wait = slideWait
	}
	select {
	case <-ctx.Done():
	case r := <-l.fetched:
		l.finishFetch(r)
	case <-l.winch:
		l.wake()
	case chunk, ok := <-l.input:
		if !ok {
			return io.EOF
		}
		return l.handle(ctx, l.drainInput(chunk))
	case <-time.After(wait):
	}
	return nil
}

// drainInput appends whatever else is already waiting, so a burst of mouse motion costs one repaint.
func (l *loop) drainInput(chunk string) string {
	for {
		select {
		case more, ok := <-l.input:
			if !ok {
				return chunk
			}
			chunk += more
		default:
			return chunk
		}
	}
}

// refreshInterval is how often the window data is read: every second while the sidebar is on screen, rarely while its
// window is hidden, since nobody sees it. Every window has its own sidebar process, so the hidden ones are most of them.
func (l *loop) refreshInterval() time.Duration {
	if l.visible {
		return refreshEvery
	}
	return hiddenEvery
}

// wake is what a tmux hook sends when the sidebar's window comes on screen: show it now, from fresh data.
func (l *loop) wake() {
	l.visible = true
	l.refreshed = time.Time{}
	l.paint.prev = nil
}

func (l *loop) startFetch(ctx context.Context, now time.Time) {
	if l.fetching || now.Sub(l.refreshed) < l.refreshInterval() {
		return
	}
	l.fetching, l.refreshed = true, now
	gen := l.gen
	go func() {
		snap, err := l.fetch(ctx, true)
		l.fetched <- fetchResult{snap: snap, err: err, gen: gen}
	}()
}

func (l *loop) finishFetch(r fetchResult) {
	l.fetching = false
	switch {
	case r.err != nil:
		l.logError(r.err)
	case r.gen != l.gen:
		l.refreshed = time.Time{}
	default:
		l.pending = r.snap
	}
}

func (l *loop) applyPending() {
	if l.pending != nil && l.drag == nil {
		l.apply(l.pending)
		l.pending = nil
	}
}

// draw paints the current screen and refreshes the heights hit-testing uses.
func (l *loop) draw(now time.Time) error {
	cols, rows, err := term.Size(int(os.Stdout.Fd()))
	if err != nil {
		return err
	}
	l.heights = l.env.Heights(l.wins, cols, l.layout)
	l.slide = l.animate(ui.Targets(l.heights, l.drag), now)
	_, err = io.WriteString(os.Stdout, l.paint.diff(l.lines(cols, rows, now), now))
	return err
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// readInput forwards stdin chunks; bytes that are not UTF-8 are dropped.
func readInput(r io.Reader) <-chan string {
	ch := make(chan string)
	go func() {
		defer close(ch)
		buf := make([]byte, 1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				ch <- strings.ToValidUTF8(string(buf[:n]), "")
			}
			if err != nil {
				return
			}
		}
	}()
	return ch
}

func (l *loop) handle(ctx context.Context, chunk string) error {
	var data string
	data, l.partial = term.TakeComplete(l.partial + chunk)
	if strings.Contains(data, wakeSeq) {
		data = strings.ReplaceAll(data, wakeSeq, "")
		l.wake()
	}
	got := ui.Apply(data, l.heights, l.selected, l.drag)
	if l.debug {
		l.logInput(data, got)
	}
	l.selected, l.drag = got.Selected, got.Drag
	if got.ToggleLayout {
		if err := l.toggleLayout(ctx); err != nil {
			return err
		}
	}
	switch {
	case got.Move != nil && len(l.wins) > 0:
		return l.move(ctx, got.Move)
	case got.Switch != nil && len(l.wins) > 0:
		l.changed()
		return l.switchTo(ctx, *got.Switch)
	}
	return nil
}

func (l *loop) toggleLayout(ctx context.Context) error {
	next := ui.LayoutCompact
	if l.layout == ui.LayoutCompact {
		next = ui.LayoutTiles
	}
	l.layout = next
	_, err := tmux.Output(ctx, "set", "-g", "@sidebar_layout", next)
	return err
}

func (l *loop) switchTo(ctx context.Context, i int) error {
	if i < 0 || i >= len(l.wins) {
		return fmt.Errorf("switch to window %d: only %d windows", i, len(l.wins))
	}
	return windows.Switch(ctx, l.sessionID, l.pane, l.wins[i])
}

// changed marks everything read so far as stale and asks for a fresh read.
func (l *loop) changed() {
	l.gen++
	l.refreshed, l.pending = time.Time{}, nil
}

// move shows the new order at once and only then asks tmux for it, which takes a moment; otherwise the list would
// snap back to the old order between the drop and the next read.
func (l *loop) move(ctx context.Context, m *ui.Move) error {
	before := l.wins
	l.wins = reordered(before, m.From, m.To)
	if err := l.draw(time.Now()); err != nil {
		return err
	}
	err := windows.Move(ctx, l.sessionID, before, m.From, m.To)
	l.changed()
	return err
}

// reordered is wins with the one at from taken to slot to; every slot keeps the tmux index it had.
func reordered(wins []*windows.Win, from, to int) []*windows.Win {
	out := make([]*windows.Win, len(wins))
	for slot, i := range ui.DisplayOrder(len(wins), &ui.Drag{Source: from, Over: to}) {
		moved := *wins[i]
		moved.Index = wins[slot].Index
		out[slot] = &moved
	}
	return out
}
