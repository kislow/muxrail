package sessions

import "testing"

func TestPlanCommand(t *testing.T) {
	tests := []struct {
		name string
		plan Plan
		want string
	}{
		{"simple", Plan{Program: "/bin/codex", Args: []string{"resume", "abc"}, CWD: "/w/x"}, "/w/x\t/bin/codex resume abc"},
		{
			"env goes through env(1), sorted",
			Plan{Program: "cursor", Args: []string{"--resume"}, CWD: "/w", Env: map[string]string{"B": "2", "A": "one two"}},
			"/w\tenv 'A=one two' B=2 cursor --resume",
		},
		{"empty env adds nothing", Plan{Program: "p", CWD: "/w", Env: map[string]string{}}, "/w\tp"},
		{"quoting of args", Plan{Program: "p", Args: []string{"a b", "c'd"}, CWD: "/w"}, `/w` + "\t" + `p 'a b' 'c'"'"'d'`},
		{"missing cwd falls back to the session's", Plan{Program: "p"}, "/fallback\tp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.plan.Command("/fallback"); got != tt.want {
				t.Errorf("Command = %q, want %q", got, tt.want)
			}
		})
	}
}
