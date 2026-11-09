package sessions

import (
	"bytes"
	"encoding/json"
	"regexp"
)

// delegateRE matches the worktree and scratch folders of headless `ki delegate` jobs, which are not conversations to resume.
var delegateRE = regexp.MustCompile(`(-worktrees|/ki/jobs)/j-\d{8}-`)

const (
	cwdScanLines   = 60
	titleTailBytes = 256 << 10 // Claude re-appends the titles after the last turn, so the end of the file has them
)

var (
	customNeedle    = []byte(`"type":"custom-title"`)
	generatedNeedle = []byte(`"type":"ai-title"`)
)

// Title is the name of a Claude session found in its transcript.
type Title struct {
	Text    string
	Renamed bool // a /rename title rather than a generated one
}

type titleRecord struct {
	Type        string `json:"type"`
	CustomTitle string `json:"customTitle"`
	AITitle     string `json:"aiTitle"`
}

func (r titleRecord) text() string {
	if r.Type == "custom-title" {
		return r.CustomTitle
	}
	return r.AITitle
}

// lastTitle is the title of the last record of kind in data. Lines are prefiltered with a byte search so that
// hundreds of megabytes of transcripts are not JSON-parsed; a line that mentions the needle without being a record
// (a title-looking string quoted inside a message) fails the type check and is skipped.
func lastTitle(data, needle []byte, kind string) string {
	title := ""
	for pos := 0; pos < len(data); {
		hit := bytes.Index(data[pos:], needle)
		if hit < 0 {
			break
		}
		hit += pos
		start := bytes.LastIndexByte(data[:hit], '\n') + 1
		end := len(data)
		if nl := bytes.IndexByte(data[hit:], '\n'); nl >= 0 {
			end = hit + nl
		}
		pos = end + 1
		var rec titleRecord
		if json.Unmarshal(data[start:end], &rec) != nil || rec.Type != kind {
			continue
		}
		if t := rec.text(); t != "" {
			title = t
		}
	}
	return title
}

// scanTitle reads the end of a Claude transcript and returns its title: the last /rename title, else the last generated
// one; nil when it has neither.
func scanTitle(path string) (*Title, error) {
	data, err := tail(path, titleTailBytes)
	if err != nil {
		return nil, err
	}
	custom := lastTitle(data, customNeedle, "custom-title")
	generated := lastTitle(data, generatedNeedle, "ai-title")
	switch {
	case custom != "":
		return &Title{Text: custom, Renamed: true}, nil
	case generated != "":
		return &Title{Text: generated}, nil
	}
	return nil, nil
}

// cwdOf is the folder a transcript ran in: the first cwd field in its first lines.
func cwdOf(lines [][]byte) string {
	for _, line := range lines[:min(len(lines), cwdScanLines)] {
		if !bytes.Contains(line, []byte(`"cwd"`)) {
			continue
		}
		var rec struct {
			CWD string `json:"cwd"`
		}
		if json.Unmarshal(line, &rec) == nil && rec.CWD != "" {
			return rec.CWD
		}
	}
	return ""
}
