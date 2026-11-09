package sidebar

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/kislow/muxrail/internal/ui"
)

const onceRows = 40

// Once writes one frame for the current session, cols wide, and returns.
func Once(ctx context.Context, w io.Writer, cols int) error {
	s, err := newState(ctx)
	if err != nil {
		return err
	}
	if err := s.load(ctx); err != nil {
		return err
	}
	s.layout = ui.LayoutTiles // the one-shot frame always draws tiles
	_, err = fmt.Fprintln(w, s.frame(cols, onceRows, time.Now()))
	return err
}
