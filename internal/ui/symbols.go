package ui

import "strings"

// Symbols is the glyph set used by the renderer.
type Symbols struct {
	OK          string
	Fail        string
	Warn        string
	Dot         string
	Arrow       string
	Added       string
	Modified    string
	Deleted     string
	Warning     string
	Selected    string
	Unselected  string
	Bullet      string
	BlockFull   string
	BlockEmpty  string
	HLine       string
	VLine       string
	CornerTL    string
	CornerTR    string
	CornerBL    string
	CornerBR    string
	TeeLeft     string
	TeeRight    string
	UnicodeFlag bool
}

// Unicode is the default glyph set.
var Unicode = Symbols{
	OK: "\u2713", Fail: "\u2717", Warn: "\u26a0", Dot: "\u25cf", Arrow: "\u2192",
	Added: "+", Modified: "~", Deleted: "-", Warning: "!", Selected: "\u2713",
	Unselected: "\u2717", Bullet: "\u2022",
	BlockFull: "\u2588", BlockEmpty: "\u2591",
	HLine: "\u2500", VLine: "\u2502",
	CornerTL: "\u250c", CornerTR: "\u2510", CornerBL: "\u2514", CornerBR: "\u2518",
	TeeLeft: "\u251c", TeeRight: "\u2524", UnicodeFlag: true,
}

// ASCII is the fallback glyph set for terminals without Unicode support.
var ASCII = Symbols{
	OK: "OK", Fail: "x", Warn: "!", Dot: "*", Arrow: "->",
	Added: "+", Modified: "~", Deleted: "-", Warning: "!", Selected: "x",
	Unselected: " ", Bullet: "*",
	BlockFull: "#", BlockEmpty: ".",
	HLine: "-", VLine: "|",
	CornerTL: "+", CornerTR: "+", CornerBL: "+", CornerBR: "+",
	TeeLeft: "+", TeeRight: "+", UnicodeFlag: false,
}

// CapLevel is the capability level of an isolation feature. It mirrors
// platform.Level, kept local so that ui never imports platform.
type CapLevel int

// Capability levels.
const (
	CapFull        CapLevel = iota // fully enforced on this platform
	CapPartial                     // enforced with documented restrictions
	CapUnavailable                 // unavailable; SBT never fakes this
)

// Label renders a capability level as a human readable word.
func (l CapLevel) Label() string {
	switch l {
	case CapFull:
		return "Full"
	case CapPartial:
		return "Limited"
	default:
		return "Unavailable"
	}
}

// LevelToCap converts a capability level to a display level.
func LevelToCap(l CapLevel) Level {
	switch l {
	case CapFull:
		return LevelOK
	case CapPartial:
		return LevelWarn
	default:
		return LevelFail
	}
}

// ColorizeLevel colours s consistently with the capability level.
func (t *Theme) ColorizeLevel(level CapLevel, s string) string {
	return t.Leveled(LevelToCap(level), s)
}

// Glyph renders the status glyph for a capability level, coloured with the
// same mapping used everywhere else (green/yellow/red).
func (t *Theme) Glyph(level CapLevel) string {
	sym := t.Symbols()
	if level == CapFull {
		return t.ColorizeLevel(level, sym.OK)
	}
	if level == CapPartial {
		return t.ColorizeLevel(level, sym.Warn)
	}
	return t.ColorizeLevel(level, sym.Fail)
}

// Mark renders a capability level as a coloured glyph plus its label.
func (t *Theme) Mark(level CapLevel) string {
	sym := t.Symbols()
	if level == CapFull {
		return t.ColorizeLevel(level, sym.OK)
	}
	if level == CapPartial {
		return t.ColorizeLevel(level, sym.Warn)
	}
	return t.ColorizeLevel(level, sym.Fail)
}

// CapBar renders a ten cell capability bar, e.g. "[####------]".
func (t *Theme) CapBar(level CapLevel) string {
	sym := t.Symbols()
	full, empty := 10, 0
	switch level {
	case CapPartial:
		full, empty = 6, 4
	case CapUnavailable:
		full, empty = 4, 6
	}
	bar := strings.Repeat(sym.BlockFull, full) + strings.Repeat(sym.BlockEmpty, empty)
	return t.ColorizeLevel(level, bar)
}

// Bar renders a labelled progress bar with a colour level.
func (t *Theme) Bar(label string, value, max float64, level Level, suffix string, width int) string {
	if width < 10 {
		width = 10
	}
	ratio := 0.0
	if max > 0 {
		ratio = value / max
	}
	if ratio > 1 {
		ratio = 1
	}
	if ratio < 0 {
		ratio = 0
	}
	sym := t.Symbols()
	filled := int(ratio*float64(width) + 0.5)
	bar := strings.Repeat(sym.BlockFull, filled) + strings.Repeat(sym.BlockEmpty, width-filled)
	line := t.Gray(Pad(label, 10)) + t.Leveled(level, bar)
	if suffix != "" {
		line += "  " + t.Text(suffix)
	}
	return line
}
