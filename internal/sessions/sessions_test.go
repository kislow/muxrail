package sessions

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kislow/muxrail/internal/term"
)

func TestScanTitle(t *testing.T) {
	big := `{"type":"user","message":"` + strings.Repeat("x", 3<<20) + `"}`
	tests := []struct {
		name    string
		lines   []string
		want    string
		renamed bool
	}{
		{"last rename beats generated ones", []string{
			`{"type":"user","cwd":"/w/a","message":"hi"}`,
			`{"type":"ai-title","aiTitle":"Generated one"}`,
			`{"type":"custom-title","customTitle":"First rename"}`,
			`{"type":"custom-title","customTitle":"Last rename"}`,
			`{"type":"ai-title","aiTitle":"Generated two"}`,
		}, "Last rename", true},
		{"last generated title", []string{`{"type":"ai-title","aiTitle":"Old"}`, `{"type":"ai-title","aiTitle":"Newest generated"}`}, "Newest generated", false},
		{"title-looking text inside messages is no record", []string{
			`{"type":"user","message":"see {\"type\":\"custom-title\",\"customTitle\":\"not real\"} for details"}`,
			`{"type":"assistant","text":"\"type\":\"ai-title\" is in this line but this is no record"}`,
			`{"type":"x","note":"a raw "type":"ai-title" fragment, invalid json"}`,
		}, "", false},
		{"empty rename falls back to the generated title", []string{`{"type":"custom-title","customTitle":""}`, `{"type":"ai-title","aiTitle":"Fallback"}`}, "Fallback", false},
		{"untitled", []string{`{"type":"user","cwd":"/w/b"}`}, "", false},
		{"a huge line before the title does not stop the scan", []string{big, `{"type":"custom-title","customTitle":"After the big line"}`}, "After the big line", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "s.jsonl")
			writeFile(t, path, strings.Join(tt.lines, "\n")+"\n")
			got, err := scanTitle(path)
			if err != nil {
				t.Fatal(err)
			}
			if tt.want == "" {
				if got != nil {
					t.Errorf("title = %+v, want none", got)
				}
				return
			}
			if got == nil || got.Text != tt.want || got.Renamed != tt.renamed {
				t.Errorf("title = %+v, want %q renamed=%v", got, tt.want, tt.renamed)
			}
		})
	}
}

func TestScanTitleOnlyReadsTheEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	filler := `{"type":"user","message":"` + strings.Repeat("y", 1<<10) + `"}` + "\n"
	writeFile(t, path, `{"type":"custom-title","customTitle":"Too early"}`+"\n"+strings.Repeat(filler, 400)+`{"type":"ai-title","aiTitle":"Late"}`+"\n")
	got, err := scanTitle(path)
	if err != nil || got == nil || got.Text != "Late" || got.Renamed {
		t.Errorf("title = %+v, %v; want the generated title from the end", got, err)
	}
}

func TestCwdOf(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  string
	}{
		{"first cwd wins", []string{`{"type":"x"}`, `{"cwd":"/first"}`, `{"cwd":"/second"}`}, "/first"},
		{"garbage lines are skipped", []string{`not json`, `{"cwd":"/ok"}`}, "/ok"},
		{"none", []string{`{"type":"x"}`}, ""},
		{"cwd beyond the first 60 lines is not read", append(make([]string, 60), `{"cwd":"/late"}`), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := make([][]byte, len(tt.lines))
			for i, l := range tt.lines {
				lines[i] = []byte(l)
			}
			if got := cwdOf(lines); got != tt.want {
				t.Errorf("cwdOf = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHeadDropsALineCutByTheLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	writeFile(t, path, "aaaa\nbbbb\ncccc\n")
	tests := []struct {
		size int64
		want string
	}{
		{100, "aaaa bbbb cccc "},
		{12, "aaaa bbbb"},
		{7, "aaaa"},
		{3, ""},
	}
	for _, tt := range tests {
		lines, err := head(path, tt.size)
		if err != nil {
			t.Fatal(err)
		}
		var got string
		for _, l := range lines {
			got += string(l) + " "
		}
		if strings.TrimSpace(got) != strings.TrimSpace(tt.want) {
			t.Errorf("head(%d) = %q, want %q", tt.size, got, tt.want)
		}
	}
}

func TestRow(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	long := strings.Repeat("title words ", 10)
	tests := []struct {
		name string
		s    Session
		want []string
	}{
		{
			"renamed claude session",
			Session{Agent: "claude", ID: "id1", CWD: "/w/my-project", TS: float64(now.Unix() - 7300), Title: "Hand named", Renamed: true, Prompts: []string{"a  b\nc", "second"}},
			[]string{"claude\tid1\t/w/my-project\t", sgrMagenta + "claude" + sgrReset, sgrGreen + "●" + sgrReset, sgrDim + "  2h" + sgrReset, "Hand named" + strings.Repeat(" ", 30), "my-project", "  a b c | second" + sgrReset},
		},
		{
			"untitled shows the latest prompt",
			Session{Agent: "codex", ID: "id2", CWD: "/w/x", TS: float64(now.Unix()), Prompts: []string{"fix the build"}},
			[]string{sgrGreen + "codex " + sgrReset, sgrDim + "○" + sgrReset, "fix the build", "now"},
		},
		{"no prompt at all", Session{Agent: "cursor", ID: "id3", CWD: "/w/y", TS: float64(now.Unix())}, []string{"(no prompt)"}},
		{"label is cut to 40 columns", Session{Agent: "claude", ID: "id4", CWD: "/w/z", TS: float64(now.Unix()), Title: long}, []string{strings.TrimSpace(long)[:40]}},
		{"folder name is cut to 24", Session{Agent: "claude", ID: "id5", CWD: "/w/" + strings.Repeat("d", 40), TS: float64(now.Unix())}, []string{sgrDim + strings.Repeat("d", 24)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Row(&tt.s, now)
			for _, frag := range tt.want {
				if !strings.Contains(got, frag) {
					t.Errorf("row %q lacks %q", got, frag)
				}
			}
			if fields := strings.Split(got, "\t"); len(fields) != 4 || fields[0] != tt.s.Agent || fields[1] != tt.s.ID || fields[2] != tt.s.CWD {
				t.Errorf("tab fields = %q", fields)
			}
		})
	}
}

func TestRowColumnsLineUp(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	a := Row(&Session{Agent: "claude", ID: "1", CWD: "/w/a", TS: float64(now.Unix()), Title: "short"}, now)
	b := Row(&Session{Agent: "codex", ID: "2", CWD: "/w/longer-name", TS: float64(now.Unix() - 90000), Title: "a much longer title here"}, now)
	display := func(row string) string { return strings.SplitN(row, "\t", 4)[3] }
	if wa, wb := term.VisibleWidth(strings.TrimRight(display(a), " ")), term.VisibleWidth(strings.TrimRight(display(b), " ")); wa != wb {
		t.Errorf("display widths differ: %d vs %d\n%q\n%q", wa, wb, plainRow(display(a)), plainRow(display(b)))
	}
}

func plainRow(s string) string { return term.StripANSI(s) }

func TestSortedDropsUnresumableAndOrdersNewestFirst(t *testing.T) {
	dir := t.TempDir()
	a, b, c, d := folder(t, dir, "a"), folder(t, dir, "b"), folder(t, dir, "c"), folder(t, dir, "d")
	got := Sorted([]*Session{{ID: "old", CWD: a, TS: 1}, {ID: "none", TS: 99}, {ID: "gone", CWD: filepath.Join(dir, "deleted"), TS: 98}, {ID: "new", CWD: b, TS: 5}, {ID: "tie1", CWD: c, TS: 3}, {ID: "tie2", CWD: d, TS: 3}})
	var ids []string
	for _, s := range got {
		ids = append(ids, s.ID)
	}
	if want := "new tie1 tie2 old"; strings.Join(ids, " ") != want {
		t.Errorf("order = %v, want %s", ids, want)
	}
}
