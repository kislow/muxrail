package activity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func lines(rows ...string) []byte { return []byte(strings.Join(rows, "\n") + "\n") }

const (
	prompt   = `{"type":"user","message":{"role":"user","content":"fix it"}}`
	thinking = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"hm"}],"usage":{"input_tokens":2,"cache_creation_input_tokens":1000,"cache_read_input_tokens":500}}}`
	edit     = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/a/b/rail.go"}}],"usage":{"input_tokens":3,"cache_creation_input_tokens":10,"cache_read_input_tokens":86000}}}`
	result   = `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"ok"}]}}`
	reply    = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":1,"cache_read_input_tokens":90000}}}`
	sub      = `{"type":"assistant","isSidechain":true,"message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}`
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want *Info
	}{
		{"empty", nil, nil},
		{"only housekeeping records", lines(`{"type":"ai-title"}`, `{"type":"file-history-snapshot"}`), nil},
		{"a prompt hands the turn to the model", lines(prompt), &Info{Action: "thinking"}},
		{"thinking", lines(prompt, thinking), &Info{Action: "thinking", Context: 1502}},
		{"a tool call in flight", lines(prompt, thinking, edit), &Info{Action: "✎ Edit rail.go", Pending: true, Context: 86013}},
		{"the result hands the turn back", lines(prompt, edit, result), &Info{Action: "thinking", Context: 86013}},
		{"a finished reply has no action", lines(prompt, edit, result, reply), &Info{Context: 90001}},
		{"subagent records are skipped", lines(prompt, reply, sub), &Info{Context: 90001}},
		{"damaged lines are skipped", lines(prompt, `{not json`, edit), &Info{Action: "✎ Edit rail.go", Pending: true, Context: 86013}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parse(tt.data)
			switch {
			case tt.want == nil && got != nil:
				t.Errorf("parse = %+v, want nil", got)
			case tt.want != nil && (got == nil || *got != *tt.want):
				t.Errorf("parse = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDescribe(t *testing.T) {
	tests := []struct {
		tool  string
		input map[string]any
		want  string
	}{
		{"Edit", map[string]any{"file_path": "/x/y/z.go"}, "✎ Edit z.go"},
		{"Write", map[string]any{"file_path": "/x/README.md"}, "✎ Write README.md"},
		{"Read", map[string]any{"file_path": "/x/a.txt"}, "◇ Read a.txt"},
		{"Bash", map[string]any{"command": "go test ./...\nsecond"}, "$ go test ./..."},
		{"Bash", map[string]any{"command": "ls", "description": "List files"}, "$ List files"},
		{"Grep", map[string]any{"pattern": "TODO"}, "⌕ Grep TODO"},
		{"WebSearch", map[string]any{"query": "tmux status"}, "⌕ WebSearch tmux status"},
		{"Agent", map[string]any{"description": "review the diff"}, "◈ Agent review the diff"},
		{"mcp__newrelic__execute_nrql_query", nil, "⚙ execute_nrql_query"},
		{"TodoWrite", nil, "⚙ TodoWrite"},
	}
	for _, tt := range tests {
		t.Run(tt.tool+" "+tt.want, func(t *testing.T) {
			if got := Describe(tt.tool, tt.input); got != tt.want {
				t.Errorf("Describe = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSessionIDMatchesThePaneAndTakesTheNewest(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("1.json", `{"sessionId":"old","tmux":"main:@1.%3","startedAt":100}`)
	write("2.json", `{"sessionId":"new","tmux":"main:@1.%3","startedAt":200}`)
	write("3.json", `{"sessionId":"other-pane","tmux":"main:@1.%30","startedAt":900}`)
	write("4.json", `{"sessionId":"other-window","tmux":"main:@11.%3","startedAt":900}`)
	write("5.json", `not json`)
	if got := SessionID(home, "@1", "%3"); got != "new" {
		t.Errorf("SessionID = %q, want new (%%30 and @11 must not match %%3 and @1)", got)
	}
	if got := SessionID(home, "@9", "%9"); got != "" {
		t.Errorf("SessionID for an unknown pane = %q, want empty", got)
	}
}

func TestReadFindsTheTranscriptAndBoundsTheTail(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "projects", "-some-dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	filler := `{"type":"user","message":{"role":"user","content":"` + strings.Repeat("x", 1000) + `"}}` + "\n"
	body := strings.Repeat(filler, 400) + string(lines(prompt, edit)) // about 400 KB: the head is never read
	if err := os.WriteFile(filepath.Join(dir, "abc.jsonl"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := Read(home, "abc")
	if err != nil || info == nil || info.Action != "✎ Edit rail.go" || !info.Pending {
		t.Errorf("Read = %+v, %v; want the Edit in flight", info, err)
	}
	if info, err := Read(home, "missing"); info != nil || err != nil {
		t.Errorf("Read of an unknown session = %+v, %v; want nil, nil", info, err)
	}
}

const (
	codexMeta       = `{"type":"session_meta","payload":{"id":"x","cwd":"/work/app","source":"vscode"}}`
	codexPrompt     = `{"type":"response_item","payload":{"type":"message","role":"user"}}`
	codexReason     = `{"type":"response_item","payload":{"type":"reasoning"}}`
	codexExec       = `{"type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"go test ./...\\nmore\"}"}}`
	codexPatch      = `{"type":"response_item","payload":{"type":"custom_tool_call","name":"apply_patch","input":"*** Begin Patch\n*** Update File: /a/b/rail.go\n@@"}}`
	codexOut        = `{"type":"response_item","payload":{"type":"function_call_output"}}`
	codexTokenCount = `{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"total_tokens":73945},"model_context_window":258400}}}`
	codexNoInfo     = `{"type":"event_msg","payload":{"type":"token_count","info":null}}`
	codexReplied    = `{"type":"response_item","payload":{"type":"message","role":"assistant"}}`
)

func TestParseCodex(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want *Info
	}{
		{"nothing yet", lines(codexMeta), nil},
		{"a prompt", lines(codexPrompt), &Info{Action: "thinking"}},
		{"reasoning", lines(codexPrompt, codexReason), &Info{Action: "thinking"}},
		{"a command in flight", lines(codexPrompt, codexExec), &Info{Action: "$ go test ./...", Pending: true}},
		{"a patch names its file", lines(codexPatch), &Info{Action: "✎ Patch rail.go", Pending: true}},
		{"the output hands the turn back", lines(codexExec, codexOut), &Info{Action: "thinking"}},
		{"a final message ends the turn", lines(codexExec, codexOut, codexReplied), &Info{}},
		{"tokens carry the size and the window", lines(codexPrompt, codexTokenCount, codexExec), &Info{Action: "$ go test ./...", Pending: true, Context: 73945, Window: 258400}},
		{"a token_count without info keeps the last numbers", lines(codexPrompt, codexTokenCount, codexNoInfo), &Info{Action: "thinking", Context: 73945, Window: 258400}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCodex(tt.data)
			switch {
			case tt.want == nil && got != nil:
				t.Errorf("parseCodex = %+v, want nil", got)
			case tt.want != nil && (got == nil || *got != *tt.want):
				t.Errorf("parseCodex = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseCursor(t *testing.T) {
	user := `{"role":"user","message":{"content":[{"type":"text","text":"go"}]}}`
	shell := `{"role":"assistant","message":{"content":[{"type":"text","text":"ok"},{"type":"tool_use","name":"Shell","input":{"command":"rg -n foo","description":"Search"}}]}}`
	strrep := `{"role":"assistant","message":{"content":[{"type":"tool_use","name":"StrReplace","input":{"path":"/a/rail.go"}}]}}`
	text := `{"role":"assistant","message":{"content":[{"type":"text","text":"done"}]}}`
	tests := []struct {
		name string
		data []byte
		want *Info
	}{
		{"empty", nil, nil},
		{"a prompt", lines(user), &Info{Action: "thinking"}},
		{"a shell call", lines(user, shell), &Info{Action: "$ Search", Pending: true}},
		{"an edit names its file", lines(user, strrep), &Info{Action: "✎ Edit rail.go", Pending: true}},
		{"a later reply replaces the call", lines(user, shell, text), &Info{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCursor(tt.data)
			switch {
			case tt.want == nil && got != nil:
				t.Errorf("parseCursor = %+v, want nil", got)
			case tt.want != nil && (got == nil || *got != *tt.want):
				t.Errorf("parseCursor = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func put(t *testing.T, path, body string, age time.Duration) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	mod := time.Now().Add(-age)
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func TestCodexRolloutPicksTheNewestInteractiveSessionOfTheFolder(t *testing.T) {
	home, now := t.TempDir(), time.Now()
	day := func(back int) string {
		return filepath.Join(home, ".codex", "sessions", now.AddDate(0, 0, -back).Format("2006/01/02"))
	}
	put(t, filepath.Join(day(0), "rollout-a.jsonl"), codexMeta+"\n", 3*time.Hour)
	put(t, filepath.Join(day(2), "rollout-b.jsonl"), codexMeta+"\n", time.Hour)                                              // started two days ago, still written
	put(t, filepath.Join(day(0), "rollout-c.jsonl"), strings.Replace(codexMeta, "/work/app", "/other", 1)+"\n", time.Minute) // another folder
	put(t, filepath.Join(day(0), "rollout-d.jsonl"), strings.Replace(codexMeta, `"vscode"`, `"exec"`, 1)+"\n", time.Minute)  // codex exec
	put(t, filepath.Join(day(0), "rollout-e.jsonl"), strings.Replace(codexMeta, `"vscode"`, `{"subagent":"x"}`, 1)+"\n", 0)  // subagent thread
	put(t, filepath.Join(day(0), "rollout-f.jsonl"), codexMeta+"\n", 20*time.Hour)                                           // too old
	if got := CodexRollout(home, "/work/app", now); filepath.Base(got) != "rollout-b.jsonl" {
		t.Errorf("CodexRollout = %q, want rollout-b.jsonl", got)
	}
	if got := CodexRollout(home, "/nowhere", now); got != "" {
		t.Errorf("CodexRollout for an unknown folder = %q, want empty", got)
	}
}

func TestCursorTranscriptMatchesTheFolderByItsLetters(t *testing.T) {
	home, now := t.TempDir(), time.Now()
	proj := func(slug, id string) string {
		return filepath.Join(home, ".cursor", "projects", slug, "agent-transcripts", id, id+".jsonl")
	}
	put(t, proj("Users-jordan-config-ki-jobs", "old"), "{}\n", 5*time.Hour)
	put(t, proj("Users-jordan-config-ki-jobs", "new"), "{}\n", time.Hour)
	put(t, proj("Users-jordan-other", "x"), "{}\n", time.Minute)
	put(t, proj("Users-jordan-config-ki-jobs", "stale"), "{}\n", 30*time.Hour)
	got := CursorTranscript(home, "/Users/jordan/.config/ki/jobs", now)
	if filepath.Base(got) != "new.jsonl" {
		t.Errorf("CursorTranscript = %q, want new.jsonl (the dot of .config and the dashes must not matter)", got)
	}
	if got := CursorTranscript(home, "/Users/jordan/nowhere", now); got != "" {
		t.Errorf("CursorTranscript for an unknown folder = %q, want empty", got)
	}
}

func TestReadServesAnUnchangedTranscriptFromTheCache(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "projects", "-p", "cache.jsonl")
	put(t, path, string(lines(prompt, edit)), time.Hour)
	first, err := Read(home, "cache")
	if err != nil || first == nil || !first.Pending {
		t.Fatalf("first read = %+v, %v", first, err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Same size and modification time, different bytes: only a re-read would notice.
	swapped := strings.Replace(string(lines(prompt, edit)), "Edit", "Bash", 1)
	if len(swapped) != int(st.Size()) {
		t.Fatalf("test needs equal sizes: %d vs %d", len(swapped), st.Size())
	}
	if err := os.WriteFile(path, []byte(swapped), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, st.ModTime(), st.ModTime()); err != nil {
		t.Fatal(err)
	}
	if again, _ := Read(home, "cache"); again != first {
		t.Errorf("unchanged size and mtime should return the cached Info, got %+v", again)
	}
	longer := swapped + string(lines(result))
	if err := os.WriteFile(path, []byte(longer), 0o600); err != nil {
		t.Fatal(err)
	}
	if changed, _ := Read(home, "cache"); changed == first || changed.Pending {
		t.Errorf("a changed transcript must be read again, got %+v", changed)
	}
}

func TestReadRemembersWhereTheTranscriptIs(t *testing.T) {
	home := t.TempDir()
	old := filepath.Join(home, ".claude", "projects", "-old", "mv.jsonl")
	put(t, old, string(lines(prompt, edit)), time.Hour)
	if info, err := Read(home, "mv"); err != nil || info == nil {
		t.Fatalf("first read = %+v, %v", info, err)
	}
	// Removing the project folder from under the cache must not break the next read: it finds the file again.
	moved := filepath.Join(home, ".claude", "projects", "-new", "mv.jsonl")
	put(t, moved, string(lines(prompt, edit)), time.Minute)
	if err := os.RemoveAll(filepath.Dir(old)); err != nil {
		t.Fatal(err)
	}
	if info, err := Read(home, "mv"); err != nil || info == nil || !info.Pending {
		t.Errorf("read after the transcript moved = %+v, %v; want it found again", info, err)
	}
	if got, ok := transcripts.get(home + "\x00mv"); !ok || got != moved {
		t.Errorf("remembered path = %q, %v; want %q", got, ok, moved)
	}
}

func TestSessionIDReadsAChangedSessionFileAgain(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "sessions", "1.json")
	put(t, path, `{"sessionId":"first","tmux":"main:@1.%3","startedAt":1}`, time.Hour)
	if got := SessionID(home, "@1", "%3"); got != "first" {
		t.Fatalf("SessionID = %q, want first", got)
	}
	put(t, path, `{"sessionId":"second-session","tmux":"main:@1.%3","startedAt":2}`, time.Minute)
	if got := SessionID(home, "@1", "%3"); got != "second-session" {
		t.Errorf("SessionID after the file changed = %q, want second-session", got)
	}
}

func TestRolloutCwdIsReadOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-once.jsonl")
	put(t, path, codexMeta+"\n", time.Hour)
	if got := rolloutCwd(path); got != "/work/app" {
		t.Fatalf("rolloutCwd = %q, want /work/app", got)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if got := rolloutCwd(path); got != "/work/app" {
		t.Errorf("rolloutCwd after the file went = %q, want the remembered /work/app", got)
	}
}

func TestRolloutCwdDoesNotRememberAHalfWrittenLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-partial.jsonl")
	put(t, path, `{"type":"session_meta","payl`, time.Minute)
	if got := rolloutCwd(path); got != "" {
		t.Fatalf("rolloutCwd of a partial line = %q, want empty", got)
	}
	put(t, path, codexMeta+"\n", time.Minute)
	if got := rolloutCwd(path); got != "/work/app" {
		t.Errorf("rolloutCwd once the line is complete = %q, want /work/app", got)
	}
}
