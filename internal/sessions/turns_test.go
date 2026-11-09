package sessions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const claudeLog = `{"type":"user","message":{"role":"user","content":"fix the build"}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"Looking at it."},{"type":"tool_use","name":"Bash"}]}}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"ok"}]}}
{"type":"user","message":{"role":"user","content":"<system-reminder>ignore</system-reminder>"}}
{"type":"attachment"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done, tests pass."}]}}
`

const codexLog = `{"type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"rules"}]}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>cwd</environment_context>"}]}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"add a flag"}]}}
{"type":"response_item","payload":{"type":"reasoning"}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Added --flag."}]}}
`

const cursorLog = `{"role":"user","message":{"content":[{"type":"text","text":"<user_query>\nrename it\n</user_query>"}]}}
{"role":"assistant","message":{"content":[{"type":"tool_use","name":"Shell"}]}}
{"role":"assistant","message":{"content":[{"type":"text","text":"Renamed."}]}}
`

func TestLastTurnsReadsEachAgentsTranscript(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".claude/projects/-p/aaa-111.jsonl"), claudeLog)
	writeFile(t, filepath.Join(home, ".codex/sessions/2026/11/07/rollout-2026-11-07T10-00-00-bbb-222.jsonl"), codexLog)
	writeFile(t, filepath.Join(home, ".cursor/projects/p/agent-transcripts/ccc-333/ccc-333.jsonl"), cursorLog)

	tests := []struct {
		agent, id string
		want      []string
	}{
		{"claude", "aaa-111", []string{"user:fix the build", "assistant:Looking at it.", "assistant:Done, tests pass."}},
		{"codex", "bbb-222", []string{"user:add a flag", "assistant:Added --flag."}},
		{"cursor", "ccc-333", []string{"user:rename it", "assistant:Renamed."}},
	}
	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			tr, err := Locate(home, tt.agent, tt.id)
			if err != nil || tr == nil {
				t.Fatalf("Locate = %v, %v", tr, err)
			}
			turns, err := LastTurns(tr.Path, 10)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, turn := range turns {
				got = append(got, turn.Role+":"+turn.Text)
			}
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("turns = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLastTurnsKeepsTheLastN(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	writeFile(t, path, claudeLog)
	turns, err := LastTurns(path, 2)
	if err != nil || len(turns) != 2 || turns[1].Text != "Done, tests pass." {
		t.Errorf("turns = %v, err = %v", turns, err)
	}
}

func TestTailDropsTheCutFirstLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	writeFile(t, path, "0123456789\nsecond\nthird\n")
	got, err := tail(path, 12)
	if err != nil || string(got) != "third\n" {
		t.Errorf("tail = %q, %v; the first line was cut and must go", got, err)
	}
	if got, _ := tail(path, 1000); !strings.HasPrefix(string(got), "0123456789") {
		t.Errorf("a file smaller than the window is read whole, got %q", got)
	}
}

func TestLocateRejectsOddIDsAndUnknownTranscripts(t *testing.T) {
	home := t.TempDir()
	for _, id := range []string{"../etc/passwd", "a*b", "", "a/b"} {
		if _, err := Locate(home, "claude", id); err == nil {
			t.Errorf("id %q should be refused", id)
		}
	}
	if tr, err := Locate(home, "claude", "missing-1"); tr != nil || err != nil {
		t.Errorf("a missing transcript is nil, nil; got %v, %v", tr, err)
	}
	if tr, err := Locate(home, "agy", "x-1"); tr != nil || err != nil {
		t.Errorf("an agent with no known transcript is nil, nil; got %v, %v", tr, err)
	}
}

func TestPreview(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".claude/projects/-p/aaa-111.jsonl"), claudeLog)
	now := time.Now()

	text, err := Preview(home, "claude", "aaa-111", "/work/app", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"claude", "aaa-111", "/work/app", "last active just now", "fix the build", "Done, tests pass."} {
		if !strings.Contains(text, want) {
			t.Errorf("preview lacks %q:\n%s", want, text)
		}
	}
	none, err := Preview(home, "cursor", "zzz-9", "/work/app", now)
	if err != nil || !strings.Contains(none, "No readable transcript") {
		t.Errorf("preview without a transcript = %q, %v", none, err)
	}
}

func TestUserText(t *testing.T) {
	tests := map[string]string{
		"plain":                            "plain",
		"<user_query>\nask\n</user_query>": "ask",
		"<timestamp>Sat</timestamp>\n<user_query>\nask\n</user_query>": "ask",
		"<user_query>unterminated":                                     "unterminated",
		"<environment_context>x</environment_context>":                 "<environment_context>x</environment_context>",
	}
	for in, want := range tests {
		if got := userText(in); got != want {
			t.Errorf("userText(%q) = %q, want %q", in, got, want)
		}
	}
}
