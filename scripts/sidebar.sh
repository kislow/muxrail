#!/bin/bash
# Window sidebar orchestration. Usage: sidebar.sh toggle|ensure|wake|remember|apply <pane-or-window-id> | render
set -euo pipefail

SELF="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/sidebar.sh"
readonly SELF
readonly DEFAULT_WIDTH=34
readonly MIN_WINDOW_WIDTH=90

sidebar_panes() {
  tmux list-panes "$@" -F '#{pane_id} #{@sidebar}' | awk '$2 == 1 {print $1}'
}

# One width for every window: dragging any sidebar's border sets it, the rest follow.
sidebar_width() {
  local width
  width=$(tmux show -gv @sidebar_width 2>/dev/null || true)
  echo "${width:-$DEFAULT_WIDTH}"
}

apply_width() {
  local pane width
  width=$(sidebar_width)
  for pane in $(sidebar_panes "$@"); do
    [ "$(tmux display -p -t "$pane" '#{pane_width}')" = "$width" ] || tmux resize-pane -t "$pane" -x "$width"
  done
}

# During a mouse drag tmux names the pane under the mouse, not necessarily the sidebar, so look it up in the window.
remember_width() {
  local width
  width=$(tmux list-panes -t "$1" -F '#{@sidebar} #{pane_width}' | awk '$1 == 1 {print $2; exit}')
  [ -z "$width" ] || tmux set -g @sidebar_width "$width"
}

sidebar_enabled() {
  [ "$(tmux show -gv @sidebar_on 2>/dev/null || true)" = 1 ]
}

# The hooks and a toggle can fire together; the lock stops them both splitting the same window.
ensure() {
  local target=$1 lock new
  sidebar_enabled || return 0
  lock="${TMPDIR:-/tmp}/tmux-sidebar-$(tmux display -p -t "$target" '#{window_id}').lock"
  mkdir "$lock" 2>/dev/null || return 0
  if [ -z "$(sidebar_panes -t "$target")" ] &&
    [ "$(tmux display -p -t "$target" '#{window_width}')" -ge "$MIN_WINDOW_WIDTH" ]; then
    new=$(tmux split-window -hbdf -l "$(sidebar_width)" -t "$target" -P -F '#{pane_id}' "$SELF render")
    tmux set-option -p -t "$new" @sidebar 1
  else
    apply_width -t "$target"
  fi
  rmdir "$lock"
}

# Every window gets its sidebar up front, so switching windows never builds one mid-click.
enable_everywhere() {
  local win
  tmux set -g @sidebar_on 1
  for win in $(tmux list-windows -t "$(tmux display -p -t "$1" '#{session_id}')" -F '#{window_id}'); do
    ensure "$win"
  done
}

disable_everywhere() {
  tmux set -g @sidebar_on 0
  sidebar_panes -a | while read -r pane; do tmux kill-pane -t "$pane"; done
}

# A sidebar whose window is hidden refreshes rarely and does not animate. This types its wake sequence (ESC [ 9 9 9 9 ~, see
# internal/sidebar) into the sidebars of the window that just came on screen, so they show fresh data at once.
wake() {
  local pane
  for pane in $(sidebar_panes -t "$1"); do
    tmux send-keys -t "$pane" -H 1b 5b 39 39 39 39 7e
  done
}

toggle() {
  if sidebar_enabled; then
    disable_everywhere
  else
    enable_everywhere "$1"
  fi
}

case "${1:-}" in
  toggle) toggle "${2:?pane id required}" ;;
  ensure) ensure "${2:?pane id required}" ;;
  wake) wake "${2:?pane id required}" ;;
  remember) remember_width "${2:?pane id required}" ;;
  apply) remember_width "${2:?pane id required}"; apply_width -s -t "$2" ;;
  render) exec "${MUXRAIL_BIN:-muxrail}" sidebar ;;
  *) echo "usage: sidebar.sh toggle|ensure|wake|remember|apply <pane-or-window-id> | render" >&2; exit 2 ;;
esac
