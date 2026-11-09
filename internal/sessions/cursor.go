package sessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// cursorSessions lists the Cursor CLI sessions: transcripts at ~/.cursor/projects/<slug>/agent-transcripts/<id>/<id>.jsonl
// that also have a CLI chat folder (~/.cursor/chats/*/<id>). Without one the transcript comes from the IDE's agent,
// which `cursor-agent --resume` cannot open. The legacy <id>.txt transcripts and store.db are not read.
func cursorSessions(ctx context.Context, home string) ([]*Session, error) {
	chats, chatsErr := cursorChats(home)
	files, err := filepath.Glob(transcriptGlob(home, "cursor", "*"))
	if err != nil {
		return nil, fmt.Errorf("%w: list cursor transcripts", err)
	}
	var own []string // the files below a session folder that are not its transcript belong to subagents
	for _, f := range files {
		id := strings.TrimSuffix(filepath.Base(f), ".jsonl")
		if _, cli := chats[id]; cli && filepath.Base(filepath.Dir(f)) == id {
			own = append(own, f)
		}
	}
	found, err := readEach(ctx, own, func(path string) (*Session, error) { return cursorSession(path, chats) })
	return found, errors.Join(chatsErr, err)
}

// cursorChats maps the id of each CLI chat to the folder it ran in, from the meta.json beside its store.db.
func cursorChats(home string) (map[string]string, error) {
	metas, err := filepath.Glob(filepath.Join(home, ".cursor", "chats", "*", "*", "meta.json"))
	if err != nil {
		return nil, fmt.Errorf("%w: list cursor chats", err)
	}
	chats := make(map[string]string, len(metas))
	var errs []error
	for _, m := range metas {
		data, err := os.ReadFile(m)
		var meta struct {
			CWD string `json:"cwd"`
		}
		if err == nil {
			err = json.Unmarshal(data, &meta)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: cursor chat %s", err, filepath.Base(filepath.Dir(m))))
		}
		chats[filepath.Base(filepath.Dir(m))] = meta.CWD
	}
	return chats, errors.Join(errs...)
}

func cursorSession(path string, chats map[string]string) (*Session, error) {
	id := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	projectDir := filepath.Dir(filepath.Dir(filepath.Dir(path)))
	cwd, err := workspacePath(projectDir)
	if err != nil {
		return nil, err
	}
	if cwd == "" {
		cwd = chats[id]
	}
	if cwd == "" || delegateRE.MatchString(cwd) {
		return nil, nil
	}
	lines, err := head(path, headBytes)
	if err != nil {
		return nil, err
	}
	ts, err := mtime(path)
	if err != nil {
		return nil, err
	}
	return &Session{Agent: "cursor", ID: id, CWD: cwd, TS: ts, Prompts: promptsOf(lines)}, nil
}

// workspacePath is the folder Cursor trusted for a project, "" when it never recorded one.
func workspacePath(projectDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(projectDir, ".workspace-trusted"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("%w: read workspace file", err)
	}
	var doc struct {
		WorkspacePath string `json:"workspacePath"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("%w: parse %s", err, filepath.Join(projectDir, ".workspace-trusted"))
	}
	return doc.WorkspacePath, nil
}
