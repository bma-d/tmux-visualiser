package main

import "time"

type config struct {
	lines                int
	interval             time.Duration
	cmdTimeout           time.Duration
	maxWorkers           int
	statusHeight         int
	allPanes             bool
	includeDefaultSocket bool
	includeLisaSockets   bool
	socketGlob           string
	explicitSockets      []string
}

type sessionView struct {
	key        string
	name       string
	socketPath string
	socketHint string
	paneID     string
	lines      []string
	updated    time.Time
}

type socketTarget struct {
	path string
	key  string
	hint string
}

type sessionRef struct {
	key    string
	name   string
	paneID string
	socket socketTarget
}

type appState struct {
	sessions      map[string]sessionView
	socketCount   int
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
