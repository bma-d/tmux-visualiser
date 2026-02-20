package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDiscoverSocketTargets(t *testing.T) {
	tmpDir := t.TempDir()
	socketA := filepath.Join(tmpDir, "a.sock")
	socketB := filepath.Join(tmpDir, "b.sock")
	if err := os.WriteFile(socketA, []byte("a"), 0o600); err != nil {
		t.Fatalf("write socketA: %v", err)
	}
	if err := os.WriteFile(socketB, []byte("b"), 0o600); err != nil {
		t.Fatalf("write socketB: %v", err)
	}

	cfg := config{
		includeDefaultSocket: true,
		includeLisaSockets:   true,
		socketGlob:           filepath.Join(tmpDir, "*.sock"),
		explicitSockets: []string{
			socketA,
			filepath.Join(tmpDir, "missing.sock"),
			socketA,
		},
	}

	targets := discoverSocketTargets(cfg)
	if len(targets) != 4 {
		t.Fatalf("targets len = %d", len(targets))
	}

	got := []string{targets[0].path, targets[1].path, targets[2].path, targets[3].path}
	want := []string{"", filepath.Clean(socketA), filepath.Clean(filepath.Join(tmpDir, "missing.sock")), filepath.Clean(socketB)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %v, want %v", got, want)
	}
}

func TestListSessionsReportsUnavailableSocketHints(t *testing.T) {
	missingSocket := "/tmp/missing-explicit.sock"
	origRun := runTmuxOnSocketFn
	t.Cleanup(func() {
		runTmuxOnSocketFn = origRun
	})
	runTmuxOnSocketFn = func(_ context.Context, _ config, _ string, _ ...string) (string, error) {
		return "", errors.New("failed to connect to server")
	}

	cfg := config{
		includeDefaultSocket: false,
		includeLisaSockets:   false,
		explicitSockets:      []string{missingSocket},
	}
	_, socketCount, err := listSessions(context.Background(), cfg)
	if err == nil {
		t.Fatalf("expected error")
	}
	if socketCount != 1 {
		t.Fatalf("socketCount = %d", socketCount)
	}
	if !strings.Contains(err.Error(), "missing-explicit") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestListSessionsQualifiedKeyCollision(t *testing.T) {
	tmpDir := t.TempDir()
	socketA := filepath.Join(tmpDir, "project.sock")
	if err := os.WriteFile(socketA, []byte("x"), 0o600); err != nil {
		t.Fatalf("write socketA: %v", err)
	}

	origRun := runTmuxOnSocketFn
	t.Cleanup(func() {
		runTmuxOnSocketFn = origRun
	})
	runTmuxOnSocketFn = func(_ context.Context, _ config, socket string, args ...string) (string, error) {
		if len(args) < 1 || args[0] != "list-sessions" {
			return "", errors.New("unexpected command")
		}
		if socket == "" {
			return "alpha\nbeta", nil
		}
		if socket == socketA {
			return "alpha", nil
		}
		return "", errors.New("unknown socket")
	}

	cfg := config{
		includeDefaultSocket: true,
		includeLisaSockets:   false,
		explicitSockets:      []string{socketA},
	}
	refs, socketCount, err := listSessions(context.Background(), cfg)
	if err != nil {
		t.Fatalf("listSessions err: %v", err)
	}
	if socketCount != 2 {
		t.Fatalf("socketCount = %d", socketCount)
	}
	if len(refs) != 3 {
		t.Fatalf("refs len = %d", len(refs))
	}

	keys := map[string]bool{}
	for _, ref := range refs {
		keys[ref.key] = true
	}
	if !keys[sessionQualifiedKey("", "alpha")] {
		t.Fatalf("missing default alpha key")
	}
	if !keys[sessionQualifiedKey(socketA, "alpha")] {
		t.Fatalf("missing socket alpha key")
	}
}

func TestListSessionsReturnsPartialResultsWithFatalErrors(t *testing.T) {
	badSocket := "/tmp/private.sock"
	origRun := runTmuxOnSocketFn
	t.Cleanup(func() {
		runTmuxOnSocketFn = origRun
	})
	runTmuxOnSocketFn = func(_ context.Context, _ config, socket string, args ...string) (string, error) {
		if len(args) < 1 || args[0] != "list-sessions" {
			return "", errors.New("unexpected command")
		}
		if socket == "" {
			return "alpha", nil
		}
		if socket == badSocket {
			return "", errors.New("permission denied")
		}
		return "", errors.New("unknown socket")
	}

	cfg := config{
		includeDefaultSocket: true,
		includeLisaSockets:   false,
		explicitSockets:      []string{badSocket},
	}
	refs, socketCount, err := listSessions(context.Background(), cfg)
	if err == nil {
		t.Fatalf("expected partial error")
	}
	if !strings.Contains(err.Error(), "partial socket failures") {
		t.Fatalf("error = %q", err.Error())
	}
	if !strings.Contains(err.Error(), "private: permission denied") {
		t.Fatalf("error = %q", err.Error())
	}
	if socketCount != 2 {
		t.Fatalf("socketCount = %d", socketCount)
	}
	if len(refs) != 1 {
		t.Fatalf("refs len = %d", len(refs))
	}
	if refs[0].key != sessionQualifiedKey("", "alpha") {
		t.Fatalf("unexpected key = %q", refs[0].key)
	}
}

func TestUpdateStateKeepsPartialSessionsOnSocketError(t *testing.T) {
	badSocket := "/tmp/private.sock"
	origRun := runTmuxOnSocketFn
	t.Cleanup(func() {
		runTmuxOnSocketFn = origRun
	})
	runTmuxOnSocketFn = func(_ context.Context, _ config, socket string, args ...string) (string, error) {
		switch args[0] {
		case "list-sessions":
			if socket == "" {
				return "alpha", nil
			}
			if socket == badSocket {
				return "", errors.New("permission denied")
			}
		case "list-panes":
			if socket == "" {
				return "1 %1", nil
			}
		case "capture-pane":
			if socket == "" {
				return "line1\n", nil
			}
		}
		return "", errors.New("unexpected command")
	}

	state := appState{
		sessions: map[string]sessionView{},
		scroll:   map[string]int{},
		follow:   map[string]bool{},
	}
	cfg := config{
		lines:                50,
		maxWorkers:           1,
		includeDefaultSocket: true,
		includeLisaSockets:   false,
		explicitSockets:      []string{badSocket},
	}
	updateState(context.Background(), &state, cfg)

	if state.serverDown {
		t.Fatalf("serverDown should be false")
	}
	if !strings.Contains(state.lastErr, "partial socket failures") {
		t.Fatalf("lastErr = %q", state.lastErr)
	}
	if len(state.sessions) != 1 {
		t.Fatalf("sessions len = %d", len(state.sessions))
	}
	if _, ok := state.sessions[sessionQualifiedKey("", "alpha")]; !ok {
		t.Fatalf("missing default alpha session")
	}
}

func TestSocketUsedForListPaneCapture(t *testing.T) {
	socketPath := "/tmp/test.sock"
	calls := make([]string, 0)

	origRun := runTmuxOnSocketFn
	t.Cleanup(func() {
		runTmuxOnSocketFn = origRun
	})
	runTmuxOnSocketFn = func(_ context.Context, _ config, socket string, args ...string) (string, error) {
		calls = append(calls, socket+"|"+strings.Join(args, " "))
		switch args[0] {
		case "list-sessions":
			return "alpha", nil
		case "list-panes":
			return "0 %2\n1 %1", nil
		case "capture-pane":
			return "line1\nline2\n", nil
		default:
			return "", errors.New("unexpected command")
		}
	}

	ctx := context.Background()
	cfg := config{}
	target := makeSocketTarget(socketPath)
	refs, err := listSessionsOnSocket(ctx, cfg, target)
	if err != nil {
		t.Fatalf("listSessionsOnSocket err: %v", err)
	}
	if len(refs) != 1 || refs[0].key != sessionQualifiedKey(socketPath, "alpha") {
		t.Fatalf("refs = %#v", refs)
	}

	paneID, err := activePaneID(ctx, cfg, socketPath, "alpha")
	if err != nil {
		t.Fatalf("activePaneID err: %v", err)
	}
	if paneID != "%1" {
		t.Fatalf("paneID = %q", paneID)
	}

	lines, err := capturePane(ctx, cfg, socketPath, paneID, 20)
	if err != nil {
		t.Fatalf("capturePane err: %v", err)
	}
	if len(lines) != 2 || lines[0] != "line1" || lines[1] != "line2" {
		t.Fatalf("lines = %v", lines)
	}

	for _, call := range calls {
		if !strings.HasPrefix(call, socketPath+"|") {
			t.Fatalf("socket mismatch call: %q", call)
		}
	}
}
