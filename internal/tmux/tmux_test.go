package tmux

import (
	"context"
	"strings"
	"testing"

	"github.com/kislow/muxrail/internal/testutil"
)

func TestOutputStripsOnlyTheFinalNewline(t *testing.T) {
	testutil.Stub(t, "tmux", `printf '%%1\037\n%%2\037\037'; printf '\n'`)
	got, err := Output(context.Background(), "list-panes")
	if err != nil {
		t.Fatal(err)
	}
	want := "%1\x1f\n%2\x1f\x1f"
	if got != want {
		t.Errorf("Output = %q, want %q", got, want)
	}
}

func TestOutputKeepsTrailingSeparatorOfEmptyLastField(t *testing.T) {
	testutil.Stub(t, "tmux", `printf '@1\037\037\n'`)
	got, err := Output(context.Background(), "list-panes")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(strings.SplitN(got, Sep, 3)); n != 3 {
		t.Errorf("line %q split into %d fields, want 3", got, n)
	}
}

func TestOutputWrapsFailure(t *testing.T) {
	testutil.Stub(t, "tmux", `echo "no server running" >&2; exit 1`)
	_, err := Output(context.Background(), "list-panes", "-a")
	if err == nil || !strings.Contains(err.Error(), "no server running") || !strings.Contains(err.Error(), "list-panes -a") {
		t.Errorf("error = %v, want it to carry the command and stderr", err)
	}
}

func TestOptionAsksQuietly(t *testing.T) {
	testutil.Stub(t, "tmux", `echo "$@"`)
	got, err := Option(context.Background(), "@cap_l")
	if err != nil {
		t.Fatal(err)
	}
	if got != "show -gqv @cap_l" {
		t.Errorf("tmux was called with %q", got)
	}
}

func TestFormat(t *testing.T) {
	if got, want := Format("window_id", "@sidebar"), "#{window_id}\x1f#{@sidebar}"; got != want {
		t.Errorf("Format = %q, want %q", got, want)
	}
}

func TestValuesReadsSeveralFieldsInOneCall(t *testing.T) {
	testutil.Stub(t, "tmux", `printf '(\037\037tiles\037%s\n' "$#"`)
	got, err := Values(context.Background(), "%1", "@cap_l", "@cap_r", "@sidebar_layout", "window_active")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[0] != "(" || got[1] != "" || got[2] != "tiles" || got[3] != "5" {
		t.Errorf("Values = %q, want [( \"\" tiles 5]: unset options read empty and one tmux call carries them all", got)
	}
}

func TestValuesRejectsAShortAnswer(t *testing.T) {
	testutil.Stub(t, "tmux", `echo only-one`)
	if _, err := Values(context.Background(), "%1", "@a", "@b"); err == nil {
		t.Error("want an error when tmux returns fewer values than asked for")
	}
}
