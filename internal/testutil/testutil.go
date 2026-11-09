// Package testutil holds helpers for tests that stand in for the programs muxrail shells out to.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// Stub puts an executable shell script called name first on PATH for the test, so no real tmux or ps runs.
// The script is the body after the shebang.
func Stub(t *testing.T, name, body string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "stubs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// IsolatedTmp points TMPDIR and XDG_STATE_HOME at fresh directories so cache and agent state files never touch real
// ones, and returns the TMPDIR.
func IsolatedTmp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	return dir
}

// NoTmux clears the variables that would point a stray tmux call at a live server.
func NoTmux(t *testing.T) {
	t.Helper()
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
}
