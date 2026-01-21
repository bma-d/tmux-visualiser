package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
)

type updateResult struct {
	latest    string
	available bool
	err       error
}

type updatePrefs struct {
	IgnoreUntil time.Time `json:"ignore_until"`
}

type updateAction int

const (
	updateNone updateAction = iota
	updateNow
)

func handleUpdateKey(state *appState, ev *tcell.EventKey) (updateAction, bool) {
	switch ev.Key() {
	case tcell.KeyEsc:
		state.updatePrompt = false
		state.updateVersion = ""
		return updateNone, true
	case tcell.KeyEnter:
		state.updatePrompt = false
		return updateNow, true
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'u', 'U':
			state.updatePrompt = false
			return updateNow, true
		case 'i', 'I':
			if err := ignoreUpdatesFor(7 * 24 * time.Hour); err != nil {
				state.lastErr = err.Error()
			}
			state.updatePrompt = false
			state.updateVersion = ""
			return updateNone, true
		case 'n', 'N':
			state.updatePrompt = false
			state.updateVersion = ""
			return updateNone, true
		}
	}
	return updateNone, false
}

func checkForUpdate(ctx context.Context, currentVersion string) updateResult {
	current := normalizeVersion(currentVersion)
	if current == "" || current == "dev" || current == "unknown" {
		return updateResult{}
	}

	prefs, err := loadUpdatePrefs()
	if err == nil && time.Now().Before(prefs.IgnoreUntil) {
		return updateResult{}
	}

	latest, err := fetchLatestRelease(ctx)
	if err != nil {
		return updateResult{err: err}
	}
	latestNorm := normalizeVersion(latest)
	if latestNorm == "" {
		return updateResult{}
	}
	if compareVersions(current, latestNorm) < 0 {
		return updateResult{latest: latestNorm, available: true}
	}
	return updateResult{}
}

func fetchLatestRelease(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/bma-d/tmux-visualiser/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "tmux-visualiser")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("update check failed: %s", resp.Status)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return strings.TrimSpace(payload.TagName), nil
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	return v
}

type versionParts struct {
	major int
	minor int
	patch int
	pre   string
}

func parseVersion(v string) (versionParts, error) {
	if v == "" {
		return versionParts{}, errors.New("empty version")
	}
	main := v
	pre := ""
	if idx := strings.Index(v, "-"); idx != -1 {
		main = v[:idx]
		pre = v[idx+1:]
	}
	parts := strings.Split(main, ".")
	nums := []int{0, 0, 0}
	for i := 0; i < len(nums) && i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return versionParts{}, err
		}
		nums[i] = n
	}
	return versionParts{major: nums[0], minor: nums[1], patch: nums[2], pre: pre}, nil
}

func compareVersions(a, b string) int {
	va, err := parseVersion(a)
	if err != nil {
		return 0
	}
	vb, err := parseVersion(b)
	if err != nil {
		return 0
	}
	if va.major != vb.major {
		return compareInt(va.major, vb.major)
	}
	if va.minor != vb.minor {
		return compareInt(va.minor, vb.minor)
	}
	if va.patch != vb.patch {
		return compareInt(va.patch, vb.patch)
	}
	if va.pre == "" && vb.pre != "" {
		return 1
	}
	if va.pre != "" && vb.pre == "" {
		return -1
	}
	if va.pre == vb.pre {
		return 0
	}
	if va.pre < vb.pre {
		return -1
	}
	return 1
}

func compareInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func loadUpdatePrefs() (updatePrefs, error) {
	path, err := updatePrefsPath()
	if err != nil {
		return updatePrefs{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return updatePrefs{}, nil
		}
		return updatePrefs{}, err
	}
	var prefs updatePrefs
	if err := json.Unmarshal(data, &prefs); err != nil {
		return updatePrefs{}, err
	}
	return prefs, nil
}

func saveUpdatePrefs(prefs updatePrefs) error {
	path, err := updatePrefsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func updatePrefsPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		home, hErr := os.UserHomeDir()
		if hErr != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "tmux-visualiser", "update.json"), nil
}

func ignoreUpdatesFor(duration time.Duration) error {
	prefs := updatePrefs{IgnoreUntil: time.Now().Add(duration)}
	return saveUpdatePrefs(prefs)
}

func runUpdateFlow(screen interface{ Suspend() error }, latest string) (bool, error) {
	cmd, args, err := detectUpdateCommand()
	if err != nil {
		return false, err
	}
	if err := screen.Suspend(); err != nil {
		return false, err
	}
	fmt.Printf("Updating tmux-visualiser to %s...\n", latest)
	c := exec.Command(cmd, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	err = c.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Update failed:", err)
	}
	fmt.Println("Update complete. Please run tmux-visualiser again.")
	return true, err
}

func detectUpdateCommand() (string, []string, error) {
	if override := strings.TrimSpace(os.Getenv("TMUX_VISUALISER_UPDATE_COMMAND")); override != "" {
		if runtime.GOOS == "windows" {
			return "cmd", []string{"/C", override}, nil
		}
		return "sh", []string{"-c", override}, nil
	}

	if _, err := exec.LookPath("brew"); err == nil {
		if err := probeCommand("brew", "list", "--versions", "tmux-visualiser"); err == nil {
			return "brew", []string{"upgrade", "tmux-visualiser"}, nil
		}
	}

	if _, err := exec.LookPath("scoop"); err == nil {
		if err := probeCommand("scoop", "list", "tmux-visualiser"); err == nil {
			return "scoop", []string{"update", "tmux-visualiser"}, nil
		}
	}

	if _, err := exec.LookPath("go"); err == nil {
		return "go", []string{"install", "github.com/bma-d/tmux-visualiser@latest"}, nil
	}

	return "", nil, errors.New("no supported update command found; update via your package manager")
}

func probeCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}
