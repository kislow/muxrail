package sessions

import (
	"regexp"
	"sort"
	"strings"
)

// Plan is how to resume a session: run Program with Args in CWD with Env added.
type Plan struct {
	Program string            `json:"program"`
	Args    []string          `json:"args"`
	CWD     string            `json:"cwd"`
	Env     map[string]string `json:"env"`
}

var shellSafeRE = regexp.MustCompile(`^[a-zA-Z0-9_@%+=:,./-]+$`)

// shellQuote quotes s for a POSIX shell, leaving it bare when it has no special characters.
func shellQuote(s string) string {
	if shellSafeRE.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// Command is "cwd<TAB>shell command" for fzf's caller: env assignments go through env(1) so the shell runs them as a prefix.
func (p *Plan) Command(fallbackCWD string) string {
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
		words[i] = shellQuote(w)
	}
	cwd := p.CWD
	if cwd == "" {
		cwd = fallbackCWD
	}
	return cwd + "\t" + strings.Join(words, " ")
}
