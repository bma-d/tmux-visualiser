package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
)

type config struct {
	lines        int
	interval     time.Duration
	cmdTimeout   time.Duration
	maxWorkers   int
	statusHeight int
}

type sessionView struct {
	name    string
	paneID  string
	lines   []string
	updated time.Time
}

type appState struct {
	sessions    map[string]sessionView
	lastErr     string
	serverDown  bool
	lastRefresh time.Time
	scroll      map[string]int
	follow      map[string]bool
	focusIndex  int
	focusName   string
}

func main() {
	cfg := config{}
	flag.IntVar(&cfg.lines, "lines", 500, "number of lines to capture per session")
	flag.DurationVar(&cfg.interval, "interval", 1*time.Second, "refresh interval")
	flag.DurationVar(&cfg.cmdTimeout, "cmd-timeout", 900*time.Millisecond, "timeout for each tmux command")
	flag.IntVar(&cfg.maxWorkers, "workers", 4, "max concurrent tmux capture workers")
	flag.Parse()

	if cfg.lines < 20 {
		cfg.lines = 20
	}
	if cfg.interval < 200*time.Millisecond {
		cfg.interval = 200 * time.Millisecond
	}
	if cfg.cmdTimeout < 300*time.Millisecond {
		cfg.cmdTimeout = 300 * time.Millisecond
	}
	if cfg.maxWorkers < 1 {
		cfg.maxWorkers = 1
	}

	screen, err := tcell.NewScreen()
	if err != nil {
		fmt.Println("failed to create screen:", err)
		return
	}
	if err := screen.Init(); err != nil {
		fmt.Println("failed to init screen:", err)
		return
	}
	defer screen.Fini()

	state := appState{sessions: map[string]sessionView{}, scroll: map[string]int{}, follow: map[string]bool{}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan tcell.Event, 16)
	go func() {
		for {
			ev := screen.PollEvent()
			if ev == nil {
				close(events)
				return
			}
			events <- ev
		}
	}()

	refresh := func() {
		updateState(ctx, &state, cfg)
		draw(screen, state, cfg)
	}

	refresh()
	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()

	running := true
	for running {
		select {
		case <-ticker.C:
			refresh()
		case ev, ok := <-events:
			if !ok {
				running = false
				break
			}
			switch tev := ev.(type) {
			case *tcell.EventResize:
				screen.Sync()
				draw(screen, state, cfg)
			case *tcell.EventKey:
				switch tev.Key() {
				case tcell.KeyCtrlC:
					running = false
				case tcell.KeyUp:
					scrollFocused(&state, screen, -1)
					draw(screen, state, cfg)
				case tcell.KeyDown:
					scrollFocused(&state, screen, 1)
					draw(screen, state, cfg)
				case tcell.KeyPgUp:
					scrollFocused(&state, screen, -5)
					draw(screen, state, cfg)
				case tcell.KeyPgDn:
					scrollFocused(&state, screen, 5)
					draw(screen, state, cfg)
				case tcell.KeyHome:
					jumpScroll(&state, screen, true)
					draw(screen, state, cfg)
				case tcell.KeyEnd:
					jumpScroll(&state, screen, false)
					draw(screen, state, cfg)
				case tcell.KeyTAB:
					moveFocus(&state, 1)
					draw(screen, state, cfg)
				case tcell.KeyBacktab:
					moveFocus(&state, -1)
					draw(screen, state, cfg)
				default:
					switch tev.Rune() {
					case 'q', 'Q':
						running = false
					case 'r', 'R':
						refresh()
					case '+':
						cfg.lines += 50
						refresh()
					case '-':
						cfg.lines -= 50
						if cfg.lines < 20 {
							cfg.lines = 20
						}
						refresh()
					case 'j', 'J':
						scrollFocused(&state, screen, 1)
						draw(screen, state, cfg)
					case 'k', 'K':
						scrollFocused(&state, screen, -1)
						draw(screen, state, cfg)
					case 'n', 'N':
						moveFocus(&state, 1)
						draw(screen, state, cfg)
					case 'p', 'P':
						moveFocus(&state, -1)
						draw(screen, state, cfg)
					}
				}
			}
		}
	}
}

func updateState(ctx context.Context, state *appState, cfg config) {
	names, err := listSessions(ctx, cfg)
	state.lastRefresh = time.Now()
	if err != nil {
		state.lastErr = err.Error()
		if strings.Contains(strings.ToLower(err.Error()), "no server running") {
			state.serverDown = true
			state.sessions = map[string]sessionView{}
			return
		}
		state.serverDown = false
		return
	}
	state.lastErr = ""
	state.serverDown = false

	newSessions := make(map[string]sessionView, len(names))
	keepScroll := make(map[string]int, len(names))
	keepFollow := make(map[string]bool, len(names))
	if len(names) == 0 {
		state.sessions = newSessions
		state.scroll = keepScroll
		state.follow = keepFollow
		state.focusIndex = 0
		state.focusName = ""
		return
	}

	workers := cfg.maxWorkers
	if workers > len(names) {
		workers = len(names)
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, name := range names {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			paneID, err := activePaneID(ctx, cfg, name)
			if err != nil {
				mu.Lock()
				newSessions[name] = sessionView{name: name, paneID: "", lines: []string{err.Error()}, updated: time.Now()}
				mu.Unlock()
				return
			}

			lines, err := capturePane(ctx, cfg, paneID, cfg.lines)
			if err != nil {
				lines = []string{err.Error()}
			}

			mu.Lock()
			newSessions[name] = sessionView{name: name, paneID: paneID, lines: lines, updated: time.Now()}
			mu.Unlock()
		}()
	}

	wg.Wait()
	state.sessions = newSessions
	for _, name := range names {
		keepScroll[name] = state.scroll[name]
		if _, ok := state.follow[name]; ok {
			keepFollow[name] = state.follow[name]
		} else {
			keepFollow[name] = true
		}
	}
	state.scroll = keepScroll
	state.follow = keepFollow
	state.focusIndex = focusIndexForName(names, state.focusName)
	if state.focusIndex < 0 || state.focusIndex >= len(names) {
		state.focusIndex = 0
		state.focusName = names[0]
	} else {
		state.focusName = names[state.focusIndex]
	}
}

func listSessions(ctx context.Context, cfg config) ([]string, error) {
	out, err := runTmux(ctx, cfg, "list-sessions", "-F", "#S")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return []string{}, nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	sessions := make([]string, 0, len(lines))
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name != "" {
			sessions = append(sessions, name)
		}
	}
	sort.Strings(sessions)
	return sessions, nil
}

func activePaneID(ctx context.Context, cfg config, session string) (string, error) {
	out, err := runTmux(ctx, cfg, "list-panes", "-t", session, "-F", "#{pane_active} #{pane_id}")
	if err != nil {
		return "", err
	}
	var fallback string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fallback == "" {
			fallback = fields[1]
		}
		if fields[0] == "1" {
			return fields[1], nil
		}
	}
	if fallback == "" {
		return "", errors.New("no pane found")
	}
	return fallback, nil
}

func capturePane(ctx context.Context, cfg config, paneID string, lines int) ([]string, error) {
	if lines < 1 {
		lines = 1
	}
	rangeArg := fmt.Sprintf("-%d", lines)
	out, err := runTmux(ctx, cfg, "capture-pane", "-t", paneID, "-p", "-S", rangeArg)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return []string{"(empty)"}, nil
	}
	result := strings.Split(out, "\n")
	if len(result) > 0 && result[len(result)-1] == "" {
		result = result[:len(result)-1]
	}
	return result, nil
}

func runTmux(ctx context.Context, cfg config, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, cfg.cmdTimeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, "tmux", args...)
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

func draw(screen tcell.Screen, state appState, cfg config) {
	screen.Clear()
	width, height := screen.Size()
	if width <= 0 || height <= 0 {
		screen.Show()
		return
	}

	statusHeight := 1
	if height < 2 {
		statusHeight = 0
	}
	gridHeight := height - statusHeight

	statusStyle := tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorLightGray)
	contentStyle := tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack)
	headStyle := tcell.StyleDefault.Foreground(tcell.ColorYellow).Background(tcell.ColorBlack).Bold(true)
	focusHeadStyle := tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorYellow).Bold(true)
	focusBorder := tcell.StyleDefault.Foreground(tcell.ColorYellow).Background(tcell.ColorBlack)

	sessionNames := orderedSessionNames(state)
	sessions := make([]sessionView, 0, len(sessionNames))
	for _, name := range sessionNames {
		sessions = append(sessions, state.sessions[name])
	}

	if gridHeight <= 0 {
		drawStatus(screen, width, height-1, statusStyle, state, cfg, len(sessions))
		screen.Show()
		return
	}

	if state.serverDown {
		drawCentered(screen, 0, 0, width, gridHeight, contentStyle, "tmux server not running")
	} else if len(sessions) == 0 {
		drawCentered(screen, 0, 0, width, gridHeight, contentStyle, "no tmux sessions")
	} else {
		cols, rows := gridDims(len(sessions))
		for i, sess := range sessions {
			col := i % cols
			row := i / cols
			x0 := (width * col) / cols
			x1 := (width * (col + 1)) / cols
			y0 := (gridHeight * row) / rows
			y1 := (gridHeight * (row + 1)) / rows
			focused := i == state.focusIndex
			cellHead := headStyle
			cellBorder := contentStyle
			if focused {
				cellHead = focusHeadStyle
				cellBorder = focusBorder
			}
			drawCell(screen, x0, y0, x1, y1, sess, cellHead, contentStyle, cellBorder, state.scroll[sess.name], state.follow[sess.name])
		}
	}

	if statusHeight == 1 {
		drawStatus(screen, width, height-1, statusStyle, state, cfg, len(sessions))
	}

	screen.Show()
}

func gridDims(count int) (cols, rows int) {
	if count <= 0 {
		return 1, 1
	}
	cols = int(math.Ceil(math.Sqrt(float64(count))))
	if cols < 1 {
		cols = 1
	}
	rows = int(math.Ceil(float64(count) / float64(cols)))
	if rows < 1 {
		rows = 1
	}
	return cols, rows
}

func drawCell(screen tcell.Screen, x0, y0, x1, y1 int, sess sessionView, headStyle, bodyStyle, borderStyle tcell.Style, scrollTop int, follow bool) {
	w := x1 - x0
	h := y1 - y0
	if w <= 1 || h <= 1 {
		return
	}
	drawBox(screen, x0, y0, x1, y1, borderStyle)

	title := sess.name
	if sess.paneID != "" {
		title = fmt.Sprintf("%s (%s)", sess.name, sess.paneID)
	}
	if h > 2 {
		drawText(screen, x0+1, y0+1, w-2, title, headStyle)
	}

	contentTop := y0 + 2
	if h <= 3 {
		contentTop = y0 + 1
	}
	contentHeight := y1 - 1 - contentTop
	if contentHeight <= 0 {
		return
	}

	start := 0
	maxStart := 0
	if len(sess.lines) > contentHeight {
		maxStart = len(sess.lines) - contentHeight
	}
	if follow {
		start = maxStart
	} else {
		if scrollTop < 0 {
			scrollTop = 0
		}
		if scrollTop > maxStart {
			scrollTop = maxStart
		}
		start = scrollTop
	}
	for row := 0; row < contentHeight; row++ {
		lineIndex := start + row
		if lineIndex >= len(sess.lines) {
			break
		}
		drawText(screen, x0+1, contentTop+row, w-2, sess.lines[lineIndex], bodyStyle)
	}
}

func drawBox(screen tcell.Screen, x0, y0, x1, y1 int, style tcell.Style) {
	w := x1 - x0
	h := y1 - y0
	if w <= 1 || h <= 1 {
		return
	}
	for x := x0; x < x1; x++ {
		screen.SetContent(x, y0, '-', nil, style)
		screen.SetContent(x, y1-1, '-', nil, style)
	}
	for y := y0; y < y1; y++ {
		screen.SetContent(x0, y, '|', nil, style)
		screen.SetContent(x1-1, y, '|', nil, style)
	}
	screen.SetContent(x0, y0, '+', nil, style)
	screen.SetContent(x1-1, y0, '+', nil, style)
	screen.SetContent(x0, y1-1, '+', nil, style)
	screen.SetContent(x1-1, y1-1, '+', nil, style)
}

func drawText(screen tcell.Screen, x, y, width int, text string, style tcell.Style) {
	if width <= 0 {
		return
	}
	runes := []rune(text)
	if len(runes) > width {
		runes = runes[:width]
	}
	for i := 0; i < width; i++ {
		ch := ' '
		if i < len(runes) {
			ch = runes[i]
		}
		screen.SetContent(x+i, y, ch, nil, style)
	}
}

func drawCentered(screen tcell.Screen, x0, y0, width, height int, style tcell.Style, text string) {
	if width <= 0 || height <= 0 {
		return
	}
	y := y0 + height/2
	x := x0 + (width-len(text))/2
	if x < x0 {
		x = x0
	}
	drawText(screen, x, y, width-(x-x0), text, style)
}

func drawStatus(screen tcell.Screen, width, y int, style tcell.Style, state appState, cfg config, sessionCount int) {
	if y < 0 || width <= 0 {
		return
	}
	label := fmt.Sprintf("sessions:%d | lines:%d | interval:%s | tab:focus j/k:scroll q:quit", sessionCount, cfg.lines, cfg.interval)
	if state.lastErr != "" {
		label = fmt.Sprintf("error: %s", state.lastErr)
	}
	if len(label) < width {
		label = label + strings.Repeat(" ", width-len(label))
	}
	if len(label) > width {
		label = label[:width]
	}
	for i := 0; i < width; i++ {
		screen.SetContent(i, y, rune(label[i]), nil, style)
	}
}

func orderedSessionNames(state appState) []string {
	names := make([]string, 0, len(state.sessions))
	for name := range state.sessions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func focusIndexForName(names []string, name string) int {
	if name == "" {
		return -1
	}
	for i, n := range names {
		if n == name {
			return i
		}
	}
	return -1
}

func moveFocus(state *appState, delta int) {
	names := orderedSessionNames(*state)
	if len(names) == 0 {
		state.focusIndex = 0
		state.focusName = ""
		return
	}
	idx := state.focusIndex
	if idx < 0 || idx >= len(names) {
		idx = 0
	}
	idx = (idx + delta) % len(names)
	if idx < 0 {
		idx += len(names)
	}
	state.focusIndex = idx
	state.focusName = names[idx]
}

func scrollFocused(state *appState, screen tcell.Screen, delta int) {
	names := orderedSessionNames(*state)
	if len(names) == 0 {
		return
	}
	if state.focusIndex < 0 || state.focusIndex >= len(names) {
		state.focusIndex = 0
		state.focusName = names[0]
	}
	name := names[state.focusIndex]
	sess, ok := state.sessions[name]
	if !ok {
		return
	}
	contentHeight := contentHeightForIndex(len(names), state.focusIndex, screen)
	if contentHeight <= 0 {
		return
	}
	maxStart := 0
	if len(sess.lines) > contentHeight {
		maxStart = len(sess.lines) - contentHeight
	}
	current := state.scroll[name]
	if state.follow[name] {
		current = maxStart
	}
	next := current + delta
	if next < 0 {
		next = 0
	}
	if next > maxStart {
		next = maxStart
	}
	state.scroll[name] = next
	state.follow[name] = next == maxStart
}

func jumpScroll(state *appState, screen tcell.Screen, toTop bool) {
	names := orderedSessionNames(*state)
	if len(names) == 0 {
		return
	}
	if state.focusIndex < 0 || state.focusIndex >= len(names) {
		state.focusIndex = 0
		state.focusName = names[0]
	}
	name := names[state.focusIndex]
	sess, ok := state.sessions[name]
	if !ok {
		return
	}
	contentHeight := contentHeightForIndex(len(names), state.focusIndex, screen)
	if contentHeight <= 0 {
		return
	}
	maxStart := 0
	if len(sess.lines) > contentHeight {
		maxStart = len(sess.lines) - contentHeight
	}
	if toTop {
		state.scroll[name] = 0
		state.follow[name] = false
	} else {
		state.scroll[name] = maxStart
		state.follow[name] = true
	}
}

func contentHeightForIndex(count, index int, screen tcell.Screen) int {
	_, height := screen.Size()
	statusHeight := 1
	if height < 2 {
		statusHeight = 0
	}
	gridHeight := height - statusHeight
	if gridHeight <= 0 {
		return 0
	}
	cols, rows := gridDims(count)
	if cols <= 0 || rows <= 0 {
		return 0
	}
	row := index / cols
	y0 := (gridHeight * row) / rows
	y1 := (gridHeight * (row + 1)) / rows
	h := y1 - y0
	if h <= 1 {
		return 0
	}
	contentTop := y0 + 2
	if h <= 3 {
		contentTop = y0 + 1
	}
	contentHeight := y1 - 1 - contentTop
	if contentHeight < 0 {
		return 0
	}
	return contentHeight
}
