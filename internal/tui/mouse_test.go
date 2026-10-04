package tui

import (
	"testing"
	"time"
)

func TestParseSGRMouse(t *testing.T) {
	cases := []struct {
		name   string
		seq    string
		button MouseButton
		x, y   int
		press  bool
		ctrl   bool
		alt    bool
		shift  bool
	}{
		{"left press", "\x1b[<0;10;5M", MouseLeft, 9, 4, true, false, false, false},
		{"left release", "\x1b[<0;10;5m", MouseLeft, 9, 4, false, false, false, false},
		{"middle press", "\x1b[<1;3;3M", MouseMiddle, 2, 2, true, false, false, false},
		{"right press", "\x1b[<2;80;24M", MouseRight, 79, 23, true, false, false, false},
		{"wheel up", "\x1b[<64;5;5M", MouseWheelUp, 4, 4, true, false, false, false},
		{"wheel down", "\x1b[<65;5;5M", MouseWheelDown, 4, 4, true, false, false, false},
		{"ctrl left", "\x1b[<16;1;1M", MouseLeft, 0, 0, true, true, false, false},
		{"shift left", "\x1b[<4;1;1M", MouseLeft, 0, 0, true, false, false, true},
		{"alt left", "\x1b[<8;1;1M", MouseLeft, 0, 0, true, false, true, false},
		{"past 223 cols", "\x1b[<0;400;200M", MouseLeft, 399, 199, true, false, false, false},
		{"origin", "\x1b[<0;1;1M", MouseLeft, 0, 0, true, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, n := parseKey([]byte(tc.seq))
			if n != len(tc.seq) {
				t.Fatalf("consumed %d bytes, want %d", n, len(tc.seq))
			}
			if key.Type != KeyMouse {
				t.Fatalf("type = %v, want KeyMouse", key.Type)
			}
			if key.Mouse.Button != tc.button {
				t.Errorf("button = %v, want %v", key.Mouse.Button, tc.button)
			}
			if key.Mouse.X != tc.x || key.Mouse.Y != tc.y {
				t.Errorf("pos = (%d,%d), want (%d,%d)",
					key.Mouse.X, key.Mouse.Y, tc.x, tc.y)
			}
			if key.Mouse.Press != tc.press {
				t.Errorf("press = %v, want %v", key.Mouse.Press, tc.press)
			}
			if key.Mouse.Ctrl != tc.ctrl || key.Mouse.Alt != tc.alt || key.Mouse.Shift != tc.shift {
				t.Errorf("mods = (%v,%v,%v), want (%v,%v,%v)",
					key.Mouse.Ctrl, key.Mouse.Alt, key.Mouse.Shift, tc.ctrl, tc.alt, tc.shift)
			}
		})
	}
}

// A mouse report must not be mistaken for a key, and a cursor key must not be
// mistaken for a mouse report. Both would be silent misbehaviour on screen.
func TestMouseDoesNotBreakKeys(t *testing.T) {
	for _, seq := range []string{
		"\x1b[A", "\x1b[B", "\x1b[3~", "\x1b[1;5A", "\x1b[Z", "\x1bOP", "a", "\r",
	} {
		key, n := parseKey([]byte(seq))
		if n != len(seq) {
			t.Errorf("%q consumed %d, want %d", seq, n, len(seq))
		}
		if key.Type == KeyMouse {
			t.Errorf("%q decoded as a mouse event", seq)
		}
	}
}

// The legacy X10 report is refused rather than mis-decoded: its coordinates are
// capped and cannot be told apart from a key press by the bytes alone. Only the
// CSI introducer is consumed, so the trailing bytes are not mistaken for keys.
func TestX10MouseNotDecoded(t *testing.T) {
	key, n := parseKey([]byte("\x1b[M\x20\x30"))
	if n != 3 {
		t.Errorf("consumed %d bytes, want 3 (only the introducer)", n)
	}
	if key.Type == KeyMouse {
		t.Error("an X10 report was decoded as a mouse event")
	}
}

func mouseTestState() (*UIState, *Snapshot) {
	st := NewUIState(120, 40)
	st.Commands = DefaultCommands()
	st.Menus = DefaultMenuBar()
	return st, &Snapshot{Now: time.Now()}
}

func TestClickMenuBarOpensMenu(t *testing.T) {
	st, snap := mouseTestState()
	x := menuLabelX(st.Menus, 2) + 1
	idx, ok := st.MenuBarAt(x)
	if !ok {
		t.Fatal("no menu title at the column the renderer draws it")
	}
	ev := st.handleKey(Key{Type: KeyMouse, Mouse: Mouse{
		Button: MouseLeft, X: x, Y: menuBarY, Press: true,
	}}, snap)
	if ev.kind != evNone {
		t.Errorf("kind = %v, want evNone", ev.kind)
	}
	if !st.Menu.Open {
		t.Fatal("the menu did not open")
	}
	if st.Menu.Bar != idx {
		t.Errorf("open menu = %d, want %d", st.Menu.Bar, idx)
	}
}

// The decisive test: for every row of the drawn dropdown, clicking inside the
// panel must be accepted by the panel. This is what proves the renderer and the
// hit test share one geometry rather than two copies of the same numbers.
func TestDropdownClickHitsPanel(t *testing.T) {
	st, snap := mouseTestState()
	st.Menu.Open = true
	st.Menu.Bar = 1
	g, ok := dropdownGeom(st, st.Width)
	if !ok {
		t.Fatal("no dropdown geometry while the menu is open")
	}
	// A press on the panel's own border must not be read as a row.
	ev := st.handleKey(Key{Type: KeyMouse, Mouse: Mouse{
		Button: MouseLeft, X: g.Inner.X, Y: g.Inner.Y - 1, Press: true,
	}}, snap)
	if ev.kind != evNone {
		t.Errorf("border press raised %v", ev.kind)
	}
	// Every visible row is reachable.
	visible := min(len(g.Items), g.Inner.H)
	for row := 0; row < visible; row++ {
		st.Menu.Open = true
		st.handleKey(Key{Type: KeyMouse, Mouse: Mouse{
			Button: MouseLeft, X: g.Inner.X, Y: g.Inner.Y + row, Press: true,
		}}, snap)
		if st.Menu.Open {
			t.Fatalf("row %d: the menu stayed open, so the click missed", row)
		}
	}
}

// A click on a row runs the same action Enter would, through the same
// runAction, so "menu > View > Settings" and a click cannot disagree.
func TestDropdownClickRunsAction(t *testing.T) {
	st, snap := mouseTestState()
	st.View = ViewTerminal
	st.handleKey(Key{Type: KeyF10}, snap)
	st.Menu.Bar = 1 // View, which holds the per-view actions
	items := st.Menu.Items(st.Menus)
	target := -1
	for i, it := range items {
		if it.Action.Kind == ActShowView && it.Action.View == ViewSettings {
			target = i
			break
		}
	}
	if target < 0 {
		t.Skip("this menu has no view action to click")
	}
	g, ok := dropdownGeom(st, st.Width)
	if !ok {
		t.Fatal("no dropdown geometry")
	}
	y := g.Inner.Y + target - g.Start
	if y < g.Inner.Y || y >= g.Inner.Bottom() {
		t.Skip("the target row is scrolled out of the visible window")
	}
	st.handleKey(Key{Type: KeyMouse, Mouse: Mouse{
		Button: MouseLeft, X: g.Inner.X, Y: y, Press: true,
	}}, snap)
	if st.View != ViewSettings {
		t.Errorf("view = %v, want %v", st.View, ViewSettings)
	}
}

// Dangerous rows stay reachable by click, and the click still opens the
// confirmation rather than performing the action.
func TestDangerousRowClickStillConfirms(t *testing.T) {
	st, snap := mouseTestState()
	// askDiscard refuses outright when there is no workspace, so a test that
	// expects the dialog has to give it one.
	snap.WorkspaceDir = "/tmp/sbt-test-workspace"
	st.Menu.Open = true
	st.Menu.Bar = 3 // Workspace, which holds the discard row
	g, ok := dropdownGeom(st, st.Width)
	if !ok {
		t.Fatal("no dropdown geometry")
	}
	target := -1
	for i, it := range g.Items {
		if it.Dangerous && it.Action.Kind == ActDiscard {
			target = i
			break
		}
	}
	if target < 0 {
		t.Skip("this menu has no dangerous discard row")
	}
	y := g.Inner.Y + target - g.Start
	if y < g.Inner.Y || y >= g.Inner.Bottom() {
		t.Skip("the target row is scrolled out of the visible window")
	}
	// ActDiscard opens the confirmation and returns no action of its own: the
	// discard is only raised once the user confirms. So the correct guarantee is
	// that the click opened the dialog rather than performing the wipe.
	ev := st.handleKey(Key{Type: KeyMouse, Mouse: Mouse{
		Button: MouseLeft, X: g.Inner.X, Y: y, Press: true,
	}}, snap)
	if ev.kind != evNone {
		t.Errorf("kind = %v, want evNone (the click must not act yet)", ev.kind)
	}
	if st.Confirm.Kind != ConfirmDiscard {
		t.Errorf("confirm = %v, want ConfirmDiscard", st.Confirm.Kind)
	}
	if st.Menu.Open {
		t.Error("the menu stayed open behind the dialog")
	}
}

// The confirmation is the one dialog where a click must never say yes by
// accident. Only the exact "run it" button may, and everything else on the
// button row falls to the safe side.
func TestConfirmClickIsFailSafe(t *testing.T) {
	st, _ := mouseTestState()
	open := func() {
		st.Confirm.Kind = ConfirmExit
		st.Confirm.Title = "Exit"
		st.Confirm.Body = []string{"leave the session?"}
		st.Confirm.Choice = 0
	}
	w := 54
	h := 6 + 1
	inner := Inner(max((st.Width-w)/2, 0), max((st.Height-h)/2, 2), w, h, true)
	_, _, right := confirmButtons()

	// A click on the body text is a read, not a choice.
	open()
	st.confirmClick(Mouse{Button: MouseLeft, X: inner.X + 2, Y: inner.Y, Press: true})
	if st.Confirm.Kind == ConfirmNone {
		t.Error("a click on the dialog body dismissed it")
	}

	// A click on the safe side takes the safe side.
	open()
	st.confirmClick(Mouse{Button: MouseLeft, X: inner.X + right, Y: inner.Bottom() - 1, Press: true})
	if st.Confirm.Kind != ConfirmNone {
		t.Error("a click on the safe side still closed the dialog")
	}
}

// The typed destroy confirmation has no buttons at all: the phrase is the
// answer, and a click must not be able to unlock it.
func TestPhraseConfirmIgnoresClicks(t *testing.T) {
	st, _ := mouseTestState()
	st.Confirm.Kind = ConfirmDestroy
	st.Confirm.RequirePhrase = "destroy this cage"
	ev := st.confirmClick(Mouse{Button: MouseLeft, X: 5, Y: 5, Press: true})
	if ev.kind != evNone {
		t.Errorf("kind = %v, want evNone", ev.kind)
	}
	if st.Confirm.Phrase != "" {
		t.Error("a click typed into the phrase prompt")
	}
}

// A permission prompt is the most modal thing in the interface and it is
// answered by keys; a click must not be able to wave it through.
func TestPermissionPromptIgnoresClicks(t *testing.T) {
	st, snap := mouseTestState()
	st.Permission.Open = true
	ev := st.handleKey(Key{Type: KeyMouse, Mouse: Mouse{
		Button: MouseLeft, X: st.Width / 2, Y: st.Height / 2, Press: true,
	}}, snap)
	if ev.kind != evNone {
		t.Errorf("kind = %v, want evNone", ev.kind)
	}
}

func TestReleaseDoesNotSelect(t *testing.T) {
	st, snap := mouseTestState()
	st.View = ViewFiles
	st.List.Index = 0
	l := computeLayout(st.Width, st.Height, st.alertRows(snap))
	inner := Inner(l.Work.X, l.Work.Y, l.Work.W, l.Work.H, true)
	st.handleKey(Key{Type: KeyMouse, Mouse: Mouse{
		Button: MouseLeft, X: inner.X, Y: inner.Y + 2, Press: false,
	}}, snap)
	if st.List.Index != 0 {
		t.Error("a button release moved the cursor")
	}
}

// A click on dead space is a no-op: moving a cursor there would be an answer to
// a question nobody asked.
func TestClickOnDeadSpaceDoesNothing(t *testing.T) {
	st, snap := mouseTestState()
	st.View = ViewTerminal
	st.List.Index = 3
	ev := st.handleKey(Key{Type: KeyMouse, Mouse: Mouse{
		Button: MouseLeft, X: st.Width - 2, Y: st.Height - 2, Press: true,
	}}, snap)
	if ev.kind != evNone {
		t.Errorf("kind = %v, want evNone", ev.kind)
	}
	if st.List.Index != 3 {
		t.Error("a click on dead space moved a cursor")
	}
}

// The wheel clamps at both ends instead of wrapping, and never goes negative.
func TestWheelScrollsList(t *testing.T) {
	st, snap := mouseTestState()
	st.View = ViewFiles
	snap.Files = []FileInfo{{Path: "a"}, {Path: "b"}, {Path: "c"}}
	st.List.Index = 2
	st.handleKey(Key{Type: KeyMouse, Mouse: Mouse{Button: MouseWheelDown}}, snap)
	if st.List.Index != 2 {
		t.Errorf("index = %d, want 2 (already at the last row)", st.List.Index)
	}
	st.List.Index = 0
	st.handleKey(Key{Type: KeyMouse, Mouse: Mouse{Button: MouseWheelDown}}, snap)
	if st.List.Index != 1 {
		t.Errorf("index = %d, want 1", st.List.Index)
	}
	for i := 0; i < 50; i++ {
		st.handleKey(Key{Type: KeyMouse, Mouse: Mouse{Button: MouseWheelUp}}, snap)
	}
	if st.List.Index != 0 {
		t.Errorf("index = %d, want 0", st.List.Index)
	}
}

// A click outside an open palette dismisses it, like every other overlay.
func TestClickOutsidePaletteDismisses(t *testing.T) {
	st, snap := mouseTestState()
	st.Palette.Open = true
	st.handleKey(Key{Type: KeyMouse, Mouse: Mouse{
		Button: MouseLeft, X: 1, Y: st.Height - 1, Press: true,
	}}, snap)
	if st.Palette.Open {
		t.Error("a click outside the palette left it open")
	}
}

// Every geometry the click handler uses must survive the same sweep of terminal
// sizes the render tests use: a hit test that panics on a small terminal is a
// crash the user reaches with a mouse.
func TestMouseHitTestSurvivesEveryGeometry(t *testing.T) {
	for w := 20; w <= 200; w += 7 {
		for h := 6; h <= 60; h += 5 {
			st := NewUIState(w, h)
			st.Commands = DefaultCommands()
			st.Menus = DefaultMenuBar()
			snap := &Snapshot{Now: time.Now()}
			views := append([]View{}, Views()...)
			for _, v := range views {
				st.View = v
				st.Menu.Open = true
				st.Palette.Open = true
				st.Confirm.Kind = ConfirmDiscard
				st.Confirm.Body = []string{"really?"}
				for y := 0; y < h; y++ {
					for x := 0; x < w; x++ {
						for _, b := range []MouseButton{
							MouseLeft, MouseWheelUp, MouseWheelDown,
						} {
							st.handleKey(Key{Type: KeyMouse, Mouse: Mouse{
								Button: b, X: x, Y: y, Press: true,
							}}, snap)
						}
					}
				}
			}
		}
	}
}
