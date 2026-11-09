// Package ago formats how long ago something happened.
package ago

import (
	"strconv"
	"time"
)

var units = []struct {
	suffix string
	secs   float64
}{{"d", 86400}, {"h", 3600}, {"m", 60}}

// Since is the time from since (epoch seconds) to now in its largest whole unit: now, Nm, Nh, Nd.
func Since(now time.Time, since float64) string {
	elapsed := float64(now.UnixNano())/1e9 - since
	for _, u := range units {
		if elapsed >= u.secs {
			return strconv.Itoa(int(elapsed/u.secs)) + u.suffix
		}
	}
	return "now"
}
