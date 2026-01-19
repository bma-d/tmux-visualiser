package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

func updateState(ctx context.Context, state *appState, cfg config) {
	names, err := listSessions(ctx, cfg)
	state.lastRefresh = time.Now()
	if err != nil {
		state.lastErr = err.Error()
		if strings.Contains(strings.ToLower(err.Error()), "no server running") {
			state.serverDown = true
			state.sessions = map[string]sessionView{}
			return
		}
		state.serverDown = false
		return
	}
	state.lastErr = ""
	state.serverDown = false

	newSessions := make(map[string]sessionView, len(names))
	keepScroll := make(map[string]int, len(names))
	keepFollow := make(map[string]bool, len(names))
	if len(names) == 0 {
		state.sessions = newSessions
		state.scroll = keepScroll
		state.follow = keepFollow
		state.focusIndex = 0
		state.focusName = ""
		return
	}

	workers := cfg.maxWorkers
	if workers > len(names) {
		workers = len(names)
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, name := range names {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			paneID, err := activePaneID(ctx, cfg, name)
			if err != nil {
				mu.Lock()
				newSessions[name] = sessionView{name: name, paneID: "", lines: []string{err.Error()}, updated: time.Now()}
				mu.Unlock()
				return
			}

			lines, err := capturePane(ctx, cfg, paneID, cfg.lines)
			if err != nil {
				lines = []string{err.Error()}
			}

			mu.Lock()
			newSessions[name] = sessionView{name: name, paneID: paneID, lines: lines, updated: time.Now()}
			mu.Unlock()
		}()
	}

	wg.Wait()
	state.sessions = newSessions
	for _, name := range names {
		keepScroll[name] = state.scroll[name]
		if _, ok := state.follow[name]; ok {
			keepFollow[name] = state.follow[name]
		} else {
			keepFollow[name] = true
		}
	}
	state.scroll = keepScroll
	state.follow = keepFollow
	state.focusIndex = focusIndexForName(names, state.focusName)
	if state.focusIndex < 0 || state.focusIndex >= len(names) {
		state.focusIndex = 0
		state.focusName = names[0]
	} else {
		state.focusName = names[state.focusIndex]
	}
}

func listSessions(ctx context.Context, cfg config) ([]string, error) {
	out, err := runTmux(ctx, cfg, "list-sessions", "-F", "#S")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return []string{}, nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	sessions := make([]string, 0, len(lines))
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name != "" {
			sessions = append(sessions, name)
		}
	}
	sort.Strings(sessions)
	return sessions, nil
}

func activePaneID(ctx context.Context, cfg config, session string) (string, error) {
	out, err := runTmux(ctx, cfg, "list-panes", "-t", session, "-F", "#{pane_active} #{pane_id}")
	if err != nil {
		return "", err
	}
	var fallback string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fallback == "" {
			fallback = fields[1]
		}
		if fields[0] == "1" {
			return fields[1], nil
		}
	}
	if fallback == "" {
		return "", errors.New("no pane found")
	}
	return fallback, nil
}

func capturePane(ctx context.Context, cfg config, paneID string, lines int) ([]string, error) {
	if lines < 1 {
		lines = 1
	}
	rangeArg := fmt.Sprintf("-%d", lines)
	out, err := runTmux(ctx, cfg, "capture-pane", "-t", paneID, "-p", "-e", "-S", rangeArg)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return []string{"(empty)"}, nil
	}
	result := strings.Split(out, "\n")
	if len(result) > 0 && result[len(result)-1] == "" {
		result = result[:len(result)-1]
	}
	return result, nil
}
