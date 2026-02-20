package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
)

func startCompose(state *appState) {
	state.composeActive = true
	state.selectTarget = false
	state.sendKeyActive = false
	state.composeBuf = nil
}

func isCtrlS(ev *tcell.EventKey) bool {
	if ev.Key() == tcell.KeyCtrlS {
		return true
	}
	if ev.Key() == tcell.KeyRune && ev.Modifiers()&tcell.ModCtrl != 0 {
		return ev.Rune() == 's' || ev.Rune() == 'S'
	}
	return false
}

func startSendKey(state *appState) {
	state.sendKeyActive = true
	state.composeActive = false
	state.selectTarget = false
}

func handleComposeKey(ctx context.Context, state *appState, cfg config, ev *tcell.EventKey) bool {
	if isCtrlS(ev) {
		state.composeActive = false
		state.selectTarget = false
		state.composeBuf = nil
		return true
	}
	key, literal, ok := tmuxKeyFromEvent(ev)
	if !ok {
		return false
	}
	if err := sendKeyToFocused(ctx, state, cfg, key, literal); err != nil {
		state.lastErr = err.Error()
	}
	return true
}

func handleSelectKey(ctx context.Context, state *appState, cfg config, ev *tcell.EventKey, screen tcell.Screen) bool {
	switch ev.Key() {
	case tcell.KeyEsc:
		if err := sendKeyToFocused(ctx, state, cfg, "Escape", false); err != nil {
			state.lastErr = err.Error()
		}
		return true
	case tcell.KeyCtrlS:
		state.selectTarget = false
		return true
	case tcell.KeyEnter:
		if err := sendComposeToFocused(ctx, state, cfg); err != nil {
			state.lastErr = err.Error()
		}
		return true
	case tcell.KeyTAB:
		moveFocus(state, 1)
		return true
	case tcell.KeyBacktab:
		moveFocus(state, -1)
		return true
	case tcell.KeyUp:
		moveFocus(state, -1)
		return true
	case tcell.KeyDown:
		moveFocus(state, 1)
		return true
	case tcell.KeyCtrlC:
		if err := sendKeyToFocused(ctx, state, cfg, "C-c", false); err != nil {
			state.lastErr = err.Error()
		}
		return true
	case tcell.KeyRune:
		r := ev.Rune()
		if r >= '1' && r <= '9' {
			names := orderedSessionNames(*state)
			idx := int(r - '1')
			if idx >= 0 && idx < len(names) {
				state.focusIndex = idx
				state.focusName = names[idx]
			}
			return true
		}
		if r == 'n' || r == 'N' {
			moveFocus(state, 1)
			return true
		}
		if r == 'p' || r == 'P' {
			moveFocus(state, -1)
			return true
		}
	}
	_ = screen
	return false
}

func handleSelectMouse(ctx context.Context, state *appState, cfg config, ev *tcell.EventMouse, screen tcell.Screen) bool {
	if ev.Buttons()&tcell.Button1 == 0 {
		return false
	}
	x, y := ev.Position()
	idx := sessionIndexAt(screen, len(state.sessions), x, y)
	if idx < 0 {
		return false
	}
	names := orderedSessionNames(*state)
	if idx >= len(names) {
		return false
	}
	state.focusIndex = idx
	state.focusName = names[idx]
	if err := sendComposeToFocused(ctx, state, cfg); err != nil {
		state.lastErr = err.Error()
	}
	return true
}

func sendComposeToFocused(ctx context.Context, state *appState, cfg config) error {
	names := orderedSessionNames(*state)
	if len(names) == 0 {
		state.selectTarget = false
		return errors.New("no tmux sessions")
	}
	if state.focusIndex < 0 || state.focusIndex >= len(names) {
		state.focusIndex = 0
	}
	key := names[state.focusIndex]
	sess := state.sessions[key]
	paneID := sess.paneID
	if paneID == "" {
		var err error
		paneID, err = activePaneID(ctx, cfg, sess.socketPath, sess.name)
		if err != nil {
			return err
		}
	}
	text := string(state.composeBuf)
	if text == "" {
		state.selectTarget = false
		state.composeBuf = nil
		return nil
	}
	if err := sendKeysToPane(ctx, cfg, sess.socketPath, paneID, text); err != nil {
		return err
	}
	state.selectTarget = false
	state.composeBuf = nil
	return nil
}

func sendKeysToPane(ctx context.Context, cfg config, socketPath string, paneID string, text string) error {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			if _, err := runTmuxOnSocketFn(ctx, cfg, socketPath, "send-keys", "-t", paneID, "-l", line); err != nil {
				return err
			}
		}
		if i < len(lines)-1 {
			if _, err := runTmuxOnSocketFn(ctx, cfg, socketPath, "send-keys", "-t", paneID, "Enter"); err != nil {
				return err
			}
		}
	}
	return nil
}

func killFocusedSession(ctx context.Context, state *appState, cfg config) error {
	names := orderedSessionNames(*state)
	if len(names) == 0 {
		return errors.New("no tmux sessions")
	}
	if state.focusIndex < 0 || state.focusIndex >= len(names) {
		state.focusIndex = 0
	}
	key := names[state.focusIndex]
	sess := state.sessions[key]
	if _, err := runTmuxOnSocketFn(ctx, cfg, sess.socketPath, "kill-session", "-t", sess.name); err != nil {
		return err
	}
	state.focusName = ""
	state.focusIndex = 0
	return nil
}

func handleSendKey(ctx context.Context, state *appState, cfg config, ev *tcell.EventKey) bool {
	if isCtrlS(ev) {
		state.sendKeyActive = false
		return true
	}
	key, literal, ok := tmuxKeyFromEvent(ev)
	if !ok {
		return false
	}
	if err := sendKeyToFocused(ctx, state, cfg, key, literal); err != nil {
		state.lastErr = err.Error()
	}
	state.sendKeyActive = false
	return true
}

func tmuxKeyFromEvent(ev *tcell.EventKey) (string, bool, bool) {
	switch ev.Key() {
	case tcell.KeyEsc:
		return "Escape", false, true
	case tcell.KeyRune:
		r := ev.Rune()
		if r == 0 {
			return "", false, false
		}
		if ev.Modifiers()&tcell.ModCtrl != 0 {
			if r >= 'a' && r <= 'z' {
				return fmt.Sprintf("C-%c", r), false, true
			}
			if r >= 'A' && r <= 'Z' {
				return fmt.Sprintf("C-%c", unicode.ToLower(r)), false, true
			}
			if r == ' ' {
				return "C-Space", false, true
			}
		}
		if ev.Modifiers()&tcell.ModAlt != 0 {
			if r >= 'A' && r <= 'Z' {
				r = unicode.ToLower(r)
			}
			return fmt.Sprintf("M-%c", r), false, true
		}
		return string(r), true, true
	case tcell.KeyEnter:
		return "Enter", false, true
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		return "BSpace", false, true
	case tcell.KeyTab:
		return "Tab", false, true
	case tcell.KeyUp:
		return "Up", false, true
	case tcell.KeyDown:
		return "Down", false, true
	case tcell.KeyLeft:
		return "Left", false, true
	case tcell.KeyRight:
		return "Right", false, true
	case tcell.KeyPgUp:
		return "PgUp", false, true
	case tcell.KeyPgDn:
		return "PgDn", false, true
	case tcell.KeyHome:
		return "Home", false, true
	case tcell.KeyEnd:
		return "End", false, true
	case tcell.KeyInsert:
		return "Insert", false, true
	case tcell.KeyDelete:
		return "DC", false, true
	case tcell.KeyF1:
		return "F1", false, true
	case tcell.KeyF2:
		return "F2", false, true
	case tcell.KeyF3:
		return "F3", false, true
	case tcell.KeyF4:
		return "F4", false, true
	case tcell.KeyF5:
		return "F5", false, true
	case tcell.KeyF6:
		return "F6", false, true
	case tcell.KeyF7:
		return "F7", false, true
	case tcell.KeyF8:
		return "F8", false, true
	case tcell.KeyF9:
		return "F9", false, true
	case tcell.KeyF10:
		return "F10", false, true
	case tcell.KeyF11:
		return "F11", false, true
	case tcell.KeyF12:
		return "F12", false, true
	}
	if ev.Key() >= tcell.KeyCtrlA && ev.Key() <= tcell.KeyCtrlZ {
		offset := int(ev.Key() - tcell.KeyCtrlA)
		return fmt.Sprintf("C-%c", rune('a'+offset)), false, true
	}
	if ev.Key() == tcell.KeyCtrlSpace {
		return "C-Space", false, true
	}
	return "", false, false
}

func sendKeyToFocused(ctx context.Context, state *appState, cfg config, key string, literal bool) error {
	names := orderedSessionNames(*state)
	if len(names) == 0 {
		return errors.New("no tmux sessions")
	}
	if state.focusIndex < 0 || state.focusIndex >= len(names) {
		state.focusIndex = 0
	}
	keyName := names[state.focusIndex]
	sess := state.sessions[keyName]
	paneID := sess.paneID
	if paneID == "" {
		var err error
		paneID, err = activePaneID(ctx, cfg, sess.socketPath, sess.name)
		if err != nil {
			return err
		}
	}
	if literal {
		_, err := runTmuxOnSocketFn(ctx, cfg, sess.socketPath, "send-keys", "-t", paneID, "-l", key)
		return err
	}
	_, err := runTmuxOnSocketFn(ctx, cfg, sess.socketPath, "send-keys", "-t", paneID, key)
	return err
}

func connectFocused(ctx context.Context, state *appState, cfg config, screen tcell.Screen) (bool, error) {
	names := orderedSessionNames(*state)
	if len(names) == 0 {
		return false, errors.New("no tmux sessions")
	}
	if state.focusIndex < 0 || state.focusIndex >= len(names) {
		state.focusIndex = 0
	}
	key := names[state.focusIndex]
	sess := state.sessions[key]
	paneID := sess.paneID
	if paneID == "" {
		var err error
		paneID, err = activePaneID(ctx, cfg, sess.socketPath, sess.name)
		if err != nil {
			return false, err
		}
	}

	if canSwitchClient(sess.socketPath) {
		if _, err := runTmuxOnSocketFn(ctx, cfg, sess.socketPath, "switch-client", "-t", sess.name, ";", "select-pane", "-t", paneID); err != nil {
			return false, err
		}
		return true, nil
	}

	if err := screen.Suspend(); err != nil {
		return false, err
	}
	if err := runTmuxInteractiveOnSocketFn(sess.socketPath, "attach-session", "-t", sess.name, ";", "select-pane", "-t", paneID); err != nil {
		fmt.Fprintln(os.Stderr, "tmux attach failed:", err)
		return true, err
	}
	return true, nil
}
