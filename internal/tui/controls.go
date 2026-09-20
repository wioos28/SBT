package tui

import "time"

// NewUIState builds UI state with defaults for the current terminal.
func NewUIState(w, h int) *UIState {
	st := &UIState{
		Width:  w,
		Height: h,
		Motion: motionAllowed(),
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
	kind    eventKind
	argv    []string
	paths   []string
	dest    string
	overwrite bool
	preset  PolicyChoice
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
			st.View = ViewTerminal
			return event{kind: evStop}
		}
		st.Focus = focusInput
		return event{kind: evStop}
	case k.Alt && k.Type == KeyRune && k.Rune >= '1' && k.Rune <= '5':
		st.View = View(k.Rune - '1')
		st.Focus = focusWorkspace
		return event{kind: evStop}
	case k.Ctrl && k.Type == KeyRune && k.Rune >= '1' && k.Rune <= '5':
		st.View = View(k.Rune - '1')
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
func (st *UIState) changesKey(d ...Key) event { return event{kind: evStop} }
