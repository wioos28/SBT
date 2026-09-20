package ui

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// IsTerminal reports whether f refers to a character device (a terminal).
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

// TerminalSize returns the terminal width and height for f, falling back to
// COLUMNS/LINES and finally to 100x30 when the size cannot be queried.
func TerminalSize(f *os.File) (int, int) {
	w, h, ok := terminalSize(f)
	if ok && w > 0 && h > 0 {
		return w, h
	}
	return envSize()
}

func envSize() (int, int) {
	w, h := 100, 30
	if v, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && v > 20 {
		w = v
	}
	if v, err := strconv.Atoi(os.Getenv("LINES")); err == nil && v > 5 {
		h = v
	}
	return w, h
}

// TermiosState holds a saved terminal state for restore.
type TermiosState struct {
	opaque any
}

// RawMode switches the terminal to raw mode and returns a restore function.
// On platforms where raw mode is unavailable the returned restore function is a
// no-op and the error is non-nil, letting callers fall back to line input.
func RawMode(f *os.File) (restore func(), err error) {
	st, err := makeRaw(f)
	if err != nil {
		return func() {}, err
	}
	return func() { _ = restoreTermios(f, st) }, nil
}

// ErrInterrupted is returned when the user cancels input (Ctrl-C / Ctrl-D).
var ErrInterrupted error = errInterrupted

var errInterrupted = &interruptError{}

type interruptError struct{}

func (*interruptError) Error() string { return "interrupted" }

// ReadLineNoEcho reads a single line from f without echoing it when possible.
func ReadLineNoEcho(f *os.File, prompt string) (string, error) {
	os.Stdout.WriteString(prompt)
	defer os.Stdout.WriteString("\n")
	if restore, err := RawMode(f); err == nil {
		defer restore()
	}
	var sb strings.Builder
	buf := make([]byte, 1)
	for {
		n, rerr := f.Read(buf)
		if rerr != nil {
			if sb.Len() > 0 {
				return sb.String(), nil
			}
			return "", rerr
		}
		if n == 0 {
			continue
		}
		switch buf[0] {
		case '\r', '\n':
			return sb.String(), nil
		case 3, 4: // Ctrl-C, Ctrl-D
			return "", ErrInterrupted
		case 127, 8:
			s := sb.String()
			if len(s) > 0 {
				sb.Reset()
				sb.WriteString(s[:len(s)-1])
			}
		default:
			sb.WriteByte(buf[0])
		}
	}
}

// readLinePlain is an internal alias used by the editor fallback path.
func readLinePlain(f *os.File, prompt string) (string, error) { return ReadLinePlain(f, prompt) }

// ReadLinePlain reads a line from f using buffered line input.
func ReadLinePlain(f *os.File, prompt string) (string, error) {
	os.Stdout.WriteString(prompt)
	r := bufio.NewReader(f)
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
