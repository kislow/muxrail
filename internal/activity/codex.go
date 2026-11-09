package activity

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// freshFor bounds how old a transcript may be to belong to a running agent: Codex and Cursor do not say which pane
	// a session runs in, so the newest transcript of the pane's folder stands in for it.
	freshFor  = 12 * time.Hour
	codexDays = 8 // Codex files a rollout under the day it started, so a long session sits in an older folder
)

// CodexRollout is the newest recent Codex rollout started in cwd, or "". Subagent threads and `codex exec` runs are not
// interactive sessions and are skipped.
func CodexRollout(home, cwd string, now time.Time) string {
	var best string
	var bestMod time.Time
	for d := range codexDays {
		day := now.AddDate(0, 0, -d).Format("2006/01/02")
		files, err := filepath.Glob(filepath.Join(home, ".codex", "sessions", day, "rollout-*.jsonl"))
		if err != nil {
			continue
		}
		for _, path := range files {
			st, err := os.Stat(path)
			if err != nil || now.Sub(st.ModTime()) > freshFor || !st.ModTime().After(bestMod) {
				continue
			}
			if rolloutCwd(path) == cwd {
				best, bestMod = path, st.ModTime()
			}
		}
	}
	return best
}

// rolloutCwds holds each rollout's folder: its first line never changes, so a file is opened once.
var rolloutCwds memo[string]

// rolloutCwd is the folder an interactive session was started in; empty for anything else or an unreadable file.
func rolloutCwd(path string) string {
	if cwd, ok := rolloutCwds.get(path); ok {
		return cwd
	}
	line, err := firstLine(path)
	if err != nil {
		return ""
	}
	var meta struct {
		Type    string `json:"type"`
		Payload struct {
			Cwd    string          `json:"cwd"`
			Source json.RawMessage `json:"source"`
		} `json:"payload"`
	}
	if json.Unmarshal(line, &meta) != nil || meta.Type != "session_meta" {
		return "" // not remembered: the line may still be being written
	}
	cwd := meta.Payload.Cwd
	var source string
	if json.Unmarshal(meta.Payload.Source, &source) != nil || source == "exec" {
		cwd = "" // an object source is a subagent thread
	}
	rolloutCwds.put(path, cwd)
	return cwd
}

func firstLine(path string) (line []byte, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: open rollout", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("%w: close rollout", closeErr))
		}
	}()
	line, err = bufio.NewReaderSize(f, 1<<20).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return nil, fmt.Errorf("%w: read rollout", err)
	}
	return line, nil
}

// ReadCodex reads the end of a rollout.
func ReadCodex(path string) (*Info, error) { return readTail(path, parseCodex) }

type codexRecord struct {
	Type    string `json:"type"`
	Payload struct {
		Type      string          `json:"type"`
		Role      string          `json:"role"`
		Name      string          `json:"name"`
		Arguments string          `json:"arguments"` // function_call: a JSON document inside a string
		Input     string          `json:"input"`     // custom_tool_call: free text, such as a patch
		Info      json.RawMessage `json:"info"`
	} `json:"payload"`
}

type codexTokens struct {
	Last struct {
		Total int `json:"total_tokens"`
	} `json:"last_token_usage"`
	Window int `json:"model_context_window"`
}

// parseCodex follows the turn: a tool call opens until its output arrives, which hands the turn back to the model, as do
// reasoning and a new prompt; a final assistant message ends it. token_count events carry the context size and window.
func parseCodex(data []byte) *Info {
	var info Info
	seen := false
	for line := range bytes.SplitSeq(data, []byte("\n")) {
		var r codexRecord
		if len(line) == 0 || json.Unmarshal(line, &r) != nil {
			continue
		}
		p := r.Payload
		switch {
		case r.Type == "event_msg" && p.Type == "token_count":
			var t codexTokens
			if json.Unmarshal(p.Info, &t) == nil && t.Last.Total > 0 {
				info.Context, info.Window = t.Last.Total, t.Window
			}
		case r.Type != "response_item":
		case p.Type == "function_call" || p.Type == "custom_tool_call":
			seen, info.Action, info.Pending = true, Describe(p.Name, codexInput(p.Name, p.Arguments, p.Input)), true
		case p.Type == "function_call_output" || p.Type == "custom_tool_call_output" || p.Type == "reasoning":
			seen, info.Action, info.Pending = true, "thinking", false
		case p.Type == "message" && p.Role == "assistant":
			seen, info.Action, info.Pending = true, "", false
		case p.Type == "message" && p.Role == "user":
			seen, info.Action, info.Pending = true, "thinking", false
		}
	}
	if !seen {
		return nil
	}
	return &info
}

// codexInput is a tool call's arguments as a map. A patch names the file it changes in its first header line; other
// custom tools carry source code, which says nothing in a few words.
func codexInput(tool, arguments, input string) map[string]any {
	if tool == "apply_patch" {
		for line := range bytes.SplitSeq([]byte(input), []byte("\n")) {
			for _, header := range []string{"*** Update File: ", "*** Add File: ", "*** Delete File: "} {
				if rest, ok := bytes.CutPrefix(line, []byte(header)); ok {
					return map[string]any{"file": string(rest)}
				}
			}
		}
		return nil
	}
	var args map[string]any
	if arguments == "" || json.Unmarshal([]byte(arguments), &args) != nil {
		return nil
	}
	return args
}
