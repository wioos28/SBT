package tui

// PaletteEntry is one command in the palette.
type PaletteEntry struct {
	Name   string
	Hint   string
	Run    func(*App)
	Filter []string
}

// PaletteState is the command palette's state.
type PaletteState struct {
	Open   bool
	Query  string
	Cursor int
}

// matches reports whether the entry survives the query filter.
func (p *PaletteState) matches(e PaletteEntry) bool {
	if p.Query == "" {
		return true
	}
	if containsFold(e.Name, p.Query) {
		return true
	}
	for _, kw := range e.Filter {
		if containsFold(kw, p.Query) {
			return true
		}
	}
	return false
}

// entries returns the filtered palette entries.
func (p *PaletteState) entries(pal *Palette) []PaletteEntry {
	out := []PaletteEntry{}
	for _, e := range pal.entries() {
		if p.matches(e) {
			out = append(out, e)
		}
	}
	return out
}

// drawPalette draws the command palette above everything else.
func (i *Interpreter) drawPalette(b *Buffer, s *Snapshot, st *UIState) {
	t := i.Theme
	p := t.Palette
	w := 52
	if st.Width-4 < w {
		w = st.Width - 4
	}
	if w < 20 {
		return
	}
	x := (st.Width - w) / 2
	y := 2
	h := 12
	if h > st.Height-4 {
		h = st.Height - 4
	}
	if h < 5 {
		return
	}
	inner := t.Panel(b, x, y, w, h, "commands · type to filter", "esc close", p.Yellow, true)
	if inner.Empty() {
		return
	}
	b.WriteClipped(inner.X, inner.Y, inner.Right(), "find: "+st.Palette.Query, Style{Fg: p.Text, Bold: true})
	entries := st.Palette.entries(i.Palette)
	rows := inner.H - 2
	if rows <= 0 {
		return
	}
	if st.Palette.Cursor >= len(entries) {
		st.Palette.Cursor = len(entries) - 1
	}
	if st.Palette.Cursor < 0 {
		st.Palette.Cursor = 0
	}
	start := 0
	if len(entries) > rows {
		start = st.Palette.Cursor - rows/2
		if start < 0 {
			start = 0
		}
		if start > len(entries)-rows {
			start = len(entries) - rows
		}
	}
	for j := 0; j < rows && start+j < len(entries); j++ {
		e := entries[start+j]
		sty := Style{Fg: p.Text}
		if start+j == st.Palette.Cursor {
			sty = Style{Fg: p.Bg, Bg: p.Yellow, HasBg: true, Bold: true}
		}
		line := Pad(e.Name, inner.W-StringWidth(e.Hint)-2)
		b.WriteClipped(inner.X, inner.Y+2+j, inner.Right()-StringWidth(e.Hint)-1, line, sty)
		b.WriteRight(inner.Right(), inner.Y+2+j, e.Hint, sty)
	}
	if len(entries) == 0 {
		b.WriteClipped(inner.X, inner.Y+2, inner.Right(), "no matching command", Style{Fg: p.Muted})
	}
}

// confirmButtons is the button strip of the confirm modal.
func confirmButtons() (string, int, int) {
	const strip = " [ run it ]   [ stay safe ]"
	// Offsets of "run it" and "stay safe" inside the strip.
	return strip, StringWidth(" [ "), StringWidth(" [ run it ]   [ ")
}

// drawConfirm draws the confirmation modal. The dangerous action is always the
// left button and the safe action always the right one, so position carries the
// meaning with or without colour.
func (i *Interpreter) drawConfirm(b *Buffer, st *UIState) {
	t := i.Theme
	p := t.Palette
	w := 54
	if st.Width-6 < w {
		w = st.Width - 6
	}
	if w < 24 {
		return
	}
	x := (st.Width - w) / 2
	h := 6 + len(st.Confirm.Body)
	y := (st.Height - h) / 2
	if y < 2 {
		y = 2
	}
	inner := t.Panel(b, x, y, w, h, st.Confirm.Title, "esc = safe", p.Red, true)
	if inner.Empty() {
		return
	}
	for j, ln := range st.Confirm.Body {
		if inner.Y+j >= inner.Bottom()-1 {
			break
		}
		sty := Style{Fg: p.Text}
		if strings.HasPrefix(ln, "!") {
			sty = Style{Fg: p.Red, Bold: true}
		}
		b.WriteClipped(inner.X, inner.Y+j, inner.Right(), ln, sty)
	}
	strip, left, right := confirmButtons()
	b.WriteClipped(inner.X, inner.Bottom()-1, inner.Right(), strip, Style{Fg: p.Muted})
	col := left
	style := Style{Fg: p.Red, Bold: true, Reverse: true}
	if st.Confirm.Choice != 0 {
		col = right
		style = Style{Fg: p.Green, Bold: true, Reverse: true}
	}
	choice := "run it"
	if st.Confirm.Choice != 0 {
		choice = "stay safe"
	}
	b.WriteClipped(inner.X+col, inner.Bottom()-1, inner.Right(), choice, style)
}
