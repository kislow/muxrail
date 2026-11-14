package activity

import (
	"path/filepath"
	"strings"
)

// Describe is a tool call in a few words: what it does and to what. It knows the tool names of Claude Code, Codex,
// Cursor and Vibe; Vibe's built-ins are namespaced (file_system.bash, process.start), so the namespace is dropped first.
func Describe(tool string, input map[string]any) string {
	str := func(key string) string {
		s, _ := input[key].(string)
		return strings.TrimSpace(s)
	}
	file := filepath.Base(first(str("file_path"), str("path"), str("notebook_path"), str("file")))
	switch vibeBuiltin(tool) {
	case "bash", "start":
		return "$ " + oneLine(str("command"))
	case "read_file":
		return "◇ Read " + file
	case "edit":
		return "✎ Edit " + file
	case "write_file":
		return "✎ Write " + file
	case "grep":
		return "⌕ Grep " + str("pattern")
	case "web_search", "web_fetch":
		return "⌕ Web " + first(str("query"), str("url"))
	case "task":
		return "◈ Agent " + first(str("description"), str("agent"), str("task"))
	case "todo":
		return "☰ plan"
	case "ask_user_question":
		return "☰ question"
	}
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
	if _, name, ok := strings.Cut(tool, "."); ok { // a Vibe MCP tool (server.tool) or a built-in it does not know
		return "⚙ " + name
	}
	return "⚙ " + tool
}

// vibeBuiltin is the name of a Vibe built-in tool without its namespace, or "" for any other tool. An MCP tool is
// <server>.<tool> too, so only Vibe's own namespaces count.
func vibeBuiltin(tool string) string {
	ns, name, ok := strings.Cut(tool, ".")
	switch {
	case !ok:
		if tool == "ask_user_question" {
			return tool
		}
		return ""
	case ns == "file_system", ns == "process" && name == "start", ns == "vibe", ns == "ui", ns == "web":
		return name
	}
	return ""
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
