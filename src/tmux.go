package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func tmuxArgs(socket string, args ...string) []string {
	if strings.TrimSpace(socket) == "" {
		return args
	}
	withSocket := make([]string, 0, len(args)+2)
	withSocket = append(withSocket, "-S", socket)
	withSocket = append(withSocket, args...)
	return withSocket
}
