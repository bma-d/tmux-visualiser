# tmux-visualiser

A lightweight terminal UI that continuously discovers tmux sessions and renders live snapshots of each session's active pane. When there is one session, it fills the screen. When there are multiple, the screen is automatically split into a grid so you can see them all at once.

This avoids nested tmux clients by capturing pane contents via `tmux capture-pane` and drawing them directly.

## Requirements

- Go 1.21+ (any recent Go release is fine)
- tmux installed and on your PATH

## Run

```bash
go run ./src
```

## Install

### Homebrew (macOS/Linux)

```bash
brew install bma-d/tap/tmux-visualiser
```

### Scoop (Windows)

```powershell
scoop bucket add bma-d https://github.com/bma-d/scoop-bucket
scoop install tmux-visualiser
```

### Debian/Ubuntu (.deb)

```bash
sudo dpkg -i tmux-visualiser_*.deb
```

### Fedora/RHEL (.rpm)

```bash
sudo rpm -i tmux-visualiser_*.rpm
```

### Alpine (.apk)

```bash
sudo apk add --allow-untrusted tmux-visualiser_*.apk
```

### Go install

```bash
go install github.com/bma-d/tmux-visualiser@latest
```

### Manual download

Download the archive for your OS/arch from the Releases page and extract it to a directory on your PATH.

Optional flags:

```bash
go run ./src -lines 300 -interval 500ms -cmd-timeout 1s -workers 4 \
  -include-default-socket=true \
  -include-lisa-sockets=true \
  -socket /tmp/custom.sock \
  -socket-glob '/tmp/lisa-tmux-*-*.sock'
```

Defaults are `-lines 500` and `-interval 1s`.

## Controls

- `q` / `Ctrl+C`: quit
- `r`: refresh immediately
- `+` / `-`: increase or decrease captured lines
- `[` / `]`: decrease or increase refresh interval
- `m`: toggle mouse capture (enable scroll + click vs. allow terminal text selection)
- `Ctrl+K`: kill focused tmux session
- `Enter`: attach to focused session (exits the visualiser)
- `s`: send a single key to the focused pane (supports `Enter`, `Backspace`, `Ctrl+C`, etc.)
- `Tab` / `Shift+Tab` (or `n` / `p`): change focused session
- `j` / `k` or arrow keys: scroll focused session
- `PageUp` / `PageDown`: scroll faster
- `Home` / `End`: jump to top or bottom
- `i`: compose input (live send to focused pane; `Enter` inserts newline; `Ctrl+S` exits)
- `s`: send a single key (press the key; `Ctrl+S` cancels)
- `Esc` in modes sends Escape to tmux; use `Ctrl+S` to exit a mode
- When an update prompt appears: `U` to update, `I` to ignore for 7 days, `Ctrl+S` to dismiss

## How it works

- Discovers sockets from:
  - default tmux socket (enabled by `-include-default-socket`)
  - explicit `-socket` flags (repeatable)
  - glob matches from `-socket-glob` (enabled by `-include-lisa-sockets`)
- Polls each socket via `tmux -S <socket> list-sessions`.
- For each session, finds the active pane on that same socket.
- Captures the last N lines from that pane.
- Lays out sessions in a grid that fills your terminal.

## Notes

- If no tmux server is running, the UI shows a message and keeps polling.
- Stale/missing Lisa sockets are ignored and do not stop refresh.
- Session identity is socket-qualified, so duplicate session names across sockets are shown independently.
- The refresh interval is clamped to avoid excessive CPU usage.
- Captured output is bounded, so memory stays stable.

## Troubleshooting

- Lisa uses per-project tmux sockets. If your Lisa sessions are missing, verify:
  - `-include-lisa-sockets=true`
  - `-socket-glob` matches your Lisa socket pattern

## Changelog

### Unreleased

- (none)

### 1.1.0 - 2026-02-02

- Compose now sends keys to the focused pane in real time, matching tmux more closely.
- Escape is forwarded to tmux; Ctrl+S exits modes.
- Multi-session layout favors horizontal splits.
- Focused pane color changes by mode for clear visual feedback.

### 1.0.2 - 2026-01-26

- Prefer taller grids when the layout would otherwise be wider than it is tall.

### 1.0.0 - 2026-01-20

- Initial release.
