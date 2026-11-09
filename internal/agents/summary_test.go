package agents

import "testing"

func TestSummarize(t *testing.T) {
	snap := Snapshot{
		"%1": {Kind: "claude", Status: "working"},
		"%2": {Kind: "codex", Status: "working"},
		"%3": {Kind: "claude", Status: "waiting", Since: 200},
		"%4": {Kind: "cursor", Status: "waiting", Since: 100},
		"%5": {Kind: "claude", Status: "done"},
		"%6": {Kind: "claude"},
		"%7": {Status: "working"},
	}
	want := Summary{Working: 2, Waiting: 2, Done: 1, WaitingSince: 100}
	if got := Summarize(snap); got != want {
		t.Errorf("Summarize = %+v, want %+v (panes without an agent or a status are not counted)", got, want)
	}
}

func TestSummarizeEmpty(t *testing.T) {
	if got := Summarize(nil); got != (Summary{}) {
		t.Errorf("Summarize(nil) = %+v, want zero", got)
	}
}
