package main

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
)

type ansiState struct {
	style     tcell.Style
	bold      bool
	underline bool
	reverse   bool
}

func drawAnsiText(screen tcell.Screen, x, y, width int, text string, baseStyle tcell.Style) {
	if width <= 0 {
		return
	}
	state := ansiState{style: baseStyle}
	col := 0
	for i := 0; i < len(text) && col < width; {
		if text[i] == 0x1b && i+1 < len(text) && text[i+1] == '[' {
			end := i + 2
			for end < len(text) && text[end] != 'm' {
				end++
			}
			if end < len(text) {
				params := parseSGRParams(text[i+2 : end])
				state = applySGR(state, baseStyle, params)
				i = end + 1
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 1 {
			i++
			continue
		}
		if r == '\r' {
			i += size
			continue
		}
		if r == '\t' {
			spaces := 4 - (col % 4)
			for s := 0; s < spaces && col < width; s++ {
				screen.SetContent(x+col, y, ' ', nil, state.style)
				col++
			}
			i += size
			continue
		}
		screen.SetContent(x+col, y, r, nil, state.style)
		col++
		i += size
	}
	for col < width {
		screen.SetContent(x+col, y, ' ', nil, baseStyle)
		col++
	}
}

func parseSGRParams(s string) []int {
	if s == "" {
		return []int{0}
	}
	parts := strings.Split(s, ";")
	params := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			params = append(params, 0)
			continue
		}
		val, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		params = append(params, val)
	}
	if len(params) == 0 {
		return []int{0}
	}
	return params
}

func applySGR(state ansiState, base tcell.Style, params []int) ansiState {
	if len(params) == 0 {
		params = []int{0}
	}
	fgBase, bgBase, _ := base.Decompose()
	for i := 0; i < len(params); i++ {
		p := params[i]
		switch {
		case p == 0:
			state = ansiState{style: base}
		case p == 1:
			state.bold = true
			state.style = state.style.Bold(true)
		case p == 22:
			state.bold = false
			state.style = state.style.Bold(false)
		case p == 4:
			state.underline = true
			state.style = state.style.Underline(true)
		case p == 24:
			state.underline = false
			state.style = state.style.Underline(false)
		case p == 7:
			state.reverse = true
			state.style = state.style.Reverse(true)
		case p == 27:
			state.reverse = false
			state.style = state.style.Reverse(false)
		case p >= 30 && p <= 37:
			state.style = state.style.Foreground(ansiBasicColor(p-30, false))
		case p >= 90 && p <= 97:
			state.style = state.style.Foreground(ansiBasicColor(p-90, true))
		case p == 39:
			state.style = state.style.Foreground(fgBase)
		case p >= 40 && p <= 47:
			state.style = state.style.Background(ansiBasicColor(p-40, false))
		case p >= 100 && p <= 107:
			state.style = state.style.Background(ansiBasicColor(p-100, true))
		case p == 49:
			state.style = state.style.Background(bgBase)
		case p == 38 || p == 48:
			if i+1 >= len(params) {
				continue
			}
			mode := params[i+1]
			if mode == 5 && i+2 < len(params) {
				colorIdx := params[i+2]
				if colorIdx < 0 {
					colorIdx = 0
				}
				if colorIdx > 255 {
					colorIdx = 255
				}
				color := tcell.PaletteColor(colorIdx)
				if p == 38 {
					state.style = state.style.Foreground(color)
				} else {
					state.style = state.style.Background(color)
				}
				i += 2
			} else if mode == 2 && i+4 < len(params) {
				r := clamp8(params[i+2])
				g := clamp8(params[i+3])
				b := clamp8(params[i+4])
				color := tcell.NewRGBColor(int32(r), int32(g), int32(b))
				if p == 38 {
					state.style = state.style.Foreground(color)
				} else {
					state.style = state.style.Background(color)
				}
				i += 4
			}
		}
	}
	return state
}

func ansiBasicColor(index int, bright bool) tcell.Color {
	switch index {
	case 0:
		if bright {
			return tcell.ColorGray
		}
		return tcell.ColorBlack
	case 1:
		if bright {
			return tcell.ColorRed
		}
		return tcell.ColorMaroon
	case 2:
		if bright {
			return tcell.ColorLime
		}
		return tcell.ColorGreen
	case 3:
		if bright {
			return tcell.ColorYellow
		}
		return tcell.ColorOlive
	case 4:
		if bright {
			return tcell.ColorBlue
		}
		return tcell.ColorNavy
	case 5:
		if bright {
			return tcell.ColorFuchsia
		}
		return tcell.ColorPurple
	case 6:
		if bright {
			return tcell.ColorAqua
		}
		return tcell.ColorTeal
	case 7:
		if bright {
			return tcell.ColorWhite
		}
		return tcell.ColorSilver
	default:
		if bright {
			return tcell.ColorWhite
		}
		return tcell.ColorSilver
	}
}

func clamp8(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}
