package term

import "strings"

// Mouse is one SGR mouse report (ESC [ < button ; x ; y M|m).
type Mouse struct {
	Button  int
	X, Y    int
	Release bool
}

// Button codes of the events the sidebar reacts to.
const (
	ButtonPress  = 0
	ButtonMotion = 32
)

// TakeComplete splits off a trailing half-received escape sequence (ESC, ESC [, ESC [ < 1;2) so the next read can finish it.
func TakeComplete(buf string) (complete, partial string) {
	i := strings.LastIndexByte(buf, 0x1b)
	if i < 0 || !isPartialEscape(buf[i:]) {
		return buf, ""
	}
	return buf[:i], buf[i:]
}

func isPartialEscape(s string) bool {
	rest := strings.TrimPrefix(s, "\x1b")
	if rest == "" {
		return true
	}
	rest, ok := strings.CutPrefix(rest, "[")
	if !ok {
		return false
	}
	rest, ok = strings.CutPrefix(rest, "<")
	if !ok {
		return rest == ""
	}
	return strings.Trim(rest, "0123456789;") == ""
}

// ParseMouse returns every mouse report in data, in order, and the data left once they are removed.
func ParseMouse(data string) (events []Mouse, rest string) {
	var keep strings.Builder
	for i := 0; i < len(data); {
		if m, n, ok := mouseAt(data[i:]); ok {
			events = append(events, m)
			i += n
			continue
		}
		keep.WriteByte(data[i])
		i++
	}
	return events, keep.String()
}

func mouseAt(s string) (m Mouse, n int, ok bool) {
	body, found := strings.CutPrefix(s, "\x1b[<")
	if !found {
		return Mouse{}, 0, false
	}
	end := strings.IndexAny(body, "Mm")
	if end < 0 {
		return Mouse{}, 0, false
	}
	fields := strings.Split(body[:end], ";")
	if len(fields) != 3 {
		return Mouse{}, 0, false
	}
	var nums [3]int
	for i, f := range fields {
		v, ok := atoi(f)
		if !ok {
			return Mouse{}, 0, false
		}
		nums[i] = v
	}
	return Mouse{Button: nums[0], X: nums[1], Y: nums[2], Release: body[end] == 'm'}, 3 + end + 1, true
}

func atoi(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}
