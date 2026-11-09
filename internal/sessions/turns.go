package sessions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	tailBytes   = 512 << 10 // enough for the last turns of a long session without reading it whole
	turnRunes   = 700
	userQueryTL = "<user_query>"
	userQueryTR = "</user_query>"
)

var idRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Turn is one message of a conversation.
type Turn struct{ Role, Text string }

// Transcript is where a session's conversation lives.
type Transcript struct {
	Path     string
	Modified time.Time
}

// Locate finds a session's transcript by id. A nil Transcript with no error means the agent keeps none we can read,
// such as Cursor's legacy text files or the Cursor IDE.
func Locate(home, agent, id string) (*Transcript, error) {
	if !idRE.MatchString(id) {
		return nil, fmt.Errorf("session id %q: letters, digits, - and _ only", id)
	}
	pattern := transcriptGlob(home, agent, id)
	if pattern == "" {
		return nil, nil
	}
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return nil, err
	}
	info, err := os.Stat(matches[0])
	if err != nil {
		return nil, fmt.Errorf("%w: stat transcript", err)
	}
	return &Transcript{Path: matches[0], Modified: info.ModTime()}, nil
}

// LastTurns is the final n user and assistant messages of a transcript. Tool calls, tool output, reasoning and
// text the agent injected itself (environment blocks, system reminders) are left out.
func LastTurns(path string, n int) ([]Turn, error) {
	data, err := tail(path, tailBytes)
	if err != nil {
		return nil, err
	}
	var turns []Turn
	for _, line := range bytes.Split(data, []byte("\n")) {
		if t, ok := turnOf(line); ok {
			turns = append(turns, t)
		}
	}
	return turns[max(len(turns)-n, 0):], nil
}

func tail(path string, size int64) (data []byte, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: open transcript", err)
	}
	defer closeFile(f, &err)
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("%w: stat transcript", err)
	}
	start := max(info.Size()-size, 0)
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, fmt.Errorf("%w: seek transcript", err)
	}
	data, err = io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("%w: read transcript", err)
	}
	if start > 0 { // the first line is cut in the middle
		if _, rest, found := bytes.Cut(data, []byte("\n")); found {
			return rest, nil
		}
		return nil, nil
	}
	return data, nil
}

// record covers the three transcript shapes at once: Claude and Cursor nest a message under "message", Codex nests it
// in the payload of a response_item.
type record struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	Payload struct {
		Type    string          `json:"type"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"payload"`
}

func turnOf(line []byte) (Turn, bool) {
	var rec record
	if len(line) == 0 || json.Unmarshal(line, &rec) != nil {
		return Turn{}, false
	}
	role, content := rec.Role, rec.Message.Content
	if role == "" {
		role = rec.Type
	}
	if rec.Payload.Type == "message" {
		role, content = rec.Payload.Role, rec.Payload.Content
	}
	if role != "user" && role != "assistant" {
		return Turn{}, false
	}
	text := messageText(content)
	if role == "user" {
		text = userText(text)
	}
	if text == "" || strings.HasPrefix(text, "<") {
		return Turn{}, false
	}
	return Turn{Role: role, Text: text}, true
}

// messageText is the text parts of a message body, which is a plain string or a list of typed parts.
func messageText(content json.RawMessage) string {
	var plain string
	if json.Unmarshal(content, &plain) == nil {
		return strings.TrimSpace(plain)
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &parts) != nil {
		return ""
	}
	var texts []string
	for _, p := range parts {
		if p.Type == "text" || p.Type == "input_text" || p.Type == "output_text" {
			texts = append(texts, strings.TrimSpace(p.Text))
		}
	}
	return strings.TrimSpace(strings.Join(texts, "\n"))
}

// userText unwraps Cursor's <user_query> wrapper around what was typed, dropping the <timestamp> block before it.
func userText(text string) string {
	_, inner, ok := strings.Cut(text, userQueryTL)
	if !ok {
		return text
	}
	inner, _, _ = strings.Cut(inner, userQueryTR)
	return strings.TrimSpace(inner)
}
