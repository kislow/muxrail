package activity

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Vibe keeps one store per session in its session folder: a snapshot of the session's state as of the last quiet moment
// (generations/<CURRENT>/projection-state.json) and a journal of what happened since (journal/<n>.jsonl, the newest).
// Its hooks fire only around tool calls and after a turn, so neither a prompt nor a permission prompt reaches muxrail
// through them; the store records both, so the state is read from there.

// vibeHome is where Vibe keeps its files: $VIBE_HOME, else ~/.vibe.
func vibeHome(home string) string {
	if dir := os.Getenv("VIBE_HOME"); filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(home, ".vibe")
}

func vibeSessions(home string) string { return filepath.Join(vibeHome(home), "logs", "session") }

// VibeSessions maps the process id that holds each session's lock to the session's id. The holder is Vibe's app
// server, which runs on the terminal of the Vibe it serves. A lock left by a process that has died stays on disk, so the
// caller keeps only the ids whose process it still sees.
func VibeSessions(home string) map[int]string {
	locks, err := filepath.Glob(filepath.Join(vibeSessions(home), "active", "*.lock.json"))
	if err != nil {
		return nil
	}
	out := map[int]string{}
	for _, path := range locks {
		var lock struct {
			ProcessID int    `json:"process_id"`
			SessionID string `json:"session_id"`
		}
		data, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(data, &lock) != nil || lock.ProcessID <= 0 || !validVibeID(lock.SessionID) {
			continue
		}
		out[lock.ProcessID] = lock.SessionID
	}
	return out
}

// validVibeID accepts a plain folder name, so a lock file can never steer the reader elsewhere.
func validVibeID(id string) bool {
	return id != "" && id != "." && id != ".." && !strings.ContainsAny(id, `/\`)
}

// ReadVibe reads what a Vibe session is doing from its store; nil when the session has no store yet.
func ReadVibe(home, id string) (*Info, error) {
	if !validVibeID(id) {
		return nil, nil
	}
	dir := filepath.Join(vibeSessions(home), "unified", id)
	window := vibeWindow(home, id)
	parse := func(data []byte) *Info {
		info := parseVibe(vibeSnapshot(dir), data)
		if info != nil {
			info.Window = window
			if info.Cwd == "" {
				info.Cwd = vibeHistoryCwd(dir)
			}
			info.Cwd = expandHome(home, info.Cwd)
		}
		return info
	}
	journals, err := filepath.Glob(filepath.Join(dir, "journal", "*.jsonl"))
	if err != nil || len(journals) == 0 {
		return parse(nil), nil
	}
	return readTail(slices.Max(journals), parse) // the names are zero-padded sequence numbers, so the newest sorts last
}

// vibeSnapshot is the session state in the current generation, which the newest journal continues from; nil when there
// is none yet.
func vibeSnapshot(dir string) *vibeEnvelope {
	gen := vibeGeneration(dir)
	if gen == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(gen, "projection-state.json"))
	if err != nil {
		return nil
	}
	var state struct {
		Snapshot *vibeEnvelope `json:"snapshot"`
	}
	if json.Unmarshal(data, &state) != nil {
		return nil
	}
	return state.Snapshot
}

// vibeGeneration is the folder of the session's current generation; empty when there is none yet.
func vibeGeneration(dir string) string {
	var current struct {
		Generation string `json:"generation"`
	}
	data, err := os.ReadFile(filepath.Join(dir, "CURRENT"))
	if err != nil || json.Unmarshal(data, &current) != nil || !validVibeID(current.Generation) {
		return ""
	}
	return filepath.Join(dir, "generations", current.Generation)
}

// vibeHistoryChunks bounds how far back the history is searched for a folder: a chunk holds a couple of dozen entries.
const vibeHistoryChunks = 4

// vibeChunkCwds holds the folder each history chunk names. A chunk is named after the hash of its content and never
// changes, so what it says is read once.
var vibeChunkCwds memo[string]

// vibeHistoryCwd is the folder of the newest shell command in the session's history that changes into one, for when
// the journal since the last snapshot has none; empty when the last few chunks have none either.
func vibeHistoryCwd(dir string) string {
	gen := vibeGeneration(dir)
	if gen == "" {
		return ""
	}
	var manifest struct {
		ProjectionState struct {
			Chunks []string `json:"chunks"`
		} `json:"projection_state"`
	}
	data, err := os.ReadFile(filepath.Join(gen, "manifest.json"))
	if err != nil || json.Unmarshal(data, &manifest) != nil {
		return ""
	}
	chunks := manifest.ProjectionState.Chunks // oldest first
	for i := len(chunks) - 1; i >= max(len(chunks)-vibeHistoryChunks, 0); i-- {
		if cwd := vibeChunkCwd(dir, chunks[i]); cwd != "" {
			return cwd
		}
	}
	return ""
}

func vibeChunkCwd(dir, chunk string) string {
	if !validVibeID(chunk) {
		return ""
	}
	path := filepath.Join(dir, "chunks", chunk+".json")
	if cwd, ok := vibeChunkCwds.get(path); ok {
		return cwd
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var entries []struct {
		Type   string   `json:"type"`
		Detail vibeTool `json:"detail"`
	}
	if json.Unmarshal(data, &entries) != nil {
		return "" // not remembered: some chunks hold other records, and those are not worth reading again either way
	}
	var cwd string
	for _, e := range slices.Backward(entries) {
		if e.Type == "effect" {
			if cwd = cdTarget(e.Detail); cwd != "" {
				break
			}
		}
	}
	vibeChunkCwds.put(path, cwd)
	return cwd
}

// cdTarget is the folder a shell command changes into first (`cd "/r/app" && go test`). Vibe runs every command from
// the session's folder, so the agent names the repo it works on this way; only an absolute or ~ path counts.
func cdTarget(t vibeTool) string {
	if b := vibeBuiltin(t.ToolName); b != "bash" && b != "start" {
		return ""
	}
	command, _ := t.Input["command"].(string)
	rest, ok := strings.CutPrefix(strings.TrimSpace(command), "cd ")
	if !ok {
		return ""
	}
	rest = strings.TrimLeft(rest, " ")
	var path string
	if rest != "" && (rest[0] == '"' || rest[0] == '\'') {
		end := strings.IndexByte(rest[1:], rest[0])
		if end < 0 {
			return ""
		}
		path = rest[1 : end+1]
	} else {
		end := strings.IndexAny(rest, " ;&|\n")
		if end < 0 {
			end = len(rest)
		}
		path = rest[:end]
	}
	if !filepath.IsAbs(path) && path != "~" && !strings.HasPrefix(path, "~/") {
		return ""
	}
	return path
}

// expandHome turns a leading ~ into home and cleans the path.
func expandHome(home, path string) string {
	if path == "" {
		return ""
	}
	if rest, ok := strings.CutPrefix(path, "~"); ok {
		path = home + rest
	}
	return filepath.Clean(path)
}

// vibeIndexes holds the context windows of every session listed in the session index, read again only after it changed.
var vibeIndexes memo[vibeIndex]

type vibeIndex struct {
	mod     time.Time
	size    int64
	windows map[string]int
}

// vibeWindow is the context window of the session's model, which the session index records; zero when it does not.
func vibeWindow(home, id string) int {
	path := filepath.Join(vibeSessions(home), "unified", ".session-index.json")
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if idx, ok := vibeIndexes.get(path); ok && idx.mod.Equal(st.ModTime()) && idx.size == st.Size() {
		return idx.windows[id]
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var index struct {
		Entries []struct {
			Identity struct {
				SessionID string `json:"session_id"`
			} `json:"identity"`
			Metadata struct {
				Catalogue string `json:"completion_catalogue"` // a JSON document inside a string
			} `json:"metadata"`
		} `json:"entries"`
	}
	if json.Unmarshal(data, &index) != nil {
		return 0 // not remembered: the index may still be being written
	}
	windows := map[string]int{}
	for _, e := range index.Entries {
		var catalogue struct {
			ContextWindow int `json:"context_window"`
		}
		if json.Unmarshal([]byte(e.Metadata.Catalogue), &catalogue) == nil && catalogue.ContextWindow > 0 {
			windows[e.Identity.SessionID] = catalogue.ContextWindow
		}
	}
	vibeIndexes.put(path, vibeIndex{mod: st.ModTime(), size: st.Size(), windows: windows})
	return windows[id]
}

// vibeEnvelope is the part of a session's state muxrail reads: whether a turn runs, how the last one went, the context
// size and the prompts waiting for an answer.
type vibeEnvelope struct {
	ActiveCallbacks []vibeCallback `json:"activeCallbacks"`
	LatestTurn      *struct {
		StartedAt   int64 `json:"startedAt"`
		CompletedAt int64 `json:"completedAt"`
	} `json:"latestTurn"`
	Session struct {
		Status struct {
			Type string `json:"type"`
		} `json:"status"`
		ContextUsage *struct {
			TotalTokens int `json:"totalTokens"`
		} `json:"contextUsage"`
		UpdatedAt int64 `json:"updatedAt"`
	} `json:"session"`
}

// vibeCallback is a question to the person: an approval for a tool call, or the agent asking for input.
type vibeCallback struct {
	CallbackID string `json:"callbackId"`
	CreatedAt  int64  `json:"createdAt"`
	Detail     struct {
		Kind   string    `json:"kind"`
		Effect *vibeTool `json:"effect"`
	} `json:"detail"`
}

type vibeTool struct {
	ToolName string         `json:"toolName"`
	Input    map[string]any `json:"input"`
}

type vibeRecord struct {
	Type    string `json:"type"`
	Payload struct {
		Delta []struct {
			Op    string        `json:"op"`
			State *vibeEnvelope `json:"state"`
			Entry *struct {
				Type   string   `json:"type"`
				Role   string   `json:"role"`
				Detail vibeTool `json:"detail"`
				State  struct {
					Status string `json:"status"`
				} `json:"state"`
			} `json:"entry"`
		} `json:"delta"`
		CallbackID string       `json:"callback_id"`
		Routing    vibeCallback `json:"routing"`
	} `json:"payload"`
}

// parseVibe replays the journal over the snapshot. set_envelope replaces the session state; history entries move the
// action along as with the other agents (a running tool call, then thinking until the reply); a registered callback is
// open until it is resolved, and an open one means the session waits for the person.
func parseVibe(snap *vibeEnvelope, data []byte) *Info {
	env := snap
	open := map[string]vibeCallback{}
	var order []string
	if env != nil {
		for _, cb := range env.ActiveCallbacks {
			open[cb.CallbackID], order = cb, append(order, cb.CallbackID)
		}
	}
	var action, cwd string
	for line := range bytes.SplitSeq(data, []byte("\n")) {
		var r vibeRecord
		if len(line) == 0 || json.Unmarshal(line, &r) != nil {
			continue
		}
		switch r.Type {
		case "callback_registered":
			open[r.Payload.CallbackID], order = r.Payload.Routing, append(order, r.Payload.CallbackID)
		case "callback_resolved":
			delete(open, r.Payload.CallbackID)
		case "projection_delta":
			for _, d := range r.Payload.Delta {
				if e := d.Entry; e != nil && e.Type == "effect" {
					if c := cdTarget(e.Detail); c != "" {
						cwd = c
					}
				}
				switch e := d.Entry; {
				case d.Op == "set_envelope" && d.State != nil:
					env = d.State
				case e == nil:
				case e.Type == "effect" && e.State.Status == "running":
					action = Describe(e.Detail.ToolName, e.Detail.Input)
				case e.Type == "effect", e.Type == "reasoning", e.Type == "message" && e.Role == "user":
					action = "thinking"
				case e.Type == "message" && e.Role == "assistant":
					action = ""
				}
			}
		}
	}
	if env == nil {
		return nil
	}
	info := &Info{Action: action, Cwd: cwd}
	if env.Session.ContextUsage != nil {
		info.Context = env.Session.ContextUsage.TotalTokens
	}
	for _, id := range slices.Backward(order) { // the newest question is the one on screen
		cb, ok := open[id]
		if !ok {
			continue
		}
		info.Status, info.Since, info.Pending = "waiting", seconds(cb.CreatedAt), true
		info.Action = Describe("ask_user_question", nil)
		if cb.Detail.Effect != nil {
			info.Action = Describe(cb.Detail.Effect.ToolName, cb.Detail.Effect.Input)
		}
		return info
	}
	switch turn := env.LatestTurn; {
	case env.Session.Status.Type == "running":
		info.Status = "working"
		if turn != nil {
			info.Since = seconds(turn.StartedAt)
		}
		if info.Action == "" {
			info.Action = "thinking"
		}
	case turn != nil:
		info.Status, info.Action = "done", ""
		info.Since = seconds(firstSet(turn.CompletedAt, env.Session.UpdatedAt))
	default:
		info.Action = "" // started, no prompt yet
	}
	return info
}

func seconds(ms int64) float64 { return float64(ms) / 1000 }

// firstSet is the first non-zero value.
func firstSet(values ...int64) int64 {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}
