package main

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
)

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

	if state.composeActive {
		drawComposeOverlay(screen, width, height, state)
	} else if state.selectTarget {
		drawSelectOverlay(screen, width, height)
	}

	if statusHeight == 1 {
		drawStatus(screen, width, height-1, statusStyle, state, cfg, len(sessions))
	}

	screen.Show()
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
		drawAnsiText(screen, x0+1, contentTop+row, w-2, sess.lines[lineIndex], bodyStyle)
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
	mouseState := "off"
	if state.mouseEnabled {
		mouseState = "on"
	}
	label := fmt.Sprintf("sessions:%d | lines:%d | interval:%s | tab:focus j/k:scroll i:compose Ctrl+K:kill [ ]:interval m:mouse(%s) q:quit", sessionCount, cfg.lines, cfg.interval, mouseState)
	if state.composeActive {
		label = "compose: type freely (enter=newline) | Ctrl+S choose target | Esc cancel"
	}
	if state.selectTarget {
		label = "select target: click or Tab/Shift+Tab | Enter send | Esc cancel"
	}
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

func drawComposeOverlay(screen tcell.Screen, width, height int, state appState) {
	statusHeight := 1
	if height < 2 {
		statusHeight = 0
	}
	maxBottom := height - statusHeight
	if width < 10 || maxBottom < 5 {
		return
	}
	overlayHeight := maxInt(5, maxBottom/3)
	if overlayHeight > maxBottom {
		overlayHeight = maxBottom
	}
	y0 := maxBottom - overlayHeight
	y1 := maxBottom

	boxStyle := tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDarkSlateGray)
	headStyle := tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorLightGray).Bold(true)
	textStyle := tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDarkSlateGray)

	drawBox(screen, 0, y0, width, y1, boxStyle)
	drawText(screen, 1, y0+1, width-2, "Compose: Ctrl+S choose target | Esc cancel | Enter newline", headStyle)

	contentTop := y0 + 2
	contentHeight := y1 - 1 - contentTop
	if contentHeight <= 0 {
		return
	}
	lines := strings.Split(string(state.composeBuf), "\n")
	start := 0
	if len(lines) > contentHeight {
		start = len(lines) - contentHeight
	}
	for row := 0; row < contentHeight; row++ {
		lineIndex := start + row
		if lineIndex >= len(lines) {
			break
		}
		drawText(screen, 1, contentTop+row, width-2, lines[lineIndex], textStyle)
	}
}

func drawSelectOverlay(screen tcell.Screen, width, height int) {
	if width < 10 || height < 3 {
		return
	}
	boxStyle := tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorYellow)
	drawBox(screen, 0, 0, width, 3, boxStyle)
	drawText(screen, 1, 1, width-2, "Select target: click or Tab/Shift+Tab | Enter send | Esc cancel", boxStyle)
}
