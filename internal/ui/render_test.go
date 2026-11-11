package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kislow/muxrail/internal/gitinfo"
	"github.com/kislow/muxrail/internal/windows"
)

func TestEveryRowFitsThePane(t *testing.T) {
	env := testEnv()
	env.CapL, env.CapR = "", ""
	for _, width := range []int{18, 26, 34} {
		for _, layout := range []string{LayoutTiles, LayoutCompact} {
			t.Run(fmt.Sprintf("%s-%d", layout, width), func(t *testing.T) {
				for _, active := range []int{0, 2} {
					wins := fixtures()
					for i, w := range wins {
						w.Active = i == active
					}
					frame := env.Frame(wins, View{Width: width, Rows: 60, Layout: layout, Selected: 3})
					assertWidthAtMost(t, width, rows(frame))
					dragged := env.Frame(wins, View{Width: width, Rows: 60, Layout: layout, Selected: 3, Drag: &Drag{Source: 2, Over: 5}})
					assertWidthAtMost(t, width, rows(dragged))
				}
			})
		}
	}
}

func TestBodyRowSelection(t *testing.T) {
	env := testEnv()
	repo := &gitinfo.Info{Branch: "main", Dirty: gitinfo.Lines{Added: 1}}
	tests := []struct {
		name  string
		win   windows.Win
		width int
		want  int // body rows
	}{
		{"plain outside a repo: name and path", windows.Win{Command: "zsh", Path: "/tmp", Name: "w"}, 34, 2},
		{"plain in a repo: name, path, git", windows.Win{Command: "zsh", Path: "/tmp", Name: "w", Git: repo}, 34, 3},
		{"agent with title and git", windows.Win{Kind: "claude", Name: "w", Title: "✳ do it", Git: repo, Status: "working"}, 34, 3},
		{"agent with title, no git", windows.Win{Kind: "claude", Name: "w", Title: "✳ do it"}, 34, 2},
		{"agent without title or git falls back to the path", windows.Win{Kind: "claude", Name: "w", Path: "/tmp"}, 34, 2},
		{"agent without title but with git has no detail row", windows.Win{Kind: "claude", Name: "w", Git: repo}, 34, 2},
		{"auto-named agent title goes to the name row", windows.Win{Kind: "claude", Command: "2.1.1", Name: "2.1.1", Title: "✳ do it", Git: repo}, 34, 2},
		{"shell-looking title is ignored", windows.Win{Kind: "claude", Name: "w", Title: "me@host:~", Path: "/tmp"}, 34, 2},
		{"narrow agent: chip joins the detail row", windows.Win{Kind: "claude", Name: "w", Status: "done", Since: 1, Title: "t"}, 26, 2},
		{"narrow agent without title: chip and path", windows.Win{Kind: "claude", Name: "w", Status: "done", Path: "/p"}, 18, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.win.Since = float64(epoch.Unix() - 5)
			if got := len(env.bodyRows(&tt.win, tt.width)); got != tt.want {
				t.Errorf("%d body rows, want %d: %q", got, tt.want, env.bodyRows(&tt.win, tt.width))
			}
		})
	}
}

func TestCardHeights(t *testing.T) {
	env := testEnv()
	wins := fixtures()
	tiles := env.Heights(wins, 34, LayoutTiles)
	for i, h := range tiles {
		if h < 3 || h > 5 {
			t.Errorf("tile %d is %d rows, want 3 to 5", i, h)
		}
	}
	if len(set(tiles)) < 2 {
		t.Errorf("fixtures should give mixed heights, got %v", tiles)
	}
	for i, h := range env.Heights(wins, 34, LayoutCompact) {
		if h != 1 {
			t.Errorf("compact card %d is %d rows, want 1", i, h)
		}
	}
}

func set(xs []int) map[int]bool {
	m := map[int]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func TestCardDrawsBordersAndGrip(t *testing.T) {
	env := testEnv()
	w := &windows.Win{Index: 3, Command: "zsh", Path: "/tmp", Name: "w"}
	lines := env.card(w, dim, 34, "")
	got := plain(strings.Join(lines, "\n"))
	if !strings.HasPrefix(got, "╭─⠿─") || !strings.Contains(got, "╮") || !strings.Contains(got, "╰") || !strings.HasSuffix(got, "╯") {
		t.Errorf("card =\n%s", got)
	}
	for _, l := range lines[1 : len(lines)-1] {
		if p := plain(l); !strings.HasPrefix(p, "│") || !strings.HasSuffix(p, "│") {
			t.Errorf("body row not boxed: %q", p)
		}
	}
	if label := plain(env.card(w, dim, 34, "moving")[0]); !strings.Contains(label, "─ moving ") {
		t.Errorf("label missing from top border: %q", label)
	}
}

func TestBorderColours(t *testing.T) {
	active, other := &windows.Win{Active: true}, &windows.Win{}
	tests := []struct {
		name     string
		w        *windows.Win
		selected bool
		grabbed  bool
		want     string
	}{
		{"active is green", active, false, false, fg(2)},
		{"cursor is white", other, true, false, white},
		{"others are dim", other, false, false, dim},
		{"grabbed is magenta even when active", active, true, true, magenta},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := borderStyle(tt.w, tt.selected, tt.grabbed); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestActiveCardHasGreyBackground(t *testing.T) {
	env := testEnv()
	body := env.card(&windows.Win{Active: true, Name: "a", Path: "/p"}, dim, 34, "")[1]
	if !strings.Contains(body, greyBG) {
		t.Error("active card should have a grey background")
	}
	body = env.card(&windows.Win{Name: "a", Path: "/p"}, dim, 34, "")[1]
	if strings.Contains(body, greyBG) {
		t.Error("inactive card should not")
	}
}

func TestStateChip(t *testing.T) {
	env := testEnv()
	since := float64(epoch.Unix() - 125)
	tests := []struct {
		name  string
		win   windows.Win
		width int
		want  string
	}{
		{"plain window has none", windows.Win{Command: "zsh"}, 34, ""},
		{"waiting with word at 34", windows.Win{Kind: "claude", Status: "waiting", Since: since}, 34, "◆ waiting 2m"},
		{"waiting without word below 34", windows.Win{Kind: "claude", Status: "waiting", Since: since}, 33, "◆ 2m"},
		{"done", windows.Win{Kind: "codex", Status: "done", Since: since}, 26, "✔ 2m"},
		{"unknown state", windows.Win{Kind: "claude"}, 34, "○"},
		{"no since, no time", windows.Win{Kind: "claude", Status: "done"}, 34, "✔ done"},
		{"working spins", windows.Win{Kind: "claude", Status: "working", Since: since}, 26, spin(epoch) + " 2m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := plain(env.stateChip(&tt.win, tt.width)); got != tt.want {
				t.Errorf("chip = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSpinnerAdvancesEightTimesASecond(t *testing.T) {
	start := epoch.Add(10 * time.Millisecond) // off a frame boundary, so float rounding cannot matter
	if spin(start) == spin(start.Add(125*time.Millisecond)) {
		t.Error("the spinner should have moved after 125ms")
	}
	if spin(start) != spin(start.Add(1250*time.Millisecond)) {
		t.Error("ten frames at 8 fps wrap after 1.25s")
	}
}

func TestKfmt(t *testing.T) {
	tests := map[int]string{0: "0", 7: "7", 999: "999", 1000: "1k", 1049: "1k", 1250: "1.2k", 1300: "1.3k", 1999: "2k", 12000: "12k", 123456: "123.5k"}
	for in, want := range tests {
		if got := kfmt(in); got != want {
			t.Errorf("kfmt(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestGitRow(t *testing.T) {
	tests := []struct {
		name   string
		info   *gitinfo.Info
		room   int
		want   string
		wantNo string
	}{
		{"nil", nil, 20, "", ""},
		{"branch only", &gitinfo.Info{Branch: "main"}, 20, " main", ""},
		{"committed and dirty", &gitinfo.Info{Branch: "main", Committed: gitinfo.Lines{Added: 1300, Deleted: 12}, Dirty: gitinfo.Lines{Added: 4, Deleted: 2}}, 40,
			" main +1.3k -12 ✎ +4 -2", ""},
		{"zero parts are omitted", &gitinfo.Info{Branch: "b", Dirty: gitinfo.Lines{Deleted: 3}}, 40, " b ✎ -3", ""},
		{"branch gives way first", &gitinfo.Info{Branch: "feature/very-long-branch-name", Dirty: gitinfo.Lines{Added: 10, Deleted: 10}}, 24,
			" feature/ver…", ""},
		{"branch keeps at least six columns", &gitinfo.Info{Branch: "feature/very-long-branch-name", Committed: gitinfo.Lines{Added: 1000, Deleted: 1000}, Dirty: gitinfo.Lines{Added: 1000, Deleted: 1000}}, 10,
			" featu…", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := plain(gitRow(IconsNerd, tt.info, tt.room))
			if !strings.HasPrefix(got, tt.want) {
				t.Errorf("gitRow = %q, want prefix %q", got, tt.want)
			}
		})
	}
}

func TestTitleOf(t *testing.T) {
	tests := []struct {
		name string
		win  windows.Win
		want string
	}{
		{"status symbol is dropped", windows.Win{Kind: "claude", Title: "✳ Fix the bug"}, "Fix the bug"},
		{"plain title stays", windows.Win{Kind: "claude", Title: "Fix the bug"}, "Fix the bug"},
		{"symbol without a space stays", windows.Win{Kind: "claude", Title: "✳Fix"}, "✳Fix"},
		{"user@host:path is not a task", windows.Win{Kind: "claude", Title: "jordan@mac:~/x"}, ""},
		{"plain windows have none", windows.Win{Command: "zsh", Title: "vim"}, ""},
		{"empty", windows.Win{Kind: "claude"}, ""},
		{"surrounding space is trimmed", windows.Win{Kind: "claude", Title: "  ⠂ Work  "}, "Work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := titleOf(&tt.win); got != tt.want {
				t.Errorf("titleOf = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShortPath(t *testing.T) {
	env := testEnv()
	tests := []struct {
		path  string
		limit int
		want  string
	}{
		{"/Users/jordan/repos/x", 30, "~/repos/x"},
		{"/Users/jordan", 30, "~"},
		{"/opt/homebrew", 30, "/opt/homebrew"},
		{"/Users/jordan/repos/infra/platform", 12, "…ra/platform"},
	}
	for _, tt := range tests {
		if got := env.shortPath(tt.path, tt.limit); got != tt.want {
			t.Errorf("shortPath(%q, %d) = %q, want %q", tt.path, tt.limit, got, tt.want)
		}
	}
}

func TestAutoNamedAgentTakesItsTitleAsName(t *testing.T) {
	env := testEnv()
	w := &windows.Win{Kind: "claude", Command: "2.1.319", Name: "2.1.319", Title: "✳ Refactor picker", Path: "/tmp"}
	rows := env.bodyRows(w, 34)
	if !strings.Contains(plain(rows[0]), "Refactor picker") || strings.Contains(plain(rows[0]), "2.1.319") {
		t.Errorf("name row = %q", plain(rows[0]))
	}
	if strings.Contains(plain(strings.Join(rows, "")), "Refactor picker") && len(rows) > 1 && strings.Contains(plain(rows[1]), "Refactor picker") {
		t.Error("the title must not be repeated on the detail row")
	}
}

func TestCompactCard(t *testing.T) {
	env := testEnv()
	w := &windows.Win{Index: 2, Kind: "claude", Status: "done", Since: float64(epoch.Unix() - 60), Name: "agent-window", Command: "claude"}
	got := env.compactCard(w, 34, "▌")
	line := plain(got)
	if !strings.HasPrefix(line, "▌2 ") || !strings.Contains(line, "agent-window") || !strings.HasSuffix(strings.TrimRight(line, " "), "✔ done 1m") {
		t.Errorf("compact card = %q", line)
	}
	if strings.Contains(got, "\n") {
		t.Error("compact card must be a single line")
	}
	assertWidthAtMost(t, 33, []string{got})
}
