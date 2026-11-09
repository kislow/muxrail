// Package gitinfo reports a folder's branch and how many lines changed, committed and uncommitted.
package gitinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Lines is a count of added and deleted lines. It is stored as [added, deleted] in the cache file.
type Lines struct{ Added, Deleted int }

func (l Lines) MarshalJSON() ([]byte, error) { return json.Marshal([2]int{l.Added, l.Deleted}) }

func (l *Lines) UnmarshalJSON(data []byte) error {
	var pair [2]int
	if err := json.Unmarshal(data, &pair); err != nil {
		return fmt.Errorf("%w: lines", err)
	}
	l.Added, l.Deleted = pair[0], pair[1]
	return nil
}

// Info describes one folder's repository.
type Info struct {
	Branch string `json:"branch"`
	// Committed is the change since the base branch, or the unpushed commits when on the default branch.
	Committed Lines `json:"committed"`
	// Dirty is the working tree against HEAD.
	Dirty Lines `json:"dirty"`
}

// git runs git in dir. A failing git (not a repository, no upstream) means "no answer", which every caller treats as empty.
func git(ctx context.Context, dir string, args ...string) string {
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ParseNumstat sums `git diff --numstat` output; binary files print "-" and count as 0.
func ParseNumstat(out string) Lines {
	var l Lines
	for line := range strings.SplitSeq(out, "\n") {
		cols := strings.Split(line, "\t")
		if len(cols) < 2 {
			continue
		}
		l.Added += count(cols[0])
		l.Deleted += count(cols[1])
	}
	return l
}

func count(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// Compute inspects dir; nil when it is not in a repository.
func Compute(ctx context.Context, dir string) *Info {
	branch := git(ctx, dir, "symbolic-ref", "--short", "-q", "HEAD")
	if branch == "" {
		branch = git(ctx, dir, "rev-parse", "--short", "HEAD")
	}
	if branch == "" {
		return nil
	}
	info := &Info{Branch: branch, Dirty: ParseNumstat(git(ctx, dir, "diff", "--numstat", "HEAD"))}
	if base := baseRef(ctx, dir, branch); base != "" {
		info.Committed = ParseNumstat(git(ctx, dir, "diff", "--numstat", base+"...HEAD"))
	}
	return info
}

// baseRef is what committed changes are measured against. On the default branch the interesting commits are the
// unpushed ones; elsewhere it is everything since the default branch.
func baseRef(ctx context.Context, dir, branch string) string {
	def := git(ctx, dir, "symbolic-ref", "--short", "-q", "refs/remotes/origin/HEAD")
	if def != "" {
		_, short, _ := strings.Cut(def, "/")
		if branch != short {
			return def
		}
	}
	return git(ctx, dir, "rev-parse", "--abbrev-ref", "@{upstream}")
}
