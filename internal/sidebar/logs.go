package sidebar

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kislow/muxrail/internal/ui"
)

func errorLogPath() string { return filepath.Join(os.TempDir(), "muxrail-sidebar-error.log") }

// appendLine adds text to the file at path, creating it; a failed close counts, the write may only have been buffered.
func appendLine(path, text string) (err error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("%w: open log", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("%w: close log", closeErr))
		}
	}()
	if _, err := io.WriteString(f, text); err != nil {
		return fmt.Errorf("%w: write log", err)
	}
	return nil
}

// logError appends to a file: the pane the sidebar draws in is the only terminal there is. When even the file cannot
// be written there is nowhere left to report it.
func (s *state) logError(err error) {
	var b bytes.Buffer
	slog.New(slog.NewTextHandler(&b, nil)).Error("sidebar", "err", err)
	if appendLine(errorLogPath(), b.String()) != nil {
		return
	}
}

// logInput records raw input and what it decided, while `tmux set -g @sidebar_debug 1`.
func (s *state) logInput(data string, got *ui.Input) {
	home, err := os.UserHomeDir()
	if err != nil {
		s.logError(err)
		return
	}
	line := fmt.Sprintf("%.3f %q -> drag=%v move=%v switch=%v\n", float64(time.Now().UnixNano())/1e9, data, got.Drag, got.Move, got.Switch)
	if err := appendLine(filepath.Join(home, ".tmux", "sidebar-input.log"), line); err != nil {
		s.logError(err)
	}
}
