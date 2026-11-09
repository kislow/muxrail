package sidebar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kislow/muxrail/internal/testutil"
	"github.com/kislow/muxrail/internal/ui"
)

func TestLogInputAppendsWhatWasDecided(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.Mkdir(filepath.Join(home, ".tmux"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := &state{}
	two := 2
	s.logInput("\x1b[<0;1;3M", &ui.Input{Drag: &ui.Drag{Source: 0, Over: 0}, Switch: &two})
	s.logInput("j", &ui.Input{})
	b, err := os.ReadFile(filepath.Join(home, ".tmux", "sidebar-input.log"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"\x1b[<0;1;3M" -> drag=&{0 0 0 0 false}`) || !strings.Contains(lines[1], `"j"`) {
		t.Errorf("log = %q", b)
	}
}

func TestLogErrorGoesToTheTempDirectory(t *testing.T) {
	tmp := testutil.IsolatedTmp(t)
	(&state{}).logError(os.ErrNotExist)
	b, err := os.ReadFile(filepath.Join(tmp, "muxrail-sidebar-error.log"))
	if err != nil || !strings.Contains(string(b), "file does not exist") {
		t.Errorf("log = %q, err = %v", b, err)
	}
}
