package tui

import "strconv"

// Palette presets.
//
// A preset is a complete Palette, not an accent tint. The dark ground and the
// three severity colours were tuned together, and recolouring the ground
// without re-checking that PROTECTED still reads green against it is how a
// security interface starts lying to the eye. So each preset restates the
// ground it was chosen for; only the brand accents move.
//
// Every preset keeps the severity colours distinct. That constraint is what
// makes the interface readable without colour, and no amount of branding is
// worth breaking it.

// CyberPalette is the SBT brand pushed to neon: a near-black blue ground with a
// hot primary and a cyan information accent.
var CyberPalette = Palette{
	Name:      "cyber",
	Bg:        Hex(0x05070E),
	BgTop:     Hex(0x0B1024),
	BgBottom:  Hex(0x070A16),
	Surface:   Hex(0x0A0F1E),
	Surface2:  Hex(0x101830),
	Yellow:    Hex(0xFF2E63),
	YellowHi:  Hex(0xD81E4F),
	Green:     Hex(0x22FF9C),
	Red:       Hex(0xFF4D6D),
	Text:      Hex(0xEAF6FF),
	Fg:        Hex(0xEAF6FF),
	Muted:     Hex(0x7E8BA6),
	Border:    Hex(0x24314F),
	Warning:   Hex(0xFFC24B),
	Primary:   Hex(0xFF2E63),
	Secondary: Hex(0xFF6E93),
	Info:      Hex(0x00F0FF),
}

// MinimalPalette is the interface stripped to a warm neutral: the brand accent
// is a soft sand, the surfaces barely differ from the ground, and nothing
// competes with the text.
var MinimalPalette = Palette{
	Name:      "minimal",
	Bg:        Hex(0x14130F),
	BgTop:     Hex(0x1B1913),
	BgBottom:  Hex(0x171510),
	Surface:   Hex(0x191813),
	Surface2:  Hex(0x201E18),
	Yellow:    Hex(0xD6C7A1),
	YellowHi:  Hex(0xB8A87F),
	Green:     Hex(0x7FB08A),
	Red:       Hex(0xD98B8B),
	Text:      Hex(0xF5F1E8),
	Fg:        Hex(0xF5F1E8),
	Muted:     Hex(0x9A9384),
	Border:    Hex(0x37342C),
	Warning:   Hex(0xD8A657),
	Primary:   Hex(0xD6C7A1),
	Secondary: Hex(0xE6DCC2),
	Info:      Hex(0x9DBFC4),
}

// OceanPalette is a cool blue-green ground with a bright teal accent, chosen so
// long text stays easy on the eye while the accent still reads as live.
var OceanPalette = Palette{
	Name:      "ocean",
	Bg:        Hex(0x061018),
	BgTop:     Hex(0x0B1B28),
	BgBottom:  Hex(0x081420),
	Surface:   Hex(0x0A1622),
	Surface2:  Hex(0x0F2030),
	Yellow:    Hex(0x38BDF8),
	YellowHi:  Hex(0x0EA5E9),
	Green:     Hex(0x2DD4BF),
	Red:       Hex(0xFF7A85),
	Text:      Hex(0xEAF6FB),
	Fg:        Hex(0xEAF6FB),
	Muted:     Hex(0x7C97A8),
	Border:    Hex(0x1E3A4C),
	Warning:   Hex(0xF5C15B),
	Primary:   Hex(0x38BDF8),
	Secondary: Hex(0x7DD3FC),
	Info:      Hex(0x22D3EE),
}

// MonoPalette is deliberately colourless: greys only, so the interface is usable
// on a monochrome display and honest about relying on words, not colour, for
// meaning.
var MonoPalette = Palette{
	Name:      "mono",
	Bg:        Hex(0x0C0C0C),
	BgTop:     Hex(0x161616),
	BgBottom:  Hex(0x111111),
	Surface:   Hex(0x141414),
	Surface2:  Hex(0x1C1C1C),
	Yellow:    Hex(0xD0D0D0),
	YellowHi:  Hex(0xF0F0F0),
	Green:     Hex(0xE8E8E8),
	Red:       Hex(0x9A9A9A),
	Text:      Hex(0xFFFFFF),
	Fg:        Hex(0xFFFFFF),
	Muted:     Hex(0x8C8C8C),
	Border:    Hex(0x3A3A3A),
	Warning:   Hex(0xBDBDBD),
	Primary:   Hex(0xD0D0D0),
	Secondary: Hex(0xF0F0F0),
	Info:      Hex(0xB0B0B0),
}

// PalettePresetNames lists the dark-ground presets in display order.
func PalettePresetNames() []string {
	return []string{"sbt", "cyber", "minimal", "ocean", "mono"}
}

// PalettePreset resolves a preset name to its palette.
func PalettePreset(name string) (Palette, bool) {
	switch name {
	case "", "sbt", "orange":
		return DefaultPalette, true
	case "cyber":
		return CyberPalette, true
	case "minimal":
		return MinimalPalette, true
	case "ocean":
		return OceanPalette, true
	case "mono", "monochrome":
		return MonoPalette, true
	case "light":
		return LightPalette, true
	}
	return DefaultPalette, false
}

// darkPaletteFor is PalettePreset for the dark ground: it never returns the
// light palette, because the caller asked for a dark-ground preset.
func darkPaletteFor(name string) Palette {
	p, _ := PalettePreset(name)
	if p == LightPalette {
		return DefaultPalette
	}
	return p
}

// ParseHex decodes a "#RRGGBB" or "RRGGBB" colour. A value that is not a valid
// hex colour is reported as such so the Settings UI can refuse it rather than
// silently drawing the wrong thing.
func ParseHex(s string) (RGB, bool) {
	if s == "" {
		return RGB{}, false
	}
	if s[0] == '#' {
		s = s[1:]
	}
	if len(s) != 6 {
		return RGB{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return RGB{}, false
	}
	return Hex(uint32(v)), true
}

// HexString renders a colour as "#RRGGBB", the inverse of ParseHex.
func HexString(c RGB) string {
	const digits = "0123456789ABCDEF"
	buf := []byte{'#', 0, 0, 0, 0, 0, 0}
	for i, v := range []uint8{c.R, c.G, c.B} {
		buf[1+i*2] = digits[v>>4]
		buf[2+i*2] = digits[v&0x0F]
	}
	return string(buf)
}
