//go:build linux

package tui

import (
	"os"
	"syscall"
	"time"
)

// waitReadable reports whether f has input available within d. It is what makes
// a lone Escape key distinguishable from the start of an escape sequence.
//
// The descriptor set is indexed by hand so SBT does not need a dependency for
// one select(2) call; descriptors outside the set size are treated as readable
// rather than indexing past the end of the bitmap.
func waitReadable(f *os.File, d time.Duration) bool {
	fd := int(f.Fd())
	if fd < 0 {
		return false
	}
	if fd >= 64*len(syscall.FdSet{}.Bits) {
		return true
	}
	var rfds syscall.FdSet
	rfds.Bits[fd/64] |= int64(1) << (uint(fd) % 64)
	tv := syscall.NsecToTimeval(d.Nanoseconds())
	n, err := syscall.Select(fd+1, &rfds, nil, nil, &tv)
	return err == nil && n > 0
}
