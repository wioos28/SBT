package tui

// Contains reports whether a cell is inside the rectangle. It is the hit test
// primitive every pointer path in the cage uses.
func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.Right() && y >= r.Y && y < r.Bottom()
}

// menuBarY is the row the menu titles are drawn on. It is a constant because the
// top bar is always row 0 and the menu bar is always the row under it.
const menuBarY = 1

// menuLabelX is the column where a menu's title starts on the menu bar.
//
// It lives next to the click handler on purpose: the renderer walks the titles
// left to right, and a click handler that recomputed the columns on its own
// would drift from what is drawn the moment a title changed width. One function
// answers "where is this title" for both.
func menuLabelX(bar MenuBar, idx int) int {
	col := 1
	for j, m := range bar.Menus {
		if j == idx {
			break
		}
		col += StringWidth(" "+m.Title) + 1
	}
	return col
}

// dropGeom is where the open dropdown is drawn and which item sits on which row.
type dropGeom struct {
	// Inner is the content rectangle of the dropdown panel.
	Inner Rect
	// Items are the rows, in display order.
	Items []MenuItem
	// Start is the index of the item drawn on the first row.
	Start int
}

// dropdownGeom computes the dropdown's geometry. It mirrors menuDropdown
// exactly: same anchoring, same clamp, same scroll window.
//
// Shared geometry is the whole point. A click handler with its own copy of
// these numbers would look correct in a test and put the cursor on the wrong
// row on a narrow terminal, which is the one class of bug a user reads as "the
// mouse is broken" rather than as a layout detail.
func dropdownGeom(st *UIState, w int) (dropGeom, bool) {
	var g dropGeom
	if !st.Menu.Open {
		return g, false
	}
	st.Menu.Clamp(st.Menus)
	if st.Menu.Bar < 0 || st.Menu.Bar >= len(st.Menus.Menus) {
		return g, false
	}
	menu := st.Menus.Menus[st.Menu.Bar]
	if len(menu.Items) == 0 {
		return g, false
	}
	col := menuLabelX(st.Menus, st.Menu.Bar)
	width := menuWidth(menu)
	if col+width > w-1 {
		col = max(w-1-width, 0)
	}
	if col < 0 {
		col = 0
	}
	height := len(menu.Items) + 2
	top := 2
	if top+height > st.Height-1 {
		height = st.Height - 1 - top
	}
	if height < 3 || width < 3 {
		return g, false
	}
	inner := Inner(col, top, width, height, true)
	if inner.Empty() {
		return g, false
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
	return dropGeom{Inner: inner, Items: menu.Items, Start: start}, true
}

// handleMouse turns one mouse report into the same kind of event a key press
// produces, so every downstream path is unchanged: a click on a menu row runs
// the identical runAction a keyboard Enter would, and a click on a dangerous row
// still has to pass the confirmation.
//
// The mouse is never a shortcut past a guard. It selects; the confirm dialog,
// the typed destroy phrase and the exit prompt behave for clicks exactly as
// they do for keys, because a click is the one input a user can make by muscle
// memory.
func (st *UIState) handleMouse(m Mouse, snap *Snapshot) event {
	st.LastKeyAt = snap.Now
	// A wheel notch is not a press: terminals report it as a single event with no
	// release, so gating it on Press would silently disable the wheel entirely.
	// Only the buttons, which do have a release, need that filter.
	switch m.Button {
	case MouseWheelUp, MouseWheelDown:
		return st.wheelEvent(m, snap)
	}
	if !m.Press {
		return event{kind: evNone}
	}
	if m.Button == MouseLeft {
		return st.clickEvent(m, snap)
	}
	return event{kind: evNone}
}

// wheelEvent scrolls whatever list is under the pointer.
func (st *UIState) wheelEvent(m Mouse, snap *Snapshot) event {
	dir := -1
	if m.Button == MouseWheelDown {
		dir = 1
	}
	// An open diff owns the wheel: the user is reading it, not choosing a path.
	if st.View == ViewChanges && st.DiffOpen {
		if dir < 0 {
			st.DiffScroll = max(st.DiffScroll-3, 0)
		} else {
			st.DiffScroll += 3
			st.clampDiffScroll(snap)
		}
		return event{kind: evNone}
	}
	switch st.View {
	case ViewFiles:
		n := len(snap.Files)
		if n == 0 {
			return event{kind: evNone}
		}
		st.List.Index = clampInt(st.List.Index+dir, 0, n-1)
	case ViewChanges:
		pick := Selectable(BuildChangeRows(snap))
		if len(pick) == 0 {
			return event{kind: evNone}
		}
		st.Changes.Row = clampInt(st.Changes.Row+dir, 0, len(pick)-1)
	}
	return event{kind: evNone}
}

// clickEvent handles a left-button press.
//
// The overlay order matches handleKey: the most modal thing under the pointer
// wins. A click that misses every control is a no-op rather than a selection
// change, because the common cause is a click on dead space inside a panel and
// moving the cursor there would be an answer to a question nobody asked.
func (st *UIState) clickEvent(m Mouse, snap *Snapshot) event {
	if st.Permission.Open {
		return event{kind: evNone}
	}
	if st.Confirm.Kind != ConfirmNone {
		return st.confirmClick(m)
	}
	if st.Palette.Open {
		return st.paletteClick(m, snap)
	}
	if g, ok := dropdownGeom(st, st.Width); ok {
		if ev, hit := st.dropdownClick(m, g, snap); hit {
			return ev
		}
		// A press inside the menu bar but not on a row switches menus, the
		// same way Left and Right do.
		if m.Y == menuBarY && st.MenuBarHit(m.X) {
			return event{kind: evNone}
		}
		// Anywhere else closes the menu, which is what a click away from it
		// means everywhere else in the interface.
		st.Menu.Open = false
		return event{kind: evNone}
	}
	if m.Y == menuBarY {
		if idx, ok := st.MenuBarAt(m.X); ok {
			st.Menu.Open = true
			st.Menu.Bar = idx
			st.Menu.Cursor = 0
			st.Menu.OpenedAt = snap.Now
			return event{kind: evNone}
		}
		st.Menu.Open = false
		return event{kind: evNone}
	}
	if l, ok := st.mouseLayout(snap); ok {
		if ev, hit := st.railClick(m, l, snap); hit {
			return ev
		}
		// The companion is petted before the list views claim the press, so a
		// click on the animal is never read as a selection.
		if st.petPanelAt(m, snap) {
			st.Pet.Poke(snap.Now)
			return event{kind: evNone}
		}
		if ev, hit := st.listClick(m, l, snap); hit {
			return ev
		}
	}
	return event{kind: evNone}
}

// MenuBarAt reports which menu title covers a column.
func (st *UIState) MenuBarAt(x int) (int, bool) {
	for idx, m := range st.Menus.Menus {
		start := menuLabelX(st.Menus, idx)
		if x >= start && x < start+StringWidth(" "+m.Title) {
			return idx, true
		}
	}
	return 0, false
}

// MenuBarHit reports whether a column is over any menu title.
func (st *UIState) MenuBarHit(x int) bool {
	_, ok := st.MenuBarAt(x)
	return ok
}

// dropdownClick maps a click inside the open dropdown onto a row.
func (st *UIState) dropdownClick(m Mouse, g dropGeom, snap *Snapshot) (event, bool) {
	if !g.Inner.Contains(m.X, m.Y) {
		return event{kind: evNone}, false
	}
	row := m.Y - g.Inner.Y + g.Start
	if row < 0 || row >= len(g.Items) {
		return event{kind: evNone}, true
	}
	// The cursor is moved onto the clicked row first, so the highlight follows
	// the pointer the way it follows the arrow keys, and anything that reads
	// the cursor during the action sees the row the user actually chose.
	st.Menu.Cursor = row
	st.Menu.Open = false
	return st.runAction(g.Items[row].Action, snap), true
}

// confirmClick maps a click on the confirmation onto one of its two buttons.
//
// The buttons are located by the same offsets confirmButtons reports and the
// same geometry drawConfirm uses. A click that misses both, including a click on
// the body text, does nothing: it must never be read as consent. The safe
// choice always has its own button for exactly this reason.
func (st *UIState) confirmClick(m Mouse) event {
	if st.Confirm.RequirePhrase != "" {
		// The typed confirmation has no buttons: the phrase is the answer, and
		// a click cannot be one.
		return event{kind: evNone}
	}
	w := 54
	if st.Width-6 < w {
		w = st.Width - 6
	}
	if w < 24 {
		return event{kind: evNone}
	}
	h := 6 + len(st.Confirm.Body)
	inner := Inner(max((st.Width-w)/2, 0), max((st.Height-h)/2, 2), w, h, true)
	if !inner.Contains(m.X, m.Y) {
		return event{kind: evNone}
	}
	if m.Y != inner.Bottom()-1 {
		// The body text is not a button. A click there is a read, not a choice.
		return event{kind: evNone}
	}
	_, left, right := confirmButtons()
	if m.X >= inner.X+left && m.X < inner.X+right {
		st.Confirm.Choice = 0
		return st.confirmKey(Key{Type: KeyEnter}, &Snapshot{Now: st.LastKeyAt})
	}
	// Everything else on the button row is the safe side, so a mis-click always
	// falls on "stay safe" and never on the destructive action.
	st.Confirm.Choice = 1
	return st.confirmKey(Key{Type: KeyEnter}, &Snapshot{Now: st.LastKeyAt})
}

// paletteClick maps a click on the palette onto a command row.
func (st *UIState) paletteClick(m Mouse, snap *Snapshot) event {
	w := 52
	if st.Width-4 < w {
		w = st.Width - 4
	}
	if w < 20 {
		return event{kind: evNone}
	}
	h := 12
	if h > st.Height-4 {
		h = st.Height - 4
	}
	if h < 5 {
		return event{kind: evNone}
	}
	inner := Inner(max((st.Width-w)/2, 0), 2, w, h, true)
	if !inner.Contains(m.X, m.Y) {
		// A press outside the palette dismisses it, which is how every other
		// overlay in this interface behaves.
		st.Palette.Open = false
		return event{kind: evNone}
	}
	rows := inner.H - 2
	if rows <= 0 {
		return event{kind: evNone}
	}
	entries := st.Palette.entries(st.Commands)
	if len(entries) == 0 {
		return event{kind: evNone}
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
	j := m.Y - (inner.Y + 2)
	// Below the last entry is the empty tail of the panel, not a row: clicking
	// it must not run the command nearest the pointer.
	if j < 0 || j >= len(entries)-start {
		return event{kind: evNone}
	}
	st.Palette.Cursor = start + j
	return st.paletteKey(Key{Type: KeyEnter}, snap)
}

// railClick maps a click on the navigation rail onto a view.
func (st *UIState) railClick(m Mouse, l layout, snap *Snapshot) (event, bool) {
	if !l.HasRail {
		return event{kind: evNone}, false
	}
	if m.X < l.Rail.X || m.X >= l.Rail.Right() {
		return event{kind: evNone}, false
	}
	// The rail draws three fixed header rows, one blank, then one row per view.
	first := l.Rail.Y + 4
	for i, v := range Views() {
		y := first + i
		if y >= l.Rail.Bottom() {
			break
		}
		if y != m.Y {
			continue
		}
		if v != st.View {
			st.SetView(v, snap.Now)
			st.Focus = focusWorkspace
		}
		return event{kind: evNone}, true
	}
	// The rail's own header and footer are not navigation: a click there is
	// ignored rather than being read as "go to the first view".
	return event{kind: evNone}, true
}

// listClick maps a click on a list view onto the row under the pointer.
func (st *UIState) listClick(m Mouse, l layout, snap *Snapshot) (event, bool) {
	switch st.View {
	case ViewFiles:
		return st.clickFileRow(m, l, snap)
	case ViewChanges:
		return st.clickChangeRow(m, snap)
	default:
		return event{kind: evNone}, false
	}
}

// clickFileRow selects a workspace file by clicking it.
func (st *UIState) clickFileRow(m Mouse, l layout, snap *Snapshot) (event, bool) {
	inner := Inner(l.Work.X, l.Work.Y, l.Work.W, l.Work.H, true)
	if !inner.Contains(m.X, m.Y) {
		return event{kind: evNone}, false
	}
	if len(snap.Files) == 0 {
		return event{kind: evNone}, true
	}
	idx := st.clampScroll(len(snap.Files), inner.H) + (m.Y - inner.Y)
	if idx < 0 || idx >= len(snap.Files) {
		return event{kind: evNone}, true
	}
	st.List.Index = idx
	return event{kind: evNone}, true
}

// clickChangeRow selects a change by clicking it, and opens its diff.
//
// A click on the row the cursor is already on opens the review, so reviewing one
// change is a single click rather than click-then-Enter. Clicking a different
// row only moves the selection: a click that both selected and opened would make
// it impossible to move the cursor through a long list without loading a diff on
// every step.
func (st *UIState) clickChangeRow(m Mouse, snap *Snapshot) (event, bool) {
	listRect, _ := st.changeRects(snap)
	if listRect.Empty() || !listRect.Contains(m.X, m.Y) {
		return event{kind: evNone}, false
	}
	rows := BuildChangeRows(snap)
	pick := Selectable(rows)
	if len(pick) == 0 {
		return event{kind: evNone}, true
	}
	j := st.Changes.clampScroll(len(rows), listRect.H) + (m.Y - listRect.Y)
	if j < 0 || j >= len(rows) {
		return event{kind: evNone}, true
	}
	if rows[j].Header {
		// Headers label the runs and are not selectable, which is what stops a
		// header from ever stealing the cursor.
		return event{kind: evNone}, true
	}
	idx := -1
	for i, r := range pick {
		if r == j {
			idx = i
			break
		}
	}
	if idx < 0 {
		return event{kind: evNone}, true
	}
	if idx == st.Changes.Row && !st.DiffOpen {
		// The review is requested here, not performed: the session still has to
		// go and read the file.
		return st.currentChangeEvent(rows, pick), true
	}
	st.Changes.Row = idx
	return event{kind: evNone}, true
}

// changeRects returns the list and diff panes of the changes view.
//
// It is the shared geometry for drawing and for clicking. A click handler that
// guessed the split would be wrong on exactly the terminals where the split
// changes: the narrow ones, and the ones where the diff is open.
func (st *UIState) changeRects(snap *Snapshot) (list, diff Rect) {
	l := computeLayout(st.Width, st.Height, st.alertRows(snap))
	inner := Inner(l.Work.X, l.Work.Y, l.Work.W, l.Work.H, true)
	if inner.Empty() {
		return Rect{}, Rect{}
	}
	wide := inner.W >= 76 && st.DiffOpen
	if wide {
		listW := inner.W / 3
		if listW < 24 {
			listW = 24
		}
		return Rect{X: inner.X, Y: inner.Y, W: listW - 1, H: inner.H},
			Rect{X: inner.X + listW - 1, Y: inner.Y, W: inner.W - listW + 1, H: inner.H}
	}
	if st.DiffOpen {
		return Rect{}, inner
	}
	return inner, Rect{}
}

// alertRows reports how many rows the warning banner occupies, so a click is
// resolved against the same layout the frame was drawn with.
func (st *UIState) alertRows(snap *Snapshot) int {
	if st.Alert.Active(snap.Now) {
		return 1
	}
	return 0
}

// mouseLayout is the layout a click is resolved against.
func (st *UIState) mouseLayout(snap *Snapshot) (layout, bool) {
	l := computeLayout(st.Width, st.Height, st.alertRows(snap))
	return l, !l.Body.Empty()
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
