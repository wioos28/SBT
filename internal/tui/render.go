package tui

import (
	"math"
	"strings"
	"time"

	"github.com/wioos28/sbt/internal/shared/workspace"
)

// Interpreter renders a Snapshot into a Buffer. It holds no state of its own
// beyond the theme, so the same interpreter can draw every frame.
type Interpreter struct {
	Theme *Theme
}

// NewInterpreter builds an interpreter for a theme.
func NewInterpreter(t *Theme) *Interpreter {
	if t == nil {
		t = NewTheme(DefaultPalette)
	}
	return &Interpreter{Theme: t}
}

// layout is the geometry of one frame, derived from the terminal size.
type layout struct {
	Body    Rect // area between the top bar and the status bar
	Rail    Rect // navigation rail, left
	Work    Rect // workspace area, centre
	Side    Rect // security/resources panel, right (absent on narrow terminals)
	HasRail bool
	HasSide bool
	// HasAlert is true when the warning banner gets its own row, and AlertY is
	// the row it occupies.
	HasAlert bool
	AlertY   int
	// HasMenu is false when the terminal is too short to carry a menu bar. The
	// menu is then reachable only through the command palette, which costs no
	// vertical space.
	HasMenu bool
}

const railWidth = 18

// computeLayout applies the minimum sizes the cage needs. Below them the UI
// degrades to a plain message instead of drawing clipped garbage.
//
// The menu bar takes a row off the top of the body. It is only reserved when
// the terminal can spare one: a three-row-tall terminal showing an empty menu
// strip and no content is strictly worse than showing the content.
func computeLayout(w, h, alertRows int) layout {
	var l layout
	if h < 8 || w < 24 {
		return l
	}
	top := 1
	if h >= 12 {
		l.HasMenu = true
		top = 2
	}
	// The alert banner takes a row off the top of the body, but only when the
	// body can spare it: a banner that squeezes the workspace out of existence
	// is worse than no banner.
	if alertRows > 0 && h-top-alertRows-1 < 4 {
		alertRows = 0
	}
	l.HasAlert = alertRows > 0
	l.AlertY = top
	l.Body = Rect{X: 0, Y: top + alertRows, W: w, H: h - top - alertRows - 1}
	col := 0
	if w >= 62 {
		l.HasRail = true
		l.Rail = Rect{X: 0, Y: l.Body.Y, W: railWidth, H: l.Body.H}
		col += railWidth
	}
	sideW := 0
	if w-col >= 84 {
		l.HasSide = true
		sideW = 36
		l.Side = Rect{X: w - sideW, Y: l.Body.Y, W: sideW, H: l.Body.H}
	}
	l.Work = Rect{X: col, Y: l.Body.Y, W: w - col - sideW, H: l.Body.H}
	return l
}

// Render draws one frame. The buffer is fresh each time; the screen diffs it
// against the previous frame, so only changed cells reach the terminal.
func (i *Interpreter) Render(s *Snapshot, st *UIState) *Buffer {
	w, h := st.Width, st.Height
	b := NewBuffer(w, h)
	now := s.Now
	alertRows := 0
	if st.Alert.Active(now) {
		alertRows = 1
	}
	l := computeLayout(w, h, alertRows)
	if l.Body.Empty() {
		msg := "terminal too small for the cage (need at least 24x8)"
		b.WriteClipped(0, h/2, w, msg, Style{Fg: i.Theme.Palette.Warning})
		return b
	}
	i.topBar(b, s, st, w)
	if l.HasMenu {
		i.menuBar(b, s, st, w)
	}
	if l.HasAlert {
		i.alertBanner(b, st.Alert, w, l.AlertY, now, st.AlertAnim)
	}
	i.rail(b, s, st, l)
	if l.HasSide {
		i.securityPanel(b, s, st, l.Side)
		i.resourcesPanel(b, s, st, l.Side)
	}
	i.workspace(b, s, st, l.Work)
	i.statusBar(b, s, st, w, h)
	// The busy strip takes over the rail header while the session works, rather
	// than overlaying the workspace: an overlay across the content would cover
	// the files the user is reading.
	if l.HasRail {
		i.busyStrip(b, s, st, l.Rail)
	}
	// Notices draw last so a warning sits above the content it reports on and
	// is never hidden behind a panel.
	i.toasts(b, s, st)
	// The isolation evidence sits above the toasts but below the modals: it is
	// a fact the user may want to read while a dialog is open, not a blocker.
	i.isolationPanel(b, s.Confinement, st, now, st.GlowAnim)
	// The dropdown is drawn above the workspace but below the modal overlays: a
	// menu is a shortcut, and a confirmation about something the menu started
	// must not be half-covered by it.
	if st.Menu.Open && l.HasMenu {
		i.menuDropdown(b, s, st, w)
	}
	if st.Palette.Open {
		i.drawPalette(b, s, st)
	}
	if anyConfirmOpen(st) {
		i.drawConfirm(b, st)
	}
	if st.Permission.Open {
		i.drawPermission(b, st)
	}
	i.clock(b, st, now)
	return b
}

// menuBar draws the row of menu titles under the top bar.
//
// The bar is always visible even when nothing is open, because a menu the user
// cannot see is a menu they will not look for. The active title is marked with
// the caret and an underline as well as the accent colour, so the open menu is
// identifiable in a monochrome terminal.
func (i *Interpreter) menuBar(b *Buffer, s *Snapshot, st *UIState, w int) {
	t := i.Theme
	p := t.Palette
	y := menuBarY
	b.Fill(0, y, w, 1, ' ', Style{Fg: p.Muted, Bg: p.BgTop, HasBg: true})
	for idx, m := range st.Menus.Menus {
		label := " " + m.Title
		// menuLabelX is the single answer to "where does this title start";
		// the click handler uses the same one, so a click cannot land on the
		// gap between two titles.
		col := menuLabelX(st.Menus, idx)
		if col+StringWidth(label) >= w-2 {
			// The remaining menus do not fit. Saying so beats silently
			// dropping half the bar with no indication that it exists.
			b.WriteRight(w-1, y, " +"+"more ", Style{Fg: p.Muted, Bg: p.BgTop, HasBg: true})
			return
		}
		active := st.Menu.Open && st.Menu.Bar == idx
		style := Style{Fg: p.Muted, Bg: p.BgTop, HasBg: true}
		if active {
			style = Style{Fg: p.Bg, Bg: p.Yellow, HasBg: true, Bold: true}
		}
		col = b.Write(col, y, label, style)
	}
	// The one key that reaches the whole bar, restated where the user is
	// already looking rather than only in the help view.
	hint := "f10 menu"
	if !st.Menu.Open {
		b.WriteRight(w-1, y, hint, Style{Fg: p.Muted, Bg: p.BgTop, HasBg: true})
	}
}

// menuDropdown draws the open menu under its title.
//
// The dropdown never covers the status bar or the top bar: it is anchored to
// the title it belongs to and clamped to the panel body, so the cage verdict
// stays readable while a menu is open. That matters because the verdict is the
// one piece of state that must never be hidden by a navigation affordance.
func (i *Interpreter) menuDropdown(b *Buffer, s *Snapshot, st *UIState, w int) {
	t := i.Theme
	p := t.Palette
	st.Menu.Clamp(st.Menus)
	if st.Menu.Bar < 0 || st.Menu.Bar >= len(st.Menus.Menus) {
		return
	}
	menu := st.Menus.Menus[st.Menu.Bar]
	if len(menu.Items) == 0 {
		return
	}

	// The geometry comes from dropdownGeom, which the click handler also uses.
	// Renderer and hit test sharing one function is what keeps a click on the
	// third row from selecting the second.
	g, ok := dropdownGeom(st, w)
	if !ok {
		return
	}
	// The bar row under the title is cleared so the dropdown has a solid
	// background where it overlaps the menu strip.
	b.Fill(g.Inner.X-1, menuBarY, g.Inner.W+2, 1, ' ', Style{Fg: p.Muted, Bg: p.BgTop, HasBg: true})
	inner := t.Panel(b, g.Inner.X-1, g.Inner.Y-1, g.Inner.W+2, g.Inner.H+2,
		"", "", p.Yellow, true)
	if inner.Empty() {
		return
	}

	rows := inner.H
	start := 0
	if len(menu.Items) > rows {
		start = st.Menu.Cursor - rows/2
		if start < 0 {
			start = 0
		}
		if start > len(menu.Items)-rows {
			start = len(menu.Items) - rows
		}
	}
	for j := 0; j < rows && start+j < len(menu.Items); j++ {
		item := menu.Items[start+j]
		selected := start+j == st.Menu.Cursor
		style := Style{Fg: p.Text}
		if item.Dangerous {
			style.Fg = p.Red
		}
		if selected {
			style = Style{Fg: p.Bg, Bg: p.Yellow, HasBg: true, Bold: true}
			// The highlight eases in rather than snapping, so opening a menu
			// reads as movement instead of a jump cut.
			if st.Motion {
				style.Bg = Mix(p.Yellow, p.Surface2, 1-Breath(s.Now, st.Menu.OpenedAt, PulsePeriod)*0.5)
			}
		}
		// A toggle shows its state as a word and a mark, so the row is
		// unambiguous without colour.
		mark := "  "
		if item.Checked != nil {
			mark = "[ ] "
			if item.Checked(s) {
				mark = "[x] "
			}
		}
		label := mark + item.Title
		if item.Dangerous && !selected {
			label = "! " + mark + item.Title
		}
		hintW := StringWidth(item.Hint)
		limit := inner.Right()
		if hintW > 0 {
			limit = inner.Right() - hintW - 1
		}
		b.WriteClipped(inner.X, inner.Y+j, limit, Truncate(label, limit-inner.X), style)
		if item.Hint != "" {
			b.WriteRight(inner.Right(), inner.Y+j, item.Hint, style)
		}
	}
}

// menuWidth is the dropdown's width: wide enough for the longest row including
// its hint and the danger marker.
func menuWidth(m Menu) int {
	longest := 0
	for _, item := range m.Items {
		w := StringWidth(item.Title) + StringWidth(item.Hint) + 8
		if item.Checked != nil {
			w += 4
		}
		if item.Dangerous {
			w += 2
		}
		if w > longest {
			longest = w
		}
	}
	if longest < 24 {
		longest = 24
	}
	if longest > 52 {
		longest = 52
	}
	return longest + 2
}

// toasts draws the notice stack in the bottom right of the cage.
//
// Each notice is a card with a coloured left edge, an icon and a line of text.
// The colour is a hint; the text carries the meaning, and every severity also
// arrives with the always-present status line behind it.
func (i *Interpreter) toasts(b *Buffer, s *Snapshot, st *UIState) {
	t := i.Theme
	p := t.Palette
	live := st.Toasts.Live(s.Now)
	if len(live) == 0 || st.Width < 40 || st.Height < 12 {
		return
	}
	w := 46
	if st.Width-8 < w {
		w = st.Width - 8
	}
	if w < 24 {
		return
	}
	// Stack upward from just above the status bar so the newest notice sits
	// closest to where the eye already is. Toasts are numbered newest-first,
	// so the newest is the one drawn nearest the bar.
	bottom := st.Height - 2
	x := st.Width - w - 2
	for n, item := range live {
		h := 2
		if item.Detail != "" {
			h = 3
		}
		// n=0 is the newest notice and belongs directly above the status bar.
		y := bottom - h - n*h
		if y < 1 {
			break
		}
		accent := toastColor(item.Kind, p)
		fade := item.Fade(s.Now)
		if fade <= 0 {
			continue
		}
		// The card body fades with the notice, but the accent edge keeps full
		// strength for the whole life so a warning stays locatable as it
		// dissolves.
		body := Style{Bg: p.Surface2, HasBg: true, Fg: Mix(p.Text, p.Surface2, 1-fade)}
		b.Fill(x, y, w, h, ' ', body)
		// Accent edge: two cells of colour, brighter as the notice fades in.
		edge := Style{Bg: Glow(accent, (1-fade)*0.3), HasBg: true, Fg: accent}
		b.Fill(x, y, 2, h, ' ', edge)

		textX := x + 3
		limit := x + w - 1
		icon := item.Icon
		if st.Motion && item.Kind >= StateWarn {
			// A warning blinks its icon so it is impossible to read past,
			// while the text next to it stays steady.
			if Blink(s.Now, item.SetAt, BlinkPeriod) {
				icon = item.Icon
			} else {
				icon = " "
			}
		}
		head := Style{Fg: Mix(accent, p.Surface2, 1-fade), Bold: true}
		c := b.Write(textX, y, icon, head)
		b.WriteClipped(c+1, y, limit, Truncate(item.Text, limit-(c+1-textX)), head)
		if h == 3 {
			b.WriteClipped(textX, y+1, limit,
				Truncate(item.Detail, limit-textX),
				Style{Fg: Mix(p.Muted, p.Surface2, 1-fade)})
		}
	}
}

// toastColor is the accent of a severity.
func toastColor(kind StateKind, p Palette) RGB {
	switch kind {
	case StateOK:
		return p.Green
	case StateWarn:
		return p.Warning
	case StateDanger:
		return p.Red
	default:
		return p.Muted
	}
}

// busyStrip replaces the rail's header rows while the session is working, so
// "working" is stated exactly where the session identity already lives. It is
// drawn into the rail rather than over the workspace: an overlay across the
// middle of the content would cover files the user is trying to read, and
// drawing it over the rail header left a half-erased label behind.
func (i *Interpreter) busyStrip(b *Buffer, s *Snapshot, st *UIState, rail Rect) {
	if !st.Busy.Active || rail.Empty() || rail.H < 3 {
		return
	}
	t := i.Theme
	p := t.Palette
	label := strings.ToUpper(st.Busy.Label)
	// Clear the whole rail header block first so nothing of the previous
	// contents shows through.
	t.Blank(b, Rect{X: rail.X, Y: rail.Y, W: rail.W, H: 3}, p.Bg)
	b.WriteClipped(rail.X+1, rail.Y, rail.Right(), Truncate(label, rail.W-2),
		Style{Fg: p.YellowHi, Bold: true})
	// The spinner sits under the label with a sweeping progress bar, so the
	// rail shows both what is happening and that time is passing.
	frame := t.Spinner(s.Now, st.Busy.Since)
	b.Set(rail.X+1, rail.Y+1, k(frame), Style{Fg: p.Yellow, Bold: true})
	t.PulseBar(b, rail.X+3, rail.Y+1, rail.W-4, s.Now, st.Busy.Since, p.Yellow)
}

// clock draws the session clock in the status bar. It is the one element that
// always moves, which is what makes a still interface read as alive rather than
// frozen.
func (i *Interpreter) clock(b *Buffer, st *UIState, now time.Time) {
	if st.Width < 46 || st.Height < 8 {
		return
	}
	text := now.Format("15:04:05")
	b.WriteRight(st.Width-1, st.Height-1, text, Style{Fg: i.Theme.Palette.Border})
}

// topBar draws the identity strip: what this is, which policy, what state.
// When the session is busy the badge carries a live spinner, so "working" is
// shown in the same place the verdict is shown rather than somewhere else.
func (i *Interpreter) topBar(b *Buffer, s *Snapshot, st *UIState, w int) {
	t := i.Theme
	g := t.Glyphs()
	p := t.Palette
	bg := Style{Bg: p.BgTop, HasBg: true, Fg: p.Text}
	b.Fill(0, 0, w, 1, ' ', bg)
	col := 0
	col = b.Write(col, 0, " SBT", Style{Fg: p.YellowHi, Bg: p.BgTop, HasBg: true, Bold: true})
	if s.Version != "" {
		col = b.Write(col, 0, " "+s.Version, Style{Fg: p.Muted, Bg: p.BgTop, HasBg: true})
	}
	col = b.Write(col, 0, "  ", bg)
	col = b.Write(col, 0, modeWord(s.Mode), Style{Fg: p.Text, Bg: p.BgTop, HasBg: true, Bold: true})
	if s.Policy.Mode != "" {
		col = b.Write(col, 0, " · "+string(s.Policy.Mode), Style{Fg: p.Muted, Bg: p.BgTop, HasBg: true})
	}
	if s.Host != "" {
		b.Write(col, 0, "  "+g.Dot+" "+s.Host, Style{Fg: p.Muted, Bg: p.BgTop, HasBg: true})
	}
	badge, badgeCol := i.cageBadge(s)
	// A busy session replaces the static badge dot with a moving one, so the
	// top bar is the single place that answers "is it working, and is it safe".
	if st.Busy.Active {
		badge = t.Spinner(s.Now, st.Busy.Since) + " " + strings.ToUpper(st.Busy.Label)
		badgeCol = p.Yellow
	}
	if st.Motion {
		// A live badge breathes very slightly: enough to read as active,
		// little enough that a long session does not become a distraction.
		breath := Breath(s.Now, st.LastKeyAt, PulsePeriod)
		badgeCol = Glow(badgeCol, breath*0.18)
	}
	b.WriteRight(w, 0, badge+" ", Style{Fg: badgeCol, Bg: p.BgTop, HasBg: true, Bold: true})
	// The theme name sits immediately left of the verdict, separated by a
	// divider. Both are right-aligned, so the eye finds the pair together and
	// the verdict stays the last thing read.
	i.groundChip(b, s, w-StringWidth(badge)-2, 0)
}

// groundChip names the current theme in the top bar.
//
// It is a small thing, but an interface whose whole palette can change at
// keystroke should always say which one is in use - otherwise a user who
// toggles it and does not like the result has no way to tell what they are
// looking at.
func (i *Interpreter) groundChip(b *Buffer, s *Snapshot, x, y int) {
	th := i.Theme
	if th == nil {
		return
	}
	name := th.Ground()
	style := Style{Fg: th.Palette.Muted, Bg: th.Palette.BgTop, HasBg: true}
	if th.Light {
		style.Fg = th.Palette.Yellow
	}
	// A hairline divider keeps the theme name from reading as part of the
	// verdict, which would be a genuinely confusing mistake on a safety bar.
	divider := Style{Fg: th.Palette.Border, Bg: th.Palette.BgTop, HasBg: true}
	b.Set(x-1, y, k(th.Glyphs().Pipe), divider)
	b.WriteRight(x, y, name, style)
}

// cageBadge is the right hand status of the top bar. It is the one place the
// session state shouts, and it always carries a word.
//
// Every cage state is listed explicitly. A state that falls through to the
// default would render "READY" while the session is actually in high risk
// mode, which is precisely the case where the user most needs to be told.
func (i *Interpreter) cageBadge(s *Snapshot) (string, RGB) {
	t := i.Theme
	switch {
	case s.Sandbox.Running:
		return "▮ SANDBOX ACTIVE", t.Palette.Green
	case s.Cage.State == CageBroken:
		return "✕ CAGE BROKEN", t.Palette.Red
	case s.Cage.State == CageHigh:
		return "▲ HIGH RISK", t.Palette.Red
	case s.Cage.State == CageLimited:
		return "◐ LIMITED CAGE", t.Palette.Yellow
	case s.Sandbox.Unavailable != "":
		return "◌ SANDBOX UNAVAILABLE", t.Palette.Muted
	case s.Cage.State == CageProtected:
		return "● READY", t.Palette.Green
	default:
		return "● READY", t.Palette.Fg
	}
}

// rail draws the navigation column. Every entry names its shortcut, so the
// key list is discoverable without opening help.
func (i *Interpreter) rail(b *Buffer, s *Snapshot, st *UIState, l layout) {
	t := i.Theme
	g := t.Glyphs()
	views := Views()
	y := l.Rail.Y
	for _, row := range []string{"SBT", "SAFE", "TERM"} {
		b.WriteClipped(l.Rail.X+1, y, l.Rail.X+l.Rail.W, row, Style{Fg: t.Palette.Yellow})
		y++
	}
	y++
	for _, v := range views {
		if y >= l.Rail.Bottom() {
			break
		}
		active := v == st.View
		glyph := " "
		numStyle := Style{Fg: t.Palette.Border}
		labelStyle := Style{Fg: t.Palette.Text}
		if active {
			glyph = g.Caret
			numStyle = Style{Fg: t.Palette.YellowHi, Bold: true}
			labelStyle = Style{Fg: t.Palette.YellowHi, Bold: true, Under: true}
		}
		b.Set(l.Rail.X+1, y, k(glyph), numStyle)
		col := b.Write(l.Rail.X+2, y, itoa(int(v)+1), numStyle)
		b.Write(col, y, " "+v.Name(), labelStyle)
		y++
	}
	if y < l.Rail.Bottom()-1 && s.WorkspaceDir != "" {
		y++
		dir := s.WorkspaceDir
		if home := osGetenv("HOME"); home != "" && strings.HasPrefix(dir, home+"/") {
			dir = "~/" + dir[len(home)+1:]
		}
		lines := Wrap(dir, l.Rail.W-2)
		for _, ln := range lines {
			if y >= l.Rail.Bottom() {
				break
			}
			b.WriteClipped(l.Rail.X+1, y, l.Rail.X+l.Rail.W, ln, Style{Fg: t.Palette.Muted})
			y++
		}
	}
}

// workspace draws the centre area for the active view.
//
// A view that has just been switched to is drawn one column in from the left
// and eases back to place, so a change of screen has direction instead of
// simply happening. The slide is one column and 170ms: enough to register,
// short enough that it is never in the way. With motion off, the view simply
// appears.
func (i *Interpreter) workspace(b *Buffer, s *Snapshot, st *UIState, r Rect) {
	t := i.Theme
	t.Blank(b, r, t.Palette.Bg)
	offset := 0
	if st.Motion {
		if p := st.Transition.Progress(s.Now); p < 1 {
			offset = int(math.Ceil((1 - p) * 2))
		}
	}
	target := Rect{X: r.X + offset, Y: r.Y, W: r.W - offset, H: r.H}
	if target.Empty() {
		return
	}
	// While a view is sliding in, the outgoing view is not redrawn: a partial
	// second view reads as a rendering fault rather than a transition.
	switch st.View {
	case ViewFiles:
		i.filesView(b, s, st, target)
	case ViewChanges:
		i.changesView(b, s, st, target)
	case ViewStatus:
		i.statusView(b, s, st, target)
	case ViewExport:
		i.exportView(b, s, st, target)
	case ViewHelp:
		i.helpView(b, s, st, target)
	case ViewSettings:
		i.settingsView(b, s, st, target)
	case ViewPermissions:
		i.permissionsView(b, s, st, target)
	default:
		i.terminalView(b, s, st, target)
	}
}

// statusBar is the bottom evidence line: what the cage enforces right now, what
// happened last, and the keys that always work.
func (i *Interpreter) statusBar(b *Buffer, s *Snapshot, st *UIState, w, h int) {
	t := i.Theme
	p := t.Palette
	y := h - 1
	bg := Style{Bg: p.BgBottom, HasBg: true, Fg: p.Text}
	b.Fill(0, y, w, 1, ' ', bg)
	wrap := func(x int, text string, sty Style) int {
		sty.Bg, sty.HasBg = p.BgBottom, true
		return b.Write(x, y, text, sty)
	}
	col := 1
	col = wrap(col, "f10 menu", Style{Fg: p.YellowHi})
	col = wrap(col, "  ctrl+k commands", Style{Fg: p.Muted})
	col = wrap(col, "  "+helpViewKeys()+" views", Style{Fg: p.Muted})
	if !anyConfirmOpen(st) {
		col = wrap(col, "  ctrl+d exit", Style{Fg: p.Muted})
	}
	_ = col
	// Right side: the state line. Newest note wins; otherwise the sandbox
	// verdict.
	right, rcol := i.statusRight(s, st)
	b.WriteRight(w, y, right, Style{Fg: rcol, Bg: p.BgBottom, HasBg: true})
	_ = wrap
}

// statusRight builds the right hand side of the status bar.
func (i *Interpreter) statusRight(s *Snapshot, st *UIState) (string, RGB) {
	t := i.Theme
	p := t.Palette
	if st.Flash.Text != "" {
		col := p.Text
		switch st.Flash.Kind {
		case StateOK:
			col = p.Green
		case StateWarn:
			col = p.Yellow
		case StateDanger:
			col = p.Red
		}
		return st.Flash.Text, col
	}
	if sb := s.Sandbox; sb.Running {
		return "sandbox running - output goes to the real terminal", p.Green
	}
	if s.Sandbox.Unavailable != "" {
		return "sandbox unavailable: " + s.Sandbox.Unavailable, p.Red
	}
	if s.Cage.State == CageBroken {
		return "cage broken: " + s.Cage.Reason, p.Red
	}
	if s.Cage.State == CageLimited {
		return "limited cage: " + s.Cage.Reason, p.Yellow
	}
	if last, ok := s.LatestRun(); ok {
		word := "clean"
		if c := last.Counts(); c.Total() > 0 {
			word = c.Summary()
		}
		return "last: exit " + itoa(last.ExitCode) + " · " + word, p.Muted
	}
	return "cage ready - nothing has run yet", p.Muted
}

// securityPanel is the right column's top half: the isolation evidence.
func (i *Interpreter) securityPanel(b *Buffer, s *Snapshot, st *UIState, r Rect) {
	t := i.Theme
	p := t.Palette
	h := r.H/2 - 1
	if h < 4 {
		return
	}
	inner := t.Panel(b, r.X, r.Y, r.W, h, "isolation", "", p.Border, false)
	if inner.Empty() {
		return
	}
	y := inner.Y
	// The badge row restates the overall verdict in words.
	badge, col := i.cageBadge(s)
	b.WriteClipped(inner.X, y, inner.Right(), badge, Style{Fg: col, Bold: true})
	y += 2
	for _, ln := range s.Isolation {
		if y >= inner.Bottom() {
			break
		}
		mark, sty := "·", Style{Fg: p.Muted}
		switch ln.State {
		case StateOK:
			mark, sty = "✓", Style{Fg: p.Green}
		case StateWarn:
			mark, sty = "◐", Style{Fg: p.Yellow}
		case StateDanger:
			mark, sty = "✕", Style{Fg: p.Red}
		}
		sty.Bold = true
		// A degraded line pulses gently so the eye is drawn to the one thing
		// that is not fully isolated. The words stay constant: the meaning
		// never depends on the animation running.
		if st.Motion && (ln.State == StateWarn || ln.State == StateDanger) {
			sty.Fg = Glow(sty.Fg, 0.15+0.25*Breath(s.Now, st.Transition.At, PulsePeriod))
		}
		c := b.Write(inner.X, y, mark, sty)
		b.WriteClipped(c+1, y, inner.Right(), Pad(ln.Label, 9)+ln.Value, Style{Fg: p.Text})
		y++
	}
	if s.ProbeFail != "" && y < inner.Bottom() {
		b.WriteClipped(inner.X, y, inner.Right(), "probe: "+s.ProbeFail, Style{Fg: p.Yellow})
	}
}

// resourcesPanel is the right column's bottom half: live resource usage. The
// gauges ease toward each measurement, so a spike is seen growing instead of
// appearing fully formed between two frames.
func (i *Interpreter) resourcesPanel(b *Buffer, s *Snapshot, ui *UIState, r Rect) {
	t := i.Theme
	p := t.Palette
	y0 := r.Y + r.H/2
	h := r.H - r.H/2
	if h < 4 {
		return
	}
	inner := t.Panel(b, r.X, y0, r.W, h, "resources", "", p.Border, false)
	if inner.Empty() {
		return
	}
	stat := s.Stats
	if !s.Sandbox.Running {
		i.emptyState(b, inner, "monitor idle", "sampling starts with the first sandbox")
		return
	}
	now := s.Now
	ui.Meters.CPU.Set(stat.CPUPercent, now, ui.Motion)
	ui.Meters.Memory.Set(float64(stat.RSSBytes), now, ui.Motion)
	ui.Meters.Procs.Set(float64(stat.Procs), now, ui.Motion)

	y := inner.Y
	cpu := ui.Meters.CPU.Value(now, ui.Motion)
	t.Meter(b, inner.X, y, inner.W, "cpu", cpu, 100, meterColor(cpu/100, p),
		itoa(int(stat.CPUPercent))+"%")
	y++
	mem := ui.Meters.Memory.Value(now, ui.Motion)
	limit := float64(stat.MemLimitBytes)
	if limit <= 0 {
		limit = math.Max(mem, 1)
	}
	t.Meter(b, inner.X, y, inner.W, "mem", mem, limit, meterColor(mem/limit, p),
		formatBytes(int64(mem)))
	y++
	if stat.ProcsLimit > 0 {
		pr := ui.Meters.Procs.Value(now, ui.Motion)
		t.Meter(b, inner.X, y, inner.W, "procs", pr, float64(stat.ProcsLimit),
			meterColor(pr/float64(stat.ProcsLimit), p), itoa(stat.Procs)+" procs")
		y++
	}
	// The companion gets whatever rows are left. It is placed last and sized
	// last so it never squeezes a gauge: the numbers matter more than the cat.
	petH := inner.Bottom() - y
	if petH >= 4 {
		i.petPanel(b, ui.Pet, Rect{X: inner.X, Y: y, W: inner.W, H: petH})
	}
}

// petPanel draws the companion and reports where it is, so the click handler can
// pet it.
//
// The panel is cosmetic and is drawn from muted colours only. It never uses a
// severity colour, because a green cat sitting next to a gauge would be read as
// a verdict about the cage.
func (i *Interpreter) petPanel(b *Buffer, pet Pet, r Rect) {
	if !pet.Alive() {
		return
	}
	t := i.Theme
	p := t.Palette
	if r.H < 3 {
		return
	}
	right := ""
	if pet.Mood != "" {
		right = pet.Mood
	} else {
		right = pet.Kind.Name()
	}
	inner := t.Panel(b, r.X, r.Y, r.W, r.H, "companion", right, p.Border, false)
	if inner.Empty() {
		return
	}
	art := pet.art()
	for j, line := range art {
		if j >= inner.H {
			break
		}
		b.WriteClipped(inner.X, inner.Y+j, inner.Right(), Truncate(line, inner.W),
			Style{Fg: p.Yellow})
	}
	if inner.H > len(art) {
		b.WriteClipped(inner.X, inner.Y+len(art), inner.Right(), "click to pet",
			Style{Fg: p.Muted})
	}
}

// petBounds reports where the companion is drawn, or false when there is none.
//
// It recomputes the resources panel's geometry rather than storing the rectangle
// during the draw, because the draw happens on the App's goroutine and a
// click can arrive between frames; a stored rectangle would then describe a
// layout the user is no longer looking at.
func (st *UIState) petBounds(snap *Snapshot) (Rect, bool) {
	if !st.Pet.Alive() {
		return Rect{}, false
	}
	l := computeLayout(st.Width, st.Height, st.alertRows(snap))
	if !l.HasSide {
		return Rect{}, false
	}
	inner := Inner(l.Side.X, l.Side.Y+l.Side.H/2, l.Side.W, l.Side.H-l.Side.H/2, true)
	if inner.Empty() {
		return Rect{}, false
	}
	// Mirror resourcesPanel's row arithmetic: the cpu and mem gauges always
	// take a row, and the process gauge only exists while stats are live.
	y := inner.Y + 2
	if snap.Stats.ProcsLimit > 0 {
		y++
	}
	h := inner.Bottom() - y
	if h < 4 {
		return Rect{}, false
	}
	return Rect{X: inner.X, Y: y, W: inner.W, H: h}, true
}

// petPanelAt is the click target used by the handler.
func (st *UIState) petPanelAt(m Mouse, snap *Snapshot) bool {
	r, ok := st.petBounds(snap)
	return ok && r.Contains(m.X, m.Y)
}

// meterColor grades a gauge by how full it is. The thresholds are deliberately
// conservative: a sandbox at 70% of its memory limit is worth noticing but is
// not yet a problem, and a gauge that cries wolf is a gauge that gets ignored.
func meterColor(ratio float64, p Palette) RGB {
	switch {
	case ratio >= 0.9:
		return p.Red
	case ratio >= 0.7:
		return p.Warning
	default:
		return p.Green
	}
}

// terminalView is the default screen: the transcript of what ran here plus the
// command input. While the sandbox runs, the terminal is suspended and the real
// child owns the screen; this view is what the user sees between commands.
func (i *Interpreter) terminalView(b *Buffer, s *Snapshot, st *UIState, r Rect) {
	t := i.Theme
	p := t.Palette
	g := t.Glyphs()
	header := "session terminal"
	if sb := s.Sandbox; sb.ID != "" {
		header += "  ·  sandbox " + sb.ID
	}
	if last, ok := s.LatestRun(); ok {
		header += "  ·  last exit " + itoa(last.ExitCode)
	}
	inputH := 3
	listH := r.H - inputH
	st.Files.Shown = isTrue(s.Settings["ui.files_row"])
	if st.Files.Shown {
		listH--
	}
	inner := t.Panel(b, r.X, r.Y, r.W, listH, header, "", p.Border,
		st.View == ViewTerminal && st.Focus == focusWorkspace)
	if inner.Empty() {
		return
	}
	lines := st.Transcript.Render(inner.W, inner.H)
	for j, ln := range lines {
		sty := Style{Fg: p.Text}
		switch ln.State {
		case StateOK:
			sty.Fg = p.Green
		case StateWarn:
			sty.Fg = p.Yellow
		case StateDanger:
			sty.Fg = p.Red
		case StateMeta:
			sty.Fg = p.Muted
		}
		if ln.Meta {
			b.WriteClipped(inner.X+2, inner.Y+j, inner.Right(), ln.Text, sty)
		} else {
			b.WriteClipped(inner.X, inner.Y+j, inner.Right(), ln.Text, sty)
		}
	}
	if st.Files.Shown {
		i.filesStrip(b, s, st, Rect{X: r.X, Y: r.Bottom() - inputH - 1, W: r.W, H: 1})
	}
	ip := t.Panel(b, r.X, r.Bottom()-inputH, r.W, inputH, "command", "",
		p.Border, st.View == ViewTerminal && st.Focus == focusInput)
	if ip.Empty() {
		return
	}
	if st.Input == "" {
		b.WriteClipped(ip.X, ip.Y, ip.Right(), "type a command and press enter", Style{Fg: p.Muted})
		return
	}
	col := b.Write(ip.X, ip.Y, "run", Style{Fg: p.YellowHi, Bold: true})
	st.Typing.DrawInput(b, col+1, ip.Y, ip.Right()-1, st.Input, s.Now, Style{Fg: p.Text})
	if st.View == ViewTerminal && st.Focus == focusInput {
		// A drawn caret: the terminal cursor is hidden while the cage is up,
		// and the input must still feel live. It breathes rather than blinks,
		// because a hard on/off flash is exactly what a photosensitive user
		// turned SBT_NO_MOTION to avoid.
		cx := min(col+1+StringWidth(st.Input), ip.Right()-1)
		caretCol := p.YellowHi
		if st.Motion {
			caretCol = Glow(p.YellowHi, Breath(s.Now, st.LastKeyAt, PulsePeriod)*0.3)
		}
		b.Set(cx, ip.Y, k(g.Caret), Style{Fg: caretCol, Bold: true})
	}
	// The prompt shows what will run and, while the cage cannot verify
	// isolation, says so here rather than letting the user press enter and
	// find out.
	if st.Input != "" && s.Sandbox.Unavailable != "" {
		b.WriteClipped(ip.X, ip.Bottom(), ip.Right(), "! "+s.Sandbox.Unavailable, Style{Fg: p.Yellow})
	}
}

// filesView lists the session workspace.
func (i *Interpreter) filesView(b *Buffer, s *Snapshot, st *UIState, r Rect) {
	t := i.Theme
	p := t.Palette
	title := "workspace files"
	if len(s.Files) > 0 {
		title += "  ·  " + itoa(len(s.Files)) + " shown"
	}
	inner := t.Panel(b, r.X, r.Y, r.W, r.H, title, "", p.Border, st.View == ViewFiles)
	if inner.Empty() {
		return
	}
	if len(s.Files) == 0 {
		i.emptyState(b, inner, "no files yet", "run a command - what it creates lands here")
		return
	}
	start := st.clampScroll(len(s.Files), inner.H)
	for j := 0; j < inner.H && start+j < len(s.Files); j++ {
		idx := start + j
		f := s.Files[idx]
		sty := Style{Fg: p.Text}
		if idx == st.List.Index && st.View == ViewFiles {
			sty.Bg, sty.HasBg, sty.Fg, sty.Bold = p.Yellow, true, p.Bg, true
		}
		sizeText := formatBytes(f.Size)
		b.WriteClipped(inner.X, inner.Y+j, inner.Right()-StringWidth(sizeText)-1,
			f.Kind.Symbol()+" "+f.Path, sty)
		b.WriteRight(inner.Right(), inner.Y+j, sizeText, sty)
	}
}

// changesView is the review screen: every change of every run on the left, and
// the before/after content of the selected one on the right.
//
// The two panes sit side by side rather than one replacing the other, because
// the review act is comparison - "what changed" and "how it changed" - and
// making the user remember a path to flip between them is how reviews get
// skipped. On a narrow terminal the diff takes the whole panel and Escape brings
// the list back.
func (i *Interpreter) changesView(b *Buffer, s *Snapshot, st *UIState, r Rect) {
	t := i.Theme
	p := t.Palette
	c := s.Counts()
	inner := t.Panel(b, r.X, r.Y, r.W, r.H, "changes",
		"+"+itoa(c.Added)+" ~"+itoa(c.Modified)+" -"+itoa(c.Deleted)+" !"+itoa(c.Warnings),
		p.Border, st.View == ViewChanges)
	if inner.Empty() {
		return
	}
	rows := BuildChangeRows(s)
	if len(rows) == 0 {
		i.emptyState(b, inner, "no changes recorded", "the cage records what each run touches")
		return
	}
	// The split comes from changeRects, the same function the click handler
	// resolves a click against. Guessing it here would put the cursor on the
	// wrong path on the terminals where the split changes.
	listRect, diffRect := st.changeRects(s)
	if listRect.W > 0 {
		i.changeList(b, s, st, rows, listRect)
	}
	if diffRect.W > 0 {
		i.diffPane(b, s, st, diffRect)
	}
}

// changeList draws the flat list of changes with its cursor.
func (i *Interpreter) changeList(b *Buffer, s *Snapshot, st *UIState, rows []ChangeRow, r Rect) {
	t := i.Theme
	p := t.Palette
	pick := Selectable(rows)
	// The cursor is expressed in selectable-row space; converting it here is
	// what keeps a run header from ever stealing it.
	cursor := st.Changes.Row
	if cursor >= len(pick) {
		cursor = max(len(pick)-1, 0)
	}
	if cursor < 0 {
		cursor = 0
	}
	st.Changes.Row = cursor

	start := st.Changes.clampScroll(len(rows), r.H)
	selectedRow := -1
	if cursor < len(pick) {
		selectedRow = pick[cursor]
	}
	for j := 0; j < r.H && start+j < len(rows); j++ {
		row := rows[start+j]
		y := r.Y + j
		if row.Header {
			label := "run " + itoa(row.RunIndex+1)
			if row.RunIndex < len(s.Runs) {
				label += "  " + strings.Join(s.Runs[row.RunIndex].Command, " ")
			}
			b.WriteClipped(r.X, y, r.Right(), Truncate(label, r.W), Style{Fg: p.Muted, Bold: true})
			continue
		}
		style := Style{Fg: p.Text}
		switch row.Entry.Kind {
		case workspace.Modified:
			style.Fg = p.Yellow
		case workspace.Deleted:
			style.Fg = p.Red
		}
		text := row.Entry.Kind.Symbol() + " " + row.Entry.Path
		if row.Entry.Risk() != "" {
			// A path that needs a second look says so in the list, not only in
			// the export dialog much later.
			text = row.Entry.Kind.Symbol() + " ! " + row.Entry.Path
		}
		if start+j == selectedRow {
			// Selection is a background and a bold weight, not only a colour,
			// so it survives NO_COLOR.
			style = Style{Fg: p.Bg, Bg: p.Yellow, HasBg: true, Bold: true}
		}
		b.WriteClipped(r.X, y, r.Right(), Truncate(text, r.W), style)
	}
}

// diffPane draws the before/after content of the selected change.
func (i *Interpreter) diffPane(b *Buffer, s *Snapshot, st *UIState, r Rect) {
	t := i.Theme
	p := t.Palette
	// Find what the cursor points at, so the pane can tell whether the loaded
	// diff still belongs to what is highlighted.
	var wantRun, wantPath string
	rows := BuildChangeRows(s)
	pick := Selectable(rows)
	if st.Changes.Row >= 0 && st.Changes.Row < len(pick) {
		row := rows[pick[st.Changes.Row]]
		wantRun, wantPath = row.RunID, row.Entry.Path
	}

	header := "diff"
	right := "enter review"
	if wantPath != "" {
		right = itoa(len(s.Diff.Lines)) + " lines"
	}
	inner := t.Panel(b, r.X, r.Y, r.W, r.H, header, right, p.Border, st.DiffOpen)
	if inner.Empty() {
		return
	}
	if wantPath == "" {
		i.emptyState(b, inner, "nothing selected", "move the cursor over a change")
		return
	}
	// The pane names what it is showing even before anything is loaded, so the
	// header is never just the word "diff".
	b.WriteClipped(inner.X, inner.Y, inner.Right(), Truncate(wantPath, inner.W),
		Style{Fg: p.Text, Bold: true})
	y := inner.Y + 1
	b.WriteClipped(inner.X, y, inner.Right(),
		strings.Repeat(t.Glyphs().H, max(inner.W, 0)), Style{Fg: p.Border})
	y++

	body := Rect{X: inner.X, Y: y, W: inner.W, H: inner.Bottom() - y}
	switch {
	case !s.Diff.Loaded:
		i.emptyState(b, body, "nothing loaded yet", "press enter on a change to review it")
		return
	case s.Diff.Binary:
		// Reviewing binary content as text is noise; "binary" is the honest
		// answer and the reason is still worth showing.
		msg := "binary content is not shown as text"
		if s.Diff.Note != "" {
			msg += "  (" + s.Diff.Note + ")"
		}
		i.emptyState(b, body, "binary file", msg)
		return
	case !s.Diff.Matches(wantRun, wantPath):
		// The cursor moved before the load arrived, or the load failed. Either
		// way the pane must not show a diff belonging to another path.
		note := "this change could not be read"
		if s.Diff.Note != "" {
			note = s.Diff.Note
		}
		i.emptyState(b, body, "not available", note)
		return
	}

	if s.Diff.Note != "" {
		b.WriteClipped(inner.X, y, inner.Right(), Truncate("! "+s.Diff.Note, inner.W),
			Style{Fg: p.Warning})
		y++
	}
	rowsLeft := inner.Bottom() - y
	if rowsLeft <= 0 {
		return
	}
	// The viewport stays inside the content, and short content is shown from
	// the top rather than floating in the middle of the pane.
	offset := max(st.DiffScroll, 0)
	if maxOffset := max(len(s.Diff.Lines)-rowsLeft, 0); offset > maxOffset {
		offset = maxOffset
	}
	for j := 0; j < rowsLeft && offset+j < len(s.Diff.Lines); j++ {
		line := s.Diff.Lines[offset+j]
		style := Style{Fg: p.Text}
		switch {
		case strings.HasPrefix(line, "+"):
			style.Fg = p.Green
		case strings.HasPrefix(line, "-"):
			style.Fg = p.Red
		case strings.HasPrefix(line, "!"):
			style.Fg = p.Warning
		case strings.HasPrefix(line, "  "):
			style.Fg = p.Muted
		}
		b.WriteClipped(inner.X, y+j, inner.Right(), line, style)
	}
	if hidden := len(s.Diff.Lines) - offset - rowsLeft; hidden > 0 {
		// Saying how much is below the fold keeps a long diff from looking
		// complete when it is not.
		b.WriteRight(inner.Right(), inner.Bottom()-1, itoa(hidden)+" more", Style{Fg: p.Muted})
	}
}

// statusView is the plain-text summary: identity, isolation, warnings, notes.
func (i *Interpreter) statusView(b *Buffer, s *Snapshot, st *UIState, r Rect) {
	t := i.Theme
	p := t.Palette
	inner := t.Panel(b, r.X, r.Y, r.W, r.H, "status", "", p.Border, st.View == ViewStatus)
	if inner.Empty() {
		return
	}
	y := inner.Y
	put := func(text string, sty Style) {
		if y >= inner.Bottom() {
			return
		}
		b.WriteClipped(inner.X, y, inner.Right(), text, sty)
		y++
	}
	put("sbt "+s.Version+" · "+s.Platform, Style{Fg: p.Text, Bold: true})
	if s.Kernel != "" {
		put("kernel    "+s.Kernel, Style{Fg: p.Muted})
	}
	put("backend   "+s.Backend, Style{Fg: p.Muted})
	if s.WorkspaceDir != "" {
		put("workspace "+s.WorkspaceDir, Style{Fg: p.Muted})
	}
	put("policy    "+policyWord(s.Mode, s.Policy), Style{Fg: p.Text})
	y++
	for _, ln := range s.Isolation {
		sty := Style{Fg: p.Muted}
		switch ln.State {
		case StateOK:
			sty.Fg = p.Green
		case StateWarn:
			sty.Fg = p.Yellow
		case StateDanger:
			sty.Fg = p.Red
		}
		put(Pad(ln.Label, 12)+ln.Value, sty)
		if ln.Reason != "" {
			put("          "+ln.Reason, Style{Fg: p.Muted})
		}
	}
	if s.ProbeFail != "" {
		put("probe     unavailable: "+s.ProbeFail, Style{Fg: p.Yellow})
	}
	for _, w := range s.Warnings {
		put("! "+w, Style{Fg: p.Yellow})
	}
	for _, n := range s.Notes {
		put("· "+n, Style{Fg: p.Muted})
	}
}

// exportView shows the export picker. Selecting files is separate from
// confirming: the confirm modal is where executable warnings surface.
func (i *Interpreter) exportView(b *Buffer, s *Snapshot, st *UIState, r Rect) {
	t := i.Theme
	p := t.Palette
	inner := t.Panel(b, r.X, r.Y, r.W, r.H, "export", "a all · esc close", p.Border, st.View == ViewExport)
	if inner.Empty() {
		return
	}
	sel := st.Export
	dest := sel.Destination
	if dest == "" {
		dest = "(choose a destination with export --to /path)"
	}
	y := inner.Y
	b.WriteClipped(inner.X, y, inner.Right(), "to   "+dest, Style{Fg: p.Text, Bold: true})
	y += 2
	if len(s.Files) == 0 {
		i.emptyState(b, Rect{X: inner.X, Y: y, W: inner.W, H: inner.Bottom() - y},
			"nothing to export", "run a command first")
		return
	}
	if sel.All {
		b.WriteClipped(inner.X, y, inner.Right(), "[x] everything the session produced", Style{Fg: p.YellowHi, Bold: true})
	} else {
		b.WriteClipped(inner.X, y, inner.Right(), "[ ] everything  (press a)", Style{Fg: p.Muted})
	}
	y += 2
	rows := inner.Bottom() - y
	if rows <= 0 {
		return
	}
	start := sel.clampScroll(len(s.Files), rows)
	for j := 0; j < rows && start+j < len(s.Files); j++ {
		idx := start + j
		f := s.Files[idx]
		box, sty := "[ ]", Style{Fg: p.Text}
		if sel.All || sel.Has(idx) {
			box, sty = "[x]", Style{Fg: p.YellowHi, Bold: true}
		}
		if sel.Warn[idx] {
			box += " !exec"
		}
		b.WriteClipped(inner.X, y+j, inner.Right(), box+" "+f.Path, sty)
	}
	if sel.Message != "" {
		b.WriteClipped(inner.X, inner.Bottom()-1, inner.Right(), sel.Message, Style{Fg: p.Yellow})
	}
}

// helpView is static text about the cage.
func (i *Interpreter) helpView(b *Buffer, s *Snapshot, st *UIState, r Rect) {
	t := i.Theme
	inner := t.Panel(b, r.X, r.Y, r.W, r.H, "help", "esc close", t.Palette.Border, st.View == ViewHelp)
	if inner.Empty() {
		return
	}
	for j, ln := range HelpLines {
		if inner.Y+j >= inner.Bottom() {
			break
		}
		sty := Style{Fg: t.Palette.Text}
		if strings.HasPrefix(ln, "  ") {
			sty.Fg = t.Palette.Muted
		}
		b.WriteClipped(inner.X, inner.Y+j, inner.Right(), ln, sty)
	}
}

// emptyState centres a two line message. Silence is not a state: every empty
// panel says why it is empty and what to do next.
func (i *Interpreter) emptyState(b *Buffer, r Rect, title, hint string) {
	if r.Empty() {
		return
	}
	t := i.Theme
	b.WriteClipped(r.X, r.Y+r.H/2-1, r.Right(), title, Style{Fg: t.Palette.Text, Bold: true})
	b.WriteClipped(r.X, r.Y+r.H/2, r.Right(), hint, Style{Fg: t.Palette.Muted})
}

// filesStrip draws the one-line file activity row under the terminal.
//
// It is one row on purpose: the point is to keep recently touched paths in
// view, not to become a second file browser. The selected path is highlighted
// and marked, because a strip where you cannot tell where the cursor is is
// decoration rather than navigation.
func (i *Interpreter) filesStrip(b *Buffer, s *Snapshot, st *UIState, r Rect) {
	t := i.Theme
	p := t.Palette
	g := t.Glyphs()
	if r.Empty() {
		return
	}
	b.Fill(r.X, r.Y, r.W, 1, ' ', Style{Fg: p.Text})
	col := b.Write(r.X, r.Y, "FILES:", Style{Fg: p.Muted})
	if len(s.Files) == 0 {
		b.WriteClipped(col+1, r.Y, r.Right(), "  "+tr("files.none", "no active files"), Style{Fg: p.Muted})
		return
	}
	st.Files.Clamp(len(s.Files))
	sep := "  |  "
	for j, f := range s.Files {
		name := f.Path
		sty := Style{Fg: p.Muted}
		if j == st.Files.Index {
			sty = Style{Fg: p.Primary, Bold: true}
			name = g.Caret + name
		}
		next := col + StringWidth(sep)
		if next+StringWidth(name) > r.Right()-12 {
			b.WriteRight(r.Right(), r.Y, "more "+itoa(len(s.Files)-j), Style{Fg: p.Muted})
			break
		}
		b.Write(col, r.Y, sep, Style{Fg: p.Border})
		b.WriteClipped(col+StringWidth(sep), r.Y, r.Right(), name, sty)
		col += StringWidth(sep) + StringWidth(name)
	}
	b.WriteRight(r.Right(), r.Y, tr("files.hint", "left/right  enter opens"), Style{Fg: p.Muted})
}
