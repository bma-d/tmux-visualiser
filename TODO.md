# TODO: Show All Lisa tmux Sessions

## Goal
Make `tmux-visualiser` show sessions from:
- default tmux server socket
- all Lisa project sockets (`/tmp/lisa-tmux-*-*.sock`)

Current blocker: code uses plain `tmux ...` calls only (no `-S socket`), so only one server is visible.

## Current Code Hotspots
- `src/tmux.go`: `runTmux`, `runTmuxInteractive`
- `src/state.go`: `listSessions`, `activePaneID`, `capturePane`
- `src/input.go`: send/kill/attach paths use tmux commands
- `src/types.go`: session model lacks socket identity
- `src/ui.go`: header does not distinguish same-name sessions across sockets
- `src/main.go`: no socket-selection flags

## Implementation Plan

### 1. Add socket-aware command execution
- [ ] Add `runTmuxOnSocket(ctx, cfg, socket string, args ...string)`
- [ ] Add `runTmuxInteractiveOnSocket(socket string, args ...string)`
- [ ] Keep current wrappers as default-socket convenience wrappers

### 2. Add socket discovery
- [ ] Add discovery function returning ordered socket targets
- [ ] Include default socket target
- [ ] Include existing Lisa sockets via glob `/tmp/lisa-tmux-*-*.sock`
- [ ] Ignore missing/stale sockets without failing whole refresh
- [ ] Dedupe socket paths

### 3. Make session identity socket-qualified
- [ ] Introduce stable key format: `<socket>::<session>`
- [ ] Update state map keys to use qualified key (avoid name collisions)
- [ ] Keep display name readable (session name + short socket tag)

### 4. Query all sockets each refresh
- [ ] For each socket: run `list-sessions -F #S`
- [ ] Merge into one session list
- [ ] For each merged session: run pane discovery/capture on its socket
- [ ] Preserve worker pool behavior for concurrency control

### 5. Route actions to correct socket
- [ ] `killFocusedSession`: use target socket
- [ ] compose/send-key paths: use target socket
- [ ] attach/connect: `tmux -S <socket> attach-session ...`

### 6. UI clarity for multi-socket sessions
- [ ] Header should show socket hint (short hash or basename)
- [ ] Status bar should show socket count + session count
- [ ] Keep layout behavior unchanged

### 7. Error semantics
- [ ] Treat per-socket "no server running"/missing-socket as non-fatal
- [ ] Show fatal/global errors in status bar only when no sockets succeed

### 8. CLI flags
- [ ] Add `-include-default-socket` (default true)
- [ ] Add `-include-lisa-sockets` (default true)
- [ ] Add optional repeatable `-socket` for explicit sockets
- [ ] Add optional `-socket-glob` override (default `/tmp/lisa-tmux-*-*.sock`)

### 9. Tests
- [ ] Unit: socket discovery (dedupe, stale paths, ordering)
- [ ] Unit: merged session keying + collision handling
- [ ] Unit: list/pane/capture uses correct socket
- [ ] Unit: action routing (kill/send/attach) uses correct socket
- [ ] UI: header includes socket hint

### 10. Docs
- [ ] Update `README.md` with multi-socket behavior and flags
- [ ] Add troubleshooting note: Lisa uses per-project sockets

## Acceptance Criteria
- [ ] If two separate Lisa projects each have active sessions, one visualiser instance shows all of them.
- [ ] If a session name exists on two sockets, both appear and remain independently controllable.
- [ ] Focus/send/kill/attach always affect the intended session/socket pair.
- [ ] Stale Lisa sockets do not break refresh loop or blank the UI.

## Manual Verification Script
- [ ] Start default tmux session (`tmux new -d -s plain-test`)
- [ ] Start Lisa session in project A (`./lisa session spawn ...`)
- [ ] Start Lisa session in project B (`./lisa session spawn ...`)
- [ ] Run visualiser and confirm all 3 sessions visible
- [ ] Send key + kill from visualiser to each target and verify effects
