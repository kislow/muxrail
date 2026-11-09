package shell

import "testing"

func TestQuote(t *testing.T) {
	tests := map[string]string{
		"claude":           "claude",
		"--resume":         "--resume",
		"/opt/bin/claude":  "/opt/bin/claude",
		"a=b":              "a=b",
		"":                 "''",
		"two words":        "'two words'",
		"it's":             `'it'"'"'s'`,
		"$HOME":            "'$HOME'",
		"semi;colon":       "'semi;colon'",
		"naïve":            "'naïve'",
		"user@host:1/x,y+": "user@host:1/x,y+",
	}
	for in, want := range tests {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %q, want %q", in, got, want)
		}
	}
}
