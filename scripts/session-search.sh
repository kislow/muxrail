#!/bin/bash
# Fuzzy-search every agent session (names, projects, prompts) and resume the pick in a new tmux window.
# Tab cycles the agent filter, ctrl-s the sort order; the right pane previews the latest messages.
# Usage (from a popup): session-search.sh
set -euo pipefail

command -v fzf >/dev/null || export PATH="/opt/homebrew/bin:$PATH"
readonly MUXRAIL="${MUXRAIL_BIN:-muxrail}"

pick_session() {
  local state=$1
  local reload="reload($MUXRAIL search --state $state)+transform-header($MUXRAIL search header $state)"
  "$MUXRAIL" search --state "$state" |
    fzf --ansi --reverse --no-hscroll --delimiter=$'\t' --with-nth=4 --prompt='session > ' \
      --header="$("$MUXRAIL" search header "$state")" \
      --bind "tab:execute-silent($MUXRAIL search cycle agent $state)+$reload" \
      --bind "ctrl-s:execute-silent($MUXRAIL search cycle sort $state)+$reload" \
      --preview "$MUXRAIL show {1} {2} {3}" --preview-window='right,50%,wrap,border-left'
}

main() {
  local state pick agent id cwd plan run_cwd cmd
  state=$(mktemp "${TMPDIR:-/tmp}/muxrail-search.XXXXXX")
  # Expanded now, not when the trap fires: state is local to main and gone by then.
  # shellcheck disable=SC2064
  trap "rm -f $(printf '%q' "$state")" EXIT
  pick=$(pick_session "$state") || return 0
  IFS=$'\t' read -r agent id cwd _ <<<"$pick"
  plan=$("$MUXRAIL" plan "$agent" "$id" "$cwd") || return 0
  IFS=$'\t' read -r run_cwd cmd <<<"$plan"
  tmux new-window -c "$run_cwd" "zsh -ic $(printf '%q' "$cmd"'; exec zsh')"
}

main
