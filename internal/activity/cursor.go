package activity

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// CursorTranscript is the newest recent Cursor CLI transcript of the project whose folder is cwd, or "". Cursor names a
// project folder after the path with its separators changed, so the two are compared by their letters and digits.
func CursorTranscript(home, cwd string, now time.Time) string {
	want := alphanumeric(cwd)
	projects, err := filepath.Glob(filepath.Join(home, ".cursor", "projects", "*"))
	if err != nil {
		return ""
	}
	var best string
	var bestMod time.Time
	for _, project := range projects {
		if alphanumeric(filepath.Base(project)) != want {
			continue
		}
		files, err := filepath.Glob(filepath.Join(project, "agent-transcripts", "*", "*.jsonl"))
		if err != nil {
			continue
		}
		for _, path := range files {
			if st, err := os.Stat(path); err == nil && now.Sub(st.ModTime()) <= freshFor && st.ModTime().After(bestMod) {
				best, bestMod = path, st.ModTime()
			}
		}
	}
	return best
}

func alphanumeric(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}

// ReadCursor reads the end of a transcript.
func ReadCursor(path string) (*Info, error) { return readTail(path, parseCursor) }

// parseCursor reads what the last record says. A Cursor transcript records tool calls but neither their results nor
// token counts, so a call is taken to be in flight until a later record replaces it.
func parseCursor(data []byte) *Info {
	var info Info
	seen := false
	for line := range bytes.SplitSeq(data, []byte("\n")) {
		var r struct {
			Role    string `json:"role"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if len(line) == 0 || json.Unmarshal(line, &r) != nil {
			continue
		}
		switch r.Role {
		case "assistant":
			seen = true
			info.Action, info.Pending = lastBlock(r.Message.Content)
		case "user":
			seen = true
			info.Action, info.Pending = "thinking", false
		}
	}
	if !seen {
		return nil
	}
	return &info
}
