//go:build darwin

package tui

import (
	"os"
	"syscall"
	"time"
)

// waitReadable reports whether f has input available within d. See the Linux
// implementation for why this exists.
//
// The Darwin signature of syscall.Select is different from Linux's: it returns
// only an error, because on BSD the ready descriptors are reported by rewriting
// the descriptor set in place rather than through a separate count. So the check
// is done on the set afterwards rather than on a return value. That difference
// is the whole reason this file exists instead of a shared implementation.
func waitReadable(f *os.File, d time.Duration) bool {
	fd := int(f.Fd())
	if fd < 0 {
		return false
	}
	bit := int32(1) << (uint(fd) % 32)
	if fd/32 >= len(syscall.FdSet{}.Bits) {
		return true
	}
	rfds := syscall.FdSet{}
	rfds.Bits[fd/32] |= bit
	tv := syscall.NsecToTimeval(d.Nanoseconds())
	if err := syscall.Select(fd+1, &rfds, nil, nil, &tv); err != nil {
		return false
	}
	return rfds.Bits[fd/32]&bit != 0
}
