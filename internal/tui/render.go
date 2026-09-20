package tui

import (
	"strings"
	"time"
)

// Interpreter renders a Snapshot into a Buffer. It holds no state of its own
// beyond the theme, so the same interpreter can draw every frame.
type Interpreter struct {
	Theme *Theme
}

// NewInterpreter builds an interpreter for a theme.
func NewInterpreter(t *Theme) *Interpreter { return &Interpreter{Theme: t} }

// layout is the geometry of one frame, derived from the terminal size.
type layout struct {
	Body Rect // area between the top bar and the status bar
	Rail Rect // navigation rail, left
	Work Rect // workspace area, centre
	Side Rect // security/resources panel, right (absent on narrow terminals)
	Rail bool
	Side bool
}

const railWidth = 18

// computeLayout applies the minimum sizes the cage needs. Below them the UI
// degrades to a plain message instead of drawing clipped garbage.
func computeLayout(w, h int) layout {
	var l layout
	if h < 8 || w < 24 {
		return l
	}
	l.Body = Rect{X: 0, Y: 1, W: w, H: h - 2}
	col := 0
	if w >= 62 {
		l.Rail = true
		l.Rail = Rect{X: 0, Y: l.Body.Y, W: railWidth, H: l.Body.H}
		col += railWidth
	}
	if w-col >= 84 {
		l.Side = true
		l.Side = Rect{X: w - 36, Y: l.Body.Y, W: 36, H: l.Body.H}
	}
	l.Work = Rect{X: col, Y: l.Body.Y, W: w - col - l.Side.W*l.SideBool(), H: l.Body.H}
	return l
}

// Render draws one frame. The buffer is fresh each time; the screen diffs it
// against the previous frame, so only changed cells reach the terminal.
func (i *Interpreter) Render(s *Snapshot, st *UIState) *Buffer {
	w, h := st.Width, st.Height
	b := NewBuffer(w, h)
	l := computeLayout(w, h)
	if l.Body.Empty() {
		msg := "terminal too small for the cage (need at least 24x8)"
		b.WriteClipped(0, h/2, w, msg, Style{Fg: i.Theme.Palette.Warning})
		return b
	}
	i.topBar(b, s, w)
	i.rail(b, s, st, l)
	if l.Side {
		i.securityPanel(b, s, l.Side)
		i.resourcesPanel(b, s, l.Side)
	}
	i.workspace(b, s, st, l.Work)
	i.statusBar(b, s, st, w, h)
	return b
}

// topBar draws the identity strip: what this is, which policy, what state.
func (i *Interpreter) topBar(b *Buffer, s *Snapshot, w int) {
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
	if s.Policy.Name != "" {
		col = b.Write(col, 0, " · "+s.Policy.Name, Style{Fg: p.Muted, Bg: p.BgTop, HasBg: true})
	}
	if s.Host != "" {
		b.Write(col, 0, "  "+g.Dot+" "+s.Host, Style{Fg: p.Muted, Bg: p.BgTop, HasBg: true})
	}
	badge, badgeCol := i.cageBadge(s)
	b.WriteRight(w, 0, badge+" ", Style{Fg: badgeCol, Bg: p.BgTop, HasBg: true, Bold: true})
}

// cageBadge is the right hand status of the top bar. It is the one place the
// session state shouts, and it always carries a word.
func (i *Interpreter) cageBadge(s *Snapshot) (string, RGB) {
	t := i.Theme
	switch {
	case s.Sandbox.Running:
		return "▮ SANDBOX ACTIVE", t.Palette.Green
	case s.Cage.State == CageBroken:
		return "✕ CAGE BROKEN", t.Palette.Red
	case s.Cage.State == CageLimited:
		return "◐ LIMITED CAGE", t.Palette.Yellow
	case s.Sandbox.Unavailable != "":
		return "◌ SANDBOX UNAVAILABLE", t.Palette.Muted
	default:
		return "● READY", t.Palette.Fg
	}
}

// rail draws the navigation column. Every entry names its shortcut, so the
// key list is discoverable without opening help.
func (i *Interpreter) rail(b *Buffer, s *Snapshot, st *UIState, l layout) {
	t := i.Theme
	g := t.Glyphs()
	views := []View{ViewTerminal, ViewFiles, ViewChanges, ViewStatus, ViewExport, ViewHelp}
	y := l.Rail.Y
	for _, row := range g.Seal {
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
			labelStyle = Style{Fg: t.Palette.YellowHi, Bold: true, Underline: true}
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
func (i *Interpreter) workspace(b *Buffer, s *Snapshot, st *UIState, r Rect) {
	t := i.Theme
	t.Blank(b, r, t.Palette.Bg)
	switch st.View {
	case ViewFiles:
		i.filesView(b, s, st, r)
	case ViewChanges:
		i.changesView(b, s, st, r)
	case ViewStatus:
		i.statusView(b, s, st, r)
	case ViewExport:
		i.exportView(b, s, st, r)
	case ViewHelp:
		i.helpView(b, s, st, r)
	default:
		i.terminalView(b, s, st, r)
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
		return b.WriteClipped(x, y, w, text, sty)
	}
	col := 1
	col = wrap(col, "ctrl+k commands", Style{Fg: p.Muted})
	col = wrap(col, "  alt+1..5 views", Style{Fg: p.Muted})
	if !anyConfirmOpen(st) {
		col = wrap(col, "  ctrl+d exit", Style{Fg: p.Muted})
	}
	// Right side: the state line. Newest note wins; otherwise the sandbox
	// verdict.
	right, rcol := i.statusRight(s, st)
	if rcol == 0 {
		rcol = p.Text
	}
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
		if c := last.Counts(); !c.Empty() {
			word = c.Summary()
		}
		return "last: exit " + itoa(last.Exit) + " · " + word, p.Muted
	}
	return "cage ready - nothing has run yet", p.Muted
}

// securityPanel is the right column's top half: the isolation evidence.
func (i *Interpreter) securityPanel(b *Buffer, s *Snapshot, r Rect) {
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
		c := b.Write(inner.X, y, mark, sty)
		b.WriteClipped(c+1, y, inner.Right(), Pad(ln.Label, 9)+ln.Value, Style{Fg: p.Text})
		y++
	}
	if s.ProbeFail != "" && y < inner.Bottom() {
		b.WriteClipped(inner.X, y, inner.Right(), "probe: "+s.ProbeFail, Style{Fg: p.Yellow})
	}
}

// resourcesPanel is the right column's bottom half: live resource usage.
func (i *Interpreter) resourcesPanel(b *Buffer, s *Snapshot, r Rect) {
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
	st := s.Stats
	if !st.Collected.IsZero() && s.Now.Sub(st.Collected) > 10*time.Second {
		// Sampling stalled: say so instead of showing stale numbers as if
		// they were current.
		b.WriteClipped(inner.X, inner.Y, inner.Right(), "sampling stalled", Style{Fg: p.Yellow})
		return
	}
	if !s.Sandbox.Running {
		i.emptyState(b, inner, "monitor idle", "sampling starts with the first sandbox")
		return
	}
	y := inner.Y
	t.Meter(b, inner.X, y, inner.W, "cpu", st.CPUPercent, 100, p.Yellow,
		itoa(int(st.CPUPercent)) + "%")
	y++
	if st.CPULimit > 0 {
		t.Meter(b, inner.X, y, inner.W, "cpu max", st.CPUPercent, st.CPULimit, p.Yellow,
			itoa(int(st.CPULimit)) + "% cap")
	} else {
		t.Spark(b, inner.X+7, y, inner.W-8, st.CPUHist, p.Yellow)
		y++
	}
	y++
	t.Meter(b, inner.X, y, inner.W, "mem", float64(st.RSSBytes), float64(st.MemLimitBytes), p.Yellow,
		formatBytes(st.RSSBytes))
	y++
	if st.MemLimitBytes > 0 {
		t.Meter(b, inner.X, y, inner.W, "mem max", float64(st.RSSBytes), float64(st.MemLimitBytes), p.Yellow,
			itoa(int(st.MemLimitBytes/1024/1024)) + "MiB cap")
	}
	y++
	if st.Procs > 0 {
		t.Meter(b, inner.X, y, inner.W, "procs", float64(st.Procs), float64(st.ProcLimit), p.Yellow,
			itoa(st.Procs) + " procs")
	}
}

func (i *Interpreter) workspace(b *Buffer, s *Snapshot, st *UIState, r Rect) {
	t := i.Theme
	t.Blank(b, r, t.Palette.Bg)
	switch st.View {
	case ViewFiles:
		i.filesView(b, s, st, r)
	case ViewChanges:
		i.changesView(b, s, st, r)
	case ViewStatus:
		i.statusView(b, s, st, r)
	case ViewExport:
		i.exportView(b, s, st, r)
	case ViewHelp:
		i.helpView(b, s, st, r)
	default:
		i.terminalView(b, s, st, r)
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
		header += "  ·  last exit " + itoa(last.Exit)
	}
	inputH := 3
	listH := r.H - inputH
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
	b.WriteClipped(col+1, ip.Y, ip.Right()-1, st.Input, Style{Fg: p.Text})
	if st.View == ViewTerminal && st.Focus == focusInput {
		// A drawn caret: the terminal cursor is hidden while the cage is up,
		// and the input must still feel live.
		b.Set(min(col+1+StringWidth(st.Input), ip.Right()-1), ip.Y, k(g.Caret), Style{Fg: p.YellowHi})
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
		b.WriteClipped(inner.X, inner.Y+j, inner.Right()-StringWidth(f.Size)-1,
			f.Kind.Symbol+" "+f.Path, sty)
		b.WriteRight(inner.Right(), inner.Y+j, f.Size, sty)
	}
}

// changesView lists every change across runs, newest run last.
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
	if len(s.Runs) == 0 {
		i.emptyState(b, inner, "no changes recorded", "the cage records what each run touches")
		return
	}
	type row struct {
		text  string
		state StateKind
	}
	var rows []row
	for i, run := range s.Runs {
		rows = append(rows, row{"run " + itoa(i+1) + "  " + run.Command, StateMeta})
		rc := run.Counts()
		if rc.Summary() == "" {
			rows = append(rows, row{"   clean - nothing changed", StateMeta})
		}
		for _, f := range run.Files {
			state := StateOK
			switch f.Kind {
			case workspace.KindModified:
				state = StateWarn
			case workspace.KindDeleted:
				state = StateDanger
			}
			rows = append(rows, row{"  "+f.Kind.Symbol+" "+f.Path, state})
		}
		if rc.Warnings > 0 {
			rows = append(rows, row{"  !"+itoa(rc.Warnings)+" notes on this run", StateWarn})
		}
	}
	start := st.clampScroll(len(rows), inner.H)

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

	for j := 0; j < inner.H && start+j < len(rows); j++ {
		rw := rows[start+j]
		sty := Style{Fg: p.Text}
		switch rw.state {
		case StateOK:
			sty.Fg = p.Green
		case StateWarn:
			sty.Fg = p.Yellow
		case StateDanger:
			sty.Fg = p.Red
		case StateMeta:
			sty.Fg = p.Muted
		}
		b.WriteClipped(inner.X, inner.Y+j, inner.Right(), rw.text, sty)
	}
}


