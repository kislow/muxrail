package sessions

import (
	"sort"
	"strings"

	"github.com/kislow/muxrail/internal/shell"
)

// Plan is how to resume a session: run Program with Args in CWD with Env added.
type Plan struct {
	Program string            `json:"program"`
	Args    []string          `json:"args"`
	CWD     string            `json:"cwd"`
	Env     map[string]string `json:"env"`
}

// Command is "cwd<TAB>shell command", the form `muxrail plan` prints.
func (p *Plan) Command(fallbackCWD string) string {
	cwd, cmd := p.Shell(fallbackCWD)
	return cwd + "\t" + cmd
}

// Shell is the folder to run in (the plan's, else fallbackCWD) and the shell command that resumes the session: env
// assignments go through env(1) so the shell runs them as a prefix.
func (p *Plan) Shell(fallbackCWD string) (cwd, cmd string) {
	words := []string{}
	if len(p.Env) > 0 {
		keys := make([]string, 0, len(p.Env))
		for k := range p.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		words = append(words, "env")
		for _, k := range keys {
			words = append(words, k+"="+p.Env[k])
		}
	}
	words = append(words, p.Program)
	words = append(words, p.Args...)
	for i, w := range words {
		words[i] = shell.Quote(w)
	}
	cwd = p.CWD
	if cwd == "" {
		cwd = fallbackCWD
	}
	return cwd, strings.Join(words, " ")
}
