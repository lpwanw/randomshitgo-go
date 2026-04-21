# procs

A personal multi-project dev juggler — run all your services from a single terminal TUI.
`procs` keeps each project's logs in its own pane, lets you start/stop/restart individual
processes or named groups with a single keystroke, and gives you a full attach-mode bridge
so you can type directly into any child process.

## Features

- Single-binary, zero runtime dependencies (no Node.js, no Bun required)
- Terminal UI built with [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- Per-project log panes with scroll, filter, and rotation
- Named groups — boot the whole stack with one key
- Auto-restart on failure with configurable exponential backoff
- Attach mode: raw PTY bridge with `Ctrl-] Ctrl-]` to detach
- Git branch display per project
- Active port/socket display (processes that LISTEN show their port)
- Goroutine-safe; race-detector clean

## Install

**Go install (requires Go 1.22+):**
```sh
go install github.com/lpwanw/randomshitgo-go/cmd/procs@latest
```

**From source:**
```sh
git clone https://github.com/lpwanw/randomshitgo-go
cd randomshitgo-go
make install        # installs to /usr/local/bin/procs
```

**Direct binary (macOS/Linux):**
Download from [Releases](https://github.com/lpwanw/randomshitgo-go/releases), extract, and place on `$PATH`.

**Homebrew:** Tap coming in a future release.

## Quick Start

1. Create `~/.config/procs/config.yml`:

```yaml
projects:
  api:
    path: ~/code/myapi
    cmd: bun run dev
    restart: on-failure
  web:
    path: ~/code/myweb
    cmd: npm run dev
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
| `b` | Open branch picker |
| `/` | Filter logs |
| `PgUp` / `Ctrl-B` | Scroll log up |
| `PgDn` / `Ctrl-F` | Scroll log down |
| `g` | Scroll to top |
| `G` | Scroll to bottom |
| `?` | Toggle help overlay |
| `q` / `Ctrl-C` | Quit (stops all processes) |
| `Esc` | Cancel / close overlay |

## Attach Mode

Press `a` to attach to a running process. The terminal enters raw mode and all
input goes directly to the child PTY. To return to `procs`:

```
Press Ctrl-] twice (within 400ms)
```

This mirrors the telnet detach convention. The detach sequence is
`0x1d 0x1d` (`Ctrl-]` `Ctrl-]`).

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

## Differences from the TypeScript/Bun version

| Aspect | Bun original | Go port (this) |
|--------|-------------|----------------|
| Runtime | Bun + TypeScript | Single Go binary (CGO_ENABLED=0) |
| TUI framework | Ink (React) | Bubble Tea |
| Attach bridge | Node streams | `creack/pty` + raw mode |
| Distribution | `bun install` | Direct binary / `go install` |
| Windows support | Partial | PTY limited (no ConPTY yet) |

## License

MIT — see [LICENSE](LICENSE).
