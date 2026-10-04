package tui

import (
	"strings"
	"time"
)

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
	next := b.Write(x, y, glyph, Style{Fg: col, Bold: true})
	next = b.Write(next, y, " "+text, Style{Fg: t.Palette.Text, Bold: bold})
	return next
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

// SpinnerFrames are the frames of the indeterminate activity indicator.
var SpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ASCIISpinnerFrames is the fallback for terminals that cannot draw braille.
var ASCIISpinnerFrames = []string{"|", "/", "-", "\\", "|", "/", "-", "\\"}

// spinnerFrames picks the frame set matching the theme.
func (t *Theme) spinnerFrames() []string {
	if t != nil && t.ASCII {
		return ASCIISpinnerFrames
	}
	return SpinnerFrames
}

// Spinner returns the current activity frame. With motion disabled it returns
// the first frame: the indicator still appears, it simply does not move.
func (t *Theme) Spinner(now, since time.Time) string {
	frames := t.spinnerFrames()
	if len(frames) == 0 {
		return ""
	}
	if t != nil && !t.Motion {
		return frames[0]
	}
	return frames[SpinnerFrame(now, since, len(frames))]
}

// AnimatedMeter draws a bar that eases toward its measured value instead of
// snapping to it. A meter that jumps reads as a glitch; one that moves reads as
// a measurement.
//
// Upward moves are eased so a burst of CPU is visible as it grows. Downward
// moves are not eased: usage genuinely falling is news, and smoothing it would
// delay it. The caller owns the eased value (see MeterEase) so this function
// stays a pure draw.
func (t *Theme) AnimatedMeter(b *Buffer, x, y, w int, label string, shown, target float64, col RGB, suffix string) {
	t.Meter(b, x, y, w, label, shown, target, col, suffix)
}

// ProgressBar draws a labelled horizontal bar that fills over time, used for
// the sandbox startup indicator. progress is 0..1.
func (t *Theme) ProgressBar(b *Buffer, x, y, w int, label string, progress float64, col RGB) {
	if w < 8 {
		return
	}
	labelW := 0
	if label != "" {
		labelW = StringWidth(label) + 1
	}
	b.WriteClipped(x, y, x+labelW, label, Style{Fg: t.Palette.Muted})
	barW := w - labelW - 2
	if barW < 4 {
		return
	}
	g := t.Glyphs()
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	filled := int(progress*float64(barW) + 0.5)
	b.WriteClipped(x+labelW, y, x+labelW+barW,
		strings.Repeat(g.BlockFull, filled)+strings.Repeat(g.BlockEmpty, barW-filled),
		Style{Fg: col})
}

// PulseBar draws a bar whose fill sweeps back and forth, the still-motion
// equivalent of ProgressBar for an indeterminate operation. It is what makes
// "starting sandbox" read as in-progress instead of merely stated.
func (t *Theme) PulseBar(b *Buffer, x, y, w int, now, since time.Time, col RGB) {
	if w < 8 {
		return
	}
	g := t.Glyphs()
	b.Fill(x, y, w, 1, ' ', Style{Fg: t.Palette.Text})
	pos := 0
	if t != nil && t.Motion {
		pos = Sweep(now, since, SweepPeriod, w)
	}
	// A short bright head with a dimmer trail: the tail is what communicates
	// direction, so the bar looks like it is going somewhere.
	head := min(pos, w-1)
	b.Set(x+head, y, k(g.BlockFull), Style{Fg: Glow(col, 0.35), Bold: true})
	for i := 1; i <= 6; i++ {
		c := x + head - i
		if c < x {
			break
		}
		b.Set(c, y, k(g.BlockFull), Style{Fg: Mix(col, t.Palette.Bg, float64(i)/7.0)})
	}
}

// Bar renders a meter whose fill is already computed, which is how the
// renderer animates a value it is easing itself.
func (t *Theme) Bar(b *Buffer, x, y, barW int, ratio float64, col RGB) {
	if barW < 1 {
		return
	}
	g := t.Glyphs()
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := int(ratio*float64(barW) + 0.5)
	b.WriteClipped(x, y, x+barW,
		strings.Repeat(g.BlockFull, filled)+strings.Repeat(g.BlockEmpty, barW-filled),
		Style{Fg: col})
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
			b.WriteClipped(col, y, x+w, "  ", Style{Fg: t.Palette.Muted})
			col += 2
		}
		b.WriteClipped(col, y, x+w, h[0], Style{Fg: t.Palette.YellowHi, Bold: true})
		col += StringWidth(h[0])
		b.WriteClipped(col, y, x+w, " "+h[1], Style{Fg: t.Palette.Muted})
		col += StringWidth(" " + h[1])
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
