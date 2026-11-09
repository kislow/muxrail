package picker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kislow/muxrail/internal/testutil"
)

func TestParsePick(t *testing.T) {
	tests := []struct {
		name, line string
		want       Pick
		wantErr    bool
	}{
		{"a row", "claude\tsid-1\t/Users/jordan/repos/acme/api\t12m  api  fix the build\n", Pick{"claude", "sid-1", "/Users/jordan/repos/acme/api"}, false},
		{"display may hold tabs", "codex\tc-2\t/w\ta\tb", Pick{"codex", "c-2", "/w"}, false},
		{"empty cwd stays empty", "cursor\tid\t\tshown", Pick{"cursor", "id", ""}, false},
		{"three fields", "claude\tsid\t/w", Pick{"claude", "sid", "/w"}, false},
		{"no tabs", "claude sid /w", Pick{}, true},
		{"empty id", "claude\t\t/w\tx", Pick{}, true},
		{"empty", "", Pick{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePick(tt.line)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("ParsePick(%q) = %+v, %v; want %+v, error %v", tt.line, got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestResumeCommand(t *testing.T) {
	tests := []struct {
		name, shell, cmd, want string
	}{
		{"bash", "/bin/bash", "claude --resume sid-1", "/bin/bash -ic 'claude --resume sid-1; exec /bin/bash'"},
		{"no SHELL falls back to sh", "", "claude --resume sid-1", "/bin/sh -ic 'claude --resume sid-1; exec /bin/sh'"},
		{
			"quotes in the command survive",
			"/bin/bash", `env 'A=b c' codex resume x`,
			`/bin/bash -ic 'env '"'"'A=b c'"'"' codex resume x; exec /bin/bash'`,
		},
		{"a shell path with a space", "/opt/my sh/zsh", "claude", `'/opt/my sh/zsh' -ic 'claude; exec '"'"'/opt/my sh/zsh'"'"''`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResumeCommand(tt.shell, tt.cmd); got != tt.want {
				t.Errorf("ResumeCommand = %q, want %q", got, tt.want)
			}
		})
	}
}

// Every shell the quoting targets must run the command and read it back the same: ask sh to echo what it was given.
func TestResumeCommandRunsInTheShell(t *testing.T) {
	cmd := ResumeCommand("/bin/sh", "printf %s \"it's\"")
	out, err := exec.Command("/bin/sh", "-c", strings.Replace(cmd, "; exec /bin/sh", "", 1)).Output()
	if err != nil {
		t.Fatalf("%s: %v", cmd, err)
	}
	if string(out) != "it's" {
		t.Errorf("output = %q, want it's", out)
	}
}

// pickerEnv stands in for fzf, tmux, claude and the muxrail binary the popup calls; every tmux call is logged.
func pickerEnv(t *testing.T, fzfBody string) (bin, tmuxLog string) {
	t.Helper()
	testutil.IsolatedTmp(t)
	testutil.Stub(t, "muxrail", `printf 'claude\tsid-1\t/Users/jordan/repos/acme/api\tshown\n'`)
	testutil.Stub(t, "claude", "")
	testutil.Stub(t, "fzf", fzfBody)
	tmuxLog = filepath.Join(t.TempDir(), "tmux.log")
	testutil.Stub(t, "tmux", `for a in "$@"; do printf '%s\n' "$a"; done >>`+tmuxLog)
	bin, err := exec.LookPath("muxrail")
	if err != nil {
		t.Fatal(err)
	}
	return bin, tmuxLog
}

func TestRunResumesThePick(t *testing.T) {
	const pick = `cat >/dev/null; printf 'claude\tsid-1\t/Users/jordan/repos/acme/api\tshown\n'`
	// The plan resolves claude to its absolute path on PATH, so only the shell around it is fixed.
	tests := []struct{ name, shell, prefix, suffix string }{
		{"bash", "/bin/bash", "/bin/bash -ic '", "/claude --resume sid-1; exec /bin/bash'"},
		{"SHELL unset", "", "/bin/sh -ic '", "/claude --resume sid-1; exec /bin/sh'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin, tmuxLog := pickerEnv(t, pick)
			t.Setenv("SHELL", tt.shell)
			if err := Run(context.Background(), bin, "/Users/jordan"); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(tmuxLog)
			if err != nil {
				t.Fatal(err)
			}
			args := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(args) != 4 || args[0] != "new-window" || args[1] != "-c" || args[2] != "/Users/jordan/repos/acme/api" {
				t.Fatalf("tmux got %q, want new-window -c <cwd> <command>", args)
			}
			if !strings.HasPrefix(args[3], tt.prefix) || !strings.HasSuffix(args[3], tt.suffix) {
				t.Errorf("command = %q, want %q ... %q", args[3], tt.prefix, tt.suffix)
			}
		})
	}
}

func TestRunPassesFzfTheListAndItsBindings(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "fzf.args")
	bin, _ := pickerEnv(t, `printf '%s\n' "$@" >`+argsFile+`; cat >`+argsFile+`.in; exit 130`)
	if err := Run(context.Background(), bin, "/Users/jordan"); err != nil {
		t.Fatal(err)
	}
	in, err := os.ReadFile(argsFile + ".in")
	if err != nil || !strings.HasPrefix(string(in), "claude\tsid-1\t") {
		t.Errorf("fzf stdin = %q, %v; want the session rows", in, err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{bin + " search cycle agent ", bin + " search cycle sort ", "reload(" + bin + " search --state ", bin + " show {1} {2} {3}"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("fzf args lack %q:\n%s", want, args)
		}
	}
}

func TestRunCancelIsQuiet(t *testing.T) {
	for _, code := range []string{"130", "1"} {
		t.Run("exit "+code, func(t *testing.T) {
			bin, tmuxLog := pickerEnv(t, "cat >/dev/null; exit "+code)
			if err := Run(context.Background(), bin, "/Users/jordan"); err != nil {
				t.Errorf("Run = %v, want nil when the user cancels", err)
			}
			if _, err := os.Stat(tmuxLog); err == nil {
				t.Error("a cancelled pick opened a window")
			}
		})
	}
}

func TestRunNeedsFzf(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := Run(context.Background(), "muxrail", "/Users/jordan")
	if err == nil || !strings.Contains(err.Error(), "fzf") {
		t.Errorf("Run = %v, want an error that names fzf", err)
	}
}
