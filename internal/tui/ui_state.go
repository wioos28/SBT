package tui

import (
	"time"

	"github.com/wioos28/sbt/internal/shared/workspace"
)

// Focus identifies which region owns the keyboard.
type Focus int

// Focus targets.
const (
	focusWorkspace Focus = iota
	focusInput
)

// Flash is a transient status bar message.
type Flash struct {
	Text  string
	Kind  StateKind
	SetAt time.Time
}

// Transcript is the session terminal's message log. It is deliberately not the
// command's own output - that goes to the real terminal while the cage is
// suspended - but the record of what SBT did: runs, exits, notes, warnings.
type Transcript struct {
	Lines []Line
}

// Add appends a line.
func (tr *Transcript) Add(text string, kind StateKind) {
	tr.Lines = append(tr.Lines, Line{Text: text, State: kind})
}

// AddMeta appends a secondary, indented line.
func (tr *Transcript) AddMeta(text string) {
	tr.Lines = append(tr.Lines, Line{Text: text, State: StateMeta, Meta: true})
}

// AddRun records a finished command as transcript lines: one fact per line.
func (tr *Transcript) AddRun(rs RunSummary) {
	line := Line{Text: "run: " + rs.Command, State: StateOff}
	if rs.Exit == 0 {
		line.State = StateOK
	} else {
		line.State = StateDanger
	}
	tr.Lines = append(tr.Lines, line)
	tr.AddMeta("exit " + itoa(rs.Exit) + " · " + formatDuration(rs.Duration))
	if rs.Policy != "" {
		tr.AddMeta("policy " + rs.Policy)
	}
	if s := rs.Counts.Summary(); s != "" {
		kind := StateMeta
		if rs.Counts.Warnings > 0 {
			kind = StateWarn
		}
		tr.Lines = append(tr.Lines, Line{Text: "changes " + s, State: kind, Meta: true})
	}
	if rs.Note != "" {
		tr.Lines = append(tr.Lines, Line{Text: "note " + rs.Note, State: StateWarn, Meta: true})
	}
}

// Render returns the last lines that fit, already wrapped to the width.
func (tr *Transcript) Render(w, h int) []Line {
	if h <= 0 {
		return nil
	}
	var out []Line
	for _, ln := range tr.Lines {
		for _, piece := range Wrap(ln.Text, max(w, 1)) {
			out = append(out, Line{Text: piece, State: ln.State, Meta: ln.Meta})
		}
	}
	if len(out) > h {
		out = out[len(out)-h:]
	}
	return out
}

// ListState is the shared cursor of the list views.
type ListState struct {
	Index int
}

// clampScroll recentres the list viewport around the cursor.
func (st *UIState) clampScroll(n, rows int) int {
	if rows <= 0 || n == 0 {
		return 0
	}
	if st.List.Index >= n {
		st.List.Index = n - 1
	}
	if st.List.Index < 0 {
		st.List.Index = 0
	}
	start := st.ListScroll
	if st.List.Index < start {
		start = st.List.Index
	}
	if st.List.Index >= start+rows {
		start = st.List.Index - rows + 1
	}
	if start > n-rows {
		start = n - rows
	}
	if start < 0 {
		start = 0
	}
	st.ListScroll = start
	return start
}

// ExportSel is the export picker's state. Warn carries the rows SBT will not
// export without an explicit acknowledgement: executables.
type ExportSel struct {
	Selected    map[int]bool
	Warn        map[int]bool
	All         bool
	Cursor      int
	Scroll      int
	Destination string
	Message     string
	ExeAck      bool
}

// NewExportSel builds picker state for a file list.
func NewExportSel(files []FileInfo, destination string) ExportSel {
	sel := ExportSel{
		Selected:    map[int]bool{},
		Warn:        map[int]bool{},
		Destination: destination,
	}
	for i, f := range files {
		if f.Executable {
			sel.Warn[i] = true
		}
	}
	return sel
}

// Has reports whether a row is exported, directly or via "all".
func (s ExportSel) Has(i int) bool { return s.All || s.Selected[i] }

// Toggle flips one row's selection, leaving "all" mode.
func (s ExportSel) Toggle(i int) {
	if s.All {
		s.All = false
		s.Selected = map[int]bool{}
		for j := range s.Warn {
			s.Selected[j] = true
		}
		delete(s.Selected, i)
		return
	}
	if s.Selected[i] {
		delete(s.Selected, i)
		return
	}
	s.Selected[i] = true
}

// clampScroll keeps the export viewport on the cursor row.
func (s ExportSel) clampScroll(n, rows int) int {
	if rows <= 0 || n == 0 {
		return 0
	}
	if s.Cursor >= n {
		s.Cursor = n - 1
	}
	if s.Cursor < 0 {
		s.Cursor = 0
	}
	if s.Cursor < s.Scroll {
		s.Scroll = s.Cursor
	}
	if s.Cursor >= s.Scroll+rows {
		s.Scroll = s.Cursor - rows + 1
	}
	if s.Scroll > n-rows {
		s.Scroll = n - rows
	}
	if s.Scroll < 0 {
		s.Scroll = 0
	}
	return s.Scroll
}

// paths is the set of selected indices, honouring "all" mode.
func (s *ExportSel) paths() []int {
	out := []int{}
	if s.All {
		for i := range s.Warn {
			out = append(out, i)
		}
		for i := range s.Selected {
			if !s.Warn[i] {
				out = append(out, i)
			}
		}
		return out
	}
	for i := range s.Selected {
		out = append(out, i)
	}
	return out
}

// executableCount is how many selected paths are executables, which is what
// the confirmation dialog has to disclose.
func (s *ExportSel) executableCount() int {
	n := 0
	for _, i := range s.paths() {
		if s.Warn[i] {
			n++
		}
	}
	return n
}

// ChangeRow is one selectable row of the changes view: a single changed path
// belonging to a single run.
//
// The changes view is built as a flat list of these rather than a nested
// "runs, then files" tree. A flat list is what makes one cursor, one scroll
// offset and one up/down key enough to review everything the session produced,
// which is the whole job of this view.
type ChangeRow struct {
	// RunID is the journal id of the run that recorded the change.
	RunID string
	// RunIndex is the run's position in Snapshot.Runs.
	RunIndex int
	// Entry is the changed path itself.
	Entry workspace.Entry
	// Header marks a run separator row rather than a change.
	Header bool
}

// ChangeSel is the cursor of the changes view.
type ChangeSel struct {
	// Row is the index into the flat change list.
	Row int
	// Scroll is the first visible row.
	Scroll int
}

// BuildChangeRows flattens the run history into the changes view's rows. It is
// shared by the renderer and the key handler so the row the user highlights is
// by construction the row that is drawn - the two can never index different
// lists.
func BuildChangeRows(s *Snapshot) []ChangeRow {
	var rows []ChangeRow
	for i, run := range s.Runs {
		id := ""
		if i < len(s.RunIDs) {
			id = s.RunIDs[i]
		}
		rows = append(rows, ChangeRow{RunID: id, RunIndex: i, Header: true})
		if len(run.Entries) == 0 {
			continue
		}
		for _, e := range run.Entries {
			rows = append(rows, ChangeRow{RunID: id, RunIndex: i, Entry: e})
		}
	}
	return rows
}

// Selectable filters out the run header rows, which exist to label the list but
// are not themselves changes.
func Selectable(rows []ChangeRow) []int {
	var out []int
	for i, r := range rows {
		if !r.Header {
			out = append(out, i)
		}
	}
	return out
}

// clampScroll keeps the change viewport on the cursor row.
func (c *ChangeSel) clampScroll(n, rows int) int {
	if rows <= 0 || n == 0 {
		return 0
	}
	if c.Row >= n {
		c.Row = n - 1
	}
	if c.Row < 0 {
		c.Row = 0
	}
	start := c.Scroll
	if c.Row < start {
		start = c.Row
	}
	if c.Row >= start+rows {
		start = c.Row - rows + 1
	}
	if start > n-rows {
		start = n - rows
	}
	if start < 0 {
		start = 0
	}
	c.Scroll = start
	return start
}

// MenuState is the menu bar's own state: which menu is open, which row the
// cursor is on.
//
// The bar is always drawn. The dropdown is only drawn while Open, and opening
// it never moves focus away from the view behind it: the user is looking at a
// menu, but the work they will come back to has not lost its state.
type MenuState struct {
	Open   bool
	Bar    int // index into MenuBar.Menus
	Cursor int // row within the open menu
	// OpenedAt is when the menu opened, so the highlight can ease in rather
	// than appearing fully lit.
	OpenedAt time.Time
}

// Clamp keeps the cursors inside the current menu set. A menu bar can be
// swapped at runtime (a plugin, a test), so the state cannot assume its own
// indices are still valid.
func (m *MenuState) Clamp(bar MenuBar) {
	if len(bar.Menus) == 0 {
		m.Open = false
		m.Bar, m.Cursor = 0, 0
		return
	}
	if m.Bar < 0 {
		m.Bar = 0
	}
	if m.Bar >= len(bar.Menus) {
		m.Bar = len(bar.Menus) - 1
	}
	items := bar.Menus[m.Bar].Items
	if len(items) == 0 {
		m.Cursor = 0
		return
	}
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	if m.Cursor >= len(items) {
		m.Cursor = len(items) - 1
	}
}

// Items returns the rows of the currently open menu.
func (m MenuState) Items(bar MenuBar) []MenuItem {
	if len(bar.Menus) == 0 {
		return nil
	}
	idx := m.Bar
	if idx < 0 || idx >= len(bar.Menus) {
		idx = 0
	}
	return bar.Menus[idx].Items
}

// ConfirmKind is what a confirmation dialog is about. Every destructive action
// in SBT passes through one of these.
type ConfirmKind int

// Confirmation kinds.
const (
	ConfirmNone ConfirmKind = iota
	ConfirmPolicy
	ConfirmExport
	ConfirmDiscard
	ConfirmExit
)

// Confirm is a pending yes/no dialog.
type Confirm struct {
	Kind   ConfirmKind
	Title  string
	Body   []string
	Choice int // 0 = the dangerous action, 1 = the safe action
	Policy PolicyChoice
	Export bool // the export dialog carries executable warnings
}

// UIState is everything the interface remembers between frames. It is plain
// data: no goroutines, no locks, no I/O - which is what makes the keyboard
// behaviour testable without a terminal.
type UIState struct {
	Width, Height int

	View       View
	Focus      Focus
	List       ListState
	ListScroll int

	Input      string
	Transcript Transcript

	Palette   PaletteState
	Export    ExportSel
	Confirm   Confirm
	Boot      BootState
	Flash     Flash
	LastKeyAt time.Time

	// Menubar is the menu registry this state draws, and Menu the cursor
	// within it.
	Menus MenuBar
	Menu  MenuState

	// Changes is the cursor of the changes view: which run, which entry.
	Changes ChangeSel
	// DiffScroll is the first visible line of the open diff.
	DiffScroll int
	// DiffOpen is true while the diff pane has the keyboard.
	DiffOpen bool

	// Motion is whether animation is allowed. It is decided once at startup
	// from SBT_NO_MOTION / NO_MOTION and can be toggled from the palette, so a
	// user who finds the movement uncomfortable can turn it off mid-session
	// without restarting.
	Motion  bool
	InitRun bool // init animation has not been shown yet

	// Toasts is the notice stack drawn in the corner of the cage.
	Toasts Toasts
	// Transition carries the incoming view so it can ease in.
	Transition ViewTransition
	// Busy is the indeterminate progress shown while the session works.
	Busy Busy
	// Commands is the palette registry this state filters.
	Commands CommandSet
	// Meters are the animated gauges carried between frames.
	Meters Meters
	// Quit records that the session asked to close.
	Quit bool
}

// SetView switches the workspace view and starts the fade-in for it.
func (st *UIState) SetView(v View, now time.Time) {
	if st.View == v && st.Transition.At.IsZero() {
		return
	}
	st.Transition = ViewTransition{From: st.View, To: v, At: now}
	st.View = v
}

// max returns the larger of two ints.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
