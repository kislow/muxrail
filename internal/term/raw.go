package term

import (
	"fmt"

	"golang.org/x/term"
)

// MakeRaw puts the terminal on fd in raw mode and returns the function that restores it.
func MakeRaw(fd int) (restore func() error, err error) {
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("%w: enter raw mode", err)
	}
	return func() error { return term.Restore(fd, state) }, nil
}

// Size is the terminal's columns and rows.
func Size(fd int) (cols, rows int, err error) {
	cols, rows, err = term.GetSize(fd)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: terminal size", err)
	}
	return cols, rows, nil
}
