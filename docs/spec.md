# muxrail behaviour spec

This document describes the behaviour of the `muxrail` binary (`cmd/`, `internal/`) and the `muxrail.tmux` bindings that call it.
`make e2e` is the acceptance check for the parts only a real tmux client shows.

Target: macOS (Apple silicon), tmux 3.8, Warp terminal, a Nerd Font for logos.
`muxrail.tmux` binds the keys and hooks to the binary; `muxrail sidebar <action>` and `muxrail search --pick` hold the glue.

## Commands

| Command | What it does |
|---|---|
| `muxrail sidebar` | Full-screen TUI run in a narrow pane at the left of every window (the pane runs the binary by absolute path). Reads `TMUX_PANE`. |
| `muxrail sidebar toggle\|ensure\|wake\|remember\|apply <pane-id>` | Control path the tmux keys and hooks call; see Sidebar control. |
| `muxrail tab <window-id> [plain]` | Prints the tab-bar icon for a window as tmux style codes, no trailing newline. Called from `#(...)` in `window-status-format`. |
| `muxrail search` | One searchable list of sessions as TSV for fzf: `agent<TAB>id<TAB>cwd<TAB>display`. |
| `muxrail search --pick` | The `<prefix> h` popup: fzf over that list (found with `exec.LookPath`), then `plan`, then a new tmux window in the session's folder running `$SHELL -ic '<cmd>; exec $SHELL'` (`/bin/sh` when `SHELL` is unset). Cancelling fzf exits quietly. |
| `muxrail hook [--kind NAME] start\|working\|waiting\|done` | Records the agent's state for `$TMUX_PANE` on the server in `$TMUX`. Silent, always exits 0. |
| `muxrail plan <agent> <id> <cwd>` | Prints `cwd<TAB>shell command` that resumes the session. |
| `muxrail --once <cols>` (hidden, for tests) | Prints one sidebar frame for the current session and exits. |

## Data

### tmux

Every call shells out to `tmux` (no library). Windows come from one `list-panes -s -t <session_id> -F` call whose
fields are joined with `\x1f` (unit separator): `window_id window_index window_active pane_id pane_active @sidebar
pane_current_command pane_current_path window_name pane_title`.

- **Never trim the output with a whitespace trim.** `\x1f` counts as whitespace in some runtimes, and when the last
  field (a new window's pane title) is empty the trailing separator would be eaten and the line would have one field
  too few. Strip only the final `\n`. This bug already happened once.
- Panes with the pane option `@sidebar` = `1` are the sidebars themselves and are skipped.
- A window with several panes is represented by one pane: an agent pane beats the active pane, which beats the rest.

### Agent snapshot (pane id -> kind, status, since)

Built from two sources and cached in `$TMPDIR/tmux-sidebar-agents.json` for 3 seconds, shared by every sidebar
process (each window has its own sidebar process; do not make each one run the slow calls). Write the cache
atomically (temp file + rename). Cache miss or stale: rebuild and write.

1. **Process detection (kind).** `ps -axo tty=,stat=,comm=,args=`; keep foreground processes (`+` in stat) on a
   `tty*` terminal; map tty -> pane id from `tmux list-panes -a -F '#{pane_id} #{pane_tty}'` (strip `/dev/`).
   Classification, using the basename of `comm` and of the first word of `args` (ps truncates `comm` to 16 characters,
   so long paths only show in `args`):
   - `claude`, or a name that is a version number (`2.1.319`, tmux shows Claude by its version) -> `claude`
   - `codex` -> `codex`
   - `agy` or `gemini` -> `gemini`
   - first arg word is `agent` or `cursor-agent` **and** args contain `index.js` -> `cursor` (Cursor's CLI is node)
   - anything else (`node server.js`, `-zsh`) -> none
2. **Hook state (status, timing).** `muxrail hook [--kind NAME] start|working|waiting|done`, called from the agent's hooks,
   writes `{"status", "updated_ts" (epoch seconds), "kind"}` to
   `$XDG_STATE_HOME/muxrail/agents/<socket name>/<pane id without %>.json` (default `~/.local/state/muxrail/agents`) for the
   pane in `$TMUX_PANE`: temp file in the same directory, then rename. Pane ids restart at `%0` on every tmux server, so each
   server has its own folder; the socket name is the base name of the first comma-separated field of `$TMUX`
   (`<socket path>,<pid>,<session>`), and a name that is empty, `.`, `..` or holds a `/` is refused.
   `start` records an empty status. A repeated status keeps its `updated_ts`, so it is when the status began. The command
   ignores stdin, prints nothing, exits 0 whatever happens, does nothing without `$TMUX_PANE` or `$TMUX`, and removes the snapshot
   cache so the write shows on the next read. The snapshot merges the files by pane id; the process-detected kind wins,
   the file's `kind` fills in. The sidebar reads the folder of the server it queries (`tmux display-message -p '#{socket_path}'`) and deletes there
   only the files whose pane is not in `tmux list-panes -a`; another server's folder is never touched. No files, or no
   socket: carry on without status.

### Git stats per folder

Cached in `$TMPDIR/tmux-sidebar-git.json` (5 second TTL, entries older than 5 minutes pruned, atomic write), shared the
same way. For a path: branch = `symbolic-ref --short -q HEAD`, else `rev-parse --short HEAD`; not a repo -> none.
- **dirty** = sum of `git diff --numstat HEAD` (binary files print `-`: count 0).
- **committed** = `git diff --numstat <base>...HEAD`, base = `origin/HEAD` (from `symbolic-ref --short -q
  refs/remotes/origin/HEAD`) unless the current branch *is* the default branch, in which case base = `@{upstream}`
  (unpushed commits). No base -> 0/0.

## Rendering

All output uses ANSI/SGR (16 colours + the one 256-colour Claude orange `38;5;173`), no truecolor, so Warp's theme applies.

### Width rules

Terminal display width: combining marks 0, East Asian Wide/Fullwidth (emoji) 2, everything else 1 (Nerd Font
private-use glyphs are 1). Provide `width`, `clip` (to a limit with a trailing `…`), `fit` (cut styled text to N
columns without splitting an escape sequence; append SGR reset when cut), `pad`. **Every drawn row must be at most the
pane width**, or the terminal wraps and every later row shifts (this broke clicking once).

### Icons

Icons occupy a two-column cell so names line up: one-column glyph + space, or a two-column emoji with no space.

| Window | Icon | Colour |
|---|---|---|
| claude | U+EC82 | `38;5;173` |
| codex | U+EC81 | cyan |
| cursor | U+EC5C | bright white |
| gemini / agy | U+E7F0 | blue |
| other agent (process matches the version pattern but kind unknown) | locomotive emoji U+1F682 | magenta |
| plain window | train emoji U+1F686 | dim |

Brand glyphs are Nerd Font 3.5+ codicons. Emoji (not Nerd glyphs) are used for the defaults so they work in any terminal.

### State chip

For agent windows only: state glyph + (word when width >= 34) + time in that state.
Glyphs: working = braille spinner (frames `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`, 8 fps, cyan), waiting `◆` yellow, done `✔` green, unknown `○` grey.
Time = `ago(since)`: `now` under a minute, then `Nm`, `Nh`, `Nd`. No `since` -> no time.

### Layouts

Window cards, header `WINDOWS  N` (+ `   C-a b hides` from 28 columns) and a dim rule, footer hint on the last row
(`click · drag · j/k · J/K · v`, or `drop to place it there` while dragging). Layout is `tiles` (default) or `compact`,
stored in the tmux global option `@sidebar_layout`, read on each refresh, toggled by `v` (which sets the option).

**tiles**: a rounded box per window. Top border: `╭─⠿─` (grip, always) + optional label + `─…╮`; bottom `╰─…╯`. Body rows, only
those with content:
1. pill (`@cap_l` + index + `@cap_r`, coloured by index, read from tmux global options each refresh) + icon cell + bold
   name + state chip right-aligned. Name = window name; if the window is auto-named (a bare version number) and the pane
   title gives a task title, use the title.
2. Agents: the task title dim (the pane title with its leading status symbol removed; ignore titles that look like
   `user@host:path`); if there is no title and no git info, the short path. Plain windows: the short path
   (`~`-relative, left-ellipsised). Below 30 columns the chip moves here, in front of the title.
3. Git row when the folder is a repo: branch icon U+E0A0 + branch (clipped first when tight, min 6) + committed
   `+A -D` (green/red, `k` suffix from 1000: `1.3k`) + `✎` + dirty `+a -d`; zero parts omitted.
Border colour: green for the active window, white for the keyboard cursor, dim otherwise, magenta for the grabbed card.
Active window rows have a grey background.

**compact**: one line per window, no box: a 1-column marker bar (`▌` green active / white cursor / magenta grabbed /
space) + pill + icon + name + chip.

### Frame output

Write `ESC[H` then each line followed by `ESC[K` and `\r\n` (no screen clear: it flickers). Redraw only when the frame
text changed, plus a forced repaint every 5 s. Window data refreshes every 1 s; while any agent is `working` wake up
about every 0.2 s to animate the spinner.

## Input

Enter raw mode, hide the cursor, enable mouse tracking `ESC[?1002h` (button-motion) and SGR encoding `ESC[?1006h`;
restore everything (including termios) on exit. tmux forwards press, motion and release to the pane because the pane
enabled the mode.

- SGR events `ESC[<btn;x;yM|m`. Press = `btn 0` + `M`; motion while held = `btn 32` + `M`; release = `btn 0` + `m`.
- Buffer a trailing partial escape sequence until the next read.
- **Hit-testing uses each card's real height** (cards vary 3-5 rows, compact is 1) in the *real* order, never the drag
  preview order. Rows above the first card (header) hit nothing.
- Press on a card: select it and start a drag. Motion over another card: set the drop slot. Release: same card = switch
  to that window; different card = move; release off the list uses the last slot.
- **Live drag preview**: while dragging, render the list with the grabbed card moved to the drop slot, its pill showing the
  number it will get, magenta border and a `moving` label in its top border (compact: magenta marker). The real windows
  do not move until release.
- Keys: `j`/`k`/arrows move the cursor; `J`/`K` move the selected window down/up one; `Enter` switches; `v` toggles layout.
  The cursor snaps to the active window when the active window changes (by window id, so a reorder does not move it).
- Optional debug log: while `tmux show -gv @sidebar_debug` is `1`, append each raw input chunk and what it decided to
  `~/.tmux/sidebar-input.log`.

## tmux actions

- **Switch**: one tmux invocation: `select-window -t <session>:<index> ; if-shell -F -t <sidebar pane> '#{pane_active}'
  'last-pane -t <sidebar pane>'` (clicking focused the sidebar; give focus back to the previous pane, but only if the sidebar
  is the active pane).
- **Move** window `src` to the slot of `dst`: `move-window -a` (src above dst) or `-b` (src below), `-d` unless the moved
  window is the active one (otherwise the selection slides to a neighbour), `-s <src window id> -t <dst window id>`,
  chained with `; move-window -r -t <session id>` to close index gaps (`renumber-windows` does not run on moves).

## Sidebar control

`muxrail sidebar <action> <pane-id>` runs from the keys and hooks in `muxrail.tmux`:

- `toggle`: sets `@sidebar_on` to 1 and `ensure`s every window of the pane's session, or sets it to 0 and kills every pane marked `@sidebar`.
- `ensure`: does nothing unless `@sidebar_on` is 1. Under the per-window lock (directory `$TMPDIR/tmux-sidebar-<window_id>.lock`, taken
  with mkdir; a window that is locked is skipped), a window with no sidebar and at least 90 columns gets one:
  `split-window -hbdf -l <width>` running `<absolute binary path> sidebar`, marked with the pane option `@sidebar 1`. Any other window has its sidebar
  sized to the shared width. A stored `@sidebar_width` above 80 is ignored and the default applies. `MUXRAIL_BIN` overrides the path, which is otherwise `os.Executable`.
- `wake`: types `ESC [ 9 9 9 9 ~` into the sidebars of the window that came on screen.
- `remember`: stores the width of the window's sidebar as `@sidebar_width`. tmux names the pane under the mouse during a drag, so the
  sidebar is looked up in the window. Nothing is stored when the sidebar is the window's only pane or is wider than half the window:
  it got that width by filling the window, not by a drag.
- `apply`: `remember`, then resizes every sidebar of the session to `@sidebar_width` (default 34).

The sidebar process exits when its window has no other pane (`#{window_panes}` is 1 on two reads in a row, taken with the data it
already reads each tick), so tmux closes the window as it does for any window whose last pane exits.

## `tab`

Choose the window's pane like the sidebar does (agent pane > active pane, skip sidebars). Output for kind K:
- not `plain`: `#[fg=<tmux colour>]<glyph>#[fg=default]<pad>`; `plain` (the current tab has a coloured background):
  `<glyph><pad>`; `pad` completes the cell to two columns. tmux colours: claude `colour173`, codex `cyan`, cursor
  `brightwhite`, gemini `blue`, locomotive `magenta`, train `brightblack`.
- Uses the same agent snapshot and cache as the sidebar.

## `search` and `plan`

Sessions come from one reader per agent (`internal/sessions`), run concurrently. Every reader makes bounded reads only
(the first 512 KB of a transcript, the last 256 KB for Claude titles, mtime from `stat`), never a whole transcript, and a
file it cannot read is skipped and reported beside the list.

- claude: top-level `~/.claude/projects/<slug>/<id>.jsonl` (files below a session folder are subagents). Folder = first
  `cwd` field in the first 60 lines; time = mtime. A session needs a title or a `~/.claude/history.jsonl` entry (headless
  `claude -p` runs have neither). Title: `{"type":"custom-title","customTitle":..}` (a `/rename`) wins over
  `{"type":"ai-title","aiTitle":..}`, the last one in the tail counts, lines that are not real records (a title-looking string
  inside a message) are ignored. Prompts: the history entries with the session's `sessionId` (latest ten, newest first),
  else the first typed prompts of the transcript. Headless `ki delegate` folders (`-worktrees/j-\d{8}-`, `/ki/jobs/j-\d{8}-`)
  are skipped.
- codex: `~/.codex/sessions/YYYY/MM/DD/rollout-<ts>-<id>.jsonl`. The first record is `session_meta`: `payload.id`,
  `payload.cwd`, `payload.source`. Subagent threads (`source` is an object) and `codex exec` runs (`"exec"`) are skipped.
  Prompts: the first user messages, minus injected text (starting with `<`, or the `# AGENTS.md instructions` preamble).
- cursor (CLI): `~/.cursor/projects/<slug>/agent-transcripts/<id>/<id>.jsonl` that also has a CLI chat folder
  `~/.cursor/chats/*/<id>` (without one it is an IDE agent transcript `cursor-agent --resume` cannot open). Folder =
  `workspacePath` of `~/.cursor/projects/<slug>/.workspace-trusted`, else `cwd` of the chat's `meta.json`; a session with no
  folder is dropped, as are delegate folders. Legacy `<id>.txt` transcripts and `store.db` are not read. Prompts: the
  `<user_query>` text of the first user messages.

Sort by time, newest first. Display = coloured agent name, `●` (renamed, green) or `○` (generated/none), label (title, else
first prompt) padded to 46, folder name padded to 24, age, then two spaces and the prompts joined by ` | ` (clipped on screen by
fzf but still searched).

`plan`: the agent's program found with `exec.LookPath` (`claude`, `codex`, `cursor-agent`), args `--resume <id>` for claude and
cursor-agent, `resume <id>` for codex, run in the session's folder. Codex also gets `CODEX_SQLITE_HOME=~/.codex` unless
`~/.codex/config.toml` sets `sqlite_home`. Print `cwd<TAB>` + shell-quoted `[env K=V ...] program args...`. A missing
program or an unknown agent exits non-zero with a message.

## Acceptance

1. Go unit tests, table-driven, covering at least: width/clip/fit/pad (ASCII, emoji, combining, escape sequences), process
   classification (the cases above incl. truncated `comm`), numstat parsing, `kfmt`, git committed/dirty on a scratch repo
   (default branch with unpushed commits, feature branch, dirty tree, not a repo), card rendering at widths 18, 26 and 34
   (every row <= width; heights; row selection rules), compact rendering, hit-testing with mixed heights, drag preview order and
   pills, input parsing (press, motion, release, partial escapes, `j k J K v Enter`), `tab` output strings, the title
   scanner and `plan` output.
2. `make e2e` passes repeatedly (`e2e/drag_test.go` and `e2e/tabs_test.go` are polling-based; run it at least 3 times).
3. `go vet ./...`, `gofmt -l .` clean; the binary builds with `CGO_ENABLED=0`.

## Gotchas learned the hard way

- tmux `list-panes -F` and whitespace trims eat `\x1f` (see above).
- Hit-testing on fixed row counts broke as soon as cards changed height.
- Motion events arrive only with `?1002h`; plain `?1000h` gives press and release only.
- After a click the tmux client shows another window, whose sidebar is a different process; tests must read that one.
- `ps` `comm` is truncated; Claude shows as `claude` in ps but as its version number in tmux's `pane_current_command`.
- Emoji are 2 columns; pad one-column logos so names align.
- Do not clear the screen each frame; overwrite in place with `ESC[K` per line.
- A copied `/bin/sleep` is killed by macOS; use a symlink when faking agent processes in tests.
