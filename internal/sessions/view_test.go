package sessions

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func sample() []*Session {
	return []*Session{
		{Agent: "cursor", ID: "1", CWD: "/a", TS: 30, Title: "banana"},
		{Agent: "claude", ID: "2", CWD: "/b", TS: 20, Title: "Apple"},
		{Agent: "codex", ID: "3", CWD: "/c", TS: 10, Prompts: []string{"cherry"}},
		{Agent: "claude", ID: "4", CWD: "/d", TS: 5, Title: "date"},
	}
}

func ids(list []*Session) []string {
	var out []string
	for _, s := range list {
		out = append(out, s.ID)
	}
	return out
}

func TestViewApply(t *testing.T) {
	tests := []struct {
		name string
		view View
		want []string
	}{
		{"all by time keeps the order", View{Sort: "time"}, []string{"1", "2", "3", "4"}},
		{"one agent", View{Agent: "claude", Sort: "time"}, []string{"2", "4"}},
		{"by name ignores case and uses the first prompt when untitled", View{Sort: "name"}, []string{"2", "1", "3", "4"}},
		{"by agent then newest", View{Sort: "agent"}, []string{"2", "4", "3", "1"}},
		{"an agent with nothing", View{Agent: "agy", Sort: "time"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ids(tt.view.Apply(sample())); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Apply = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCyclingSkipsAgentsWithoutSessions(t *testing.T) {
	v := &View{Sort: "time"}
	v.Remember(sample())
	if want := []string{"claude", "codex", "cursor"}; !reflect.DeepEqual(v.Present, want) {
		t.Fatalf("present = %v, want %v", v.Present, want)
	}
	var seen []string
	for range 5 {
		v.CycleAgent()
		seen = append(seen, v.Agent)
	}
	if want := []string{"claude", "codex", "cursor", "", "claude"}; !reflect.DeepEqual(seen, want) {
		t.Errorf("agent cycle = %q, want %q (an agent with no sessions is skipped)", seen, want)
	}
	for _, want := range []string{"name", "agent", "time"} {
		if v.CycleSort(); v.Sort != want {
			t.Errorf("sort = %q, want %q", v.Sort, want)
		}
	}
}

func TestViewStateRoundTripAndDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	if got := LoadView(path); got.Agent != "" || got.Sort != "time" {
		t.Errorf("missing file should give the default view, got %+v", got)
	}
	writeFile(t, path, "")
	if got := LoadView(path); got.Sort != "time" {
		t.Errorf("an empty file (mktemp) should give the default view, got %+v", got)
	}
	v := &View{Agent: "codex", Sort: "name", Present: []string{"codex"}}
	if err := v.Save(path); err != nil {
		t.Fatal(err)
	}
	if got := LoadView(path); !reflect.DeepEqual(got, v) {
		t.Errorf("round trip = %+v, want %+v", got, v)
	}
	writeFile(t, path, `{"sort":"bogus"}`)
	if got := LoadView(path); got.Sort != "time" {
		t.Errorf("an unknown sort falls back to time, got %q", got.Sort)
	}
}

func TestCycleAdvancesTheSavedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	if err := (&View{Sort: "time", Present: []string{"claude", "codex"}}).Save(path); err != nil {
		t.Fatal(err)
	}
	if err := Cycle(path, "agent"); err != nil {
		t.Fatal(err)
	}
	if err := Cycle(path, "sort"); err != nil {
		t.Fatal(err)
	}
	if got := LoadView(path); got.Agent != "claude" || got.Sort != "name" {
		t.Errorf("state = %+v", got)
	}
	if Cycle(path, "colour") == nil {
		t.Error("an unknown setting should be an error")
	}
}

func TestHeaderNamesTheView(t *testing.T) {
	if h := (&View{Agent: "codex", Sort: "name"}).Header(); !strings.Contains(h, "agent: codex") || !strings.Contains(h, "sort: name") {
		t.Errorf("header = %q", h)
	}
	if h := (&View{Sort: "time"}).Header(); !strings.Contains(h, "agent: all") {
		t.Errorf("header = %q", h)
	}
}
