package activity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Records in the shape Vibe 3.1 journals them, cut down to the fields muxrail reads.
const (
	vibeRunning  = `{"type":"projection_delta","payload":{"delta":[{"op":"set_envelope","state":{"activeCallbacks":[],"latestTurn":{"startedAt":1794460595482,"status":"in_progress"},"session":{"status":{"type":"running"},"contextUsage":{"totalTokens":8354}}}}]}}`
	vibePrompt   = `{"type":"projection_delta","payload":{"delta":[{"op":"append_entry","entry":{"type":"message","role":"user"}}]}}`
	vibeBash     = `{"type":"projection_delta","payload":{"delta":[{"op":"append_entry","entry":{"type":"effect","detail":{"kind":"tool","toolName":"file_system.bash","input":{"command":"go test ./...\nmore"}},"state":{"status":"running"}}}]}}`
	vibeBashDone = `{"type":"projection_delta","payload":{"delta":[{"op":"replace_entry","entry":{"type":"effect","detail":{"kind":"tool","toolName":"file_system.bash"},"state":{"status":"completed"}}}]}}`
	vibeAsk      = `{"type":"callback_registered","payload":{"callback_id":"approval-1","kind":"approval","routing":{"callbackId":"approval-1","createdAt":1794460616564,"detail":{"kind":"approval","effect":{"kind":"tool","toolName":"file_system.write_file","input":{"path":"/r/b.txt"}}}}}}`
	vibeAnswered = `{"type":"callback_resolved","payload":{"callback_id":"approval-1","state":"resolved"}}`
	vibeReply    = `{"type":"projection_delta","payload":{"delta":[{"op":"append_entry","entry":{"type":"message","role":"assistant"}}]}}`
	vibeIdle     = `{"type":"projection_delta","payload":{"delta":[{"op":"set_envelope","state":{"activeCallbacks":[],"latestTurn":{"startedAt":1794460595482,"completedAt":1794460620000,"status":"completed"},"session":{"status":{"type":"idle"},"contextUsage":{"totalTokens":8673}}}}]}}`
	vibeQuiet    = `{"snapshot":{"activeCallbacks":[],"latestTurn":{"startedAt":1794684391808,"completedAt":1794684399515,"status":"completed"},"session":{"status":{"type":"idle"},"contextUsage":{"totalTokens":162216}}}}`
)

func TestParseVibe(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		status  string
		action  string
		pending bool
		context int
		since   float64
	}{
		{"prompt sent", lines(vibePrompt, vibeRunning), "working", "thinking", false, 8354, 1794460595.482},
		{"tool running", lines(vibePrompt, vibeRunning, vibeBash), "working", "$ go test ./...", false, 8354, 1794460595.482},
		{"tool ran", lines(vibeRunning, vibeBash, vibeBashDone), "working", "thinking", false, 8354, 1794460595.482},
		{"approval open", lines(vibeRunning, vibeBash, vibeAsk), "waiting", "✎ Write b.txt", true, 8354, 1794460616.564},
		{"approval answered", lines(vibeRunning, vibeAsk, vibeAnswered), "working", "thinking", false, 8354, 1794460595.482},
		{"turn done", lines(vibeRunning, vibeBash, vibeReply, vibeIdle), "done", "", false, 8673, 1794460620},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseVibe(nil, tt.data)
			if got == nil {
				t.Fatal("parseVibe = nil")
			}
			if got.Status != tt.status || got.Action != tt.action || got.Pending != tt.pending || got.Context != tt.context || got.Since != tt.since {
				t.Errorf("parseVibe = %+v, want status %q action %q pending %v context %d since %v", got, tt.status, tt.action, tt.pending, tt.context, tt.since)
			}
		})
	}
}

func TestParseVibeStartsFromTheSnapshot(t *testing.T) {
	var snap struct {
		Snapshot *vibeEnvelope `json:"snapshot"`
	}
	mustUnmarshal(t, vibeQuiet, &snap)
	if got := parseVibe(snap.Snapshot, nil); got == nil || got.Status != "done" || got.Context != 162216 {
		t.Errorf("parseVibe(snapshot, empty journal) = %+v, want done with the snapshot's context", got)
	}
	if got := parseVibe(nil, lines(vibePrompt)); got != nil {
		t.Errorf("parseVibe(no snapshot, no envelope) = %+v, want nil", got)
	}
	started := `{"activeCallbacks":[],"latestTurn":null,"session":{"status":{"type":"idle"}}}`
	var env vibeEnvelope
	mustUnmarshal(t, started, &env)
	if got := parseVibe(&env, nil); got == nil || got.Status != "" || got.Action != "" {
		t.Errorf("parseVibe(new session) = %+v, want no status before the first prompt", got)
	}
}

// vibeStore writes a session store under home's Vibe folder: a lock held by pid, a snapshot, a journal and the index.
func vibeStore(t *testing.T, home, id string, pid string, journal []byte) {
	t.Helper()
	sessions := filepath.Join(home, ".vibe", "logs", "session")
	dir := filepath.Join(sessions, "unified", id)
	files := map[string]string{
		filepath.Join(sessions, "active", id+".lock.json"):                             `{"process_id":` + pid + `,"session_id":"` + id + `"}`,
		filepath.Join(dir, "CURRENT"):                                                  `{"generation":"0000000000000007"}`,
		filepath.Join(dir, "generations", "0000000000000007", "projection-state.json"): vibeQuiet,
		filepath.Join(dir, "journal", "0000000000000003.jsonl"):                        string(lines(vibeRunning, vibeAsk)), // older than the snapshot
		filepath.Join(dir, "journal", "0000000000000008.jsonl"):                        string(journal),
		filepath.Join(sessions, "unified", ".session-index.json"): `{"entries":[{"identity":{"session_id":"` + id +
			`"},"metadata":{"completion_catalogue":"{\"context_window\": 1048576}"}}]}`,
	}
	for path, body := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadVibeReadsTheNewestJournalOverTheSnapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("VIBE_HOME", "")
	vibeStore(t, home, "s1", "4242", lines(vibePrompt, vibeRunning, vibeBash))
	info, err := ReadVibe(home, "s1")
	if err != nil || info == nil || info.Status != "working" || info.Action != "$ go test ./..." || info.Window != 1048576 {
		t.Errorf("ReadVibe = %+v, %v; want the bash call in flight with the 1M window", info, err)
	}

	quiet := t.TempDir()
	vibeStore(t, quiet, "s2", "4243", nil) // just snapshotted: the new journal is still empty
	if info, err := ReadVibe(quiet, "s2"); err != nil || info == nil || info.Status != "done" || info.Context != 162216 {
		t.Errorf("ReadVibe after a snapshot = %+v, %v; want the snapshot's state", info, err)
	}
	for _, id := range []string{"missing", "../s1", ""} {
		if info, err := ReadVibe(home, id); info != nil || err != nil {
			t.Errorf("ReadVibe(%q) = %+v, %v; want nil, nil", id, info, err)
		}
	}
}

func TestVibeSessionsMapsTheLockHolderToItsSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("VIBE_HOME", "")
	vibeStore(t, home, "s1", "4242", nil)
	bad := filepath.Join(home, ".vibe", "logs", "session", "active", "x.lock.json")
	if err := os.WriteFile(bad, []byte(`{"process_id":1,"session_id":"../escape"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := VibeSessions(home); len(got) != 1 || got[4242] != "s1" {
		t.Errorf("VibeSessions = %v, want only 4242 -> s1", got)
	}

	moved := t.TempDir()
	t.Setenv("VIBE_HOME", filepath.Join(moved, ".vibe"))
	vibeStore(t, moved, "s9", "77", nil)
	if got := VibeSessions(home); got[77] != "s9" {
		t.Errorf("VibeSessions = %v, want $VIBE_HOME to win over ~/.vibe", got)
	}
}

func mustUnmarshal(t *testing.T, data string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(data), v); err != nil {
		t.Fatal(err)
	}
}
