package gitinfo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func lines(n int) string {
	s := ""
	for i := range n {
		s += "line " + string(rune('a'+i)) + "\n"
	}
	return s
}

// clone returns a working copy of a fresh bare origin whose default branch main holds one commit of three lines.
func clone(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	origin, work := filepath.Join(root, "origin.git"), filepath.Join(root, "work")
	run(t, root, "init", "-q", "--bare", "-b", "main", origin)
	run(t, root, "clone", "-q", origin, work)
	run(t, work, "checkout", "-q", "-b", "main")
	write(t, filepath.Join(work, "f.txt"), lines(3))
	run(t, work, "add", ".")
	run(t, work, "commit", "-q", "-m", "init")
	run(t, work, "push", "-q", "-u", "origin", "main")
	run(t, work, "remote", "set-head", "origin", "main")
	return work
}

func TestParseNumstat(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Lines
	}{
		{"empty", "", Lines{}},
		{"two files", "3\t1\ta.go\n10\t0\tb.go\n", Lines{13, 1}},
		{"binary counts zero", "-\t-\timg.png\n4\t2\ta.go", Lines{4, 2}},
		{"rename keeps counts", "1\t1\told => new\n", Lines{1, 1}},
		{"junk line is skipped", "nonsense\n2\t3\tf", Lines{2, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseNumstat(tt.in); got != tt.want {
				t.Errorf("ParseNumstat = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCompute(t *testing.T) {
	ctx := context.Background()
	t.Run("clean default branch", func(t *testing.T) {
		got := Compute(ctx, clone(t))
		if got == nil || got.Branch != "main" || got.Committed != (Lines{}) || got.Dirty != (Lines{}) {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("default branch with unpushed commits", func(t *testing.T) {
		work := clone(t)
		write(t, filepath.Join(work, "g.txt"), lines(4))
		run(t, work, "add", ".")
		run(t, work, "commit", "-q", "-m", "unpushed")
		got := Compute(ctx, work)
		if got.Branch != "main" || got.Committed != (Lines{4, 0}) {
			t.Errorf("got %+v, want 4 unpushed added lines", got)
		}
	})
	t.Run("feature branch measures against the default branch", func(t *testing.T) {
		work := clone(t)
		run(t, work, "checkout", "-q", "-b", "feat/x")
		write(t, filepath.Join(work, "f.txt"), lines(2)) // 3 lines -> 2: one deleted
		write(t, filepath.Join(work, "h.txt"), lines(5))
		run(t, work, "add", ".")
		run(t, work, "commit", "-q", "-m", "work")
		got := Compute(ctx, work)
		if got.Branch != "feat/x" || got.Committed != (Lines{5, 1}) {
			t.Errorf("got %+v, want +5 -1 committed", got)
		}
	})
	t.Run("dirty tree counts tracked changes against HEAD", func(t *testing.T) {
		work := clone(t)
		write(t, filepath.Join(work, "f.txt"), lines(3)+"extra\nmore\n")
		got := Compute(ctx, work)
		if got.Dirty != (Lines{2, 0}) || got.Committed != (Lines{}) {
			t.Errorf("got %+v, want 2 dirty added lines", got)
		}
	})
	t.Run("no remote means no committed count", func(t *testing.T) {
		dir := t.TempDir()
		run(t, dir, "init", "-q", "-b", "main")
		write(t, filepath.Join(dir, "f"), "x\n")
		run(t, dir, "add", ".")
		run(t, dir, "commit", "-q", "-m", "init")
		got := Compute(ctx, dir)
		if got == nil || got.Branch != "main" || got.Committed != (Lines{}) {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("detached head shows the short hash", func(t *testing.T) {
		work := clone(t)
		run(t, work, "checkout", "-q", "--detach")
		got := Compute(ctx, work)
		if got == nil || got.Branch == "" || got.Branch == "main" || len(got.Branch) < 7 {
			t.Errorf("got %+v, want a short hash", got)
		}
	})
	t.Run("not a repository", func(t *testing.T) {
		if got := Compute(ctx, t.TempDir()); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})
}
