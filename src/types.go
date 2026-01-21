package main

import "time"

type config struct {
	lines        int
	interval     time.Duration
	cmdTimeout   time.Duration
	maxWorkers   int
	statusHeight int
}

type sessionView struct {
	name    string
	paneID  string
	lines   []string
	updated time.Time
}

type appState struct {
	sessions      map[string]sessionView
	lastErr       string
	serverDown    bool
	lastRefresh   time.Time
	scroll        map[string]int
	follow        map[string]bool
	focusIndex    int
	focusName     string
	composeActive bool
	selectTarget  bool
	sendKeyActive bool
	updatePrompt  bool
	updateVersion string
	composeBuf    []rune
	mouseEnabled  bool
}
