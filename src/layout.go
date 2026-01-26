package main

import (
	"math"

	"github.com/gdamore/tcell/v2"
)

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
	if rows < cols {
		cols, rows = rows, cols
	}
	return cols, rows
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

func sessionIndexAt(screen tcell.Screen, count, x, y int) int {
	if count <= 0 {
		return -1
	}
	width, height := screen.Size()
	if width <= 0 || height <= 0 {
		return -1
	}
	statusHeight := 1
	if height < 2 {
		statusHeight = 0
	}
	gridHeight := height - statusHeight
	if gridHeight <= 0 {
		return -1
	}
	if x < 0 || y < 0 || x >= width || y >= gridHeight {
		return -1
	}
	cols, rows := gridDims(count)
	if cols <= 0 || rows <= 0 {
		return -1
	}
	col := (x * cols) / width
	row := (y * rows) / gridHeight
	idx := row*cols + col
	if idx >= count {
		return -1
	}
	return idx
}
