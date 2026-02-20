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
	if !canSwitchClient("") {
		t.Fatalf("expected default socket to allow switch when TMUX is set")
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
