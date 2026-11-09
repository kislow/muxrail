// Package tmux wraps the tmux command line; every call shells out, there is no library.
package tmux

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Sep separates the fields of a -F format. It is the ASCII unit separator because no pane title or path contains it.
const Sep = "\x1f"

// Output runs tmux and returns stdout without its final newline. Nothing else is trimmed: strings.TrimSpace would
// also eat a trailing Sep when the last field is empty (a new window's pane title) and the line would lose a field.
func Output(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: tmux %s: %s", err, strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSuffix(stdout.String(), "\n"), nil
}

// Option reads a global option; an option that was never set reads as "".
func Option(ctx context.Context, name string) (string, error) {
	return Output(ctx, "show", "-gqv", name)
}

// SocketPath is the socket of the server the other calls talk to, the same one for every call made with this environment.
func SocketPath(ctx context.Context) (string, error) {
	return Output(ctx, "display-message", "-p", "#{socket_path}")
}

// SessionID is the id (like $3) of the session that owns pane.
func SessionID(ctx context.Context, pane string) (string, error) {
	return Output(ctx, "display-message", "-p", "-t", pane, "#{session_id}")
}

// Format joins tmux format variables with Sep, ready for -F.
func Format(fields ...string) string {
	parts := make([]string, len(fields))
	for i, f := range fields {
		parts[i] = "#{" + f + "}"
	}
	return strings.Join(parts, Sep)
}

// Values reads format variables, user options such as @cap_l included, for a pane in one tmux call and returns them in
// the order asked; one that is unset reads as "". Reading five options one by one costs five processes a second.
func Values(ctx context.Context, pane string, fields ...string) ([]string, error) {
	out, err := Output(ctx, "display-message", "-p", "-t", pane, Format(fields...))
	if err != nil {
		return nil, err
	}
	values := strings.Split(out, Sep)
	if len(values) != len(fields) {
		return nil, fmt.Errorf("tmux gave %d values for %d fields: %q", len(values), len(fields), out)
	}
	return values, nil
}
