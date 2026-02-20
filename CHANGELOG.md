# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]
### Added
- Added `TODO.md` implementation plan to make sessions from default tmux and Lisa project sockets visible in one view.
- Added multi-socket discovery and aggregation across default tmux plus Lisa socket glob targets.
- Added socket-aware CLI flags: `-include-default-socket`, `-include-lisa-sockets`, repeatable `-socket`, and `-socket-glob`.
- Added socket-focused unit tests for discovery, session keying collisions, routing, and UI hints.

### Changed
- Changed session identity to socket-qualified keys (`<socket>::<session>`) to avoid cross-socket name collisions.
- Changed refresh, capture, send, kill, and attach flows to always route actions to the correct tmux socket.
- Changed UI labels to show socket hints per session and socket/session totals in the status bar.

### Fixed
- Fixed explicit `-socket` targets being silently dropped when the socket file is missing; they are now preserved and surfaced in unavailable-socket errors.
- Fixed attach routing inside tmux to use `switch-client` only for the current socket and use interactive attach for cross-socket targets.
- Fixed interactive attach inheriting `TMUX`, which could block cross-socket attach flows.

## [1.1.0] - 2026-02-02
### Added
- Mode-specific focus colors for the focused pane to show active mode.

### Changed
- Multi-session layout favors horizontal splits.
- Compose mode sends keys to the focused pane in real time.
- Escape is forwarded to tmux; Ctrl+S exits modes.
