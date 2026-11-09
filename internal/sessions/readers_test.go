package sessions

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// fixtureHome is an empty home directory; HOME points at it so nothing a test runs can reach the real one.
func fixtureHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func touch(t *testing.T, path string, at time.Time) {
	t.Helper()
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

// byID indexes sessions and fails on a duplicate id.
func byID(t *testing.T, list []*Session) map[string]*Session {
	t.Helper()
	m := map[string]*Session{}
	for _, s := range list {
		if m[s.ID] != nil {
			t.Fatalf("session %s listed twice", s.ID)
		}
		m[s.ID] = s
	}
	return m
}

func sortedIDs(list []*Session) []string {
	ids := make([]string, len(list))
	for i, s := range list {
		ids[i] = s.ID
	}
	sort.Strings(ids)
	return ids
}

func wantIDs(t *testing.T, list []*Session, want ...string) {
	t.Helper()
	sort.Strings(want)
	if got := sortedIDs(list); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("sessions = %v, want %v", got, want)
	}
}

func jsonl(lines ...string) string { return strings.Join(lines, "\n") + "\n" }

func TestClaudeSessions(t *testing.T) {
	home := fixtureHome(t)
	projects := filepath.Join(home, ".claude", "projects")
	when := time.Unix(1_700_000_000, 0)

	renamed := filepath.Join(projects, "-w-one", "renamed.jsonl")
	writeFile(t, renamed, jsonl(`{"type":"user","cwd":"/w/one","message":{"role":"user","content":"hello"}}`,
		`{"type":"ai-title","aiTitle":"Generated"}`, `{"type":"custom-title","customTitle":"By hand"}`))
	touch(t, renamed, when)
	writeFile(t, filepath.Join(projects, "-w-one", "history-only.jsonl"), jsonl(`{"cwd":"/w/two"}`))
	writeFile(t, filepath.Join(projects, "-w-one", "from-transcript.jsonl"), jsonl(
		`{"cwd":"/w/three"}`,
		`{"type":"user","message":{"role":"user","content":"<command-name>/model</command-name>"}}`,
		`{"type":"user","message":{"role":"user","content":"first real prompt"}}`,
		`{"type":"ai-title","aiTitle":"Titled without history"}`))
	writeFile(t, filepath.Join(projects, "-w-one", "headless.jsonl"), jsonl(`{"cwd":"/w/four"}`, `{"type":"user","message":{"role":"user","content":"claude -p run"}}`))
	writeFile(t, filepath.Join(projects, "-r-x-worktrees", "delegated.jsonl"), jsonl(`{"cwd":"/r/x-worktrees/j-20261107-092018-f841"}`, `{"type":"ai-title","aiTitle":"Headless job"}`))
	writeFile(t, filepath.Join(projects, "-cfg", "scratch.jsonl"), jsonl(`{"cwd":"/h/.config/ki/jobs/j-20261107-063922-75ef/scratch"}`, `{"type":"ai-title","aiTitle":"Scratch job"}`))
	writeFile(t, filepath.Join(projects, "-w-one", "no-cwd.jsonl"), jsonl(`{"type":"ai-title","aiTitle":"No folder"}`))
	writeFile(t, filepath.Join(projects, "-w-one", "renamed", "subagents", "agent-a1.jsonl"), jsonl(`{"cwd":"/w/one"}`, `{"type":"ai-title","aiTitle":"Subagent"}`))
	writeFile(t, filepath.Join(projects, "-w-one", "notes.txt"), "not a transcript")

	var history []string
	for i := 1; i <= 12; i++ {
		history = append(history, `{"display":"prompt `+string(rune('a'+i-1))+`","sessionId":"history-only","timestamp":`+string(rune('0'+i%10))+`}`)
	}
	history = append(history, `{"display":"  ","sessionId":"history-only"}`, `not json`, `{"display":"no session"}`,
		`{"display":"for renamed","sessionId":"renamed"}`, `{"display":"for a session that has no transcript","sessionId":"ghost"}`)
	writeFile(t, filepath.Join(home, ".claude", "history.jsonl"), jsonl(history...))

	list, err := claudeSessions(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	wantIDs(t, list, "renamed", "history-only", "from-transcript")
	got := byID(t, list)

	tests := []struct {
		name string
		id   string
		cwd  string
		want func(*Session) bool
	}{
		{"rename wins and prompts come from the history", "renamed", "/w/one", func(s *Session) bool {
			return s.Title == "By hand" && s.Renamed && s.TS == float64(when.Unix()) && strings.Join(s.Prompts, "|") == "for renamed"
		}},
		{"history prompts are the latest ten, newest first", "history-only", "/w/two", func(s *Session) bool {
			return s.Title == "" && !s.Renamed && len(s.Prompts) == 10 && s.Prompts[0] == "prompt l" && s.Prompts[9] == "prompt c"
		}},
		{"a titled session the history lacks takes its first typed prompts", "from-transcript", "/w/three", func(s *Session) bool {
			return s.Title == "Titled without history" && !s.Renamed && strings.Join(s.Prompts, "|") == "first real prompt"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := got[tt.id]
			if s.Agent != "claude" || s.CWD != tt.cwd || !tt.want(s) {
				t.Errorf("session = %+v", s)
			}
		})
	}
}

func TestClaudeSessionsSurviveAnUnreadableTranscript(t *testing.T) {
	home := fixtureHome(t)
	projects := filepath.Join(home, ".claude", "projects", "-w")
	writeFile(t, filepath.Join(projects, "good.jsonl"), jsonl(`{"cwd":"/w"}`, `{"type":"ai-title","aiTitle":"Fine"}`))
	if err := os.MkdirAll(filepath.Join(projects, "broken.jsonl"), 0o755); err != nil { // reading a directory fails
		t.Fatal(err)
	}
	list, err := claudeSessions(context.Background(), home)
	if err == nil {
		t.Error("the unreadable transcript was not reported")
	}
	wantIDs(t, list, "good")
}

func TestClaudeSessionsWithoutAnyClaude(t *testing.T) {
	list, err := claudeSessions(context.Background(), fixtureHome(t))
	if err != nil || len(list) != 0 {
		t.Errorf("list=%v err=%v", list, err)
	}
}

func codexFile(home, day, name string) string {
	return filepath.Join(home, ".codex", "sessions", "2026", "11", day, "rollout-2026-11-"+day+"T10-00-00-"+name+".jsonl")
}

func codexMetaLine(id, cwd, source string) string {
	return `{"type":"session_meta","payload":{"id":"` + id + `","cwd":"` + cwd + `","timestamp":"2026-11-07T10:00:00Z","source":` + source + `}}`
}

func TestCodexSessions(t *testing.T) {
	home := fixtureHome(t)
	user := func(text string) string {
		return `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + text + `"}]}}`
	}
	when := time.Unix(1_700_000_500, 0)
	cli := codexFile(home, "01", "cli-1")
	writeFile(t, cli, jsonl(codexMetaLine("cli-1", "/w/cli", `"cli"`),
		`{"type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"rules"}]}}`,
		user(`# AGENTS.md instructions\n\n<INSTRUCTIONS>x</INSTRUCTIONS>`), user(`<environment_context>cwd</environment_context>`),
		user("add a flag"), `{"type":"event_msg","payload":{"type":"task_complete"}}`, user("and a test")))
	touch(t, cli, when)
	writeFile(t, codexFile(home, "02", "vs-1"), jsonl(codexMetaLine("vs-1", "/w/vscode", `"vscode"`), user("from the extension")))
	writeFile(t, codexFile(home, "03", "no-source-1"), jsonl(codexMetaLine("no-source-1", "/w/old", `null`)))
	writeFile(t, codexFile(home, "04", "exec-1"), jsonl(codexMetaLine("exec-1", "/w/exec", `"exec"`), user("scripted")))
	writeFile(t, codexFile(home, "05", "sub-1"), jsonl(codexMetaLine("sub-1", "/w/sub", `{"subagent":{"thread_spawn":{"parent_thread_id":"cli-1"}}}`)))
	writeFile(t, codexFile(home, "06", "guardian-1"), jsonl(codexMetaLine("guardian-1", "/w/g", `{"subagent":{"other":"guardian"}}`)))
	writeFile(t, codexFile(home, "07", "no-cwd-1"), jsonl(codexMetaLine("no-cwd-1", "", `"cli"`)))
	writeFile(t, filepath.Join(home, ".codex", "sessions", "2026", "11", "01", "notes.jsonl"), "not a rollout")
	writeFile(t, filepath.Join(home, ".codex", "history.jsonl"), jsonl(`{"session_id":"cli-1","text":"ignored"}`))

	list, err := codexSessions(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	wantIDs(t, list, "cli-1", "vs-1", "no-source-1")
	got := byID(t, list)
	if s := got["cli-1"]; s.Agent != "codex" || s.CWD != "/w/cli" || s.TS != float64(when.Unix()) || strings.Join(s.Prompts, "|") != "add a flag|and a test" || s.Title != "" {
		t.Errorf("cli session = %+v", s)
	}
	if s := got["vs-1"]; s.CWD != "/w/vscode" || strings.Join(s.Prompts, "|") != "from the extension" {
		t.Errorf("vscode session = %+v", s)
	}
}

func TestCodexSessionsSurviveOddFiles(t *testing.T) {
	home := fixtureHome(t)
	writeFile(t, codexFile(home, "01", "good-1"), jsonl(codexMetaLine("good-1", "/w", `"cli"`)))
	writeFile(t, codexFile(home, "02", "garbage-1"), "this is not json\n")
	writeFile(t, codexFile(home, "03", "wrong-first-1"), jsonl(`{"type":"event_msg","payload":{}}`))
	writeFile(t, codexFile(home, "04", "empty-1"), "")
	list, err := codexSessions(context.Background(), home)
	if err == nil {
		t.Error("the odd files were not reported")
	}
	wantIDs(t, list, "good-1")
}

func cursorUser(text string) string {
	return `{"role":"user","message":{"content":[{"type":"text","text":"<timestamp>Sat</timestamp>\n<user_query>\n` + text + `\n</user_query>"}]}}`
}

func cursorTranscript(home, slug, id string) string {
	return filepath.Join(home, ".cursor", "projects", slug, "agent-transcripts", id, id+".jsonl")
}

func cursorChat(t *testing.T, home, id, cwd string) {
	t.Helper()
	writeFile(t, filepath.Join(home, ".cursor", "chats", "abc123", id, "meta.json"), `{"schemaVersion":1,"cwd":"`+cwd+`"}`)
	writeFile(t, filepath.Join(home, ".cursor", "chats", "abc123", id, "store.db"), "sqlite is never read")
}

func trustWorkspace(t *testing.T, home, slug, folder string) {
	t.Helper()
	writeFile(t, filepath.Join(home, ".cursor", "projects", slug, ".workspace-trusted"), `{"trustedAt":"2026-10-25T09:54:50.858Z","workspacePath":"`+folder+`"}`)
}

func TestCursorSessions(t *testing.T) {
	home := fixtureHome(t)
	when := time.Unix(1_700_000_700, 0)

	trusted := cursorTranscript(home, "w-trusted", "trusted-1")
	writeFile(t, trusted, jsonl(cursorUser("rename it"), `{"role":"assistant","message":{"content":[{"type":"text","text":"done"}]}}`, cursorUser("and again")))
	touch(t, trusted, when)
	trustWorkspace(t, home, "w-trusted", "/w/trusted")
	cursorChat(t, home, "trusted-1", "/elsewhere")

	writeFile(t, cursorTranscript(home, "w-meta-only", "meta-1"), jsonl(cursorUser("folder from the chat")))
	cursorChat(t, home, "meta-1", "/w/meta")

	writeFile(t, cursorTranscript(home, "w-ide", "ide-1"), jsonl(cursorUser("typed in the IDE")))
	trustWorkspace(t, home, "w-ide", "/w/ide")

	writeFile(t, cursorTranscript(home, "w-lost", "lost-1"), jsonl(cursorUser("no folder anywhere")))
	cursorChat(t, home, "lost-1", "")

	writeFile(t, cursorTranscript(home, "w-deleg", "deleg-1"), jsonl(cursorUser("job")))
	trustWorkspace(t, home, "w-deleg", "/r/x-worktrees/j-20261107-092018-f841")
	cursorChat(t, home, "deleg-1", "/r/x-worktrees/j-20261107-092018-f841")

	legacy := filepath.Join(home, ".cursor", "projects", "w-trusted", "agent-transcripts", "legacy-1", "legacy-1.txt")
	writeFile(t, legacy, "user:\nold text transcript")
	cursorChat(t, home, "legacy-1", "/w/legacy")
	sub := filepath.Join(home, ".cursor", "projects", "w-trusted", "agent-transcripts", "trusted-1", "subagents", "other-id.jsonl")
	writeFile(t, sub, jsonl(cursorUser("subagent")))
	cursorChat(t, home, "other-id", "/w/sub")

	list, err := cursorSessions(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	wantIDs(t, list, "trusted-1", "meta-1")
	got := byID(t, list)
	if s := got["trusted-1"]; s.Agent != "cursor" || s.CWD != "/w/trusted" || s.TS != float64(when.Unix()) || strings.Join(s.Prompts, "|") != "rename it|and again" {
		t.Errorf("trusted session = %+v", s)
	}
	if s := got["meta-1"]; s.CWD != "/w/meta" || strings.Join(s.Prompts, "|") != "folder from the chat" {
		t.Errorf("session whose folder comes from the chat = %+v", s)
	}
}

func TestCursorSessionsSurviveABrokenWorkspaceFile(t *testing.T) {
	home := fixtureHome(t)
	writeFile(t, cursorTranscript(home, "w-good", "good-1"), jsonl(cursorUser("fine")))
	trustWorkspace(t, home, "w-good", "/w/good")
	cursorChat(t, home, "good-1", "/w/good")
	writeFile(t, cursorTranscript(home, "w-bad", "bad-1"), jsonl(cursorUser("unlucky")))
	writeFile(t, filepath.Join(home, ".cursor", "projects", "w-bad", ".workspace-trusted"), "{not json")
	cursorChat(t, home, "bad-1", "/w/bad")
	writeFile(t, filepath.Join(home, ".cursor", "chats", "abc123", "broken-chat", "meta.json"), "{not json")

	list, err := cursorSessions(context.Background(), home)
	if err == nil {
		t.Error("the broken files were not reported")
	}
	wantIDs(t, list, "good-1")
}

func TestListMergesEveryAgentNewestFirst(t *testing.T) {
	home := fixtureHome(t)
	cwdC, cwdX, cwdU := folder(t, home, "w/c"), folder(t, home, "w/x"), folder(t, home, "w/u")
	cl := filepath.Join(home, ".claude", "projects", "-w", "c-1.jsonl")
	writeFile(t, cl, jsonl(`{"cwd":"`+cwdC+`"}`, `{"type":"ai-title","aiTitle":"Claude one"}`))
	touch(t, cl, time.Unix(1_700_000_100, 0))
	cx := codexFile(home, "01", "x-1")
	writeFile(t, cx, jsonl(codexMetaLine("x-1", cwdX, `"cli"`)))
	touch(t, cx, time.Unix(1_700_000_300, 0))
	cu := cursorTranscript(home, "w-u", "u-1")
	writeFile(t, cu, jsonl(cursorUser("cursor one")))
	trustWorkspace(t, home, "w-u", cwdU)
	cursorChat(t, home, "u-1", cwdU)
	touch(t, cu, time.Unix(1_700_000_200, 0))
	writeFile(t, filepath.Join(home, ".claude", "projects", "-w", "bad.jsonl", "x"), "a directory named like a transcript")

	list, err := List(context.Background(), home)
	if err == nil {
		t.Error("the unreadable transcript was not reported beside the list")
	}
	var order []string
	for _, s := range list {
		order = append(order, s.Agent+":"+s.ID)
	}
	if want := "codex:x-1 cursor:u-1 claude:c-1"; strings.Join(order, " ") != want {
		t.Errorf("order = %v, want %s", order, want)
	}
}

func TestListWithNoAgentsInstalled(t *testing.T) {
	list, err := List(context.Background(), fixtureHome(t))
	if err != nil || len(list) != 0 {
		t.Errorf("list=%v err=%v", list, err)
	}
}

func TestListStopsWhenCancelled(t *testing.T) {
	home := fixtureHome(t)
	writeFile(t, filepath.Join(home, ".claude", "projects", "-w", "c-1.jsonl"), jsonl(`{"cwd":"/w"}`, `{"type":"ai-title","aiTitle":"x"}`))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	list, err := List(ctx, home)
	if err == nil || len(list) != 0 {
		t.Errorf("list=%v err=%v", list, err)
	}
}

// folder is a real directory under dir: sessions whose folder is gone are not listed.
func folder(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
