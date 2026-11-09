// Package shell quotes words for the POSIX shells that tmux and fzf hand commands to.
package shell

import (
	"regexp"
	"strings"
)

var safeRE = regexp.MustCompile(`^[a-zA-Z0-9_@%+=:,./-]+$`)

// Quote quotes s for a POSIX shell, leaving it bare when it has no special characters.
func Quote(s string) string {
	if safeRE.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
