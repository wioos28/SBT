package tui

// The Settings screen drawing.
//
// The layout is two columns when there is room for them and one when there is
// not: the section list is a convenience, never a requirement, so a 40-column
// terminal gets the same settings with the current section named in the border.
// Nothing here is clipped mid-word by accident - every write is clipped to the
// panel it is drawn in.

const settingsRail = 18

// settingsView draws the Settings centre.
func (i *Interpreter) settingsView(b *Buffer, s *Snapshot, st *UIState, r Rect) {
	t := i.Theme
	p := t.Palette
	ss := &st.Settings
	ss.clamp()
	sec := settingSections[ss.Section]

	wide := r.W >= 52
	right := ""
	if ss.Message != "" {
		right = ss.Message
	}
	inner := t.Panel(b, r.X, r.Y, r.W, r.H, "settings", right, p.Border, st.View == ViewSettings)
	if inner.Empty() {
		return
	}

	body := inner
	if wide {
		i.settingsRail(b, inner, st)
		body = Rect{X: inner.X + settingsRail, Y: inner.Y, W: inner.W - settingsRail, H: inner.H}
	}

	y := body.Y
	rows := body.H - 2
	if rows < 1 {
		rows = 1
	}
	if ss.Cursor < ss.Scroll {
		ss.Scroll = ss.Cursor
	}
	if ss.Cursor >= ss.Scroll+rows {
		ss.Scroll = ss.Cursor - rows + 1
	}
	if ss.Scroll < 0 {
		ss.Scroll = 0
	}

	for j := 0; j < rows && ss.Scroll+j < len(sec.Rows); j++ {
		idx := ss.Scroll + j
		row := sec.Rows[idx]
		selected := idx == ss.Cursor
		sty := Style{Fg: p.Text}
		if selected {
			sty = Style{Fg: p.Primary, Bold: true}
		}
		value := ""
		if row.Kind == settingAction {
			value = "enter"
		} else {
			value = settingValueLabel(row.Key, s.Settings[row.Key])
		}
		if selected && ss.Editing {
			value = ss.Draft + t.Glyphs().Caret
		}
		label := Pad(row.Label, body.W-18)
		b.WriteClipped(body.X, y+j, body.Right(), label, sty)
		valSty := sty
		if row.Key == "security.require_isolation" && !isTrue(s.Settings[row.Key]) {
			valSty.Fg = p.Red
		}
		b.WriteRight(body.Right(), y+j, Truncate(value, 16), valSty)
		if row.Kind == settingToggle {
			g := t.Glyphs()
			mark := g.Dot
			if isTrue(s.Settings[row.Key]) {
				mark = g.Check
			}
			b.Set(body.Right()-17, y+j, k(mark), valSty)
		}
	}

	hintY := body.Bottom() - 1
	if hintY > 0 && hintY <= body.Bottom() {
		b.WriteClipped(body.X, hintY, body.Right(),
			"left/right section  up/down row  enter edit  esc back", Style{Fg: p.Muted})
	}
}

// settingsRail draws the section column.
func (i *Interpreter) settingsRail(b *Buffer, r Rect, st *UIState) {
	t := i.Theme
	p := t.Palette
	col := Rect{X: r.X, Y: r.Y, W: settingsRail, H: r.H}
	for j := 0; j < len(settingSections); j++ {
		y := col.Y + j
		if y >= col.Bottom() {
			break
		}
		title := settingSections[j].Title
		if j == st.Settings.Section {
			b.Fill(col.X, y, col.W-1, 1, ' ', Style{Bg: p.Primary, HasBg: true, Fg: p.Bg})
			b.WriteClipped(col.X+1, y, col.Right(), title, Style{Bg: p.Primary, HasBg: true, Fg: p.Bg, Bold: true})
			continue
		}
		b.WriteClipped(col.X+1, y, col.Right(), title, Style{Fg: p.Muted})
	}
}

// isTrue reports whether a stored setting is enabled.
func isTrue(v any) bool {
	b, ok := v.(bool)
	return ok && b
}
