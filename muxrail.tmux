#!/usr/bin/env bash
# tpm entry point:  set -g @plugin 'kislow/muxrail'   (needs the muxrail binary on PATH or in MUXRAIL_BIN)
# Registers the keys and hooks; all of them run the muxrail binary. The tab bar is the user's own tmux.conf unless  set -g @muxrail_tabs rail  (see README).
set -euo pipefail

# Tabs as stops on a rail: a station coloured by the agent's state, the index, the logo and the name. A window still
# auto-named like 2.1.318 shows a short conversation title instead.
rail_tabs() {
  local name='#{?#{m/r:^[0-9]+[.][0-9]+[.][0-9]+$,#{window_name}},#{=12:#{s/^[^ ]+ //:pane_title}},#W}'
  tmux set -g window-status-separator ""
  tmux set -g window-status-format "#[fg=brightblack]──#[default] #(muxrail tab #{window_id} rail #I)$name "
  tmux set -g window-status-current-format "#[fg=green,bold]━━#[default] #(muxrail tab #{window_id} rail-current #I)#[bold]$name #[fg=green]━━#[default]"
}
[ "$(tmux show-option -gqv @muxrail_tabs)" = rail ] && rail_tabs

tmux bind-key -r "<" if-shell -F '#{window_start_flag}' 'display-message "first window"' 'swap-window -d -t -1'
tmux bind-key -r ">" if-shell -F '#{window_end_flag}' 'display-message "last window"' 'swap-window -d -t +1'

BIN="${MUXRAIL_BIN:-$(command -v muxrail || true)}"
if [ -z "$BIN" ]; then
  tmux display-message "muxrail: binary not found. Install it or set MUXRAIL_BIN, then reload the config"
  exit 0
fi
# Single-quoted once, so a path with spaces survives the shell tmux runs the commands in.
squote() {
  local q="'"
  printf "'%s'" "${1//$q/$q\\$q$q}"
}
BIN_Q="$(squote "$BIN")"

tmux bind-key b run-shell -b "$BIN_Q sidebar toggle #{pane_id}"
tmux bind-key h display-popup -EE -w 90% -h 85% -T " search sessions " "$BIN_Q search --pick"

tmux set-hook -g after-select-window "run-shell -b \"$BIN_Q sidebar ensure #{pane_id}; $BIN_Q sidebar wake #{pane_id}\""
tmux set-hook -g client-session-changed "run-shell -b \"$BIN_Q sidebar wake #{pane_id}\""
tmux set-hook -g client-attached "run-shell -b \"$BIN_Q sidebar wake #{pane_id}\""
tmux set-hook -g after-new-window "run-shell -b \"$BIN_Q sidebar ensure #{pane_id}\""
tmux set-hook -g after-resize-pane "run-shell -b \"$BIN_Q sidebar remember #{pane_id}\""
tmux bind-key -n MouseDragEnd1Border run-shell -b "sleep 0.3; $BIN_Q sidebar apply #{pane_id}"
