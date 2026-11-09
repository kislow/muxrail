package sessions

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
)

var sortModes = []string{"time", "name", "agent"}

// View is how the popup narrows and orders the list. Tab cycles the agent and Ctrl+S the sort; the choice lives in a
// state file the popup passes in, so two popups never share it. Present is the agents that have sessions, saved by
// the last listing so cycling never lands on an empty agent.
type View struct {
	Agent   string   `json:"agent"` // empty means every agent
	Sort    string   `json:"sort"`
	Present []string `json:"present"`
}

// LoadView reads the state file; a missing or unreadable one is the default view.
func LoadView(path string) *View {
	v := &View{Sort: sortModes[0]}
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, v) != nil || !slices.Contains(sortModes, v.Sort) {
		return &View{Sort: sortModes[0]}
	}
	return v
}

// Save writes the view to the state file.
func (v *View) Save(path string) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("%w: encode view", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("%w: write view", err)
	}
	return nil
}

// Apply returns the sessions of the chosen agent in the chosen order; list is already newest first.
func (v *View) Apply(list []*Session) []*Session {
	out := make([]*Session, 0, len(list))
	for _, s := range list {
		if v.Agent == "" || s.Agent == v.Agent {
			out = append(out, s)
		}
	}
	switch v.Sort {
	case "name":
		sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(label(out[i])) < strings.ToLower(label(out[j])) })
	case "agent":
		sort.SliceStable(out, func(i, j int) bool { return out[i].Agent < out[j].Agent })
	}
	return out
}

// Remember records which agents have sessions in list.
func (v *View) Remember(list []*Session) {
	v.Present = v.Present[:0]
	for _, a := range agentNames {
		if slices.ContainsFunc(list, func(s *Session) bool { return s.Agent == a }) {
			v.Present = append(v.Present, a)
		}
	}
}

// CycleAgent moves to the next agent that has sessions, through every agent and back to all.
func (v *View) CycleAgent() {
	order := append([]string{""}, v.Present...)
	v.Agent = order[(slices.Index(order, v.Agent)+1)%len(order)]
}

// CycleSort moves to the next sort order.
func (v *View) CycleSort() {
	v.Sort = sortModes[(slices.Index(sortModes, v.Sort)+1)%len(sortModes)]
}

// Header is the line above the list.
func (v *View) Header() string {
	agent := v.Agent
	if agent == "" {
		agent = "all"
	}
	return fmt.Sprintf("agent: %s · sort: %s   (tab agent, ctrl-s sort)   ● renamed  ○ untitled", agent, v.Sort)
}

// Cycle advances one setting ("agent" or "sort") of the view saved at path.
func Cycle(path, what string) error {
	v := LoadView(path)
	switch what {
	case "agent":
		v.CycleAgent()
	case "sort":
		v.CycleSort()
	default:
		return errors.New("cycle agent or sort")
	}
	return v.Save(path)
}
