package sessions

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
)

// codexSessions lists ~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl, leaving out the sessions a person never typed in.
func codexSessions(ctx context.Context, home string) ([]*Session, error) {
	files, err := filepath.Glob(transcriptGlob(home, "codex", "*"))
	if err != nil {
		return nil, fmt.Errorf("%w: list codex transcripts", err)
	}
	return readEach(ctx, files, codexSession)
}

type codexMeta struct {
	Payload struct {
		ID     string          `json:"id"`
		CWD    string          `json:"cwd"`
		Source json.RawMessage `json:"source"`
	} `json:"payload"`
	Type string `json:"type"`
}

// interactive is false for subagent threads (source is an object, such as {"subagent": ...}) and for `codex exec` runs.
func (m *codexMeta) interactive() bool {
	var source string
	if err := json.Unmarshal(m.Payload.Source, &source); err != nil {
		return len(m.Payload.Source) == 0
	}
	return source != "exec"
}

func codexSession(path string) (*Session, error) {
	lines, err := head(path, headBytes)
	if err != nil {
		return nil, err
	}
	var meta codexMeta
	if len(lines) == 0 || json.Unmarshal(lines[0], &meta) != nil || meta.Type != "session_meta" {
		return nil, fmt.Errorf("%s does not start with a session_meta record", filepath.Base(path))
	}
	if meta.Payload.ID == "" || meta.Payload.CWD == "" || !meta.interactive() {
		return nil, nil
	}
	ts, err := mtime(path)
	if err != nil {
		return nil, err
	}
	return &Session{Agent: "codex", ID: meta.Payload.ID, CWD: meta.Payload.CWD, TS: ts, Prompts: promptsOf(lines[1:])}, nil
}
