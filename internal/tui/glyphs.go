package tui

// Glyphs is the glyph set the renderer draws with. Two sets exist: the Unicode
// one used by every modern terminal and an ASCII fallback for terminals whose
// encoding cannot be trusted (SBT_ASCII, non-UTF-8 locales).
type Glyphs struct {
	// Single line borders.
	TL, TR, BL, BR, H, V string
	// Heavy borders used by the cage.
	HTL, HTR, HBL, HBR, HH, HV string
	// Tees for panels joined to the frame.
	LeftTee, RightTee, TopTee, BottomTee, Cross string
	// Status marks. Every one of these is paired with a word in the UI, so the
	// glyph is a hint and never the only signal.
	Dot, Ring, Check, Cross_, Warn, Arrow, Bullet, Caret, Pipe string
	// Meters.
	BlockFull, BlockEmpty string
	// Sparkline levels from empty to full.
	Spark []string
	// File marks.
	File, Dir, Exec, Link string
}

// UnicodeGlyphs is the default glyph set.
var UnicodeGlyphs = Glyphs{
	TL: "┌", TR: "┐", BL: "└", BR: "┘", H: "─", V: "│",
	HTL: "┏", HTR: "┓", HBL: "┗", HBR: "┛", HH: "━", HV: "┃",
	LeftTee: "├", RightTee: "┤", TopTee: "┬", BottomTee: "┴", Cross: "┼",
	Dot: "●", Ring: "○", Check: "✓", Cross_: "✗", Warn: "!", Arrow: "→",
	Bullet: "•", Caret: "▸", Pipe: "│",
	BlockFull: "█", BlockEmpty: "░",
	Spark: []string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"},
	File: "·", Dir: "/", Exec: "*", Link: "@",
}

// ASCIIGlyphs is the fallback used when the terminal cannot draw the Unicode
// set.
var ASCIIGlyphs = Glyphs{
	TL: "+", TR: "+", BL: "+", BR: "+", H: "-", V: "|",
	HTL: "+", HTR: "+", HBL: "+", HBR: "+", HH: "=", HV: "|",
	LeftTee: "+", RightTee: "+", TopTee: "+", BottomTee: "+", Cross: "+",
	Dot: "*", Ring: "o", Check: "v", Cross_: "x", Warn: "!", Arrow: "->",
	Bullet: "*", Caret: ">", Pipe: "|",
	BlockFull: "#", BlockEmpty: ".",
	Spark: []string{".", ":", "-", "=", "+", "*", "#", "@"},
	File: " ", Dir: "/", Exec: "*", Link: "@",
}
