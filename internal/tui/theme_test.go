package tui

import (
	"strings"
	"testing"
	"time"
)

// sum is the brightness of a colour, used by the palette checks below.
func sum(c RGB) int { return int(c.R) + int(c.G) + int(c.B) }

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// contrast is a coarse perceptual distance between two colours. It is not WCAG
// contrast: what these tests need is "clearly a different ground", not an
// accessibility audit, so the numbers only have to separate obviously distinct
// pairs.
func contrast(a, b RGB) int {
	return abs(int(a.R)-int(b.R)) + abs(int(a.G)-int(b.G)) + abs(int(a.B)-int(b.B))
}

// timeAt is a fixed clock so a rendered frame is reproducible.
func timeAt() time.Time { return time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC) }

// The dark palette is white-forward: white is the primary text. This is the most
// visible property of the interface, and a future tweak that nudges Text towards
// grey would silently undo the whole look.
func TestDarkPaletteLeadsWithWhite(t *testing.T) {
	p := DefaultPalette
	if p.Text != (RGB{255, 255, 255}) {
		t.Fatalf("the dark ground must lead with white text, got %+v", p.Text)
	}
	for name, c := range map[string]RGB{
		"Muted": p.Muted, "Border": p.Border, "Yellow": p.Yellow,
	} {
		if sum(c) >= sum(p.Text) {
			t.Fatalf("%s (%+v) must be dimmer than the white text", name, c)
		}
	}
}

// The light palette must be genuinely light, with dark text on it. A theme whose
// background is not actually white is not a light theme, and one whose text is
// not dark is unreadable.
func TestLightPaletteIsActuallyLight(t *testing.T) {
	p := LightPalette
	if sum(p.Bg) < 600 {
		t.Fatalf("the light ground must be a bright background, got %+v", p.Bg)
	}
	if sum(p.Text) >= sum(p.Bg) {
		t.Fatalf("the light ground needs dark text, got %+v on %+v", p.Text, p.Bg)
	}
	// This is where a mechanically inverted palette fails, so every foreground
	// is checked against the ground it will actually sit on.
	for name, c := range map[string]RGB{
		"Text": p.Text, "Muted": p.Muted, "Yellow": p.Yellow,
		"Green": p.Green, "Red": p.Red, "Warning": p.Warning, "Border": p.Border,
	} {
		if contrast(c, p.Bg) < 90 {
			t.Fatalf("%s (%+v) has too little contrast on the light ground (%+v)",
				name, c, p.Bg)
		}
	}
}

// Severity colours must stay distinct in both themes. They are the only thing
// The theme toggle must actually swap the ground, and must be reversible.
func TestThemeToggleSwapsTheGround(t *testing.T) {
	th := NewThemeAuto()
	before := th.Palette
	light := th.ToggleLight()
	if light != th.Light {
		t.Fatalf("ToggleLight must report the new ground: got %v, flag is %v", light, th.Light)
	}
	if th.Palette == before {
		t.Fatal("toggling must change the palette")
	}
	th.ToggleLight()
	if th.Palette != before {
		t.Fatal("toggling twice must restore the original palette")
	}
}

// SBT_THEME must be honoured, because a user who set it once should not have to
// set it every session.
func TestThemeEnvSelectsThePalette(t *testing.T) {
	t.Setenv("SBT_THEME", "light")
	if got := paletteForEnv(); got != LightPalette {
		t.Fatal("SBT_THEME=light must select the light palette")
	}
	t.Setenv("SBT_THEME", "dark")
	if got := paletteForEnv(); got != DefaultPalette {
		t.Fatal("SBT_THEME=dark must select the dark palette")
	}
	// A light terminal background is honoured when nothing explicit is set.
	t.Setenv("SBT_THEME", "")
	t.Setenv("COLORFGBG", "0;15")
	if got := paletteForEnv(); got != LightPalette {
		t.Fatal("a light terminal background must be honoured")
	}
	t.Setenv("COLORFGBG", "15;0")
	if got := paletteForEnv(); got != DefaultPalette {
		t.Fatal("a dark terminal background must not select the light palette")
	}
}

// Both grounds must render every view at every size. A theme that only works on
// one of them is not a theme switch.
func TestBothThemesRenderEveryView(t *testing.T) {
	for _, light := range []bool{false, true} {
		th := NewTheme(DefaultPalette)
		th.SetLight(light)
		in := NewInterpreter(th)
		for _, view := range []View{
			ViewTerminal, ViewFiles, ViewChanges, ViewStatus, ViewExport, ViewHelp,
		} {
			for _, sz := range [][2]int{{40, 10}, {80, 24}, {120, 40}} {
				snap := baseSnapshot()
				snap.Now = timeAt()
				snap.WorkspaceDir = "/tmp/ws"
				snap.Light = light
				st := newState(sz[0], sz[1])
				st.View = view
				in.Render(&snap, st)
			}
		}
	}
}

// The menu must show the current ground as a tick, so the user does not have to
// remember which one they are on.
func TestLookMenuShowsTheCurrentGround(t *testing.T) {
	st := newState(120, 40)
	idx := st.Menus.Index("Look")
	if idx < 0 {
		t.Fatal("there must be a Look menu")
	}
	var row *MenuItem
	for i := range st.Menus.Menus[idx].Items {
		if st.Menus.Menus[idx].Items[i].Checked != nil {
			row = &st.Menus.Menus[idx].Items[i]
			break
		}
	}
	if row == nil {
		t.Fatal("the Look menu must contain a toggle with a visible state")
	}
	snap := baseSnapshot()
	snap.Light = true
	if !row.Checked(&snap) {
		t.Fatal("the ground toggle must report the light ground as current")
	}
	snap.Light = false
	if row.Checked(&snap) {
		t.Fatal("the ground toggle must report the dark ground as current")
	}
}

// The theme toggle must be reachable from the palette as well as the menu, so
// neither surface ends up the only way in.
func TestThemeToggleIsInThePaletteToo(t *testing.T) {
	for _, c := range DefaultCommands().Commands() {
		if c.Action.Kind == ActToggleTheme {
			return
		}
	}
	t.Fatal("the palette must also offer the theme toggle")
}

// The two grounds must be nameable in the interface, or the status line would
// have nothing to say about which one is active.
func TestGroundNamesAreDistinct(t *testing.T) {
	th := NewTheme(DefaultPalette)
	th.SetLight(false)
	dark := th.Ground()
	th.SetLight(true)
	light := th.Ground()
	if dark == light || dark == "" || light == "" {
		t.Fatalf("the two grounds need distinct names, got %q and %q", dark, light)
	}
	if !strings.Contains(dark, "DARK") || !strings.Contains(light, "LIGHT") {
		t.Fatalf("ground names should say which is which, got %q and %q", dark, light)
	}
}

// that tells PROTECTED from BROKEN at a glance, so flattening them in either
// direction would be a safety regression rather than a style choice.
func TestSeverityColoursStayDistinct(t *testing.T) {
	for name, p := range map[string]Palette{"dark": DefaultPalette, "light": LightPalette} {
		if p.Green == p.Red {
			t.Fatalf("%s: protected and danger must not share a colour", name)
		}
		if p.Green == p.Warning || p.Red == p.Warning {
			t.Fatalf("%s: limited and danger must not share a colour", name)
		}
	}
}
