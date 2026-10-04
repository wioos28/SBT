package tui

import (
	"strings"
	"time"
)

// NewUIState builds UI state with defaults for the current terminal.
func NewUIState(w, h int) *UIState {
	st := &UIState{
		Width:   w,
		Height:  h,
		Motion:  motionAllowed(),
		InitRun: true,
	}
	return st
}

// motionAllowed honours SBT_NO_MOTION / NO_MOTION.
func motionAllowed() bool {
	for _, k := range []string{"SBT_NO_MOTION", "NO_MOTION"} {
		if v := osGetenv(k); v != "" && v != "0" {
			return false
		}
	}
	return true
}

// CloseAll collapses every overlay and returns to the terminal view.
func (st *UIState) CloseAll() {
	st.Palette.Open = false
	st.Confirm = Confirm{Kind: ConfirmNone}
	st.View = ViewTerminal
	st.Focus = focusInput
}

// flash sets a status bar message; the session clears it on the next tick.
func (st *UIState) flash(text string, kind StateKind, now time.Time) {
	st.Flash = Flash{Text: text, Kind: kind, SetAt: now}
}

// FlashExpire drops an expired flash message (6s).
func (st *UIState) FlashExpire(now time.Time) {
	if st.Flash.Text != "" && now.Sub(st.Flash.SetAt) > 6*time.Second {
		st.Flash = Flash{}
	}
}

// event is what a key press produces before anything is executed.
type event struct {
	kind      eventKind
	argv      []string
	paths     []string
	dest      string
	overwrite bool
	preset    PolicyChoice
}

type eventKind int

const (
	evNone eventKind = iota
	evHandled
	evRun
	evStop
	evSetPolicy
	evExport
	evDiscard
	evExit
	// evRefresh asks the session to re-run the platform probe. It is separate
	// from evStop because refreshing is a state change, not a redraw.
	evRefresh
)

// handleKey is the pure keyboard layer: one key in, one event out, plus any
// state change that is purely presentational. Nothing here starts a process,
// writes a file or touches the sandbox - those go through the events.
func (st *UIState) handleKey(k Key, snap *Snapshot) event {
	st.LastKeyAt = snap.Now
	if st.Palette.Open {
		return st.paletteKey(k, snap)
	}
	if st.Confirm.Kind != ConfirmNone {
		return st.confirmKey(k, snap)
	}
	if st.View == ViewExport {
		return st.exportKey(k, snap)
	}
	return st.baseKey(k, snap)
}

// baseKey handles keys when no overlay owns the keyboard.
func (st *UIState) baseKey(k Key, snap *Snapshot) event {
	switch {
	case k.Type == KeyRune && k.Ctrl && (k.Rune == 'k' || k.Rune == 'K'):
		st.Palette.Open = true
		st.Palette.Query = ""
		st.Palette.Cursor = 0
		return event{kind: evStop}
	case k.Type == KeyRune && k.Ctrl && k.Rune == 'd':
		return st.askExit(snap)
	case k.Type == KeyEsc:
		if st.View != ViewTerminal {
			st.SetView(ViewTerminal, snap.Now)
			return event{kind: evStop}
		}
		st.Focus = focusInput
		return event{kind: evStop}
	case k.Alt && k.Type == KeyRune && k.Rune >= '1' && k.Rune <= '6':
		st.SetView(View(k.Rune-'1'), snap.Now)
		st.Focus = focusWorkspace
		return event{kind: evStop}
	case k.Ctrl && k.Type == KeyRune && k.Rune >= '1' && k.Rune <= '6':
		st.SetView(View(k.Rune-'1'), snap.Now)
		st.Focus = focusWorkspace
		return event{kind: evStop}
	}
	switch st.View {
	case ViewTerminal:
		return st.terminalKey(k, snap)
	case ViewFiles:
		return st.listKey(k, snap, len(snap.Files))
	case ViewChanges:
		return st.changesKey(k, snap)
	case ViewStatus:
		return event{kind: evStop}
	case ViewExport:
		return st.exportKey(k, snap)
	}
	return event{kind: evStop}
}

// terminalKey handles the terminal view: typing a command.
func (st *UIState) terminalKey(k Key, snap *Snapshot) event {
	switch k.Type {
	case KeyEnter:
		argv := fields(st.Input)
		if len(argv) == 0 {
			return event{kind: evStop}
		}
		st.Input = ""
		return event{kind: evRun, argv: argv}
	case KeyBackspace:
		if st.Focus != focusInput {
			st.Focus = focusInput
			return event{kind: evStop}
		}
		if st.Input != "" {
			r := []rune(st.Input)
			st.Input = string(r[:len(r)-1])
		}
		return event{kind: evStop}
	case KeyUp, KeyDown, KeyPgUp, KeyPgDn, KeyHome, KeyEnd:
		// The transcript follows the newest lines; scrolling comes with the
		// diff viewer in a later pass.
		return event{kind: evStop}
	}
	if k.Type == KeyRune && !k.Ctrl {
		st.Focus = focusInput
		st.Input += string(k.Rune)
		return event{kind: evStop}
	}
	return event{kind: evStop}
}

// listKey handles the files and export lists.
func (st *UIState) listKey(k Key, snap *Snapshot, n int) event {
	switch k.Type {
	case KeyUp:
		if st.List.Index > 0 {
			st.List.Index--
		}
	case KeyDown:
		if st.List.Index < n-1 {
			st.List.Index++
		}
	case KeyPgUp:
		st.List.Index -= 10
		if st.List.Index < 0 {
			st.List.Index = 0
		}
	case KeyPgDn:
		st.List.Index += 10
		if st.List.Index >= n {
			st.List.Index = n - 1
		}
	case KeyHome:
		st.List.Index = 0
	case KeyEnd:
		st.List.Index = n - 1
	}
	return event{kind: evStop}
}

// changesKey handles the changes view.
func (st *UIState) changesKey(k Key, snap *Snapshot) event { return event{kind: evStop} }

// paletteKey handles the command palette. Typing filters, up/down move the
// cursor, enter runs the highlighted command.
func (st *UIState) paletteKey(k Key, snap *Snapshot) event {
	entries := st.Palette.entries(st.Commands)
	switch k.Type {
	case KeyEsc:
		st.Palette.Open = false
		return event{kind: evStop}
	case KeyUp:
		if st.Palette.Cursor > 0 {
			st.Palette.Cursor--
		}
		return event{kind: evStop}
	case KeyDown:
		if st.Palette.Cursor < len(entries)-1 {
			st.Palette.Cursor++
		}
		return event{kind: evStop}
	case KeyPgUp:
		st.Palette.Cursor -= 10
		if st.Palette.Cursor < 0 {
			st.Palette.Cursor = 0
		}
		return event{kind: evStop}
	case KeyPgDn:
		st.Palette.Cursor += 10
		if st.Palette.Cursor > len(entries)-1 {
			st.Palette.Cursor = len(entries) - 1
		}
		if st.Palette.Cursor < 0 {
			st.Palette.Cursor = 0
		}
		return event{kind: evStop}
	case KeyBackspace:
		if r := []rune(st.Palette.Query); len(r) > 0 {
			st.Palette.Query = string(r[:len(r)-1])
			st.Palette.Cursor = 0
		}
		return event{kind: evStop}
	case KeyEnter:
		st.Palette.Open = false
		if st.Palette.Cursor < 0 || st.Palette.Cursor >= len(entries) {
			return event{kind: evStop}
		}
		return st.runCommand(entries[st.Palette.Cursor].Action, snap)
	}
	if k.Type == KeyRune && !k.Ctrl {
		st.Palette.Query += string(k.Rune)
		st.Palette.Cursor = 0
	}
	return event{kind: evStop}
}

// runCommand turns a palette selection into the same event the key bindings
// would have produced. Going through one place is what keeps "ctrl+k exit" and
// "exit" from ever disagreeing about what exit means.
func (st *UIState) runCommand(a Action, snap *Snapshot) event {
	switch a.Kind {
	case ActShowView:
		st.SetView(a.View, snap.Now)
		st.Focus = focusWorkspace
	case ActRun:
		argv := fields(st.Input)
		if len(argv) == 0 {
			st.flash("type a command first", StateWarn, snap.Now)
			return event{kind: evStop}
		}
		st.Input = ""
		return event{kind: evRun, argv: argv}
	case ActExport:
		st.SetView(ViewExport, snap.Now)
		st.Focus = focusWorkspace
	case ActExportAll:
		st.SetView(ViewExport, snap.Now)
		st.Export.All = true
		if st.Export.Selected == nil {
			st.Export.Selected = map[int]bool{}
		}
		if st.Export.Warn == nil {
			st.Export.Warn = map[int]bool{}
			for i, f := range snap.Files {
				if f.Executable {
					st.Export.Warn[i] = true
				}
			}
		}
		st.askExport(snap)
	case ActDiscard:
		st.askDiscard(snap)
	case ActExit:
		return st.askExit(snap)
	case ActRefresh:
		st.flash("re-checking the platform report…", StateMeta, snap.Now)
		return event{kind: evRefresh}
	case ActMotion:
		st.Motion = !st.Motion
		if st.Motion {
			st.flash("animation on", StateOK, snap.Now)
		} else {
			st.flash("animation off", StateOK, snap.Now)
		}
	case ActDismissWarnings:
		st.Toasts.Clear()
		st.flash("warnings dismissed", StateMeta, snap.Now)
	}
	return event{kind: evStop}
}

// confirmKey drives the confirmation modal. Enter or space activates the
// highlighted choice; escape always resolves to the safe side, so a user who
// is unsure can never destroy anything by reflex.
func (st *UIState) confirmKey(k Key, snap *Snapshot) event {
	if k.Type == KeyEsc || k.Type == KeyBackTab {
		st.Confirm.Choice = 1
		st.closeConfirm()
		return event{kind: evStop}
	}
	if k.Type == KeyLeft || k.Type == KeyRight || k.Type == KeyTab {
		st.Confirm.Choice = 1 - st.Confirm.Choice
		return event{kind: evStop}
	}
	if k.Type == KeyEnter || (k.Type == KeyRune && k.Rune == ' ') {
		return st.acceptConfirm(snap)
	}
	return event{kind: evStop}
}

// acceptConfirm resolves the dialog into the event it was opened for. Only the
// dangerous choice (Choice 0) produces a destructive event.
func (st *UIState) acceptConfirm(snap *Snapshot) event {
	c := st.Confirm
	confirmed := c.Choice == 0
	st.closeConfirm()
	if !confirmed {
		st.flash("cancelled - nothing changed", StateMeta, snap.Now)
		return event{kind: evStop}
	}
	switch c.Kind {
	case ConfirmExit:
		return event{kind: evExit}
	case ConfirmDiscard:
		return event{kind: evDiscard}
	case ConfirmExport:
		// The picker works in row indices; the session works in paths. The
		// mapping happens here, where both the selection and the file list are
		// available, so neither side has to guess what the other's numbering
		// means.
		paths := make([]string, 0, len(snap.Files))
		for _, i := range st.Export.paths() {
			if i >= 0 && i < len(snap.Files) {
				paths = append(paths, snap.Files[i].Path)
			}
		}
		return event{kind: evExport, paths: paths, dest: st.Export.Destination, overwrite: st.Export.ExeAck}
	case ConfirmPolicy:
		return event{kind: evSetPolicy, preset: c.Policy}
	}
	return event{kind: evStop}
}

// closeConfirm clears the dialog, returning to the terminal view.
func (st *UIState) closeConfirm() {
	st.Confirm = Confirm{Kind: ConfirmNone}
}

// askExport opens the export confirmation. When the selection contains an
// executable the dialog names that fact explicitly: exporting a binary is a
// legitimate thing to want, but never an accident.
func (st *UIState) askExport(snap *Snapshot) {
	if len(snap.Files) == 0 {
		st.flash("nothing to export yet", StateWarn, snap.Now)
		return
	}
	if st.Export.Destination == "" {
		st.flash("choose a destination first (export --to PATH)", StateWarn, snap.Now)
		return
	}
	body := []string{"write " + itoa(len(st.Export.paths())) + " path(s) to", st.Export.Destination}
	if n := st.Export.executableCount(); n > 0 {
		body = append(body, "! "+itoa(n)+" executable file(s) included - this writes a binary to the host")
		st.Export.ExeAck = true
	}
	st.Confirm = Confirm{
		Kind:   ConfirmExport,
		Title:  "Export",
		Body:   body,
		Choice: 1,
		Export: st.Export.ExeAck,
	}
}

// askDiscard opens the confirmation that deletes the session workspace.
func (st *UIState) askDiscard(snap *Snapshot) {
	if snap.WorkspaceDir == "" {
		st.flash("no workspace to discard", StateWarn, snap.Now)
		return
	}
	st.Confirm = Confirm{
		Kind:   ConfirmDiscard,
		Title:  "Discard workspace",
		Body:   []string{"delete " + snap.WorkspaceDir + " and everything in it", "this cannot be undone"},
		Choice: 1,
	}
}

// askExit opens the exit confirmation, mentioning unsaved changes so the
// choice is informed rather than blind.
func (st *UIState) askExit(snap *Snapshot) event {
	body := []string{"leave SBT?"}
	if c := snap.Counts(); c.Total() > 0 {
		body = append(body, "! "+c.Summary()+" not yet exported")
	}
	st.Confirm = Confirm{
		Kind:   ConfirmExit,
		Title:  "Exit",
		Body:   body,
		Choice: 1,
	}
	return event{kind: evStop}
}

// exportKey drives the export picker: space toggles a row, "a" selects all,
// arrows move, enter asks for confirmation. It never writes anything itself.
func (st *UIState) exportKey(k Key, snap *Snapshot) event {
	if st.Export.Selected == nil {
		st.Export = NewExportSel(snap.Files, st.Export.Destination)
	}
	n := len(snap.Files)
	switch k.Type {
	case KeyEsc:
		st.SetView(ViewTerminal, snap.Now)
		st.Focus = focusInput
		return event{kind: evStop}
	case KeyEnter:
		st.askExport(snap)
		return event{kind: evStop}
	case KeyUp:
		if st.Export.Cursor > 0 {
			st.Export.Cursor--
		}
	case KeyDown:
		if st.Export.Cursor < n-1 {
			st.Export.Cursor++
		}
	case KeyHome:
		st.Export.Cursor = 0
	case KeyEnd:
		st.Export.Cursor = max(n-1, 0)
	case KeyPgUp:
		st.Export.Cursor -= 10
		if st.Export.Cursor < 0 {
			st.Export.Cursor = 0
		}
	case KeyPgDn:
		st.Export.Cursor += 10
		if st.Export.Cursor > n-1 {
			st.Export.Cursor = max(n-1, 0)
		}
	}
	if k.Type == KeyRune && !k.Ctrl && !k.Alt {
		switch k.Rune {
		case ' ':
			if n > 0 {
				st.Export.Toggle(st.Export.Cursor)
			}
		case 'a', 'A':
			st.Export.All = !st.Export.All
			if !st.Export.All {
				st.Export.Selected = map[int]bool{}
			}
		}
	}
	return event{kind: evStop}
}

func anyConfirmOpen(st *UIState) bool { return st != nil && st.Confirm.Kind != ConfirmNone }

func containsFold(s, q string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(q))
}

func Wrap(s string, width int) []string {
	if width <= 0 || s == "" {
		return []string{s}
	}
	if StringWidth(s) <= width {
		return []string{s}
	}
	out := []string{}
	word := ""
	for _, r := range s {
		if r == ' ' && word != "" {
			if StringWidth(word) > width {
				out = append(out, word)
				word = ""
				continue
			}
			out = append(out, word)
			word = ""
			continue
		}
		word += string(r)
	}
	if word != "" {
		out = append(out, word)
	}
	return out
}

// formatBytes renders a byte count for a narrow panel. It is the label beside
// a meter, so it favours brevity over precision: the exact number is available
// elsewhere and a 12 character suffix would push the bar out of the panel.
func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return itoa(int(n)) + "B"
	}
	exp := 0
	// One division always happens, because n >= unit was established above;
	// the loop then keeps dividing while the result is still big enough to be
	// worth another unit. Written the other way round the exponent and the
	// scale disagree by one, which reported 1024 as "1024KiB" or as "1MiB".
	n /= unit
	for n >= unit && exp < 5 {
		n /= unit
		exp++
	}
	return itoa(int(n)) + string("KMGTPE"[exp]) + "iB"
}

func formatDuration(d time.Duration) string { return d.String() }
