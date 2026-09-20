package ui

import "strings"

// Box renders a bordered panel matching the SBT status dashboard style.
//
//	+------------------------------------------+
//	| SBT STATUS                               |
//	+------------------------------------------+
//	| Mode           LOW                       |
//	+------------------------------------------+
func (t *Theme) Box(title string, lines []string, level Level, width int) string {
	sym := t.Symbols()
	if width < 24 {
		width = 24
	}
	inner := width - 2
	var b strings.Builder
	top := sym.CornerTL + strings.Repeat(sym.HLine, inner) + sym.CornerTR
	b.WriteString(t.Leveled(level, top) + "\n")
	if title != "" {
		b.WriteString(t.Leveled(level, sym.VLine) + " " + Pad(Truncate(title, inner-2), inner-2) + " " + t.Leveled(level, sym.VLine) + "\n")
		sep := sym.TeeLeft + strings.Repeat(sym.HLine, inner) + sym.TeeRight
		b.WriteString(t.Leveled(level, sep) + "\n")
	}
	for _, ln := range lines {
		b.WriteString(t.Leveled(level, sym.VLine) + " " + Pad(Truncate(ln, inner-2), inner-2) + " " + t.Leveled(level, sym.VLine) + "\n")
	}
	bottom := sym.CornerBL + strings.Repeat(sym.HLine, inner) + sym.CornerBR
	b.WriteString(t.Leveled(level, bottom) + "\n")
	return b.String()
}

// BoxString renders a box as a single string (no trailing newline).
func (t *Theme) BoxString(title string, lines []string, level Level, width int) string {
	return strings.TrimRight(t.Box(title, lines, level, width), "\n")
}

// ModeName is one of the three SBT security modes.
type ModeName string

// Security modes.
const (
	ModeNormal ModeName = "NORMAL"
	ModeLow    ModeName = "LOW"
	ModeDanger ModeName = "DANGER"
)

// ModeLevel maps a security mode to its display level.
func ModeLevel(m ModeName) Level {
	switch m {
	case ModeLow:
		return LevelLow
	case ModeDanger:
		return LevelDanger
	default:
		return LevelInfo
	}
}

// ModeBanner renders the top mode banner used in sandbox sessions.
func (t *Theme) ModeBanner(m ModeName, right string, width int) string {
	label := "SBT " + string(m) + " MODE"
	if m == ModeDanger {
		label = "SBT HIGH RISK MODE"
	}
	line := label
	if right != "" {
		gap := width - VisibleWidth(label) - VisibleWidth(right) - 2
		if gap < 1 {
			gap = 1
		}
		line = label + strings.Repeat(" ", gap) + right
	}
	return t.Leveled(ModeLevel(m), line)
}

// StatusLine renders a one line status bar for the frame footer.
func (t *Theme) StatusLine(m ModeName, items []string, width int) string {
	left := " SBT " + string(m) + " "
	right := strings.Join(items, t.Gray(" | ")) + " "
	gap := width - VisibleWidth(left) - VisibleWidth(right)
	if gap < 0 {
		gap = 0
		right = Truncate(right, width-VisibleWidth(left))
		gap = width - VisibleWidth(left) - VisibleWidth(right)
		if gap < 0 {
			gap = 0
		}
	}
	bar := left + strings.Repeat(" ", gap) + right
	// Pad to the full width so the background of the footer stays uniform.
	if pad := width - VisibleWidth(bar); pad > 0 {
		bar += strings.Repeat(" ", pad)
	}
	return t.Leveled(ModeLevel(m), bar)
}

// Section renders a titled section header, e.g. "FILES" or "RESOURCES".
func (t *Theme) Section(title string) string {
	return t.Bold(t.Gray(strings.ToUpper(title)))
}

// KeyValues renders aligned "Label   Value" rows with a fixed label column.
func KeyValues(rows [][2]string) []string {
	labelWidth := 0
	for _, r := range rows {
		if len(r[0]) > labelWidth {
			labelWidth = len(r[0])
		}
	}
	if labelWidth > 18 {
		labelWidth = 18
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, Pad(r[0], labelWidth+2)+r[1])
	}
	return out
}
