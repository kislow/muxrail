package ago

import (
	"testing"
	"time"
)

func TestSince(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	at := func(secondsAgo float64) float64 { return float64(now.Unix()) - secondsAgo }
	tests := []struct {
		name string
		ts   float64
		want string
	}{
		{"just now", at(5), "now"},
		{"under a minute", at(59), "now"},
		{"a minute", at(60), "1m"},
		{"minutes", at(125), "2m"},
		{"an hour", at(3600), "1h"},
		{"hours", at(7300), "2h"},
		{"a day", at(86400), "1d"},
		{"days", at(86400*9 + 10), "9d"},
		{"the future is now", at(-30), "now"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Since(now, tt.ts); got != tt.want {
				t.Errorf("Since = %q, want %q", got, tt.want)
			}
		})
	}
}
