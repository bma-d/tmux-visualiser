package main

import (
	"context"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

type suspendTestScreen struct {
	tcell.Screen
	suspended bool
}

func (s *suspendTestScreen) Suspend() error {
	s.suspended = true
	return nil
}

func TestActionRoutingUsesSessionSocket(t *testing.T) {
	socketPath := "/tmp/lisa-a.sock"
	state := appState{
		sessions: map[string]sessionView{
			sessionQualifiedKey(socketPath, "alpha"): {
				key:        sessionQualifiedKey(socketPath, "alpha"),
				name:       "alpha",
				socketPath: socketPath,
				socketHint: "lisa-a",
				paneID:     "%9",
			},
		},
		focusName:  sessionQualifiedKey(socketPath, "alpha"),
		focusIndex: 0,
	}
	cfg := config{}

	calls := make([]string, 0)
	origRun := runTmuxOnSocketFn
	t.Cleanup(func() {
		runTmuxOnSocketFn = origRun
	})
	runTmuxOnSocketFn = func(_ context.Context, _ config, socket string, args ...string) (string, error) {
		calls = append(calls, socket+"|"+strings.Join(args, " "))
		return "", nil
	}

	if err := sendKeyToFocused(context.Background(), &state, cfg, "Enter", false); err != nil {
		t.Fatalf("sendKeyToFocused err: %v", err)
	}
	if err := killFocusedSession(context.Background(), &state, cfg); err != nil {
		t.Fatalf("killFocusedSession err: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("calls len = %d", len(calls))
	}
	if !strings.Contains(calls[0], socketPath+"|send-keys") {
		t.Fatalf("send route = %q", calls[0])
	}
	if !strings.Contains(calls[1], socketPath+"|kill-session") {
		t.Fatalf("kill route = %q", calls[1])
	}
}

func TestConnectFocusedUsesSwitchOnlyForCurrentSocket(t *testing.T) {
	socketPath := "/tmp/lisa-b.sock"
	sessionKey := sessionQualifiedKey(socketPath, "beta")
	base := tcell.NewSimulationScreen("UTF-8")
	if err := base.Init(); err != nil {
		t.Fatalf("init screen: %v", err)
	}
	defer base.Fini()

	screen := &suspendTestScreen{Screen: base}
	cfg := config{}
	state := appState{
		sessions: map[string]sessionView{
			sessionKey: {
				key:        sessionKey,
				name:       "beta",
				socketPath: socketPath,
				socketHint: "lisa-b",
				paneID:     "%4",
			},
		},
		focusName:  sessionKey,
		focusIndex: 0,
	}

	origRun := runTmuxOnSocketFn
	origInteractive := runTmuxInteractiveOnSocketFn
	t.Cleanup(func() {
		runTmuxOnSocketFn = origRun
		runTmuxInteractiveOnSocketFn = origInteractive
	})

	var switchCall string
	runTmuxOnSocketFn = func(_ context.Context, _ config, socket string, args ...string) (string, error) {
		switchCall = socket + "|" + strings.Join(args, " ")
		return "", nil
	}
	t.Setenv("TMUX", socketPath+",1,0")
	exit, err := connectFocused(context.Background(), &state, cfg, screen)
	if err != nil {
		t.Fatalf("connectFocused switch err: %v", err)
	}
	if !exit {
		t.Fatalf("connectFocused switch exit = false")
	}
	if !strings.Contains(switchCall, socketPath+"|switch-client -t beta ; select-pane -t %4") {
		t.Fatalf("switchCall = %q", switchCall)
	}
	if screen.suspended {
		t.Fatalf("screen should not suspend for switch-client")
	}

	var attachCall string
	runTmuxInteractiveOnSocketFn = func(socket string, args ...string) error {
		attachCall = socket + "|" + strings.Join(args, " ")
		return nil
	}
	screen.suspended = false
	t.Setenv("TMUX", "/tmp/other.sock,2,0")
	exit, err = connectFocused(context.Background(), &state, cfg, screen)
	if err != nil {
		t.Fatalf("connectFocused attach err: %v", err)
	}
	if !exit {
		t.Fatalf("connectFocused attach exit = false")
	}
	if !screen.suspended {
		t.Fatalf("screen not suspended")
	}
	if !strings.Contains(attachCall, socketPath+"|attach-session -t beta ; select-pane -t %4") {
		t.Fatalf("attachCall = %q", attachCall)
	}
}

func TestDrawCellAndStatusIncludeSocketInfo(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init screen: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(80, 8)

	sess := sessionView{
		key:        "default::alpha",
		name:       "alpha",
		socketHint: "lisa-123",
		paneID:     "%1",
		lines:      []string{"line"},
	}
	drawCell(
		screen,
		0, 0, 80, 7,
		sess,
		tcell.StyleDefault,
		tcell.StyleDefault,
		tcell.StyleDefault,
		0,
		true,
	)

	title := readScreenRow(screen, 1, 80)
	if !strings.Contains(title, "alpha [lisa-123] (%1)") {
		t.Fatalf("title = %q", title)
	}

	state := appState{socketCount: 2}
	drawStatus(screen, 80, 7, tcell.StyleDefault, state, config{lines: 500, interval: 1}, 3)
	status := readScreenRow(screen, 7, 80)
	if !strings.Contains(status, "sockets:2 | sessions:3") {
		t.Fatalf("status = %q", status)
	}
}

func readScreenRow(screen tcell.SimulationScreen, y, width int) string {
	chars := make([]rune, 0, width)
	for x := 0; x < width; x++ {
		r, _, _, _ := screen.GetContent(x, y)
		chars = append(chars, r)
	}
	return string(chars)
}
