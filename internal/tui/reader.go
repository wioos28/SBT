package tui

import (
	"errors"
	"io"
	"os"
)

// KeyReader turns terminal bytes into key events.
type KeyReader struct {
	in  *os.File
	buf []byte
}

// NewKeyReader binds a reader to a terminal input file.
func NewKeyReader(in *os.File) *KeyReader { return &KeyReader{in: in} }

// ErrClosed is returned once the input stream is gone.
var ErrClosed = errors.New("input closed")

// ReadKey blocks until one key is available.
//
// Escape is the only ambiguous key: on its own it is the Escape key, followed
// by a byte it starts an escape sequence. The reader waits briefly for the rest
// of a sequence and, when nothing arrives, reports Escape - which is why SBT
// can bind both "Esc closes the dialog" and "Alt+1 switches view".
func (k *KeyReader) ReadKey() (Key, error) {
	for {
		if len(k.buf) > 0 {
			if key, n := parseKey(k.buf); n > 0 {
				k.buf = k.buf[n:]
				return key, nil
			}
			if k.buf[0] == 0x1b && !waitReadable(k.in, escDelay) {
				// Nothing followed: it was Escape, and whatever partial
				// sequence was buffered is garbage the user never typed.
				k.buf = k.buf[:0]
				return Key{Type: KeyEsc}, nil
			}
		}
		var b [1]byte
		n, err := k.in.Read(b[:])
		if err != nil {
			if errors.Is(err, io.EOF) {
				return Key{}, ErrClosed
			}
			return Key{}, err
		}
		if n == 0 {
			continue
		}
		k.buf = append(k.buf, b[0])
	}
}

// Pending reports whether undecoded input is buffered.
func (k *KeyReader) Pending() bool { return len(k.buf) > 0 }
