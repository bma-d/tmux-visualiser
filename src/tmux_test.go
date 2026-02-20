package main

import (
	"reflect"
	"testing"
)

func TestCanSwitchClient(t *testing.T) {
	t.Setenv("TMUX", "/tmp/current.sock,123,0")
	if !canSwitchClient("/tmp/current.sock") {
		t.Fatalf("expected same socket to allow switch")
	}
	if canSwitchClient("/tmp/other.sock") {
		t.Fatalf("expected different socket to skip switch")
	}

	t.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")
	if !canSwitchClient("") {
		t.Fatalf("expected default socket to allow switch when current client is on default socket")
	}

	t.Setenv("TMUX", "/tmp/custom.sock,123,0")
	if canSwitchClient("") {
		t.Fatalf("expected default socket to skip switch when current client is on non-default socket")
	}

	t.Setenv("TMUX", "/tmp/default.sock,123,0")
	if canSwitchClient("") {
		t.Fatalf("expected custom socket named default.sock to skip default switch")
	}

	t.Setenv("TMUX", "/tmp/custom/default,123,0")
	if canSwitchClient("") {
		t.Fatalf("expected non-tmux default basename path to skip default switch")
	}
}

func TestCanSwitchClientNoTMUX(t *testing.T) {
	t.Setenv("TMUX", "")
	if canSwitchClient("") {
		t.Fatalf("switch should be disabled when TMUX is empty")
	}
}

func TestTmuxSocketFromEnv(t *testing.T) {
	if got := tmuxSocketFromEnv("/tmp/a.sock,42,0"); got != "/tmp/a.sock" {
		t.Fatalf("socket = %q", got)
	}
	if got := tmuxSocketFromEnv("   "); got != "" {
		t.Fatalf("socket = %q", got)
	}
}

func TestEnvWithoutTMUX(t *testing.T) {
	input := []string{"PATH=/usr/bin", "TMUX=/tmp/a.sock,1,0", "HOME=/tmp"}
	got := envWithoutTMUX(input)
	want := []string{"PATH=/usr/bin", "HOME=/tmp"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("envWithoutTMUX = %v, want %v", got, want)
	}
}

func TestTmuxArgsDefaultSocket(t *testing.T) {
	got := tmuxArgs("", "list-sessions", "-F", "#S")
	want := []string{"-L", defaultSocketKey, "list-sessions", "-F", "#S"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tmuxArgs = %v, want %v", got, want)
	}
}

func TestIsDefaultSocketPath(t *testing.T) {
	if !isDefaultSocketPath("/tmp/tmux-1000/default") {
		t.Fatalf("expected canonical default path to match")
	}
	if isDefaultSocketPath("/tmp/custom/default") {
		t.Fatalf("expected non-canonical default basename path to fail")
	}
	if isDefaultSocketPath("/tmp/tmux-user/default") {
		t.Fatalf("expected non-numeric tmux dir suffix to fail")
	}
}
