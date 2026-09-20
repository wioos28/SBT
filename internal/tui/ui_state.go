package tui

import (
	"time"
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

	View View
	Focus Focus
	List ListState
	ListScroll int

	Input      string
	Transcript Transcript

	Palette   PaletteState
	Export    ExportSel
	Confirm   Confirm
	Boot      BootState
	Flash     Flash
	LastKeyAt time.Time

	Motion  bool // motion allowed (SBT_NO_MOTION / NO_MOTION respected)
	InitRun bool // init animation has not been shown yet
}

// max returns the larger of two ints.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

