package tui

import (
	"strings"
	"testing"
	"time"
)

// findItem returns the index of the titled row in the named menu.
func findItem(t *testing.T, st *UIState, menu, title string) int {
	t.Helper()
	idx := st.Menus.Index(menu)
	if idx < 0 {
		t.Fatalf("no %q menu", menu)
	}
	for i, item := range st.Menus.Menus[idx].Items {
		if item.Title == title {
			return i
		}
	}
	t.Fatalf("no %q row in the %q menu", title, menu)
	return 0
}

// A menu row must go through the same action dispatcher the palette uses, so
// "menu > view > changes" and "ctrl+k changes" cannot disagree.
func TestMenuEnterRunsTheSameActionAsThePalette(t *testing.T) {
	snap := baseSnapshot()
	st := newState(120, 40)
	st.Menu.Open = true
	st.Menu.Bar = st.Menus.Index("View")
	st.Menu.Cursor = findItem(t, st, "View", "Changes")

	if _, consumed := st.menuKey(Key{Type: KeyEnter}, &snap); !consumed {
		t.Fatal("enter in a menu must be consumed by the menu")
	}
	if st.Menu.Open {
		t.Fatal("the menu must close when a row is chosen")
	}
	if st.View != ViewChanges {
		t.Fatalf("menu > View > Changes must switch view, got %v", st.View)
	}
}

// A destructive row reached through the menu must still confirm. The menu is a
// shortcut, not a way around the dialog.
func TestMenuDestructiveRowStillConfirms(t *testing.T) {
	snap := baseSnapshot()
	snap.WorkspaceDir = "/tmp/ws"
	st := newState(120, 40)
	st.Menu.Open = true
	st.Menu.Bar = st.Menus.Index("Workspace")
	st.Menu.Cursor = findItem(t, st, "Workspace", "Discard workspace…")
	if _, consumed := st.menuKey(Key{Type: KeyEnter}, &snap); !consumed {
		t.Fatal("enter must be consumed by the menu")
	}
	if st.Confirm.Kind != ConfirmDiscard {
		t.Fatalf("discard through the menu must confirm, got %v", st.Confirm.Kind)
	}
	if st.Confirm.Choice != 1 {
		t.Fatal("a destructive dialog must default to the safe choice")
	}
}

// Escape closes a menu without activating anything.
func TestMenuEscapeClosesWithoutActing(t *testing.T) {
	snap := baseSnapshot()
	st := newState(120, 40)
	st.Menu.Open = true
	st.Menu.Bar = st.Menus.Index("Help")
	st.Menu.Cursor = findItem(t, st, "Help", "Exit")
	if _, consumed := st.menuKey(Key{Type: KeyEsc}, &snap); !consumed {
		t.Fatal("escape must be consumed by the menu")
	}
	if st.Menu.Open {
		t.Fatal("escape must close the menu")
	}
	if st.Confirm.Kind != ConfirmNone {
		t.Fatal("escape must not activate the highlighted row")
	}
}

// The menu opens with one key and never swallows ordinary typing, or the
// command input would break.
func TestMenuOpenKeyDoesNotSwallowTyping(t *testing.T) {
	snap := baseSnapshot()
	st := newState(120, 40)
	st.handleKey(Key{Type: KeyF10}, &snap)
	if !st.Menu.Open {
		t.Fatal("f10 must open the menu bar")
	}
	st.Menu.Open = false

	st.handleKey(Key{Type: KeyRune, Rune: 'e'}, &snap)
	st.handleKey(Key{Type: KeyRune, Rune: 'c'}, &snap)
	if st.Input != "ec" {
		t.Fatalf("typing must reach the command input, got %q", st.Input)
	}
}

// High risk mode must be reachable, and must confirm with the real facts rather
// than a bare label.
func TestHighRiskMenuRowConfirmsWithTheRealDiff(t *testing.T) {
	snap := baseSnapshot()
	st := newState(120, 40)
	st.Menu.Open = true
	st.Menu.Bar = st.Menus.Index("Security")
	st.Menu.Cursor = findItem(t, st, "Security", "High risk policy…")
	st.menuKey(Key{Type: KeyEnter}, &snap)

	if st.Confirm.Kind != ConfirmPolicy {
		t.Fatalf("high risk must ask first, got %v", st.Confirm.Kind)
	}
	if st.Confirm.Choice != 1 {
		t.Fatal("the high risk dialog must default to the safe choice")
	}
	joined := strings.Join(st.Confirm.Body, " ")
	if !strings.Contains(joined, "network") {
		t.Fatalf("the dialog must name what is given up, got %q", joined)
	}
	// Confirming must produce the policy change, not merely close a dialog.
	st.Confirm.Choice = 0
	ev := st.acceptConfirm(&snap)
	if ev.kind != evSetPolicy {
		t.Fatalf("confirming must emit the policy change, got %v", ev.kind)
	}
	if ev.preset != PolicyHigh {
		t.Fatalf("the confirmed preset must be high risk, got %v", ev.preset)
	}
}

// Every menu row must be reachable and named, and must actually raise an action.
func TestEveryMenuRowIsNamedAndActed(t *testing.T) {
	bar := DefaultMenuBar()
	if len(bar.Menus) == 0 {
		t.Fatal("the menu bar must not be empty")
	}
	seen := map[string]bool{}
	for _, m := range bar.Menus {
		if m.Title == "" {
			t.Fatal("a menu must have a title")
		}
		if seen[m.Title] {
			t.Fatalf("duplicate menu title %q", m.Title)
		}
		seen[m.Title] = true
		if len(m.Items) == 0 {
			t.Fatalf("menu %q has no rows", m.Title)
		}
		for _, item := range m.Items {
			if item.Title == "" {
				t.Fatalf("menu %q has an unnamed row", m.Title)
			}
			if item.Action.Kind == ActNone {
				t.Fatalf("menu %q row %q raises no action", m.Title, item.Title)
			}
		}
	}
}

// The help text must name the keys that actually exist.
func TestHelpLinesMatchTheRealBindings(t *testing.T) {
	joined := strings.Join(HelpLines, "\n")
	if len(helpViews) != len(Views()) {
		t.Fatalf("the help view list must be the real view list: %d vs %d",
			len(helpViews), len(Views()))
	}
	wantRange := "alt+1.." + Views()[len(Views())-1].Shortcut()[len("alt+"):]
	if !strings.Contains(joined, wantRange) {
		t.Fatalf("the help must name the real view range %q, got:\n%s", wantRange, joined)
	}
	for _, want := range []string{"f10", "ctrl+k", "ctrl+d", "enter"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("the help must document %q, got:\n%s", want, joined)
		}
	}
}

// Stopping a sandbox must be a real action, not a side effect of a cursor move.
func TestStopIsABindableAction(t *testing.T) {
	snap := baseSnapshot()
	st := newState(120, 40)
	if ev := st.handleKey(Key{Type: KeyRune, Rune: '.', Ctrl: true}, &snap); ev.kind != evStop {
		t.Fatalf("ctrl+. must stop the sandbox, got %v", ev.kind)
	}
}

// An idle interface must stop repainting. Repainting ten times a second while
// nothing moves is work the user pays for in battery and CPU.
func TestIdleInterfaceStopsRepainting(t *testing.T) {
	snap := baseSnapshot()
	st := newState(120, 40)
	app := &App{State: st, Snap: snap}
	st.Motion = true
	st.Transition = ViewTransition{To: ViewTerminal}

	// Long after the transition finished and with nothing moving, the cage is
	// still and must be left alone.
	app.Snap.Now = time.Now().Add(time.Hour)
	if app.needsFrames() {
		t.Fatal("a settled interface must not keep repainting")
	}

	// A starting sandbox does need frames, for the spinner.
	st.Busy = Busy{Active: true, Label: "starting sandbox"}
	if !app.needsFrames() {
		t.Fatal("a busy session must keep repainting")
	}
}

// An open menu must be visible: a menu the user cannot see is one they will not
// look for, and every other feature would then be unreachable by discovery.
func TestMenuBarIsAlwaysDrawn(t *testing.T) {
	snap := baseSnapshot()
	st := newState(120, 40)
	text := renderText(120, 40, &snap, st)
	for _, title := range []string{"Session", "View", "Security", "Workspace", "Help"} {
		if !strings.Contains(text, title) {
			t.Fatalf("the menu bar must show %q, got:\n%s", title, text)
		}
	}
}

// An open menu must show its rows, and must not cover the cage verdict - the
// verdict is the one thing a navigation affordance must never hide.
func TestOpenMenuShowsRowsWithoutHidingTheVerdict(t *testing.T) {
	snap := baseSnapshot()
	st := newState(120, 40)

	closed := renderText(120, 40, &snap, st)
	if !strings.Contains(closed, "READY") {
		t.Fatalf("the cage verdict must be on screen to begin with, got:\n%s", closed)
	}

	st.Menu.Open = true
	st.Menu.Bar = st.Menus.Index("View")
	open := renderText(120, 40, &snap, st)
	if !strings.Contains(open, "Terminal") {
		t.Fatalf("an open menu must list its rows, got:\n%s", open)
	}
	if !strings.Contains(open, "READY") {
		t.Fatalf("the cage verdict must stay visible under a menu, got:\n%s", open)
	}
}
