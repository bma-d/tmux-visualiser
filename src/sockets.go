package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const defaultSocketKey = "default"
const defaultLisaSocketGlob = "/tmp/lisa-tmux-*-*.sock"

var listLisaSocketPathsFn = listLisaSocketPaths
var listProcessCommandsFn = listProcessCommands

var lisaSocketCache = struct {
	mu      sync.Mutex
	at      time.Time
	paths   []string
	errText string
}{}

const lisaSocketCacheTTL = 5 * time.Second

func discoverSocketTargets(cfg config) ([]socketTarget, []string) {
	targets := make([]socketTarget, 0)
	seen := make(map[string]struct{})
	discoveryErrors := make([]string, 0)

	add := func(path string, requireExists bool) {
		raw := strings.TrimSpace(path)
		if raw == "" {
			target := makeSocketTarget("")
			if _, ok := seen[target.key]; ok {
				return
			}
			seen[target.key] = struct{}{}
			targets = append(targets, target)
			return
		}

		clean := filepath.Clean(raw)
		if requireExists && !socketPathExists(clean) {
			return
		}

		target := makeSocketTarget(clean)
		if _, ok := seen[target.key]; ok {
			return
		}
		seen[target.key] = struct{}{}
		targets = append(targets, target)
	}

	if cfg.includeDefaultSocket {
		add("", false)
		envSocket := tmuxSocketFromEnv(os.Getenv("TMUX"))
		if envSocket != "" && !isDefaultSocketPath(envSocket) {
			add(envSocket, false)
		}
	}

	for _, path := range cfg.explicitSockets {
		add(path, false)
	}

	if cfg.includeLisaSockets {
		for _, pattern := range lisaSocketGlobs(cfg.socketGlob) {
			matches, err := filepath.Glob(pattern)
			if err != nil {
				discoveryErrors = append(discoveryErrors, fmt.Sprintf("socket-glob %q: %v", pattern, err))
				continue
			}
			sort.Strings(matches)
			for _, path := range matches {
				add(path, true)
			}
		}
		lisaSockets, err := listLisaSocketPathsFn(cfg)
		if err != nil {
			discoveryErrors = append(discoveryErrors, fmt.Sprintf("lisa-sockets: %v", err))
		}
		for _, path := range lisaSockets {
			add(path, false)
		}
	}

	return targets, discoveryErrors
}

func lisaSocketGlobs(configured string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	add := func(pattern string) {
		clean := strings.TrimSpace(pattern)
		if clean == "" {
			return
		}
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}

	configured = strings.TrimSpace(configured)
	if configured == "" {
		configured = defaultLisaSocketGlob
	}
	add(configured)
	if configured == defaultLisaSocketGlob {
		add("/private/tmp/lisa-tmux-*-*.sock")
		add("/tmp/lisa-codex-nosb.sock")
		add("/private/tmp/lisa-codex-nosb.sock")
	}
	return out
}

func listLisaSocketPaths(cfg config) ([]string, error) {
	lisaSocketCache.mu.Lock()
	if !lisaSocketCache.at.IsZero() && time.Since(lisaSocketCache.at) < lisaSocketCacheTTL {
		paths := append([]string(nil), lisaSocketCache.paths...)
		errText := lisaSocketCache.errText
		lisaSocketCache.mu.Unlock()
		if errText != "" {
			return paths, errors.New(errText)
		}
		return paths, nil
	}
	lisaSocketCache.mu.Unlock()

	paths := make([]string, 0, 8)
	processPaths, processErr := listLisaSocketPathsFromProcessTable()
	if processErr == nil {
		paths = append(paths, processPaths...)
	}
	lisaPaths, lisaErr := listLisaSocketPathsFromLISA(cfg)
	if lisaErr == nil {
		paths = append(paths, lisaPaths...)
	}
	paths = dedupePaths(paths)

	errText := ""
	switch {
	case processErr != nil && lisaErr != nil:
		errText = processErr.Error() + " | " + lisaErr.Error()
	case processErr != nil:
		errText = processErr.Error()
	case lisaErr != nil:
		errText = lisaErr.Error()
	}
	lisaSocketCache.mu.Lock()
	lisaSocketCache.at = time.Now()
	lisaSocketCache.paths = append([]string(nil), paths...)
	lisaSocketCache.errText = errText
	lisaSocketCache.mu.Unlock()

	if errText != "" {
		return paths, errors.New(errText)
	}
	return paths, nil
}

func dedupePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		clean := filepath.Clean(strings.TrimSpace(path))
		if clean == "" || clean == "." {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func listLisaSocketPathsFromProcessTable() ([]string, error) {
	commands, err := listProcessCommandsFn()
	if err != nil {
		return nil, err
	}
	paths := extractTmuxSocketPathsFromCommands(commands)
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if isLikelyLisaSocketPath(path) {
			out = append(out, path)
		}
	}
	return out, nil
}

func listProcessCommands() ([]string, error) {
	out, err := exec.Command("ps", "axo", "command=").Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(out), "\n")
	cmds := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		cmds = append(cmds, line)
	}
	return cmds, nil
}

func extractTmuxSocketPathsFromCommands(commands []string) []string {
	paths := make([]string, 0, len(commands))
	for _, line := range commands {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 3 {
			continue
		}
		if strings.ToLower(filepath.Base(fields[0])) != "tmux" {
			continue
		}
		for i := 1; i < len(fields)-1; i++ {
			if fields[i] != "-S" {
				continue
			}
			candidate := strings.TrimSpace(fields[i+1])
			if candidate == "" {
				continue
			}
			paths = append(paths, filepath.Clean(candidate))
			break
		}
	}
	return dedupePaths(paths)
}

func listLisaSocketPathsFromLISA(cfg config) ([]string, error) {
	timeout := cfg.cmdTimeout
	if timeout <= 0 {
		timeout = 900 * time.Millisecond
	}
	if timeout < time.Second {
		timeout = time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "lisa", "session", "list", "--all-sockets", "--with-next-action", "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return []string{}, nil
		}
		var execErr *exec.Error
		if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
			return []string{}, nil
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("lisa list timed out")
		}
		return nil, fmt.Errorf("lisa list failed: %s", strings.TrimSpace(string(out)))
	}

	var payload struct {
		Items []struct {
			ProjectRoot string `json:"projectRoot"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, fmt.Errorf("lisa list invalid json")
	}
	paths := make([]string, 0, len(payload.Items))
	for _, item := range payload.Items {
		root := canonicalProjectRoot(item.ProjectRoot)
		if root == "" {
			continue
		}
		paths = append(paths, tmuxSocketPathForProjectRoot(root))
		legacy := tmuxLegacySocketPathForProjectRoot(root)
		if legacy != "" && legacy != paths[len(paths)-1] {
			paths = append(paths, legacy)
		}
	}
	return dedupePaths(paths), nil
}

func canonicalProjectRoot(projectRoot string) string {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return ""
	}
	abs, err := filepath.Abs(root)
	if err == nil {
		root = abs
	}
	eval, err := filepath.EvalSymlinks(root)
	if err == nil {
		root = eval
	}
	return filepath.Clean(root)
}

func tmuxSocketPathForProjectRoot(projectRoot string) string {
	root := canonicalProjectRoot(projectRoot)
	if root == "" {
		return ""
	}
	return filepath.Join(preferredTmuxSocketDir(), fmt.Sprintf("lisa-tmux-%s-%s.sock", projectSlug(root), projectHash(root)))
}

func tmuxLegacySocketPathForProjectRoot(projectRoot string) string {
	root := canonicalProjectRoot(projectRoot)
	if root == "" {
		return ""
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("lisa-tmux-%s-%s.sock", projectSlug(root), projectHash(root)))
}

func projectSlug(projectRoot string) string {
	base := filepath.Base(projectRoot)
	return sanitizeID(base, 10)
}

func sanitizeID(s string, max int) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		out = "project"
	}
	if len(out) > max {
		out = out[:max]
	}
	return out
}

func projectHash(projectRoot string) string {
	sum := md5.Sum([]byte(projectRoot))
	return hex.EncodeToString(sum[:])[:8]
}

func preferredTmuxSocketDir() string {
	if info, err := os.Stat("/tmp"); err == nil && info.IsDir() {
		return "/tmp"
	}
	tmp := strings.TrimSpace(os.TempDir())
	if tmp == "" {
		return "/tmp"
	}
	return filepath.Clean(tmp)
}

func isLikelyLisaSocketPath(path string) bool {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(path)))
	if base == "lisa-codex-nosb.sock" {
		return true
	}
	return strings.HasPrefix(base, "lisa-") && strings.HasSuffix(base, ".sock")
}

func makeSocketTarget(path string) socketTarget {
	return socketTarget{
		path: path,
		key:  socketKey(path),
		hint: socketHint(path),
	}
}

func socketKey(path string) string {
	if strings.TrimSpace(path) == "" {
		return defaultSocketKey
	}
	return filepath.Clean(path)
}

func socketHint(path string) string {
	if strings.TrimSpace(path) == "" {
		return defaultSocketKey
	}
	base := filepath.Base(path)
	if base == "." || base == string(filepath.Separator) {
		return filepath.Clean(path)
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func sessionQualifiedKey(socketPath, sessionName string) string {
	return socketKey(socketPath) + "::" + sessionName
}

func paneQualifiedKey(socketPath, sessionName, paneID string) string {
	if strings.TrimSpace(paneID) == "" {
		return sessionQualifiedKey(socketPath, sessionName)
	}
	return sessionQualifiedKey(socketPath, sessionName) + "::" + strings.TrimSpace(paneID)
}

func socketPathExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func isSocketUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	return isSocketUnavailableMessage(err.Error())
}

func isSocketUnavailableMessage(msg string) bool {
	text := strings.ToLower(strings.TrimSpace(msg))
	if text == "" {
		return false
	}
	if strings.Contains(text, "permission denied") {
		return false
	}
	return strings.Contains(text, "no server running") ||
		strings.Contains(text, "failed to connect to server") ||
		strings.Contains(text, "connection refused") ||
		strings.Contains(text, "no such file or directory")
}
