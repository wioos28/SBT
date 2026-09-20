//go:build darwin

package tui

import (
	"os"
	"syscall"
	"time"
)

// waitReadable reports whether f has input available within d. See the Linux
// implementation for why this exists.
func waitReadable(f *os.File, d time.Duration) bool {
	fd := int(f.Fd())
	if fd < 0 {
		return false
	}
	if fd >= 32*len(syscall.FdSet{}.Bits) {
		return true
	}
	var rfds syscall.FdSet
	rfds.Bits[fd/32] |= int32(1) << (uint(fd) % 32)
	tv := syscall.NsecToTimeval(d.Nanoseconds())
	n, err := syscall.Select(fd+1, &rfds, nil, nil, &tv)
	return err == nil && n > 0
}
