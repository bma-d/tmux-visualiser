package main

import "github.com/gdamore/tcell/v2"

func moveFocus(state *appState, delta int) {
	names := orderedSessionNames(*state)
	if len(names) == 0 {
		state.focusIndex = 0
		state.focusName = ""
		return
	}
	idx := state.focusIndex
	if idx < 0 || idx >= len(names) {
		idx = 0
	}
	idx = (idx + delta) % len(names)
	if idx < 0 {
		idx += len(names)
	}
	state.focusIndex = idx
	state.focusName = names[idx]
}

func scrollFocused(state *appState, screen tcell.Screen, delta int) {
	names := orderedSessionNames(*state)
	if len(names) == 0 {
		return
	}
	if state.focusIndex < 0 || state.focusIndex >= len(names) {
		state.focusIndex = 0
		state.focusName = names[0]
	}
	name := names[state.focusIndex]
	sess, ok := state.sessions[name]
	if !ok {
		return
	}
	contentHeight := contentHeightForIndex(len(names), state.focusIndex, screen)
	if contentHeight <= 0 {
		return
	}
	maxStart := 0
	if len(sess.lines) > contentHeight {
		maxStart = len(sess.lines) - contentHeight
	}
	current := state.scroll[name]
	if state.follow[name] {
		current = maxStart
	}
	next := current + delta
	if next < 0 {
		next = 0
	}
	if next > maxStart {
		next = maxStart
	}
	state.scroll[name] = next
	state.follow[name] = next == maxStart
}

func jumpScroll(state *appState, screen tcell.Screen, toTop bool) {
	names := orderedSessionNames(*state)
	if len(names) == 0 {
		return
	}
	if state.focusIndex < 0 || state.focusIndex >= len(names) {
		state.focusIndex = 0
		state.focusName = names[0]
	}
	name := names[state.focusIndex]
	sess, ok := state.sessions[name]
	if !ok {
		return
	}
	contentHeight := contentHeightForIndex(len(names), state.focusIndex, screen)
	if contentHeight <= 0 {
		return
	}
	maxStart := 0
	if len(sess.lines) > contentHeight {
		maxStart = len(sess.lines) - contentHeight
	}
	if toTop {
		state.scroll[name] = 0
		state.follow[name] = false
	} else {
		state.scroll[name] = maxStart
		state.follow[name] = true
	}
}
