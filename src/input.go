package main

import (
	"context"
	"errors"
	"strings"

	"github.com/gdamore/tcell/v2"
)

func startCompose(state *appState) {
	state.composeActive = true
	state.selectTarget = false
	state.composeBuf = nil
}

func handleComposeKey(ctx context.Context, state *appState, cfg config, ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyEsc:
		state.composeActive = false
		state.selectTarget = false
		state.composeBuf = nil
		return true
	case tcell.KeyCtrlS:
		if len(state.sessions) == 0 {
			state.lastErr = "no tmux sessions"
			return true
		}
		state.composeActive = false
		state.selectTarget = true
		return true
	case tcell.KeyCtrlU:
		state.composeBuf = nil
		return true
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if len(state.composeBuf) > 0 {
			state.composeBuf = state.composeBuf[:len(state.composeBuf)-1]
		}
		return true
	case tcell.KeyEnter:
		state.composeBuf = append(state.composeBuf, '\n')
		return true
	case tcell.KeyCtrlC:
		state.composeActive = false
		state.selectTarget = false
		return true
	case tcell.KeyRune:
		r := ev.Rune()
		if r != 0 {
			state.composeBuf = append(state.composeBuf, r)
			return true
		}
	}
	_ = ctx
	_ = cfg
	return false
}

func handleSelectKey(ctx context.Context, state *appState, cfg config, ev *tcell.EventKey, screen tcell.Screen) bool {
	switch ev.Key() {
	case tcell.KeyEsc:
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
		state.selectTarget = false
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
	name := names[state.focusIndex]
	sess := state.sessions[name]
	paneID := sess.paneID
	if paneID == "" {
		var err error
		paneID, err = activePaneID(ctx, cfg, name)
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
	if err := sendKeysToPane(ctx, cfg, paneID, text); err != nil {
		return err
	}
	state.selectTarget = false
	state.composeBuf = nil
	return nil
}

func sendKeysToPane(ctx context.Context, cfg config, paneID string, text string) error {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != "" {
			if _, err := runTmux(ctx, cfg, "send-keys", "-t", paneID, "-l", line); err != nil {
				return err
			}
		}
		if i < len(lines)-1 {
			if _, err := runTmux(ctx, cfg, "send-keys", "-t", paneID, "Enter"); err != nil {
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
	name := names[state.focusIndex]
	if _, err := runTmux(ctx, cfg, "kill-session", "-t", name); err != nil {
		return err
	}
	state.focusName = ""
	state.focusIndex = 0
	return nil
}
