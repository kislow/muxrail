// Package activity reads what a running agent is doing from its transcript: the tool call in flight and how full its
// context is. Only Claude Code records which tmux pane a session runs in, so only Claude has it.
package activity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Info is what a Claude transcript says about a running session.
type Info struct {
	// Action is what the session is doing right now ("Edit rail.go", "thinking"); empty once its last reply is done.
	Action string
	// Pending is true when Action is a tool call that has not run yet: for a waiting session, a permission prompt.
	Pending bool
	// Context is how many tokens the context held after the last reply.
	Context int
	// Window is the context window the agent reports for the model; zero when the transcript does not say.
	Window int
	// Cwd is the folder the session last ran in, which follows the agent as it changes directory; empty when the
	// transcript does not say. The pane's own folder stays where the agent was started.
	Cwd string
}

// tailBytes is how much of a transcript's end is read: a tool result can be large, so it is generous.
const tailBytes = 256 << 10

type sessionFile struct {
	SessionID string `json:"sessionId"`
	Tmux      string `json:"tmux"`
	StartedAt int64  `json:"startedAt"`
}

// sessionFiles holds the parsed session files by path; a file is read again only after it changed.
var sessionFiles memo[parsedSession]

type parsedSession struct {
	mod  time.Time
	size int64
	file sessionFile
	ok   bool
}

func readSession(path string) (sessionFile, bool) {
	st, err := os.Stat(path)
	if err != nil {
		return sessionFile{}, false
	}
	if hit, found := sessionFiles.get(path); found && hit.mod.Equal(st.ModTime()) && hit.size == st.Size() {
		return hit.file, hit.ok
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return sessionFile{}, false
	}
	var s sessionFile
	ok := json.Unmarshal(data, &s) == nil
	sessionFiles.put(path, parsedSession{mod: st.ModTime(), size: st.Size(), file: s, ok: ok})
	return s, ok
}

// SessionID is the id of the newest Claude session started in the given tmux pane, from ~/.claude/sessions/<pid>.json.
// It is empty when none is found.
func SessionID(home, windowID, pane string) string {
	files, _ := filepath.Glob(filepath.Join(home, ".claude", "sessions", "*.json")) // only a bad pattern fails
	suffix := ":" + windowID + "." + pane
	var id string
	var newest int64
	for _, path := range files {
		s, ok := readSession(path)
		if !ok || !strings.HasSuffix(s.Tmux, suffix) || s.SessionID == "" {
			continue
		}
		if s.StartedAt >= newest {
			id, newest = s.SessionID, s.StartedAt
		}
	}
	return id
}

// transcripts holds where each session's transcript lives. Finding it means looking in every project folder, hundreds of
// them, and a transcript never moves.
var transcripts memo[string]

// Read finds the transcript of a session and reads its end. A session without a transcript yet has no Info.
func Read(home, sessionID string) (*Info, error) {
	key := home + "\x00" + sessionID
	if path, ok := transcripts.get(key); ok {
		if info, err := readTail(path, parse); err == nil {
			return info, nil
		}
		transcripts.forget(key)
	}
	paths, err := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", sessionID+".jsonl"))
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	transcripts.put(key, paths[0])
	return readTail(paths[0], parse)
}

// cached remembers what a transcript said while its size and modification time stay the same, so a sidebar that asks
// every second reads and parses a transcript only after the agent wrote to it.
var cached = struct {
	sync.Mutex
	byPath map[string]cachedInfo
}{byPath: map[string]cachedInfo{}}

type cachedInfo struct {
	mod  time.Time
	size int64
	info *Info
}

func readTail(path string, parse func([]byte) *Info) (*Info, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("%w: stat transcript", err)
	}
	cached.Lock()
	hit, ok := cached.byPath[path]
	cached.Unlock()
	if ok && hit.mod.Equal(st.ModTime()) && hit.size == st.Size() {
		return hit.info, nil
	}
	data, err := tail(path)
	if err != nil {
		return nil, err
	}
	info := parse(data)
	cached.Lock()
	cached.byPath[path] = cachedInfo{mod: st.ModTime(), size: st.Size(), info: info}
	cached.Unlock()
	return info, nil
}

func tail(path string) (data []byte, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: open transcript", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("%w: close transcript", cerr)
		}
	}()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("%w: stat transcript", err)
	}
	start := max(info.Size()-tailBytes, 0)
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, fmt.Errorf("%w: seek transcript", err)
	}
	if data, err = io.ReadAll(f); err != nil {
		return nil, fmt.Errorf("%w: read transcript", err)
	}
	if start > 0 { // the first line is cut in the middle
		_, data, _ = bytes.Cut(data, []byte("\n"))
	}
	return data, nil
}

type record struct {
	Type    string `json:"type"`
	Message struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
		Usage   *struct {
			Input       int `json:"input_tokens"`
			CacheCreate int `json:"cache_creation_input_tokens"`
			CacheRead   int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
	Sidechain bool   `json:"isSidechain"`
	Cwd       string `json:"cwd"`
}

type block struct {
	Type  string         `json:"type"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// parse walks the records in order. Each content block is its own record, so the last block of the last reply says what
// is happening: a tool_use is a call in flight, thinking is thinking, text is a finished reply; a user record after
// it (a tool result or a new prompt) hands the turn back to the model.
func parse(data []byte) *Info {
	var info Info
	seen := false
	for line := range bytes.SplitSeq(data, []byte("\n")) {
		var r record
		if len(line) == 0 || json.Unmarshal(line, &r) != nil || r.Sidechain {
			continue
		}
		if r.Cwd != "" {
			info.Cwd = r.Cwd
		}
		switch r.Message.Role {
		case "assistant":
			seen = true
			if u := r.Message.Usage; u != nil && u.Input+u.CacheCreate+u.CacheRead > 0 {
				info.Context = u.Input + u.CacheCreate + u.CacheRead
			}
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

func lastBlock(content json.RawMessage) (action string, pending bool) {
	var blocks []block
	if json.Unmarshal(content, &blocks) != nil || len(blocks) == 0 {
		return "", false
	}
	switch b := blocks[len(blocks)-1]; b.Type {
	case "tool_use":
		return Describe(b.Name, b.Input), true
	case "thinking":
		return "thinking", false
	}
	return "", false
}
