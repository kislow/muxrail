package sessions

import "path/filepath"

// transcriptGlob matches the transcript files of agent under home. Locate passes one session id; the readers pass "*"
// to list them all, so each agent's layout is written down once.
func transcriptGlob(home, agent, id string) string {
	switch agent {
	case "claude":
		return filepath.Join(home, ".claude", "projects", "*", id+".jsonl")
	case "codex":
		return filepath.Join(home, ".codex", "sessions", "*", "*", "*", "rollout-*-"+id+".jsonl")
	case "cursor":
		return filepath.Join(home, ".cursor", "projects", "*", "agent-transcripts", id, id+".jsonl")
	}
	return ""
}
