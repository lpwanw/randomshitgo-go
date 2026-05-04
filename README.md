# procs

A personal multi-project dev juggler — run all your services from a single terminal TUI.
`procs` keeps each project's logs in its own pane, lets you start/stop/restart individual
processes or named groups with a single keystroke, and gives you a full attach-mode bridge
so you can type directly into any child process.

## Features

- Single static binary, zero runtime dependencies
- Terminal UI built with [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- Per-project log panes with scroll, filter, and rotation
- Named groups — boot the whole stack with one key
- Auto-restart on failure with configurable exponential backoff
- Attach mode: raw PTY bridge with `Ctrl-] Ctrl-]` to detach; OS-native paste (Cmd-V / Ctrl-Shift-V / Shift-Insert) is forwarded to the child as bracketed paste when the child enabled mode 2004
- Mouse drag-select on the log pane → release auto-copies to system clipboard. Wheel still scrolls. Hold Option (macOS Terminal/iTerm) or Shift (most Linux terminals) to fall back to the terminal's native selection across borders/scrollbar
- Git branch display per project + in-TUI branch picker with live filter, plus `:fetch` / `:pull --ff-only`
- Active port/socket display (processes that LISTEN show their port)
- Per-process CPU% and memory (RSS) in the status bar (whole-process-tree sum; updates every 2 s). CPU can exceed 100% on multi-core workloads — matches `htop` convention.
- Goroutine-safe; race-detector clean

## Install

### Go install (requires Go 1.22+)
```sh
go install github.com/lpwanw/randomshitgo-go/cmd/procs@latest
```
Binary lands in `$(go env GOBIN)` — falls back to `$(go env GOPATH)/bin`.
Make sure that directory is on your `PATH`.

### From source
```sh
git clone https://github.com/lpwanw/randomshitgo-go
cd randomshitgo-go
make install                  # default PREFIX=/usr/local → /usr/local/bin/procs
# may need sudo for /usr/local; or install to a user-writable prefix:
make install PREFIX=$HOME/.local
```

### Direct binary (macOS/Linux)
Download the archive for your OS/arch from
[Releases](https://github.com/lpwanw/randomshitgo-go/releases), extract, and
place `procs` on your `PATH`.

**Homebrew:** tap coming in a future release.

## Update

| Installed via | Command |
|---------------|---------|
| `go install`  | `go install github.com/lpwanw/randomshitgo-go/cmd/procs@latest` |
| source checkout | `make update` (runs `git pull --ff-only && make install`) |
| direct binary | re-download the latest release and overwrite the binary |

## Uninstall

| Installed via | Command |
|---------------|---------|
| `go install`  | `rm "$(command -v procs)"` (typically `$GOPATH/bin/procs`) |
| source checkout | `make uninstall` (matches the `PREFIX` you installed with) |
| direct binary | delete the binary you placed on `PATH` |

Remove your config and cached logs if you don't want them around:
```sh
rm -rf ~/.config/procs ~/.cache/procs
```

## Quick Start

1. Create `~/.config/procs/config.yml`:

```yaml
projects:
  api:
    path: ~/code/myapi
    cmd: go run ./cmd/api
    restart: on-failure
  web:
    path: ~/code/myweb
    cmd: ./scripts/dev
    restart: on-failure

groups:
  fullstack: [api, web]
```

2. Run `procs`. The TUI starts with all projects listed in the sidebar.

## Config

Full reference in [`examples/config.yml`](examples/config.yml).

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `projects.<id>.path` | string | required | Working directory for the process |
| `projects.<id>.cmd` | string | required | Shell command to run |
| `projects.<id>.restart` | `never` / `on-failure` | `never` | Restart policy |
| `projects.<id>.env` | map[string]string | `{}` | Inline env vars injected into the child. Quote numeric values in YAML (`PORT: "8080"`) |
| `projects.<id>.env_file` | string | — | Path to a `KEY=VALUE` env file (supports `#` comments, `export` prefix, quoted values). Merged under inline `env:` — inline wins on conflict |
| `groups.<name>` | `[id, ...]` | — | Named group for batch start |
| `settings.log_buffer_lines` | int | 1000 | Per-project in-memory ring size |
| `settings.log_dir` | string | `~/.cache/procs/logs` | Log file directory |
| `settings.log_rotate_size_mb` | int | 10 | Rotate log file after N MB |
| `settings.log_rotate_keep` | int | 5 | Rotated log files to keep |
| `settings.shutdown_grace_ms` | int | 5000 | Grace period before SIGKILL |
| `settings.group_start_delay_ms` | int | 300 | Delay between group member starts |
| `settings.restart_backoff_ms` | `[int, ...]` | `[1000,2000,4000,8000,16000]` | Backoff schedule (ms) |
| `settings.restart_max_attempts` | int | 5 | Max restart attempts |
| `settings.pty_cols` / `pty_rows` | int | 120 / 40 | PTY dimensions |

## Keybindings

| Key | Action |
|-----|--------|
| `k` / `↑` | Select previous process |
| `j` / `↓` | Select next process |
| `s` | Start selected process |
| `r` | Restart selected process |
| `x` | Stop selected process |
| `X` | Stop all processes |
| `a` | Attach to selected process (raw PTY) |
| `S` | Open group picker → start group |
| `c` / `b` | Open branch picker (type to filter; arrows to navigate; `Enter` checks out; `Esc` clears filter then closes) |
| `f` | `git fetch --prune` for the selected project (async; toast on result) |
| `p` | `git pull --ff-only` for the selected project — never merges |
| `/` | Search logs (vim-style; matches highlighted inline) |
| `n` / `N` | Jump to next / previous search match |
| `1`–`9` | Quick-jump to project 1–9 in the sidebar |
| `PgUp` / `Ctrl-B` | Scroll log up |
| `PgDn` / `Ctrl-F` | Scroll log down |
| `g` | Scroll to top |
| `G` | Scroll to bottom |
| `Tab` | Enter **log focus** — hand the keyboard to the log pane for vim-style nav + yank (double-Esc to return) |
| `:set nu` / `:set nonu` | Toggle line-number gutter in the log panel |
| `:set sev` / `:set nosev` | Toggle severity colouring (`ERROR`/`WARN`/`INFO`/`DEBUG` → fg colour). On by default |
| `:set json` / `:set nojson` | Toggle pretty-print for single-line JSON log lines |
| `:set sql` / `:set nosql` | Toggle keyword-aware SQL formatting (handles Rails/Sequel/GORM prefixes) |
| `:set wrap` / `:set nowrap` | Toggle hard-wrap of long lines at viewport width. On by default |
| `:clear` / `:c` / `Ctrl-L` | Empty the in-memory log buffer for the selected project (files on disk untouched) |
| `:w {path}` | Dump the currently visible log buffer to `{path}` (supports `~` / `$VAR`) |
| `:fetch` | `git fetch --prune` the selected project (async; result toasted) |
| `:pull` | `git pull --ff-only` the selected project — never merges; refuses diverged branches |
| `?` | Toggle help overlay |
| `:` | Open command bar (`:q` to quit) |
| `Ctrl-C` | Quit — press twice within 2 s to confirm |
| `Esc` | Cancel / close overlay |

### Log focus (vim motions + copy)

Press `Tab` to hand the keyboard to the log pane. The sidebar dims, a
cursor appears, line numbers show, and all process-control keys become
inert — only vim motions and yank commands are active. Focus persists
across multiple yanks; to return to process-switching press **`Esc`
twice** within 2 seconds (mirrors the double-Ctrl-C quit pattern).

Counts work in front of any motion or operator: `3w`, `2yy`, `y3e`,
`3f.`. The command buffer shows up in the status bar while you type
(`3yi` …) so multi-key commands are discoverable.

**Motions:**

| Key | Moves to |
|-----|----------|
| `h j k l` / arrows | char / line |
| `w W` | next word / WORD |
| `b B` | prev word / WORD |
| `e E` | end of (WORD-)word forward |
| `ge gE` | end of (WORD-)word backward |
| `0 ^ $` | line start / first-non-blank / end |
| `+ -` | next / prev line first-non-blank |
| `gg G` | buffer top / bottom |
| `Space` | pause / resume sticky auto-scroll |
| `Ctrl-u / d` | half-page up / down |
| `Ctrl-b / f` | full-page up / down |
| `H M L` | viewport top / mid / bottom |
| `f{c} F{c}` | find char forward / backward (current line) |
| `t{c} T{c}` | till char (stop one before) |
| `; ,` | repeat last find (same / reverse direction) |
| `/` `n` `N` | filter bar; cursor-jump to next / prev match |

**Visual + yank:**

| Key | Action |
|-----|--------|
| `v V`         | char / line visual |
| `y` (visual)  | yank selection → clipboard |
| `yy` / `Y`    | yank current line (count: `3yy`) |
| `y{motion}`   | yank to motion target (`yw`, `y3e`, `yf.`, `y$`) |

**Text objects (current-line scoped):**

| Object | Meaning |
|--------|---------|
| `iw aw` / `iW aW` | inner / around word (WORD) |
| `` i" a" / i' a' / i` a` `` | inner / around quote pair |
| `i( a( i) a)` | inner / around parens |
| `i[ a[ i] a]` | inner / around brackets |
| `i{ a{ i} a}` | inner / around braces |
| `i< a< i> a>` | inner / around angle brackets |

**Exit:** first `Esc` cancels any pending operator, count, or
selection; second `Esc` within 2 s returns to sidebar / process
switching. The status label flips `NORMAL` → `LOG` → `COPY` as focus
and selection change. The line-number gutter is always shown while log
focus is active; outside focus, toggle it via `:set nu` / `:set nonu`
from the command bar.

## Attach Mode

Press `a` to attach to a running process. The terminal enters raw mode and all
input goes directly to the child PTY. To return to `procs`:

```
Press Ctrl-] twice (within 400ms)
```

This mirrors the telnet detach convention. The detach sequence is
`0x1d 0x1d` (`Ctrl-]` `Ctrl-]`).

**Paste:** use your terminal's native paste (Cmd-V on macOS, Ctrl-Shift-V or
Shift-Insert on Linux). The bracketed-paste sequence is forwarded to the
child PTY; if the child enabled DEC mode 2004 (`?2004h`), it sees the
content wrapped in `\x1b[200~` … `\x1b[201~` so shells treat it as data
instead of typed commands. Newlines are normalised to CR (the universal
"Enter").

## Troubleshooting

**No config file found:**
```
procs: config file not found at "/Users/you/.config/procs/config.yml"
```
Create the file at that path (see Quick Start), or pass `-c /path/to/config.yml`.

**Port not shown in sidebar:**
The port column only shows after the process has opened a listening socket.
Wait a moment for the process to bind, then the display updates automatically.

**Git branch not shown:**
The project's `path` must be inside a Git repository. If `git` is not on
`$PATH` or the directory is not a repo, the branch column is left blank.

**Process does not restart:**
Check that `restart: on-failure` is set in your config. `restart: never`
(the default) means the process stays stopped after it exits.

## License

MIT — see [LICENSE](LICENSE).
