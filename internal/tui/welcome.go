package tui

import "time"

// The welcome panel.
//
// It is the screen between "the checks are done" and "you can type". It names
// the product, states what the session actually verified, and lists the keys
// that get the user moving - keyboard first, because every action below has a
// key and none of them needs a command to be typed.
//
// The verdict line is the real cage state, not a reassuring slogan: a limited
// or broken cage says so here, before the user has typed anything.

// Welcome draws the welcome panel. On a terminal too small for the box it
// degrades to a single line rather than drawing clipped garbage.
func (i *Interpreter) Welcome(b *Buffer, snap *Snapshot, st *UIState, now time.Time) {
	t := i.Theme
	p := t.Palette
	w, h := b.W, b.H
	if w < 24 || h < 8 {
		b.WriteClipped(0, 0, w, Wordmark+" - Secure Sandbox Terminal", Style{Fg: p.Primary, Bold: true})
		return
	}
	boxW := 52
	if w-4 < boxW {
		boxW = w - 4
	}
	boxH := 13
	if h-2 < boxH {
		boxH = h - 2
	}
	x := (w - boxW) / 2
	y := (h - boxH) / 2
	if y < 1 {
		y = 1
	}
	t.Panel(b, x, y, boxW, boxH, "", "", p.Primary, true)
	inner := Inner(x, y, boxW, boxH, true)

	mark := Wordmark
	mx := inner.X + (inner.W-StringWidth(mark))/2
	if mx < inner.X {
		mx = inner.X
	}
	logoCol := -1
	if t.Motion {
		logoCol = Sweep(now, st.Boot.Started, SweepPeriod, StringWidth(mark)+6) - 3
	}
	col := mx
	for _, ch := range mark {
		lit := 0.0
		if logoCol >= 0 {
			d := col - (mx + logoCol)
			if d < 0 {
				d = -d
			}
			if d < 5 {
				lit = 1 - float64(d)/5
			}
		}
		fg := p.Primary
		if lit > 0 {
			fg = Glow(p.Primary, lit*0.6)
		}
		b.Set(col, inner.Y+1, ch, Style{Fg: fg, Bold: true})
		col++
	}

	cy := inner.Y + 3
	center := func(text string, sty Style) {
		cx := inner.X + (inner.W-StringWidth(text))/2
		if cx < inner.X {
			cx = inner.X
		}
		b.WriteClipped(cx, cy, inner.Right(), text, sty)
		cy++
	}
	center("Welcome to SBT", Style{Fg: p.Text, Bold: true})
	center("Secure Sandbox Terminal", Style{Fg: p.Info})
	cy++

	verdict := snap.Cage.State.Label()
	vstyle := Style{Fg: p.Green, Bold: true}
	if snap.Cage.State == CageLimited || snap.Cage.State == CageBroken {
		vstyle = Style{Fg: p.Warning, Bold: true}
	}
	center("cage "+verdict+"  -  "+snap.Platform, vstyle)
	cy++

	hints := [][2]string{{"^v", "navigate"}, {"<>", "sections"}, {"ENTER", "select"}, {"ESC", "back"}}
	line := ""
	for j, hp := range hints {
		if j > 0 {
			line += "   "
		}
		line += hp[0] + " " + hp[1]
	}
	center(line, Style{Fg: p.Muted})
	center("/help   /setting   /permissions   /status", Style{Fg: p.Muted})
	cy++
	center("press any key to enter the terminal", Style{Fg: p.Secondary})
}
