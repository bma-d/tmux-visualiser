package main

import (
	"context"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestGridDims(t *testing.T) {
	cases := []struct {
		count int
		cols  int
		rows  int
	}{
		{0, 1, 1},
		{1, 1, 1},
		{2, 2, 1},
		{3, 2, 2},
		{4, 2, 2},
		{5, 3, 2},
		{9, 3, 3},
	}
	for _, c := range cases {
		cols, rows := gridDims(c.count)
		if cols != c.cols || rows != c.rows {
			t.Fatalf("gridDims(%d) = %dx%d, want %dx%d", c.count, cols, rows, c.cols, c.rows)
		}
	}
}

func TestOrderedSessionNames(t *testing.T) {
	state := appState{sessions: map[string]sessionView{"b": {}, "a": {}, "c": {}}}
	names := orderedSessionNames(state)
	if len(names) != 3 || names[0] != "a" || names[1] != "b" || names[2] != "c" {
		t.Fatalf("orderedSessionNames = %v", names)
	}
}

func TestFocusIndexForName(t *testing.T) {
	names := []string{"alpha", "beta", "gamma"}
	if idx := focusIndexForName(names, "beta"); idx != 1 {
		t.Fatalf("focusIndexForName beta = %d", idx)
	}
	if idx := focusIndexForName(names, "missing"); idx != -1 {
		t.Fatalf("focusIndexForName missing = %d", idx)
	}
}

func TestSessionIndexAt(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init screen: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(100, 40)

	if idx := sessionIndexAt(screen, 4, 10, 10); idx != 0 {
		t.Fatalf("idx at (10,10) = %d", idx)
	}
	if idx := sessionIndexAt(screen, 4, 60, 10); idx != 1 {
		t.Fatalf("idx at (60,10) = %d", idx)
	}
	if idx := sessionIndexAt(screen, 4, 10, 25); idx != 2 {
		t.Fatalf("idx at (10,25) = %d", idx)
	}
	if idx := sessionIndexAt(screen, 4, 60, 25); idx != 3 {
		t.Fatalf("idx at (60,25) = %d", idx)
	}
	if idx := sessionIndexAt(screen, 4, 10, 39); idx != -1 {
		t.Fatalf("idx at (10,39) = %d", idx)
	}
}

func TestContentHeightForIndex(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init screen: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(80, 20)

	if h := contentHeightForIndex(2, 0, screen); h != 16 {
		t.Fatalf("contentHeightForIndex = %d", h)
	}
}

func TestParseSGRParams(t *testing.T) {
	params := parseSGRParams("")
	if len(params) != 1 || params[0] != 0 {
		t.Fatalf("parse empty = %v", params)
	}
	params = parseSGRParams("1;31;0")
	if len(params) != 3 || params[0] != 1 || params[1] != 31 || params[2] != 0 {
		t.Fatalf("parse 1;31;0 = %v", params)
	}
	params = parseSGRParams(";;")
	if len(params) != 3 || params[0] != 0 || params[1] != 0 || params[2] != 0 {
		t.Fatalf("parse ;; = %v", params)
	}
}

func TestApplySGR(t *testing.T) {
	base := tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack)
	state := ansiState{style: base}

	state = applySGR(state, base, []int{31})
	fg, _, _ := state.style.Decompose()
	if fg != tcell.ColorMaroon {
		t.Fatalf("fg = %v", fg)
	}

	state = applySGR(state, base, []int{1})
	_, _, attr := state.style.Decompose()
	if attr&tcell.AttrBold == 0 {
		t.Fatalf("bold not set")
	}

	state = applySGR(state, base, []int{38, 2, 10, 20, 30})
	fg, _, _ = state.style.Decompose()
	if fg != tcell.NewRGBColor(10, 20, 30) {
		t.Fatalf("rgb fg = %v", fg)
	}

	state = applySGR(state, base, []int{48, 5, 200})
	_, bg, _ := state.style.Decompose()
	if bg != tcell.PaletteColor(200) {
		t.Fatalf("palette bg = %v", bg)
	}

	state = applySGR(state, base, []int{0})
	fg, bg, _ = state.style.Decompose()
	if fg != tcell.ColorWhite || bg != tcell.ColorBlack {
		t.Fatalf("reset fg/bg = %v/%v", fg, bg)
	}
}

func TestDrawAnsiText(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init screen: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(10, 2)
	base := tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack)

	drawAnsiText(screen, 0, 0, 10, "A\x1b[31mB\x1b[0mC", base)

	r, _, _, _ := screen.GetContent(0, 0)
	if r != 'A' {
		t.Fatalf("got %q at 0", r)
	}
	_, _, styleB, _ := screen.GetContent(1, 0)
	fg, _, _ := styleB.Decompose()
	if fg != tcell.ColorMaroon {
		t.Fatalf("B fg = %v", fg)
	}
	_, _, styleC, _ := screen.GetContent(2, 0)
	fg, _, _ = styleC.Decompose()
	if fg != tcell.ColorWhite {
		t.Fatalf("C fg = %v", fg)
	}
}

func TestHandleComposeKey(t *testing.T) {
	state := appState{sessions: map[string]sessionView{"one": {}}}
	cfg := config{}
	ctx := context.Background()

	if !handleComposeKey(ctx, &state, cfg, tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone)) {
		t.Fatalf("expected handled")
	}
	if string(state.composeBuf) != "a" {
		t.Fatalf("buf = %q", string(state.composeBuf))
	}

	handleComposeKey(ctx, &state, cfg, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if string(state.composeBuf) != "a\n" {
		t.Fatalf("buf after enter = %q", string(state.composeBuf))
	}

	handleComposeKey(ctx, &state, cfg, tcell.NewEventKey(tcell.KeyBackspace, 0, tcell.ModNone))
	if string(state.composeBuf) != "a" {
		t.Fatalf("buf after backspace = %q", string(state.composeBuf))
	}

	state.composeActive = true
	handleComposeKey(ctx, &state, cfg, tcell.NewEventKey(tcell.KeyCtrlS, 0, tcell.ModNone))
	if state.composeActive || !state.selectTarget {
		t.Fatalf("composeActive/selectTarget = %v/%v", state.composeActive, state.selectTarget)
	}
}

func TestMoveFocus(t *testing.T) {
	state := appState{sessions: map[string]sessionView{"b": {}, "a": {}, "c": {}}, focusIndex: 0}
	moveFocus(&state, 1)
	if state.focusIndex != 1 {
		t.Fatalf("focusIndex = %d", state.focusIndex)
	}
	moveFocus(&state, -1)
	if state.focusIndex != 0 {
		t.Fatalf("focusIndex = %d", state.focusIndex)
	}
}

func TestAnsiBasicColor(t *testing.T) {
	if c := ansiBasicColor(1, true); c != tcell.ColorRed {
		t.Fatalf("bright red = %v", c)
	}
	if c := ansiBasicColor(6, false); c != tcell.ColorTeal {
		t.Fatalf("teal = %v", c)
	}
}

func TestClamp8(t *testing.T) {
	if clamp8(-5) != 0 {
		t.Fatalf("clamp8 -5")
	}
	if clamp8(300) != 255 {
		t.Fatalf("clamp8 300")
	}
	if clamp8(123) != 123 {
		t.Fatalf("clamp8 123")
	}
}
