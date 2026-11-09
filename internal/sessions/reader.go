package sessions

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
)

const (
	headBytes  = 512 << 10 // the first records of a transcript: session metadata and the opening prompts
	maxWorkers = 16
)

// Reader lists the sessions of one agent under a home directory. A file it cannot read is skipped and reported in the
// error returned beside the sessions that were found.
type Reader func(ctx context.Context, home string) ([]*Session, error)

var readers = []Reader{claudeSessions, codexSessions, cursorSessions}

// List is every session of every agent, newest first. The readers run concurrently; an unreadable transcript does not
// stop the list, its error comes back with it.
func List(ctx context.Context, home string) ([]*Session, error) {
	found := make([][]*Session, len(readers))
	errs := make([]error, len(readers))
	var wg sync.WaitGroup
	for i, read := range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			found[i], errs[i] = read(ctx, home)
		}()
	}
	wg.Wait()
	var all []*Session
	for _, f := range found {
		all = append(all, f...)
	}
	return Sorted(all), errors.Join(errs...)
}

// readEach runs fn for every file on a bounded pool and gathers the sessions it returns (nil: skip the file) and the
// errors, in file order.
func readEach(ctx context.Context, files []string, fn func(path string) (*Session, error)) ([]*Session, error) {
	results := make([]*Session, len(files))
	errs := make([]error, len(files))
	var wg sync.WaitGroup
	sem := make(chan struct{}, min(maxWorkers, 2*runtime.NumCPU()))
	for i, f := range files {
		if ctx.Err() != nil {
			errs[i] = fmt.Errorf("%w: reading sessions", ctx.Err())
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			results[i], errs[i] = fn(f)
		}()
	}
	wg.Wait()
	var kept []*Session
	for _, s := range results {
		if s != nil {
			kept = append(kept, s)
		}
	}
	return kept, errors.Join(errs...)
}

// head is the complete lines among the first size bytes of path. A line cut by the limit is dropped, so a record
// bigger than size is never half parsed.
func head(path string, size int64) (lines [][]byte, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: open transcript", err)
	}
	defer closeFile(f, &err)
	data, err := io.ReadAll(io.LimitReader(f, size))
	if err != nil {
		return nil, fmt.Errorf("%w: read transcript", err)
	}
	lines = bytes.Split(data, []byte("\n"))
	if int64(len(data)) == size { // the last piece has no newline yet, so it may be cut
		lines = lines[:len(lines)-1]
	}
	return lines, nil
}

func mtime(path string) (float64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("%w: stat transcript", err)
	}
	return float64(info.ModTime().UnixNano()) / 1e9, nil
}

// closeFile folds a failed close into err, which the caller returns, unless the work had already failed.
func closeFile(f *os.File, err *error) {
	if closeErr := f.Close(); closeErr != nil {
		*err = errors.Join(*err, fmt.Errorf("%w: close transcript", closeErr))
	}
}
