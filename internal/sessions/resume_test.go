package sessions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// programsOnPath puts an executable file for each name first (and only) on PATH and returns their directory.
func programsOnPath(t *testing.T, names ...string) string {
	t.Helper()
	bin := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(bin, n), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	return bin
}

func TestResume(t *testing.T) {
	bin := programsOnPath(t, "claude", "codex", "cursor-agent")
	home := fixtureHome(t)
	tests := []struct {
		agent, want string
	}{
		{"claude", "/w/x\t" + bin + "/claude --resume sid-1"},
		{"codex", "/w/x\tenv CODEX_SQLITE_HOME=" + home + "/.codex " + bin + "/codex resume sid-1"},
		{"cursor", "/w/x\t" + bin + "/cursor-agent --resume sid-1"},
	}
	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			plan, err := Resume(home, tt.agent, "sid-1", "/w/x")
			if err != nil {
				t.Fatal(err)
			}
			if got := plan.Command("/fallback"); got != tt.want {
				t.Errorf("Command = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResumeCodexKeepsTheConfiguredSQLiteHome(t *testing.T) {
	programsOnPath(t, "codex")
	tests := []struct {
		name, config string
		wantEnv      bool
	}{
		{"no config", "", true},
		{"config without sqlite_home", "model = \"x\"\n[projects.\"/w\"]\ntrust_level = \"trusted\"\n", true},
		{"sqlite_home set", "model = \"x\"\nsqlite_home = \"/data/codex\"\n", false},
		{"sqlite_home with spaces", "  sqlite_home=\"/data/codex\"\n", false},
		{"a mere mention in a comment", "# sqlite_home = \"/old\"\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := fixtureHome(t)
			if tt.config != "" {
				writeFile(t, filepath.Join(home, ".codex", "config.toml"), tt.config)
			}
			plan, err := Resume(home, "codex", "sid-1", "/w")
			if err != nil {
				t.Fatal(err)
			}
			if got := len(plan.Env) > 0; got != tt.wantEnv {
				t.Errorf("env = %v, want CODEX_SQLITE_HOME set: %v", plan.Env, tt.wantEnv)
			}
		})
	}
}

func TestResumeErrors(t *testing.T) {
	home := fixtureHome(t)
	programsOnPath(t) // an empty PATH: no agent is installed
	tests := []struct {
		name, agent, want string
	}{
		{"missing claude", "claude", "cannot resume claude session sid-2"},
		{"missing codex", "codex", "cannot resume codex session sid-2"},
		{"missing cursor-agent", "cursor", "cannot resume cursor session sid-2"},
		{"an agent muxrail does not know", "gemini", "unknown agent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := Resume(home, tt.agent, "sid-2", "/w")
			if err == nil || plan != nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("plan=%v err=%v, want an error containing %q", plan, err, tt.want)
			}
		})
	}
}
