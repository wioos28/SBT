package ui

import (
	"errors"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

// Editor is a small line editor used by the sandbox shell. It provides command
// history and cursor movement, and degrades to buffered line input when the
// terminal cannot be switched to raw mode.
type Editor struct {
	In      *os.File
	Out     *os.File
	Theme   *Theme
	History []string
	// MaxHistory bounds the retained history entries.
	MaxHistory int
}

// NewEditor creates an editor bound to a terminal pair.
func NewEditor(theme *Theme, in, out *os.File) *Editor {
	return &Editor{In: in, Out: out, Theme: theme, MaxHistory: 200}
}

// ReadLine prints prompt and returns the edited line.
//
// A returned ui.ErrInterrupted means Ctrl-C, io.EOF means Ctrl-D on an empty
// line; both are handled by the shell as "abandon/exit" without killing SBT.
func (e *Editor) ReadLine(prompt string) (string, error) {
	if !IsTerminal(e.In) || !IsTerminal(e.Out) {
		line, err := readLinePlain(e.In, prompt)
		e.remember(line)
		return line, err
	}
	restore, err := RawMode(e.In)
	if err != nil {
		line, lerr := readLinePlain(e.In, prompt)
		e.remember(line)
		return line, lerr
	}
	defer restore()

	buf := []rune{}
	cursor := 0
	histIdx := len(e.History)
	draft := ""
	render := func() {
		s := string(buf)
		e.Out.WriteString("\r\x1b[2K" + prompt + s)
		if back := len(buf) - cursor; back > 0 {
			e.Out.WriteString("\x1b[" + itoa(back) + "D")
		}
	}
	render()

	readByte := func() (byte, error) {
		var b [1]byte
		n, rerr := e.In.Read(b[:])
		if rerr != nil {
			return 0, rerr
		}
		if n == 0 {
			return 0, nil
		}
		return b[0], nil
	}

	for {
		c, rerr := readByte()
		if rerr != nil {
			e.Out.WriteString("\n")
			return "", rerr
		}
		switch c {
		case '\r', '\n':
			e.Out.WriteString("\n")
			line := string(buf)
			e.remember(line)
			return line, nil
		case 3: // Ctrl-C
			e.Out.WriteString("^C\n")
			return "", ErrInterrupted
		case 4: // Ctrl-D
			if len(buf) == 0 {
				e.Out.WriteString("\n")
				return "", io.EOF
			}
		case 127, 8: // backspace
			if cursor > 0 {
				buf = append(buf[:cursor-1], buf[cursor:]...)
				cursor--
			}
			render()
		case 21: // Ctrl-U
			buf = buf[:0]
			cursor = 0
			render()
		case 11: // Ctrl-K
			buf = buf[:cursor]
			render()
		case 1: // Ctrl-A
			cursor = 0
			render()
		case 5: // Ctrl-E
			cursor = len(buf)
			render()
		case 12: // Ctrl-L
			e.Out.WriteString("\x1b[2J\x1b[H")
			render()
		case 23: // Ctrl-W
			for cursor > 0 && buf[cursor-1] == ' ' {
				buf = append(buf[:cursor-1], buf[cursor:]...)
				cursor--
			}
			for cursor > 0 && buf[cursor-1] != ' ' {
				buf = append(buf[:cursor-1], buf[cursor:]...)
				cursor--
			}
			render()
		case 27: // escape sequence
			var seq [2]byte
			for i := range seq {
				b, serr := readByte()
				if serr != nil {
					break
				}
				seq[i] = b
			}
			if seq[0] != '[' {
				if seq[0] == 0 {
					continue
				}
				break
			}
			switch seq[1] {
			case 'A': // up
				if len(e.History) == 0 {
					break
				}
				if histIdx == len(e.History) {
					draft = string(buf)
				}
				if histIdx > 0 {
					histIdx--
				}
				buf = []rune(e.History[histIdx])
				cursor = len(buf)
				render()
			case 'B': // down
				if histIdx < len(e.History) {
					histIdx++
				}
				if histIdx >= len(e.History) {
					buf = []rune(draft)
				} else {
					buf = []rune(e.History[histIdx])
				}
				cursor = len(buf)
				render()
			case 'C': // right
				if cursor < len(buf) {
					cursor++
					e.Out.WriteString("\x1b[C")
				}
			case 'D': // left
				if cursor > 0 {
					cursor--
					e.Out.WriteString("\x1b[D")
				}
			case 'H':
				cursor = 0
				render()
			case 'F':
				cursor = len(buf)
				render()
			}
		default:
			if c < 32 {
				continue
			}
			r, size := utf8.DecodeRune([]byte{c})
			if r == utf8.RuneError && size == 1 && c >= 0x80 {
				// Collect the remaining bytes of a multi-byte rune.
				extra := multiByteLen(c) - 1
				raw := []byte{c}
				for i := 0; i < extra; i++ {
					b, berr := readByte()
					if berr != nil {
						break
					}
					raw = append(raw, b)
				}
				r, _ = utf8.DecodeRune(raw)
			}
			buf = append(buf[:cursor], append([]rune{r}, buf[cursor:]...)...)
			cursor++
			render()
		}
	}
}

func (e *Editor) remember(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	if n := len(e.History); n > 0 && e.History[n-1] == line {
		return
	}
	e.History = append(e.History, line)
	if e.MaxHistory > 0 && len(e.History) > e.MaxHistory {
		e.History = e.History[len(e.History)-e.MaxHistory:]
	}
}

func multiByteLen(b byte) int {
	switch {
	case b >= 0xF0:
		return 4
	case b >= 0xE0:
		return 3
	case b >= 0xC0:
		return 2
	default:
		return 1
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// ErrEOF is returned when stdin is closed during a prompt.
var ErrEOF = errors.New("end of input")
