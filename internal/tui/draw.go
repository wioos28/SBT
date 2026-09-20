package tui

import "strings"

// Rect is a rectangle in cell coordinates. X, Y are inclusive, W, H are sizes.
type Rect struct {
	X, Y, W, H int
}

// Empty reports whether the rectangle has no drawable area.
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }

// Right is the first column after the rectangle.
func (r Rect) Right() int { return r.X + r.W }

// Bottom is the first row after the rectangle.
func (r Rect) Bottom() int { return r.Y + r.H }

// Inner is the content area of a bordered rectangle of the given geometry.
func Inner(x, y, w, h int, border bool) Rect {
	if !border {
		return Rect{X: x, Y: y, W: w, H: h}
	}
	return Rect{X: x + 1, Y: y + 1, W: w - 2, H: h - 2}
}

// Panel draws a bordered section and returns its content rectangle.
//
// Titles sit in the top border (like a terminal frame) so no vertical space is
// wasted on a separate heading row, and the right hand slot carries the
// section's own status - which is where SBT puts "SANDBOX ACTIVE", "3 runs" and
// similar evidence.
func (t *Theme) Panel(b *Buffer, x, y, w, h int, title, right string, border RGB, focused bool) Rect {
	if w < 5 || h < 3 {
		return Rect{}
	}
	g := t.Glyphs()
	bs := Style{Fg: border}
	tl, tr, bl, br := g.TL, g.TR, g.BL, g.BR
	hz, vt := g.H, g.V
	if focused {
		// Focus is a shape change, not only a colour change: heavy lines plus
		// the accent colour make the active panel identifiable without colour.
		tl, tr, bl, br = g.HTL, g.HTR, g.HBL, g.HBR
		hz, vt = g.HH, g.HV
	}
	b.Set(x, y, k(tl), bs)
	b.Set(x+w-1, y, k(tr), bs)
	b.Set(x, y+h-1, k(bl), bs)
	b.Set(x+w-1, y+h-1, k(br), bs)
	for i := x + 1; i < x+w-1; i++ {
		b.Set(i, y, k(hz), bs)
		b.Set(i, y+h-1, k(hz), bs)
	}
	for j := y + 1; j < y+h-1; j++ {
		b.Set(x, j, k(vt), bs)
		b.Set(x+w-1, j, k(vt), bs)
	}
	titleStyle := Style{Fg: t.Palette.Text, Bold: true}
	if focused {
		titleStyle.Fg = border
	}
	if title != "" {
		b.WriteClipped(x+2, y, x+w-2, title, titleStyle)
	}
	if right != "" {
		b.WriteRight(x+w-2, y, right, Style{Fg: t.Palette.Muted})
	}
	if focused {
		// A literal marker so the active panel is unmistakable even in a
		// monochrome terminal.
		b.Set(x+1, y, k(g.Caret), Style{Fg: border, Bold: true})
	}
	return Inner(x, y, w, h, true)
}

// Chip draws a status chip such as "● LOW MODE" and returns the next free
// column. The glyph is always followed by words, never used alone.
func (t *Theme) Chip(b *Buffer, x, y int, glyph, text string, col RGB, bold bool) int {
	g := t.Glyphs()
	if glyph == "" {
		glyph = g.Dot
	}
	col = b.Write(x, y, glyph, Style{Fg: col, Bold: true})
	col = b.Write(col, y, " "+text, Style{Fg: t.Palette.Text, Bold: bold})
	return col
}

// Meter draws a labelled bar with its value, the shape used by the monitor and
// the resource section.
func (t *Theme) Meter(b *Buffer, x, y, w int, label string, value, max float64, col RGB, suffix string) {
	g := t.Glyphs()
	if w < 12 {
		return
	}
	const labelW = 6
	valueW := StringWidth(suffix)
	barW := w - labelW - valueW - 2
	if barW < 4 {
		barW = 4
	}
	b.WriteClipped(x, y, x+labelW, Pad(label, labelW), Style{Fg: t.Palette.Muted})
	ratio := 0.0
	if max > 0 {
		ratio = value / max
	}
	if ratio > 1 {
		ratio = 1
	}
	if ratio < 0 {
		ratio = 0
	}
	filled := int(ratio*float64(barW) + 0.5)
	bar := strings.Repeat(g.BlockFull, filled) + strings.Repeat(g.BlockEmpty, barW-filled)
	b.WriteClipped(x+labelW, y, x+labelW+barW, bar, Style{Fg: col})
	if suffix != "" {
		b.WriteRight(x+w, y, suffix, Style{Fg: t.Palette.Text})
	}
}

// Spark draws a sparkline of normalised values.
func (t *Theme) Spark(b *Buffer, x, y, w int, values []float64, col RGB) {
	g := t.Glyphs()
	if len(values) > w {
		values = values[len(values)-w:]
	}
	max := 0.0
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	if max <= 0 {
		b.WriteClipped(x, y, x+w, strings.Repeat(g.BlockEmpty, len(values)), Style{Fg: t.Palette.Border})
		return
	}
	col0 := x
	for _, v := range values {
		idx := int(v / max * float64(len(g.Spark)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(g.Spark) {
			idx = len(g.Spark) - 1
		}
		b.Set(col0, y, k(g.Spark[idx]), Style{Fg: col})
		col0++
	}
}

// KeyHints renders a compact shortcut hint line, e.g. "alt+1 rail  ctrl+k
// palette". Hints are words first so they stay readable without colour.
func (t *Theme) KeyHints(b *Buffer, x, y, w int, hints [][2]string) {
	col := x
	for i, h := range hints {
		if i > 0 {
			col = b.WriteClipped(col, y, x+w, "  ", Style{Fg: t.Palette.Muted})
		}
		col = b.WriteClipped(col, y, x+w, h[0], Style{Fg: t.Palette.YellowHi, Bold: true})
		col = b.WriteClipped(col, y, x+w, " "+h[1], Style{Fg: t.Palette.Muted})
		if col >= x+w {
			return
		}
	}
}

// Blank fills a region with the background colour used by panels.
func (t *Theme) Blank(b *Buffer, r Rect, surface RGB) {
	b.Fill(r.X, r.Y, r.W, r.H, ' ', Style{Bg: surface, HasBg: true, Fg: t.Palette.Text})
}

// k converts a glyph string to its first rune.
func k(s string) rune {
	for _, r := range s {
		return r
	}
	return ' '
}
