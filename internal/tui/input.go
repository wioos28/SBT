package tui

import (
	"strings"
	"time"
	"unicode/utf8"
)

// KeyType identifies a key that is not a plain character.
type KeyType int

// Key types.
const (
	KeyRune KeyType = iota
	KeyEnter
	KeyTab
	KeyBackTab
	KeyEsc
	KeyBackspace
	KeyDelete
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyPgUp
	KeyPgDn
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF10
	// KeyMouse carries a decoded mouse report in Mouse. It is one key type
	// rather than three so the reader stays a single stream of events and the
	// App loop keeps one wakeup path.
	KeyMouse
)

// Key is one decoded key press.
type Key struct {
	Type  KeyType
	Rune  rune
	Ctrl  bool
	Alt   bool
	Shift bool
	// Mouse is meaningful only when Type is KeyMouse.
	Mouse Mouse
}

// name is the short identifier of a key type, used by Name and the bindings.
func (t KeyType) name() string {
	switch t {
	case KeyRune:
		return ""
	case KeyEnter:
		return "enter"
	case KeyTab:
		return "tab"
	case KeyBackTab:
		return "shift+tab"
	case KeyEsc:
		return "esc"
	case KeyBackspace:
		return "backspace"
	case KeyDelete:
		return "delete"
	case KeyUp:
		return "up"
	case KeyDown:
		return "down"
	case KeyLeft:
		return "left"
	case KeyRight:
		return "right"
	case KeyHome:
		return "home"
	case KeyEnd:
		return "end"
	case KeyPgUp:
		return "pgup"
	case KeyPgDn:
		return "pgdn"
	case KeyF1:
		return "f1"
	case KeyF2:
		return "f2"
	case KeyF3:
		return "f3"
	case KeyF4:
		return "f4"
	case KeyF5:
		return "f5"
	case KeyF6:
		return "f6"
	case KeyF10:
		return "f10"
	case KeyMouse:
		return "mouse"
	default:
		return "?"
	}
}

// Name renders the key as it is shown in hints, e.g. "alt+2" or "ctrl+k".
func (k Key) Name() string {
	name := k.Type.name()
	if k.Type == KeyRune {
		name = string(k.Rune)
	}
	switch {
	case k.Ctrl && k.Alt:
		return "ctrl+alt+" + name
	case k.Ctrl:
		return "ctrl+" + name
	case k.Alt:
		return "alt+" + name
	default:
		return name
	}
}

// escDelay is how long the reader waits for the rest of an escape sequence
// before deciding the user pressed Escape on its own. It is short enough that
// Escape feels immediate and long enough for a real sequence to arrive.
const escDelay = 40 * time.Millisecond

// parseKey decodes one key from the front of seq. It returns the number of
// bytes consumed, or 0 when seq is an incomplete prefix that more input may
// still complete.
//
// The decoder is deliberately permissive about terminal dialects: arrows,
// Home/End/Page keys, Delete, function keys and the Alt+<key> form (ESC then a
// byte) all occur on real terminals.
func parseKey(seq []byte) (Key, int) {
	if len(seq) == 0 {
		return Key{}, 0
	}
	b := seq[0]
	switch {
	case b == 0x1b:
		if len(seq) == 1 {
			return Key{}, 0 // may be Escape, may be the start of a sequence
		}
		switch seq[1] {
		case '[':
			return parseCSI(seq)
		case 'O':
			return parseSS3(seq)
		default:
			r, size := utf8.DecodeRune(seq[1:])
			if r == utf8.RuneError && size <= 1 {
				return Key{}, 2
			}
			return Key{Type: KeyRune, Rune: r, Alt: true, Shift: isUpper(r)}, 1 + size
		}
	case b == '\r' || b == '\n':
		return Key{Type: KeyEnter}, 1
	case b == '\t':
		return Key{Type: KeyTab}, 1
	case b == 127:
		return Key{Type: KeyBackspace}, 1
	case b == 0:
		return Key{Type: KeyRune, Rune: ' ', Ctrl: true}, 1
	case b < 27:
		return Key{Type: KeyRune, Rune: rune('a' + b - 1), Ctrl: true}, 1
	case b < 32:
		return Key{}, 1
	default:
		r, size := utf8.DecodeRune(seq)
		if r == utf8.RuneError && size <= 1 {
			if len(seq) < utf8.UTFMax {
				return Key{}, 0
			}
			return Key{}, 1
		}
		return Key{Type: KeyRune, Rune: r, Shift: isUpper(r)}, size
	}
}

// parseCSI decodes an ESC [ ... sequence.
func parseCSI(seq []byte) (Key, int) {
	// Scan for the final byte, which is in 0x40..0x7E.
	for i := 2; i < len(seq); i++ {
		c := seq[i]
		switch {
		case c >= '0' && c <= '9', c == ';', c == '?', c == '<':
			continue
		case c >= 0x40 && c <= 0x7e:
			return csiFinal(c, string(seq[2:i]), i+1)
		default:
			// A control byte inside a sequence means the sequence is garbage;
			// consume what was seen and move on.
			return Key{}, 0
		}
	}
	return Key{}, 0
}

// csiFinal turns a CSI final byte and its parameters into a key.
func csiFinal(final byte, params string, consumed int) (Key, int) {
	ctrl, alt := modifiers(params)
	switch final {
	case 'A', 'B', 'C', 'D', 'H', 'F':
		var t KeyType
		switch final {
		case 'A':
			t = KeyUp
		case 'B':
			t = KeyDown
		case 'C':
			t = KeyRight
		case 'D':
			t = KeyLeft
		case 'H':
			t = KeyHome
		case 'F':
			t = KeyEnd
		}
		return Key{Type: t, Ctrl: ctrl, Alt: alt}, consumed
	case 'Z':
		return Key{Type: KeyBackTab}, consumed
	case 'P', 'Q', 'R', 'S':
		t := map[byte]KeyType{'P': KeyF1, 'Q': KeyF2, 'R': KeyF3, 'S': KeyF4}[final]
		return Key{Type: t, Ctrl: ctrl, Alt: alt}, consumed
	case '~':
		return parseTilde(params, consumed)
	case 'M', 'm':
		// A mouse report shares the CSI shape; the leading "<" is what tells
		// it apart from a cursor key. 'm' is the release form.
		return parseSGRMouse(params, final == 'm', consumed)
	default:
		return Key{}, consumed
	}
}

// Mouse is one decoded mouse event, in the same coordinate space as Buffer:
// X and Y are zero-based cell offsets, the origin at the top left.
type Mouse struct {
	// Button is the physical button or wheel notch.
	Button MouseButton
	// X and Y are the cell the pointer was at when the event happened.
	X, Y int
	// Ctrl, Alt and Shift are the modifier keys held during the event.
	Ctrl, Alt, Shift bool
	// Press reports a press or a release. Selection follows the press, so a
	// release is ignored; it is kept because the terminal reports both and a
	// drag needs the distinction.
	Press bool
}

// MouseButton identifies what produced a mouse event.
type MouseButton int

// The buttons and wheel notches SBT reacts to.
const (
	MouseNone MouseButton = iota
	MouseLeft
	MouseMiddle
	MouseRight
	MouseWheelUp
	MouseWheelDown
)

// parseSGRMouse decodes the SGR (mode 1006) mouse report, the form every current
// terminal sends: ESC [ < button ; column ; row (M for press, m for release).
//
// The older X10 form is deliberately not decoded. It caps coordinates at 223,
// which silently mis-targets clicks in the bottom right of a large terminal,
// and it cannot be disambiguated from a key press by looking at the bytes. A
// terminal that only speaks X10 therefore gets no click support rather than
// clicks that land on the wrong cell - the same fail-closed rule the rest of
// SBT follows.
func parseSGRMouse(params string, release bool, consumed int) (Key, int) {
	if !strings.HasPrefix(params, "<") {
		return Key{}, consumed
	}
	fields := splitParams(params[1:])
	if len(fields) != 3 {
		// A truncated report is not an event; dropping it keeps the reader
		// aligned on the next real key.
		return Key{}, consumed
	}
	code, ok := atoiOK(fields[0])
	if !ok {
		return Key{}, consumed
	}
	col, ok1 := atoiOK(fields[1])
	row, ok2 := atoiOK(fields[2])
	if !ok1 || !ok2 {
		return Key{}, consumed
	}
	btn := MouseNone
	switch code & 3 {
	case 0:
		btn = MouseLeft
	case 1:
		btn = MouseMiddle
	case 2:
		btn = MouseRight
	}
	// The wheel is reported above the button bits and the modifier bits, so a
	// wheel notch survives a Ctrl+wheel report. Wheel down is 65 rather than
	// 128: the two directions are consecutive, not a bit apart, so the button
	// bits below the wheel bit are what tell them apart.
	switch {
	case code&64 != 0:
		btn = MouseWheelUp
		if code&3 != 0 {
			btn = MouseWheelDown
		}
	case code&128 != 0:
		btn = MouseWheelDown
	}
	// Coordinates are one-based on the wire and zero-based in the buffer.
	return Key{Type: KeyMouse, Mouse: Mouse{
		Button: btn,
		X:      max(col-1, 0),
		Y:      max(row-1, 0),
		Ctrl:   code&16 != 0,
		Alt:    code&8 != 0,
		Shift:  code&4 != 0,
		Press:  !release,
	}}, consumed
}

// parseTilde decodes the ESC [ <n> ~ family, including the modifyOtherKeys form
// ESC [ 27 ; <mod> ; <code> ~ which some terminals send for Ctrl+<digit>.
func parseTilde(params string, consumed int) (Key, int) {
	fields := splitParams(params)
	if len(fields) >= 3 && fields[0] == "27" {
		mod, _ := atoiOK(fields[1])
		code, ok := atoiOK(fields[2])
		if !ok {
			return Key{}, consumed
		}
		ctrl, alt := modifierFlags(mod)
		return Key{Type: KeyRune, Rune: rune(code), Ctrl: ctrl, Alt: alt}, consumed
	}
	if len(fields) == 0 {
		return Key{}, consumed
	}
	n, _ := atoiOK(fields[0])
	var t KeyType
	switch n {
	case 1, 2:
		t = KeyHome
	case 3:
		t = KeyDelete
	case 4:
		t = KeyEnd
	case 5:
		t = KeyPgUp
	case 6:
		t = KeyPgDn
	case 15:
		t = KeyF5
	case 17:
		t = KeyF6
	case 21:
		t = KeyF10
	default:
		return Key{}, consumed
	}
	key := Key{Type: t}
	if len(fields) > 1 {
		if mod, ok := atoiOK(fields[1]); ok {
			key.Ctrl, key.Alt = modifierFlags(mod)
		}
	}
	return key, consumed
}

// parseSS3 decodes an ESC O ... sequence, the "application cursor" dialect.
func parseSS3(seq []byte) (Key, int) {
	if len(seq) < 3 {
		return Key{}, 0
	}
	switch seq[2] {
	case 'A':
		return Key{Type: KeyUp}, 3
	case 'B':
		return Key{Type: KeyDown}, 3
	case 'C':
		return Key{Type: KeyRight}, 3
	case 'D':
		return Key{Type: KeyLeft}, 3
	case 'H':
		return Key{Type: KeyHome}, 3
	case 'F':
		return Key{Type: KeyEnd}, 3
	case 'P':
		return Key{Type: KeyF1}, 3
	case 'Q':
		return Key{Type: KeyF2}, 3
	case 'R':
		return Key{Type: KeyF3}, 3
	case 'S':
		return Key{Type: KeyF4}, 3
	default:
		return Key{}, 3
	}
}

// modifiers decodes the modifier parameter of a CSI sequence ("1;5A" is Ctrl).
func modifiers(params string) (ctrl, alt bool) {
	fields := splitParams(params)
	if len(fields) < 2 {
		return false, false
	}
	mod, ok := atoiOK(fields[1])
	if !ok {
		return false, false
	}
	return modifierFlags(mod)
}

// modifierFlags decodes an xterm modifier code into its flags.
func modifierFlags(mod int) (ctrl, alt bool) {
	if mod < 1 {
		return false, false
	}
	mod--
	return mod&4 != 0, mod&2 != 0
}

func splitParams(s string) []string {
	if s == "" {
		return nil
	}
	out := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ';' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func atoiOK(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, true
}

func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }
