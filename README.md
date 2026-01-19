# tmux-visualiser

A lightweight terminal UI that continuously discovers tmux sessions and renders live snapshots of each session's active pane. When there is one session, it fills the screen. When there are multiple, the screen is automatically split into a grid so you can see them all at once.

This avoids nested tmux clients by capturing pane contents via `tmux capture-pane` and drawing them directly.

## Requirements

- Go 1.21+ (any recent Go release is fine)
- tmux installed and on your PATH

## Run

```bash
go run .
```

Optional flags:

```bash
go run . -lines 300 -interval 500ms -cmd-timeout 1s -workers 4
```

Defaults are `-lines 500` and `-interval 1s`.

## Controls

- `q` / `Ctrl+C`: quit
- `r`: refresh immediately
- `+` / `-`: increase or decrease captured lines
- `Tab` / `Shift+Tab` (or `n` / `p`): change focused session
- `j` / `k` or arrow keys: scroll focused session
- `PageUp` / `PageDown`: scroll faster
- `Home` / `End`: jump to top or bottom

## How it works

- Polls `tmux list-sessions` to discover sessions.
- For each session, finds the active pane.
- Captures the last N lines from that pane.
- Lays out sessions in a grid that fills your terminal.

## Notes

- If no tmux server is running, the UI shows a message and keeps polling.
- The refresh interval is clamped to avoid excessive CPU usage.
- Captured output is bounded, so memory stays stable.
