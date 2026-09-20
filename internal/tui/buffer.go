package tui

import "strings"

// Cell is one character cell.
type Cell struct {
	R rune
	S Style
}

// Buffer is a rectangular grid of styled cells. Views draw into a Buffer and
// the Screen turns the difference between two buffers into terminal output.
type Buffer struct {
	W, H  int
	Cells []Cell
}

// NewBuffer returns an empty buffer of the given size.
func NewBuffer(w, h int) *Buffer {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	b := &Buffer{W: w, H: h, Cells: make([]Cell, w*h)}
	b.Clear()
	return b
}

// Clear resets every cell to a space with the base style.
func (b *Buffer) Clear() {
	for i := range b.Cells {
		b.Cells[i] = Cell{R: ' '}
	}
}

// Resize changes the buffer geometry, discarding the content.
func (b *Buffer) Resize(w, h int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if b.W == w && b.H == h {
		b.Clear()
		return
	}
	b.W, b.H = w, h
	b.Cells = make([]Cell, w*h)
	b.Clear()
}

// In reports whether a coordinate is inside the buffer.
func (b *Buffer) In(x, y int) bool { return x >= 0 && y >= 0 && x < b.W && y < b.H }

// Set writes one cell.
func (b *Buffer) Set(x, y int, r rune, s Style) {
	if !b.In(x, y) {
		return
	}
	b.Cells[y*b.W+x] = Cell{R: r, S: s}
}

// Fill paints a rectangle with a rune and a style.
func (b *Buffer) Fill(x, y, w, h int, r rune, s Style) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			b.Set(i, j, r, s)
		}
	}
}

// Write draws text starting at x and returns the next free column. Wide runes
// occupy two cells and are followed by a padding cell so the grid stays aligned
// with what the terminal displays.
func (b *Buffer) Write(x, y int, text string, s Style) int {
	col := x
	for _, r := range text {
		w := runeWidth(r)
		switch {
		case w == 0:
			continue // combining marks are dropped rather than misaligned
		case w == 2:
			b.Set(col, y, r, s)
			b.Set(col+1, y, 0, s) // padding cell rendered as nothing
			col += 2
		default:
			b.Set(col, y, r, s)
			col++
		}
	}
	return col
}

// WriteClipped draws text clipped to a maximum column (exclusive).
func (b *Buffer) WriteClipped(x, y, limit int, text string, s Style) {
	if limit <= x {
		return
	}
	col := x
	for _, r := range text {
		w := runeWidth(r)
		if w == 0 {
			continue
		}
		if col+w > limit {
			break
		}
		if w == 2 {
			b.Set(col, y, r, s)
			b.Set(col+1, y, 0, s)
			col += 2
			continue
		}
		b.Set(col, y, r, s)
		col++
	}
}

// WriteRight draws text so that it ends at column right (exclusive).
func (b *Buffer) WriteRight(right, y int, text string, s Style) {
	w := StringWidth(text)
	if w == 0 {
		return
	}
	b.WriteClipped(right-w, y, right, text, s)
}

// StringWidth is the printable width of a string in cells.
func StringWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

// Pad pads or truncates s to exactly width cells.
func Pad(s string, width int) string {
	w := StringWidth(s)
	if w == width {
		return s
	}
	if w > width {
		return Truncate(s, width)
	}
	return s + strings.Repeat(" ", width-w)
}

// Truncate cuts s to at most width cells, appending an ellipsis when it cut.
func Truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if StringWidth(s) <= width {
		return s
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := runeWidth(r)
		if w+rw > width-1 {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String() + "…"
}

// runeWidth is the number of terminal cells a rune occupies. Zero-width
// characters return 0, East Asian wide characters return 2.
func runeWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case r < 32:
		return 1
	case r < 0x0300:
		return 1
	case r >= 0x0300 && r <= 0x036F, // combining diacritics
		r >= 0x200B && r <= 0x200F, // zero width space and marks
		r == 0xFEFF:                // BOM
		return 0
	case r >= 0x1100 && r <= 0x115F, // Hangul Jamo
		r >= 0x2E80 && r <= 0x303E, // CJK radicals and punctuation
		r >= 0x3041 && r <= 0x33FF,
		r >= 0x3400 && r <= 0x4DBF,
		r >= 0x4E00 && r <= 0x9FFF,
		r >= 0xA000 && r <= 0xA4CF,
		r >= 0xAC00 && r <= 0xD7A3,
		r >= 0xF900 && r <= 0xFAFF,
		r >= 0xFE30 && r <= 0xFE6F,
		r >= 0xFF00 && r <= 0xFF60,
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x1F300 && r <= 0x1F64F,
		r >= 0x1F900 && r <= 0x1F9FF,
		r >= 0x20000 && r <= 0x3FFFD:
		return 2
	default:
		return 1
	}
}
