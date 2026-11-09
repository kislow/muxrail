package activity

import (
	"path/filepath"
	"strings"
)

// Describe is a tool call in a few words: what it does and to what. It knows the tool names of Claude Code, Codex and
// Cursor.
func Describe(tool string, input map[string]any) string {
	str := func(key string) string {
		s, _ := input[key].(string)
		return strings.TrimSpace(s)
	}
	file := filepath.Base(first(str("file_path"), str("path"), str("notebook_path"), str("file")))
	switch tool {
	case "Edit", "MultiEdit", "Write", "NotebookEdit":
		return "✎ " + tool + " " + file
	case "StrReplace":
		return "✎ Edit " + file
	case "apply_patch":
		return "✎ Patch " + file
	case "Read":
		return "◇ Read " + file
	case "Bash", "Shell":
		return "$ " + oneLine(first(str("description"), str("command")))
	case "exec_command":
		return "$ " + oneLine(first(str("cmd"), str("command")))
	case "Grep", "Glob":
		return "⌕ " + tool + " " + first(str("pattern"), str("glob_pattern"))
	case "WebFetch", "WebSearch":
		return "⌕ " + tool + " " + first(str("query"), str("url"))
	case "Task", "Agent", "spawn_agent":
		return "◈ Agent " + first(str("description"), str("subagent_type"), str("agent_type"))
	case "update_plan":
		return "☰ plan"
	}
	if _, name, ok := strings.Cut(strings.TrimPrefix(tool, "mcp__"), "__"); ok && strings.HasPrefix(tool, "mcp__") {
		return "⚙ " + name
	}
	return "⚙ " + tool
}

func first(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func oneLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
