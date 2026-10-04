// Package tui renders the SBT Sandbox Terminal interface.
//
// The whole application is one cage: an outer frame whose accent colour is the
// session state, containing a top bar, a navigation rail, the workspace, the
// monitor and a status bar. Nothing here is decorative - colour always carries
// the same meaning and is always accompanied by a word, so the interface still
// works with colour disabled.
//
// The renderer is a cell buffer: views write styled runes into a Screen and the
// Screen writes only the lines that changed. No external dependency is used:
// SBT talks to the terminal directly.
package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// RGB is a 24 bit colour.
type RGB struct{ R, G, B uint8 }

// Hex builds an RGB from a 0xRRGGBB value.
func Hex(v uint32) RGB { return RGB{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v)} }

// Palette is the SBT colour set, exactly as specified for the product.
type Palette struct {
	Bg       RGB
	BgTop    RGB
	BgBottom RGB
	Surface  RGB
	Surface2 RGB
	Yellow   RGB
	YellowHi RGB
	Green    RGB
	Red      RGB
	Text     RGB
	Fg       RGB
	Muted    RGB
	Border   RGB
	Warning  RGB
}

// DefaultPalette is the SBT palette: a white-forward dark scheme.
//
// White leads the interface - the primary text is pure white and the greys are
// cool and low-contrast - so the interface reads as text on a ground rather
// than as coloured blocks. The accent and the three severity colours are the
// exception, and deliberately so: they answer "is this safe?", which is the one
// question the interface exists to answer. Flattening those to white would make
// PROTECTED and BROKEN look identical at a glance, so they keep their colour
// and their word.
var DefaultPalette = Palette{
	Bg:       Hex(0x0A0C10),
	BgTop:    Hex(0x12151C),
	BgBottom: Hex(0x0D1015),
	Surface:  Hex(0x111419),
	Surface2: Hex(0x171B22),
	Yellow:   Hex(0xF7D046),
	YellowHi: Hex(0xE8BC2F),
	Green:    Hex(0x34D399),
	Red:      Hex(0xFF6B6B),
	Text:     Hex(0xFFFFFF),
	Fg:       Hex(0xFFFFFF),
	Muted:    Hex(0x8B95A7),
	Border:   Hex(0x2B323D),
	Warning:  Hex(0xFBBF24),
}

// LightPalette is the same interface on a white ground.
//
// It is not an inversion of the dark palette. Every colour is chosen against a
// light background for contrast, because mechanically flipping a dark scheme
// produces pastels that are unreadable in daylight - which is exactly the
// situation a light theme is for.
var LightPalette = Palette{
	Bg:       Hex(0xFFFFFF),
	BgTop:    Hex(0xF7F9FC),
	BgBottom: Hex(0xF1F4F9),
	Surface:  Hex(0xFBFCFE),
	Surface2: Hex(0xF4F7FB),
	Yellow:   Hex(0xB45309),
	YellowHi: Hex(0x92400E),
	Green:    Hex(0x047857),
	Red:      Hex(0xDC2626),
	Text:     Hex(0x0B1220),
	Fg:       Hex(0x0B1220),
	Muted:    Hex(0x5B6678),
	Border:   Hex(0xC8D2E0),
	Warning:  Hex(0xC2410C),
}

// ColorDepth is how much colour the terminal can take.
type ColorDepth int

// Supported colour depths, most capable first.
const (
	ColorTrue ColorDepth = iota
	Color256
	Color16
	ColorNone
)

// CageState is the state the outer frame reports. It drives the accent colour
// and, because colour alone is never the message, always comes with a label.
type CageState int

// Cage states, ordered by severity.
const (
	// CageOff is an idle session with no verified isolation available.
	CageOff CageState = iota
	// CageProtected is an idle session whose isolation the probe verified.
	CageProtected
	// CageLow is a running sandbox under the standard policy.
	CageLow
	// CageHigh is a running sandbox whose policy the user relaxed.
	CageHigh
	// CageLimited and CageBroken are the explicit runtime warnings the UI renders.
	CageLimited
	CageBroken
)

// Label is the word shown next to the accent colour.
func (c CageState) Label() string {
	switch c {
	case CageProtected:
		return "PROTECTED"
	case CageLow:
		return "LOW MODE"
	case CageHigh:
		return "HIGH RISK"
	case CageLimited:
		return "LIMITED"
	case CageBroken:
		return "BROKEN"
	default:
		return "NORMAL"
	}
}

// Severity orders the cage states so a session can report the worst one.
func (c CageState) Severity() int { return int(c) }

// Style is a rune attribute set: colours plus a few flags.
type Style struct {
	Fg      RGB
	Bg      RGB
	HasBg   bool
	Bold    bool
	Dim     bool
	Italic  bool
	Reverse bool
	Under   bool
}

// Theme renders styles for a terminal. A zero Theme is usable and renders plain
// text, so no caller has to nil-check.
type Theme struct {
	Palette Palette
	Depth   ColorDepth
	ASCII   bool
	Motion  bool
	// Light records which ground is in use, so the menu can show the current
	// choice and the status bar can name it.
	Light bool
}

// NewTheme builds the theme for a terminal.
func NewTheme(p Palette) *Theme {
	return &Theme{
		Palette: p,
		Depth:   DetectDepth(),
		ASCII:   !SupportsUnicode(),
		Motion:  MotionAllowed(),
	}
}

// NewThemeAuto builds the theme the environment asks for.
//
// SBT_THEME=light or dark chooses explicitly; otherwise a terminal that reports
// a light background is honoured, so SBT does not open as a white-on-white
// screen on someone who never asked for it. Everything else gets the dark
// palette, because the severity colours in it were chosen for a dark ground.
func NewThemeAuto() *Theme {
	t := NewTheme(paletteForEnv())
	t.Light = t.Palette == LightPalette
	return t
}

// paletteForEnv resolves the palette from the environment.
func paletteForEnv() Palette {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SBT_THEME"))) {
	case "light", "white", "day":
		return LightPalette
	case "dark", "night":
		return DefaultPalette
	}
	if terminalIsLight() {
		return LightPalette
	}
	return DefaultPalette
}

// terminalIsLight makes a best-effort guess at whether the terminal paints a
// light background.
//
// It reads COLORFGBG rather than querying the terminal: an OSC 11 query needs a
// reply from a program that may not answer, and a session that blocks waiting
// for one is worse than one that guesses from a variable the user already set.
func terminalIsLight() bool {
	raw := os.Getenv("COLORFGBG")
	if raw == "" {
		return false
	}
	fields := strings.Split(raw, ";")
	if len(fields) < 2 {
		return false
	}
	// COLORFGBG is "<fg>;<bg>". A background of 7 or 15 is the light default.
	switch strings.TrimSpace(fields[1]) {
	case "7", "15":
		return true
	}
	return false
}

// SetLight switches the theme between the white and the dark ground.
//
// It is a swap rather than a re-detection: the user just asked for a change, so
// their choice outranks whatever the environment says.
func (t *Theme) SetLight(light bool) {
	if light {
		t.Palette = LightPalette
	} else {
		t.Palette = DefaultPalette
	}
	t.Light = light
}

// ToggleLight flips the theme and reports which ground is now in use.
func (t *Theme) ToggleLight() bool {
	t.SetLight(!t.Light)
	return t.Light
}

// Ground names the current theme for the interface and the help text.
func (t *Theme) Ground() string {
	if t != nil && t.Light {
		return "LIGHT"
	}
	return "DARK"
}

// MotionAllowed reports whether short animations may play. SBT honours
// NO_MOTION, SBT_NO_MOTION and SBT_REDUCED_MOTION so a user who cannot tolerate
// movement is not forced to watch it.
func MotionAllowed() bool {
	for _, k := range []string{"SBT_NO_MOTION", "SBT_REDUCED_MOTION", "NO_MOTION"} {
		if os.Getenv(k) != "" {
			return false
		}
	}
	return true
}

// SupportsUnicode reports whether the terminal can draw box glyphs.
func SupportsUnicode() bool {
	if os.Getenv("SBT_ASCII") != "" {
		return false
	}
	lang := strings.ToLower(os.Getenv("LC_ALL") + os.Getenv("LC_CTYPE") + os.Getenv("LANG"))
	if lang == "" {
		return true
	}
	return strings.Contains(lang, "utf")
}

// DetectDepth decides how much colour to use.
func DetectDepth() ColorDepth {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("SBT_NO_COLOR") != "" {
		return ColorNone
	}
	if os.Getenv("TERM") == "dumb" {
		return ColorNone
	}
	if os.Getenv("SBT_FORCE_TRUE_COLOR") != "" {
		return ColorTrue
	}
	if ct := os.Getenv("COLORTERM"); ct == "truecolor" || ct == "24bit" {
		return ColorTrue
	}
	switch {
	case strings.Contains(os.Getenv("TERM"), "256color"):
		return Color256
	case os.Getenv("TERM") != "":
		return Color16
	default:
		return ColorNone
	}
}

// Glyphs returns the glyph set matching the theme.
func (t *Theme) Glyphs() Glyphs {
	if t != nil && t.ASCII {
		return ASCIIGlyphs
	}
	return UnicodeGlyphs
}

// IsPlain reports whether the theme emits no escape sequences at all.
func (t *Theme) IsPlain() bool { return t == nil || t.Depth == ColorNone }

// Style renders the escape prefix for a style. It returns "" when colour is
// disabled, which keeps piped output and NO_COLOR sessions clean.
func (t *Theme) Style(s Style) string {
	if t.IsPlain() {
		return ""
	}
	var b strings.Builder
	b.WriteString("\x1b[0m")
	if s.Bold {
		b.WriteString("\x1b[1m")
	}
	if s.Dim {
		b.WriteString("\x1b[2m")
	}
	if s.Italic {
		b.WriteString("\x1b[3m")
	}
	if s.Under {
		b.WriteString("\x1b[4m")
	}
	if s.Reverse {
		b.WriteString("\x1b[7m")
	}
	if s.HasBg {
		b.WriteString(t.encode(t.Palette.Bg, true))
		b.WriteString(t.encode(s.Bg, false))
	}
	b.WriteString(t.encode(s.Fg, false))
	return b.String()
}

// Reset returns the sequence that clears all attributes.
func (t *Theme) Reset() string {
	if t.IsPlain() {
		return ""
	}
	return "\x1b[0m"
}

// encode writes one colour for the active depth.
func (t *Theme) encode(c RGB, background bool) string {
	prefix := "38"
	if background {
		prefix = "48"
	}
	switch t.Depth {
	case ColorTrue:
		return fmt.Sprintf("\x1b[%s;2;%d;%d;%dm", prefix, c.R, c.G, c.B)
	case Color256:
		return fmt.Sprintf("\x1b[%s;5;%dm", prefix, to256(c))
	case Color16:
		return fmt.Sprintf("\x1b[%dm", to16(c, background))
	default:
		return ""
	}
}

// Accent returns the cage accent colour for a state.
func (t *Theme) Accent(state CageState) RGB {
	switch state {
	case CageProtected:
		return t.Palette.Green
	case CageLow:
		return t.Palette.Yellow
	case CageHigh:
		return t.Palette.Red
	case CageLimited:
		return t.Palette.Yellow
	case CageBroken:
		return t.Palette.Red
	default:
		return t.Palette.Muted
	}
}

// to256 maps a colour onto the xterm 256 colour cube, using the grayscale ramp
// for neutrals because the cube has no useful dark greys.
func to256(c RGB) int {
	if maxDiff(c.R, c.G) < 12 && maxDiff(c.G, c.B) < 12 {
		if c.R < 8 {
			return 16
		}
		if c.R > 248 {
			return 231
		}
		return 232 + int(c.R-8)*24/247
	}
	level := func(v uint8) int { return int(v) * 5 / 255 }
	return 16 + 36*level(c.R) + 6*level(c.G) + level(c.B)
}

// to16 maps a colour onto the 16 ANSI colours by hue and brightness, which is
// all an 8/16 colour terminal can express.
func to16(c RGB, background bool) int {
	base := 30
	if background {
		base = 40
	}
	const (
		black, red, green, yellow, _, _, cyan, white = 0, 1, 2, 3, 4, 5, 6, 7
	)
	r, g, b := int(c.R), int(c.G), int(c.B)
	bright := r+g+b > 420
	idx := black
	switch {
	case r > 180 && g > 120 && b < 120:
		idx = yellow
	case r > 150 && g < 120 && b < 120:
		idx = red
	case g > 120 && r < 120:
		idx = green
	case r > 180 && g > 180 && b > 180:
		idx = white
	case b > r && b > g:
		idx = cyan
	}
	n := base + idx
	if bright && idx != black {
		n += 60
	}
	return n
}

func maxDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

// VisibleWidth returns the printable width of a string, ignoring escapes.
func VisibleWidth(s string) int {
	w, inEsc := 0, false
	for _, r := range s {
		switch {
		case inEsc:
			if r == 'm' {
				inEsc = false
			}
		case r == '\x1b':
			inEsc = true
		default:
			w++
		}
	}
	return w
}

// itoa is a tiny integer formatter used by the renderer.
func itoa(n int) string { return strconv.Itoa(n) }
