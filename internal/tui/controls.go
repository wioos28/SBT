package tui

import (
	"strings"
	"time"

	"github.com/wioos28/sbt/internal/config"
	"github.com/wioos28/sbt/internal/shared/policy"
)

// NewUIState builds UI state with defaults for the current terminal.
func NewUIState(w, h int) *UIState {
	st := &UIState{
		Width:   w,
		Height:  h,
		Motion:  motionAllowed(),
		InitRun: true,
		Menus:   DefaultMenuBar(),
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
	st.Menu.Open = false
	st.Confirm = Confirm{Kind: ConfirmNone}
	st.DiffOpen = false
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
	// runID and entry address one changed path for a diff request.
	runID string
	entry string
	// skey and sval carry a settings change the view validated and wants stored.
	skey string
	sval any
	// scope records which answer the user gave to a permission prompt.
	scope PermissionChoice
}

type eventKind int

const (
	// evNone is "this key was consumed and nothing needs to happen". It is the
	// default: a key that only moves a cursor returns this, and the caller
	// treats it as a redraw rather than an action. Keeping it distinct from
	// evStop is what lets "stop the sandbox" be a real, bindable action
	// instead of doubling as the no-op value.
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
	// evDiff asks the session to load one path's before/after content.
	evDiff
	// evDropDiff closes the diff pane without leaving the changes view.
	evDropDiff
	// evOpenPolicy asks for the high-risk confirmation.
	evOpenPolicy
	// evSetDest records a typed export destination.
	evSetDest
	// evSetSetting carries one validated settings change to the session.
	evSetSetting
	// evRepair asks the session to re-initialise the cage after a broken probe.
	evRepair
	// evDestroy is the sandbox wipe. It is only ever raised after the user typed
	// the exact phrase, so no key combination can reach it by accident.
	evDestroy
)

// wantsSession reports whether an event has to reach the session. Everything
// else is presentational and stops at the UI.
func (e event) wantsSession() bool {
	switch e.kind {
	case evRun, evStop, evSetPolicy, evExport, evDiscard, evExit, evRefresh, evDiff, evOpenPolicy, evSetSetting:
		return true
	}
	return false
}

// handleKey is the pure keyboard layer: one key in, one event out, plus any
// state change that is purely presentational. Nothing here starts a process,
// writes a file or touches the sandbox - those go through the events.
//
// The order of the checks is the overlay stack, from the most modal to the least:
// a confirmation, then the palette, then the menu, then the view's own keys. The
// most modal overlay always gets the key, which is what makes Escape reliably
// mean "back out one step" rather than "whatever the bottom layer would do".
func (st *UIState) handleKey(k Key, snap *Snapshot) event {
	st.LastKeyAt = snap.Now
	if st.Permission.Open {
		return st.permissionKey(k, snap)
	}
	if st.Confirm.Kind != ConfirmNone {
		return st.confirmKey(k, snap)
	}
	if st.Palette.Open {
		return st.paletteKey(k, snap)
	}
	if st.Menu.Open {
		if ev, ok := st.menuKey(k, snap); ok {
			return ev
		}
	}
	if st.View == ViewExport {
		return st.exportKey(k, snap)
	}
	return st.baseKey(k, snap)
}

// menuOpenKey is the single key that opens the menu bar. It is deliberately one
// binding rather than a global "any letter opens a menu" scheme: a menu bar that
// swallows ordinary typing is a menu bar that breaks the command input.
func (k Key) menuOpenKey() bool {
	return (k.Type == KeyF10) || (k.Alt && k.Type == KeyRune && k.Rune == 'm')
}

// baseKey handles keys when no overlay owns the keyboard.
func (st *UIState) baseKey(k Key, snap *Snapshot) event {
	switch {
	case k.menuOpenKey():
		st.Menu.Open = true
		st.Menu.OpenedAt = snap.Now
		st.Menu.Clamp(st.Menus)
		return event{kind: evNone}
	case k.Type == KeyRune && k.Ctrl && (k.Rune == 'k' || k.Rune == 'K'):
		st.Palette.Open = true
		st.Palette.Query = ""
		st.Palette.Cursor = 0
		return event{kind: evNone}
	case k.Type == KeyRune && k.Ctrl && k.Rune == 'd':
		return st.askExit(snap)
	case k.Type == KeyRune && k.Ctrl && k.Rune == '.':
		// Stopping is a real action, not a side effect of moving the cursor.
		return event{kind: evStop}
	case k.Type == KeyEsc:
		if st.DiffOpen {
			st.DiffOpen = false
			return event{kind: evNone}
		}
		if st.View != ViewTerminal {
			st.SetView(ViewTerminal, snap.Now)
			return event{kind: evNone}
		}
		st.Focus = focusInput
		return event{kind: evNone}
	case k.Alt && k.Type == KeyRune && k.Rune >= '1' && k.Rune <= '8':
		st.SetView(View(k.Rune-'1'), snap.Now)
		st.Focus = focusWorkspace
		return event{kind: evNone}
	case k.Ctrl && k.Type == KeyRune && k.Rune >= '1' && k.Rune <= '8':
		st.SetView(View(k.Rune-'1'), snap.Now)
		st.Focus = focusWorkspace
		return event{kind: evNone}
	}
	switch st.View {
	case ViewTerminal:
		return st.terminalKey(k, snap)
	case ViewFiles:
		return st.listKey(k, snap, len(snap.Files))
	case ViewChanges:
		return st.changesKey(k, snap)
	case ViewStatus:
		return event{kind: evNone}
	case ViewHelp:
		return event{kind: evNone}
	case ViewExport:
		return st.exportKey(k, snap)
	case ViewSettings:
		return st.settingsKey(k, snap)
	case ViewPermissions:
		return st.permissionsKey(k, snap)
	}
	return event{kind: evNone}
}

// menuKey drives the open dropdown. The second result reports whether the menu
// consumed the key; when it did not, the key falls through to the view behind
// it, so a menu can be opened and then navigated with the same keys the user
// already knows.
func (st *UIState) menuKey(k Key, snap *Snapshot) (event, bool) {
	menus := st.Menus
	st.Menu.Clamp(menus)
	items := st.Menu.Items(menus)
	switch k.Type {
	case KeyEsc:
		st.Menu.Open = false
		return event{kind: evNone}, true
	case KeyLeft:
		if len(menus.Menus) > 0 {
			st.Menu.Bar = (st.Menu.Bar - 1 + len(menus.Menus)) % len(menus.Menus)
			st.Menu.Cursor = 0
		}
		return event{kind: evNone}, true
	case KeyRight:
		if len(menus.Menus) > 0 {
			st.Menu.Bar = (st.Menu.Bar + 1) % len(menus.Menus)
			st.Menu.Cursor = 0
		}
		return event{kind: evNone}, true
	case KeyUp:
		if len(items) > 0 {
			st.Menu.Cursor = (st.Menu.Cursor - 1 + len(items)) % len(items)
		}
		return event{kind: evNone}, true
	case KeyDown:
		if len(items) > 0 {
			st.Menu.Cursor = (st.Menu.Cursor + 1) % len(items)
		}
		return event{kind: evNone}, true
	case KeyHome:
		st.Menu.Cursor = 0
		return event{kind: evNone}, true
	case KeyEnd:
		st.Menu.Cursor = max(len(items)-1, 0)
		return event{kind: evNone}, true
	case KeyEnter:
		if st.Menu.Cursor < 0 || st.Menu.Cursor >= len(items) {
			st.Menu.Open = false
			return event{kind: evNone}, true
		}
		// The menu closes before the action runs: an overlay that stays open
		// behind a confirmation dialog reads as a rendering fault.
		st.Menu.Open = false
		return st.runAction(items[st.Menu.Cursor].Action, snap), true
	}
	return event{kind: evNone}, false
}

// terminalKey handles the terminal view: typing a command.
func (st *UIState) terminalKey(k Key, snap *Snapshot) event {
	switch k.Type {
	case KeyEnter:
		text := strings.TrimSpace(st.Input)
		if text == "" {
			// An empty input line with files in the strip means the user wants
			// to open the selected file, not to run nothing.
			if st.Files.Shown && len(snap.Files) > 0 {
				st.Files.Clamp(len(snap.Files))
				st.SetView(ViewFiles, snap.Now)
				st.List.Index = st.Files.Index
			}
			return event{kind: evNone}
		}
		if strings.HasPrefix(text, "/") {
			st.Input = ""
			st.Typing.Reset()
			return st.slashCommand(text, snap)
		}
		argv := fields(st.Input)
		if len(argv) == 0 {
			return event{kind: evNone}
		}
		// A run the host cannot isolate is asked about, never silently refused
		// and never silently allowed. The prompt states the capability, why it
		// is needed and the risk, and the user decides.
		if snap.Sandbox.Unavailable != "" && !st.PermAcknowledged {
			cap, reason, risk, detail := blockingCapability(snap)
			st.Input = ""
			st.Typing.Reset()
			return st.openPermission(cap, reason, risk, detail, argv)
		}
		st.Input = ""
		st.Typing.Reset()
		return event{kind: evRun, argv: argv, scope: scopeOf(st)}
	case KeyLeft, KeyRight:
		if len(snap.Files) == 0 {
			return event{kind: evNone}
		}
		st.Files.Clamp(len(snap.Files))
		if k.Type == KeyLeft {
			if st.Files.Index > 0 {
				st.Files.Index--
			}
		} else if st.Files.Index < len(snap.Files)-1 {
			st.Files.Index++
		}
		return event{kind: evNone}
	case KeyBackspace:
		if st.Focus != focusInput {
			st.Focus = focusInput
			return event{kind: evNone}
		}
		if st.Input != "" {
			r := []rune(st.Input)
			st.Input = string(r[:len(r)-1])
		}
		st.Typing.Sync(snap.Now, st.Input)
		return event{kind: evNone}
	case KeyUp, KeyDown, KeyPgUp, KeyPgDn, KeyHome, KeyEnd:
		// The transcript follows the newest lines; scrolling comes with the
		// diff viewer in a later pass.
		return event{kind: evNone}
	}
	if k.Type == KeyRune && !k.Ctrl {
		st.Focus = focusInput
		st.Input += string(k.Rune)
		st.Typing.Sync(snap.Now, st.Input)
		return event{kind: evNone}
	}
	return event{kind: evNone}
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
	return event{kind: evNone}
}

// changesKey handles the changes view: the list of changed paths on one side,
// the diff of the selected path on the other.
//
// The list and the diff share the keyboard rather than being two separate modes
// the user has to remember to switch between, because the review act is "read
// this change, move to the next one". Enter opens the diff, Escape closes it,
// and the arrow keys always move the change cursor.
func (st *UIState) changesKey(k Key, snap *Snapshot) event {
	rows := BuildChangeRows(snap)
	pick := Selectable(rows)
	// The cursor always addresses a change, never a header, so entering always
	// has something to review.
	if st.Changes.Row >= len(pick) {
		st.Changes.Row = max(len(pick)-1, 0)
	}
	if st.Changes.Row < 0 {
		st.Changes.Row = 0
	}

	if st.DiffOpen {
		switch k.Type {
		case KeyEsc:
			st.DiffOpen = false
			st.DiffScroll = 0
			return event{kind: evNone}
		case KeyEnter, KeyTab:
			// Enter steps to the next change with its diff already open: the
			// common case is walking a whole run in order.
			if st.nextChange(pick) {
				return st.currentChangeEvent(rows, pick)
			}
			st.DiffOpen = false
			return event{kind: evNone}
		case KeyUp:
			return st.moveChange(rows, pick, -1, true)
		case KeyDown:
			return st.moveChange(rows, pick, 1, true)
		case KeyPgUp:
			st.DiffScroll = max(st.DiffScroll-10, 0)
			return event{kind: evNone}
		case KeyPgDn:
			st.DiffScroll += 10
			st.clampDiffScroll(snap)
			return event{kind: evNone}
		case KeyHome:
			st.DiffScroll = 0
			return event{kind: evNone}
		}
		return event{kind: evNone}
	}

	switch k.Type {
	case KeyUp:
		return st.moveChange(rows, pick, -1, false)
	case KeyDown:
		return st.moveChange(rows, pick, 1, false)
	case KeyPgUp:
		for n := 0; n < 10; n++ {
			st.stepChange(pick, -1)
		}
		return event{kind: evNone}
	case KeyPgDn:
		for n := 0; n < 10; n++ {
			st.stepChange(pick, 1)
		}
		return event{kind: evNone}
	case KeyHome:
		st.Changes.Row = 0
		return st.currentChangeEvent(rows, pick)
	case KeyEnd:
		st.Changes.Row = max(len(pick)-1, 0)
		return st.currentChangeEvent(rows, pick)
	case KeyEnter:
		// The pane opens only if there is a change to review. Opening an empty
		// pane on an empty session would replace "no changes recorded" with a
		// blank panel that says nothing about why.
		if ev := st.currentChangeEvent(rows, pick); ev.kind == evDiff {
			st.DiffOpen = true
			st.DiffScroll = 0
			return ev
		}
		st.flash("nothing to review yet", StateMeta, snap.Now)
	}
	return event{kind: evNone}
}

// currentChangeEvent asks the session for the selected change's diff.
func (st *UIState) currentChangeEvent(rows []ChangeRow, pick []int) event {
	if st.Changes.Row < 0 || st.Changes.Row >= len(pick) {
		return event{kind: evNone}
	}
	row := rows[pick[st.Changes.Row]]
	if row.Header || row.Entry.Path == "" {
		return event{kind: evNone}
	}
	return event{kind: evDiff, runID: row.RunID, entry: row.Entry.Path}
}

// nextChange moves the cursor one change along and reports whether it moved.
func (st *UIState) nextChange(pick []int) bool {
	if len(pick) == 0 {
		return false
	}
	if st.Changes.Row >= len(pick)-1 {
		return false
	}
	st.Changes.Row++
	return true
}

// stepChange moves the change cursor without raising a request.
func (st *UIState) stepChange(pick []int, delta int) {
	if len(pick) == 0 {
		return
	}
	st.Changes.Row += delta
	if st.Changes.Row < 0 {
		st.Changes.Row = 0
	}
	if st.Changes.Row >= len(pick) {
		st.Changes.Row = len(pick) - 1
	}
}

// moveChange moves the change cursor and, when a diff is already open, loads the
// newly selected change so the review keeps up with the cursor.
func (st *UIState) moveChange(rows []ChangeRow, pick []int, delta int, keepDiff bool) event {
	if len(pick) == 0 {
		return event{kind: evNone}
	}
	st.stepChange(pick, delta)
	st.DiffScroll = 0
	if keepDiff {
		return st.currentChangeEvent(rows, pick)
	}
	return event{kind: evNone}
}

// clampDiffScroll keeps the diff viewport inside the loaded content.
func (st *UIState) clampDiffScroll(snap *Snapshot) {
	if st.DiffScroll < 0 {
		st.DiffScroll = 0
	}
	if limit := max(len(snap.Diff.Lines)-1, 0); st.DiffScroll > limit {
		st.DiffScroll = limit
	}
}

// paletteKey handles the command palette. Typing filters, up/down move the
// cursor, enter runs the highlighted command.
func (st *UIState) paletteKey(k Key, snap *Snapshot) event {
	entries := st.Palette.entries(st.Commands)
	switch k.Type {
	case KeyEsc:
		st.Palette.Open = false
		return event{kind: evNone}
	case KeyUp:
		if st.Palette.Cursor > 0 {
			st.Palette.Cursor--
		}
		return event{kind: evNone}
	case KeyDown:
		if st.Palette.Cursor < len(entries)-1 {
			st.Palette.Cursor++
		}
		return event{kind: evNone}
	case KeyPgUp:
		st.Palette.Cursor -= 10
		if st.Palette.Cursor < 0 {
			st.Palette.Cursor = 0
		}
		return event{kind: evNone}
	case KeyPgDn:
		st.Palette.Cursor += 10
		if st.Palette.Cursor > len(entries)-1 {
			st.Palette.Cursor = len(entries) - 1
		}
		if st.Palette.Cursor < 0 {
			st.Palette.Cursor = 0
		}
		return event{kind: evNone}
	case KeyBackspace:
		if r := []rune(st.Palette.Query); len(r) > 0 {
			st.Palette.Query = string(r[:len(r)-1])
			st.Palette.Cursor = 0
		}
		return event{kind: evNone}
	case KeyEnter:
		st.Palette.Open = false
		if st.Palette.Cursor < 0 || st.Palette.Cursor >= len(entries) {
			return event{kind: evNone}
		}
		return st.runAction(entries[st.Palette.Cursor].Action, snap)
	}
	if k.Type == KeyRune && !k.Ctrl {
		st.Palette.Query += string(k.Rune)
		st.Palette.Cursor = 0
	}
	return event{kind: evNone}
}

// runAction turns a menu row or a palette selection into the same event the key
// bindings would have produced. Going through one place is what keeps "menu >
// exit", "ctrl+k exit" and "ctrl+d" from ever disagreeing about what exit means.
func (st *UIState) runAction(a Action, snap *Snapshot) event {
	switch a.Kind {
	case ActShowView:
		st.SetView(a.View, snap.Now)
		st.Focus = focusWorkspace
	case ActRun:
		argv := fields(st.Input)
		if len(argv) == 0 {
			st.flash("type a command first", StateWarn, snap.Now)
			return event{kind: evNone}
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
	case ActStop:
		return event{kind: evStop}
	case ActHighRisk:
		st.askHighRisk(snap)
	case ActRefresh:
		st.flash("re-checking the platform report…", StateMeta, snap.Now)
		return event{kind: evRefresh}
	case ActBack:
		// Back is the keyboard's Escape, expressed as an action, so a menu row
		// can offer it without the menu handler special-casing a key type.
		if st.DiffOpen {
			st.DiffOpen = false
			st.DiffScroll = 0
			return event{kind: evNone}
		}
		if st.View != ViewTerminal {
			st.SetView(ViewTerminal, snap.Now)
			st.Focus = focusInput
		}
	case ActMotion:
		st.Motion = !st.Motion
		if st.Motion {
			st.flash("animation on", StateOK, snap.Now)
		} else {
			st.flash("animation off", StateOK, snap.Now)
		}
	case ActRepairCage:
		return event{kind: evRepair}
	case ActDestroy:
		return st.askDestroy(snap)
	case ActToggleTheme:
		// The state records the choice; the App applies it to the live theme.
		// Splitting it this way keeps the keyboard layer free of side effects
		// on shared rendering state.
		snap.Light = !snap.Light
		ground := "dark"
		if snap.Light {
			ground = "light"
		}
		st.flash(ground+" theme", StateOK, snap.Now)
	case ActDismissWarnings:
		st.Toasts.Clear()
		st.flash("warnings dismissed", StateMeta, snap.Now)
	}
	return event{kind: evNone}
}

// askHighRisk opens the confirmation that relaxes the sandbox policy.
//
// The dialog lists the concrete knobs that change rather than showing the word
// "high risk": a user confirms facts, not a label. It is the one dialog in SBT
// whose dangerous choice is the LEFT button, because the left button always is -
// position carries the meaning whether or not colour is available.
func (st *UIState) askHighRisk(snap *Snapshot) {
	body := []string{"the next sandbox loses guarantees SBT cannot re-add:"}
	for _, line := range policy.Relaxed().Diff() {
		body = append(body, "! "+line)
	}
	body = append(body, "", "the cage badge will read HIGH RISK while it applies")
	st.Confirm = Confirm{
		Kind:   ConfirmPolicy,
		Title:  "High risk policy",
		Body:   body,
		Choice: 1,
		Policy: PolicyHigh,
	}
}

// confirmKey drives the confirmation modal. Enter or space activates the
// highlighted choice; escape always resolves to the safe side, so a user who
// is unsure can never destroy anything by reflex.
func (st *UIState) confirmKey(k Key, snap *Snapshot) event {
	if st.Confirm.RequirePhrase != "" {
		return st.confirmPhraseKey(k, snap)
	}
	if k.Type == KeyEsc || k.Type == KeyBackTab {
		st.Confirm.Choice = 1
		st.closeConfirm()
		return event{kind: evNone}
	}
	if k.Type == KeyLeft || k.Type == KeyRight || k.Type == KeyTab {
		st.Confirm.Choice = 1 - st.Confirm.Choice
		return event{kind: evNone}
	}
	if k.Type == KeyEnter || (k.Type == KeyRune && k.Rune == ' ') {
		return st.acceptConfirm(snap)
	}
	return event{kind: evNone}
}

// confirmPhraseKey drives the typed confirmation.
//
// The dangerous button is unreachable until the phrase matches exactly, and
// escape always lands on the safe side. The dialog opens with Choice already
// pointing at "keep", so a stray enter cannot destroy anything.
func (st *UIState) confirmPhraseKey(k Key, snap *Snapshot) event {
	switch k.Type {
	case KeyEsc:
		st.Confirm.Choice = 1
		st.closeConfirm()
		st.flash("cancelled - the sandbox was kept", StateMeta, snap.Now)
		return event{kind: evNone}
	case KeyBackspace:
		if r := []rune(st.Confirm.Phrase); len(r) > 0 {
			st.Confirm.Phrase = string(r[:len(r)-1])
		}
		return event{kind: evNone}
	case KeyEnter:
		if !st.confirmPhraseMatches() {
			st.flash("type the exact phrase to continue", StateWarn, snap.Now)
			return event{kind: evNone}
		}
		st.Confirm.Choice = 0
		return st.acceptConfirm(snap)
	}
	if k.Type == KeyRune && !k.Ctrl && !k.Alt {
		st.Confirm.Phrase += string(k.Rune)
	}
	return event{kind: evNone}
}

// confirmPhraseMatches reports whether the typed phrase is the exact one.
func (st *UIState) confirmPhraseMatches() bool {
	return st.Confirm.Phrase == st.Confirm.RequirePhrase && st.Confirm.RequirePhrase != ""
}

// askDestroy opens the destroy dialog. The safe side is preselected and the
// dangerous side is locked behind the phrase, so the destructive outcome can
// never be reached by pressing enter.
func (st *UIState) askDestroy(snap *Snapshot) event {
	body := []string{
		"SBT detected a critical condition.",
		"Destroying stops the sandbox and deletes the session workspace.",
		"Any change not exported first is lost.",
	}
	if c := snap.Counts(); c.Total() > 0 {
		body = append(body, "! "+c.Summary()+" not yet exported")
	}
	st.Confirm = Confirm{
		Kind:          ConfirmDestroy,
		Title:         "CRITICAL SECURITY EVENT",
		Body:          body,
		Choice:        1,
		RequirePhrase: DestroyPhrase,
	}
	return event{kind: evNone}
}

// DestroyPhrase is the exact text that unlocks the destructive button.
const DestroyPhrase = "DESTROY SBT SANDBOX"

// acceptConfirm resolves the dialog into the event it was opened for. Only the
// dangerous choice (Choice 0) produces a destructive event.
func (st *UIState) acceptConfirm(snap *Snapshot) event {
	c := st.Confirm
	confirmed := c.Choice == 0
	st.closeConfirm()
	if !confirmed {
		st.flash("cancelled - nothing changed", StateMeta, snap.Now)
		return event{kind: evNone}
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
	case ConfirmDestroy:
		return event{kind: evDestroy}
	}
	return event{kind: evNone}
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
	return event{kind: evNone}
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
		return event{kind: evNone}
	case KeyEnter:
		st.askExport(snap)
		return event{kind: evNone}
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
	return event{kind: evNone}
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

// slashCommand runs an in-session command typed into the input line.
//
// These exist so the common actions are reachable by name as well as by key,
// without the user having to remember an alt+<n> binding. An unknown command
// says so plainly instead of being handed to a sandbox, which is the whole
// point: a typo must never turn into a command that runs.
func (st *UIState) slashCommand(text string, snap *Snapshot) event {
	parts := strings.Fields(strings.TrimPrefix(text, "/"))
	if len(parts) == 0 {
		return event{kind: evNone}
	}
	name := strings.ToLower(parts[0])
	args := parts[1:]
	goTo := func(v View) event {
		st.SetView(v, snap.Now)
		st.Focus = focusWorkspace
		return event{kind: evNone}
	}
	switch name {
	case "help", "?":
		return goTo(ViewHelp)
	case "setting", "settings", "config":
		return goTo(ViewSettings)
	case "status":
		return goTo(ViewStatus)
	case "files":
		return goTo(ViewFiles)
	case "changes":
		return goTo(ViewChanges)
	case "export":
		return goTo(ViewExport)
	case "permissions", "perms", "security":
		return goTo(ViewPermissions)
	case "session", "terminal", "term":
		return goTo(ViewTerminal)
	case "clear":
		st.Transcript.Lines = nil
		st.flash("transcript cleared", StateMeta, snap.Now)
		return event{kind: evNone}
	case "theme":
		if len(args) == 0 {
			st.flash("usage: /theme "+strings.Join(config.ChoiceValues("ui.theme"), "|"), StateMeta, snap.Now)
			return event{kind: evNone}
		}
		return event{kind: evSetSetting, skey: "ui.theme", sval: args[0]}
	case "palette":
		if len(args) == 0 {
			st.flash("usage: /palette "+strings.Join(config.ChoiceValues("ui.palette"), "|"), StateMeta, snap.Now)
			return event{kind: evNone}
		}
		return event{kind: evSetSetting, skey: "ui.palette", sval: args[0]}
	case "lang", "language":
		if len(args) == 0 {
			st.flash("usage: /language "+strings.Join(config.ChoiceValues("language.locale"), "|"), StateMeta, snap.Now)
			return event{kind: evNone}
		}
		return event{kind: evSetSetting, skey: "language.locale", sval: args[0]}
	case "destroy", "wipe":
		return st.askDestroy(snap)
	case "repair", "reinit":
		st.flash("re-running the platform probe", StateMeta, snap.Now)
		return event{kind: evRepair}
	case "exit", "quit":
		return st.askExit(snap)
	}
	st.flash("unknown command /"+name+"  -  try /help", StateWarn, snap.Now)
	return event{kind: evNone}
}

// blockingCapability turns the measured report into the four lines the prompt
// needs: the capability that is missing, the SBT feature that wants it, the
// honest risk of proceeding without it, and the evidence.
func blockingCapability(snap *Snapshot) (capability, reason, risk, detail string) {
	capability = "verified isolation"
	reason = "running a command inside the sandbox"
	risk = "the command will not run in a verified sandbox"
	detail = snap.Sandbox.Unavailable
	for _, row := range snap.Permissions.Rows {
		if row.State != StateDanger {
			continue
		}
		capability = row.Name
		reason = row.Feature
		if reason == "" {
			reason = "running a command inside the sandbox"
		}
		risk = "the command runs with reduced isolation"
		detail = row.Reason
		break
	}
	if detail == "" {
		detail = snap.ProbeFail
	}
	return capability, reason, risk, detail
}

// scopeOf reports which acknowledgement the user already gave this session.
func scopeOf(st *UIState) PermissionChoice {
	if st.PermAcknowledged {
		return PermAllowSession
	}
	return PermAllowOnce
}

// notePermissionAnswer records the answer so the prompt is not asked twice for
// the same session. It never changes the cage.
func (st *UIState) notePermissionAnswer(choice PermissionChoice) {
	st.PermAcknowledged = choice == PermAllowSession
}
