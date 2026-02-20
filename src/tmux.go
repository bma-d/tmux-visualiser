package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var runTmuxOnSocketFn = runTmuxOnSocket
var runTmuxInteractiveOnSocketFn = runTmuxInteractiveOnSocket

func runTmux(ctx context.Context, cfg config, args ...string) (string, error) {
	return runTmuxOnSocketFn(ctx, cfg, "", args...)
}

func runTmuxOnSocket(ctx context.Context, cfg config, socket string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, cfg.cmdTimeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, "tmux", tmuxArgs(socket, args...)...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("tmux %s timed out", strings.Join(args, " "))
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.New(msg)
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}

func runTmuxInteractive(args ...string) error {
	return runTmuxInteractiveOnSocketFn("", args...)
}

func runTmuxInteractiveOnSocket(socket string, args ...string) error {
	cmd := exec.Command("tmux", tmuxArgs(socket, args...)...)
	cmd.Env = envWithoutTMUX(os.Environ())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func tmuxArgs(socket string, args ...string) []string {
	if strings.TrimSpace(socket) == "" {
		withDefault := make([]string, 0, len(args)+2)
		withDefault = append(withDefault, "-L", defaultSocketKey)
		withDefault = append(withDefault, args...)
		return withDefault
	}
	withSocket := make([]string, 0, len(args)+2)
	withSocket = append(withSocket, "-S", socket)
	withSocket = append(withSocket, args...)
	return withSocket
}

func canSwitchClient(socket string) bool {
	tmuxEnv := strings.TrimSpace(os.Getenv("TMUX"))
	if tmuxEnv == "" {
		return false
	}
	currentSocket := tmuxSocketFromEnv(tmuxEnv)
	if currentSocket == "" {
		return false
	}
	if strings.TrimSpace(socket) == "" {
		return isDefaultSocketPath(currentSocket)
	}
	return socketKey(socket) == socketKey(currentSocket)
}

func tmuxSocketFromEnv(value string) string {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return ""
	}
	if comma := strings.Index(raw, ","); comma >= 0 {
		raw = raw[:comma]
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return filepath.Clean(raw)
}

func envWithoutTMUX(env []string) []string {
	filtered := make([]string, 0, len(env))
	for _, item := range env {
		if strings.HasPrefix(item, "TMUX=") {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func isDefaultSocketPath(path string) bool {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || filepath.Base(clean) != defaultSocketKey {
		return false
	}
	parent := filepath.Base(filepath.Dir(clean))
	if !strings.HasPrefix(parent, "tmux-") {
		return false
	}
	suffix := strings.TrimPrefix(parent, "tmux-")
	if suffix == "" {
		return false
	}
	for _, r := range suffix {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
