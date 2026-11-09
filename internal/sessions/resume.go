package sessions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
)

var sqliteHomeRE = regexp.MustCompile(`(?m)^\s*sqlite_home\s*=`)

// Resume plans how to resume a session: the agent's program from PATH, run in the session's folder.
func Resume(home, agent, id, cwd string) (*Plan, error) {
	var program string
	var args []string
	var env map[string]string
	switch agent {
	case "claude":
		program, args = "claude", []string{"--resume", id}
	case "codex":
		program, args, env = "codex", []string{"resume", id}, codexEnv(home)
	case "cursor":
		program, args = "cursor-agent", []string{"--resume", id}
	default:
		return nil, fmt.Errorf("cannot resume %s session %s: unknown agent", agent, id)
	}
	path, err := exec.LookPath(program)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot resume %s session %s", err, agent, id)
	}
	return &Plan{Program: path, Args: args, CWD: cwd, Env: env}, nil
}

// codexEnv pins Codex's state database to its own home, as the resume command expects wherever it is launched from.
// A sqlite_home set in config.toml wins: forcing the variable would override that choice.
func codexEnv(home string) map[string]string {
	dir := filepath.Join(home, ".codex")
	if cfg, err := os.ReadFile(filepath.Join(dir, "config.toml")); err == nil && sqliteHomeRE.Match(cfg) {
		return nil
	}
	return map[string]string{"CODEX_SQLITE_HOME": dir}
}
