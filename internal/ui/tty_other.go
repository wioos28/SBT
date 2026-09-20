//go:build !linux && !darwin

package ui

import (
	"errors"
	"os"
)

// ErrRawModeUnsupported is returned on platforms where SBT cannot switch the
// terminal into raw mode. SBT then degrades to line oriented input and says so
// instead of pretending the feature exists.
var ErrRawModeUnsupported = errors.New("raw terminal mode is unavailable on this platform")

func terminalSize(f *os.File) (int, int, bool) { return 0, 0, false }

func makeRaw(f *os.File) (*TermiosState, error) { return nil, ErrRawModeUnsupported }

func restoreTermios(f *os.File, st *TermiosState) error { return nil }

// RawModeUnsupported reports whether a sentinel error means "no raw mode here".
func RawModeUnsupported(err error) bool { return errors.Is(err, ErrRawModeUnsupported) }
