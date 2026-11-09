package sessions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const maxPrompts = 10

// claudeSessions lists the top-level transcripts of ~/.claude/projects: files below a session folder belong to
// subagents. A headless run (`claude -p`, a `ki delegate` job) is left out: it writes neither a history entry nor,
// usually, a title, and its folder is never somewhere to resume.
func claudeSessions(ctx context.Context, home string) ([]*Session, error) {
	files, err := filepath.Glob(transcriptGlob(home, "claude", "*"))
	if err != nil {
		return nil, fmt.Errorf("%w: list claude transcripts", err)
	}
	prompts, historyErr := claudeHistory(home)
	found, err := readEach(ctx, files, func(path string) (*Session, error) { return claudeSession(path, prompts) })
	return found, errors.Join(historyErr, err)
}

func claudeSession(path string, history map[string][]string) (*Session, error) {
	lines, err := head(path, headBytes)
	if err != nil {
		return nil, err
	}
	cwd := cwdOf(lines)
	if cwd == "" || delegateRE.MatchString(cwd) {
		return nil, nil
	}
	id := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	ts, err := mtime(path)
	if err != nil {
		return nil, err
	}
	title, err := scanTitle(path)
	if err != nil || (title == nil && len(history[id]) == 0) {
		return nil, err
	}
	s := &Session{Agent: "claude", ID: id, CWD: cwd, TS: ts, Prompts: history[id]}
	if len(s.Prompts) == 0 {
		s.Prompts = promptsOf(lines)
	}
	if title != nil {
		s.Title, s.Renamed = title.Text, title.Renamed
	}
	return s, nil
}

// claudeHistory maps a session id to what was typed in it, newest first, from ~/.claude/history.jsonl. A missing file
// is an empty history.
func claudeHistory(home string) (map[string][]string, error) {
	data, err := os.ReadFile(filepath.Join(home, ".claude", "history.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: read claude history", err)
	}
	byID := map[string][]string{}
	for _, line := range bytes.Split(data, []byte("\n")) {
		var rec struct {
			Display   string `json:"display"`
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(line, &rec) != nil || rec.SessionID == "" || strings.TrimSpace(rec.Display) == "" {
			continue
		}
		byID[rec.SessionID] = append(byID[rec.SessionID], rec.Display)
	}
	for id, p := range byID {
		slices.Reverse(p)
		byID[id] = p[:min(len(p), maxPrompts)]
	}
	return byID, nil
}

// promptsOf is the first things a person typed in the lines of a transcript head, with what the agent injected
// itself (environment blocks, Codex's AGENTS.md preamble) left out.
func promptsOf(lines [][]byte) []string {
	var prompts []string
	for _, line := range lines {
		t, ok := turnOf(line)
		if !ok || t.Role != "user" || strings.HasPrefix(t.Text, "# AGENTS.md instructions") {
			continue
		}
		prompts = append(prompts, t.Text)
		if len(prompts) == maxPrompts {
			break
		}
	}
	return prompts
}
