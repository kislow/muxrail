// Package agents finds which tmux panes run a coding agent and what state each is in.
package agents

import (
	"regexp"
	"slices"
	"strings"
)

const (
	Claude = "claude"
	Codex  = "codex"
	Cursor = "cursor"
	Gemini = "gemini"
)

var versionRE = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// IsVersion reports whether name is a bare version number: tmux shows Claude's process (and names its window) that way.
func IsVersion(name string) bool { return versionRE.MatchString(name) }

// Classify names the agent behind a foreground process, or "". ps truncates comm to 16 characters, so the first word
// of args counts too; ps names Claude `claude` while tmux shows its version, and Cursor's CLI runs as node.
func Classify(comm, args string) string {
	first := basename(strings.SplitN(args, " ", 2)[0])
	names := []string{basename(comm), first}
	switch {
	case slices.Contains(names, "claude") || slices.ContainsFunc(names, IsVersion):
		return Claude
	case slices.Contains(names, "codex"):
		return Codex
	case slices.Contains(names, "agy"), slices.Contains(names, "gemini"):
		return Gemini
	case (first == "agent" || first == "cursor-agent") && strings.Contains(args, "index.js"):
		return Cursor
	}
	return ""
}

func basename(p string) string { return p[strings.LastIndexByte(p, '/')+1:] }

// ForegroundKinds maps tty name (ttys003) to agent kind from `ps -axo tty=,stat=,comm=,args=` output, keeping only
// foreground processes ('+' in stat) on a terminal.
func ForegroundKinds(psOutput string) map[string]string {
	found := map[string]string{}
	for line := range strings.SplitSeq(psOutput, "\n") {
		f := fields(line, 4)
		if len(f) < 4 || !strings.HasPrefix(f[0], "tty") || !strings.Contains(f[1], "+") {
			continue
		}
		if kind := Classify(f[2], f[3]); kind != "" {
			found[f[0]] = kind
		}
	}
	return found
}

// fields splits on runs of whitespace into at most n parts; the last part keeps the rest of the line.
func fields(line string, n int) []string {
	var out []string
	for len(out) < n-1 {
		line = strings.TrimLeft(line, " \t")
		end := strings.IndexAny(line, " \t")
		if end < 0 {
			break
		}
		out, line = append(out, line[:end]), line[end:]
	}
	if rest := strings.TrimLeft(line, " \t"); rest != "" {
		out = append(out, rest)
	}
	return out
}
