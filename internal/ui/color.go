// Package ui renders the SBT terminal interface.
//
// Colour is meaningful, never decorative:
//
//	yellow (#F5C518) = LOW / sandbox mode
//	green  (#22C55E) = protected under the current policy / success
//	red    (#E53935) = elevated risk / danger mode
//	grey               = neutral information
//
// Colour is disabled automatically when the output is not a terminal, when
// NO_COLOR is set, or when TERM=dumb.
package ui

import (
	"os"
	"strings"
)

// Semantic colour codes (24-bit truecolor).
const (
	codeReset  = "\x1b[0m"
	codeBold   = "\x1b[1m"
	codeDim    = "\x1b[2m"
	codeYellow = "\x1b[38;2;245;197;24m"
	codeRed    = "\x1b[38;2;229;57;53m"
	codeGreen  = "\x1b[38;2;34;197;94m"
	codeText   = "\x1b[38;2;229;231;235m"
	codeGray   = "\x1b[38;2;148;163;184m"
	codeCyan   = "\x1b[38;2;56;189;248m"
)

// Theme renders coloured text. All methods are safe on a zero-value Theme
// (they render plain text), so callers never need to nil-check.
type Theme struct {
	// Color enables ANSI colour output.
	Color bool
	// ASCII disables Unicode glyphs (boxes, bullets).
	ASCII bool
}

// NewTheme builds a theme for the given writer.
func NewTheme(w *os.File) *Theme {
	return &Theme{Color: SupportsColor(w), ASCII: !SupportsUnicode()}
}

// Plain returns a theme that never emits escape sequences.
func Plain() *Theme { return &Theme{Color: false, ASCII: true} }

func (t *Theme) wrap(codes string, s string) string {
	if t == nil || !t.Color || s == "" {
		return s
	}
	return codes + s + codeReset
}

// Yellow marks LOW mode / sandbox identity.
func (t *Theme) Yellow(s string) string { return t.wrap(codeYellow, s) }

// Red marks elevated risk.
func (t *Theme) Red(s string) string { return t.wrap(codeRed, s) }

// Green marks "protected under the current policy" and successful operations.
func (t *Theme) Green(s string) string { return t.wrap(codeGreen, s) }

// Gray is neutral information.
func (t *Theme) Gray(s string) string { return t.wrap(codeGray, s) }

// Text is the default foreground.
func (t *Theme) Text(s string) string { return t.wrap(codeText, s) }

// Cyan is used for secondary highlights (labels in dashboards).
func (t *Theme) Cyan(s string) string { return t.wrap(codeCyan, s) }

// Bold is emphasis.
func (t *Theme) Bold(s string) string { return t.wrap(codeBold, s) }

// Dim is de-emphasis.
func (t *Theme) Dim(s string) string { return t.wrap(codeDim, s) }

// SupportsColor reports whether ANSI colour should be used for w.
func SupportsColor(w *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	if os.Getenv("SBT_FORCE_COLOR") != "" {
		return true
	}
	return IsTerminal(w)
}

// SupportsUnicode reports whether box drawing glyphs are safe to print.
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

// Symbols returns the glyph set matching the theme.
func (t *Theme) Symbols() Symbols {
	if t != nil && t.ASCII {
		return ASCII
	}
	return Unicode
}

// Status glyphs (fallback ASCII equivalents are in Symbols).
const (
	GlyphOK    = "\u2713" // check
	GlyphFail  = "\u2717" // ballot x
	GlyphWarn  = "\u26a0" // warning sign
	GlyphDot   = "\u25cf" // black circle
	GlyphArrow = "\u2192" // right arrow
)

// Level describes the severity of a line of user-facing output.
type Level int

// Output levels.
const (
	LevelInfo Level = iota
	LevelOK
	LevelWarn
	LevelFail
	LevelLow
	LevelDanger
)

// Leveled colourises s for the given severity.
func (t *Theme) Leveled(level Level, s string) string {
	switch level {
	case LevelOK:
		return t.Green(s)
	case LevelWarn:
		return t.Yellow(s)
	case LevelFail:
		return t.Red(s)
	case LevelLow:
		return t.Yellow(s)
	case LevelDanger:
		return t.Red(s)
	default:
		return t.Text(s)
	}
}

// Pad right-pads s to width, accounting for ANSI escape sequences.
func Pad(s string, width int) string {
	w := VisibleWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// VisibleWidth returns the printable width of s, ignoring ANSI escapes.
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

// Truncate shortens s to at most width visible cells.
func Truncate(s string, width int) string {
	if width <= 1 {
		return ""
	}
	if VisibleWidth(s) <= width {
		return s
	}
	runes := []rune(s)
	cut := []rune{}
	width2 := width - 1
	for _, r := range runes {
		if len(cut) >= width2 {
			break
		}
		cut = append(cut, r)
	}
	return string(cut) + "\u2026"
}
