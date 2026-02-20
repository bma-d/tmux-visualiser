package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const defaultSocketKey = "default"

func discoverSocketTargets(cfg config) []socketTarget {
	targets := make([]socketTarget, 0)
	seen := make(map[string]struct{})

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
	}

	for _, path := range cfg.explicitSockets {
		add(path, false)
	}

	if cfg.includeLisaSockets && strings.TrimSpace(cfg.socketGlob) != "" {
		matches, err := filepath.Glob(cfg.socketGlob)
		if err == nil {
			sort.Strings(matches)
			for _, path := range matches {
				add(path, true)
			}
		}
	}

	return targets
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
	return strings.Contains(text, "no server running") ||
		strings.Contains(text, "failed to connect to server") ||
		strings.Contains(text, "error connecting to") ||
		strings.Contains(text, "connection refused") ||
		strings.Contains(text, "no such file or directory")
}
