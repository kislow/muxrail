// Package agents finds which tmux panes run a coding agent and what state each is in.
package agents

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const (
	Claude = "claude"
	Codex  = "codex"
	Cursor = "cursor"
	Gemini = "gemini"
	Vibe   = "vibe"
)

var versionRE = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// IsVersion reports whether name is a bare version number: tmux shows Claude's process (and names its window) that way.
func IsVersion(name string) bool { return versionRE.MatchString(name) }

// Classify names the agent behind a foreground process, or "". ps truncates comm to 16 characters, so the first word
// of args counts too; ps names Claude `claude` while tmux shows its version, Cursor's CLI runs as node and Vibe's
// `vibe` launcher hands over to its vibe-rs binary.
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
	case slices.Contains(names, "vibe"), slices.Contains(names, "vibe-rs"):
		return Vibe
	case (first == "agent" || first == "cursor-agent") && strings.Contains(args, "index.js"):
		return Cursor
	}
	return ""
}

func basename(p string) string { return p[strings.LastIndexByte(p, '/')+1:] }

// psFormat is the `ps -axo` format ForegroundKinds and TTYsByPID read.
const psFormat = "pid=,tty=,stat=,comm=,args="

// ForegroundKinds maps tty name (ttys003) to agent kind from `ps -axo pid=,tty=,stat=,comm=,args=` output, keeping only
// foreground processes ('+' in stat) on a terminal.
func ForegroundKinds(psOutput string) map[string]string {
	found := map[string]string{}
	for line := range strings.SplitSeq(psOutput, "\n") {
		f := fields(line, 5)
		if len(f) < 5 || !strings.HasPrefix(f[1], "tty") || !strings.Contains(f[2], "+") {
			continue
		}
		if kind := Classify(f[3], f[4]); kind != "" {
			found[f[1]] = kind
		}
	}
	return found
}

// TTYsByPID maps the id of every process on a terminal to the terminal's name, from the same ps output.
func TTYsByPID(psOutput string) map[int]string {
	found := map[int]string{}
	for line := range strings.SplitSeq(psOutput, "\n") {
		f := fields(line, 3)
		if len(f) < 2 || !strings.HasPrefix(f[1], "tty") {
			continue
		}
		if pid, err := strconv.Atoi(f[0]); err == nil {
			found[pid] = f[1]
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
