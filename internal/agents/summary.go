package agents

// Summary counts the agents by state, for the status bar.
type Summary struct {
	Working, Waiting, Done int
	// WaitingSince is when the longest-waiting agent began to wait (epoch seconds); zero when none waits.
	WaitingSince float64
}

// Summarize counts every pane that runs an agent and has a status.
func Summarize(snap Snapshot) Summary {
	var s Summary
	for _, info := range snap {
		if info.Kind == "" {
			continue
		}
		switch info.Status {
		case "working":
			s.Working++
		case "waiting":
			s.Waiting++
			if s.WaitingSince == 0 || (info.Since != 0 && info.Since < s.WaitingSince) {
				s.WaitingSince = info.Since
			}
		case "done":
			s.Done++
		}
	}
	return s
}
