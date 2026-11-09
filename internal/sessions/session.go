// Package sessions builds the searchable list of agent sessions and the command that resumes one.
package sessions

import (
	"os"
	"sort"
	"strings"
)

// Session is one resumable agent conversation.
type Session struct {
	Agent   string
	ID      string
	CWD     string
	TS      float64 // epoch seconds
	Title   string
	Renamed bool
	Prompts []string
}

var agentNames = []string{"claude", "codex", "cursor"}

func basename(p string) string { return p[strings.LastIndexByte(p, '/')+1:] }

// Sorted drops sessions that cannot be resumed (no folder, or the folder is gone: the resume command starts with a cd)
// and orders the rest newest first.
func Sorted(all []*Session) []*Session {
	kept := make([]*Session, 0, len(all))
	for _, s := range all {
		if isDir(s.CWD) {
			kept = append(kept, s)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].TS > kept[j].TS })
	return kept
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
