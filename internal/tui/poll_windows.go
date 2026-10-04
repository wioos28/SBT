//go:build windows

package tui

import (
	"os"
	"syscall"
	"time"
)

// waitReadable reports whether f has input available within d.
//
// Windows has no select(2); the equivalent primitive is waiting on the handle
// itself. A console input handle becomes signalled when a key is pressed or a
// mouse event arrives, which is exactly the question being asked here.
//
// This uses syscall rather than golang.org/x/sys/windows deliberately: SBT
// carries no module dependencies and every primitive needed here is standard
// library.
func waitReadable(f *os.File, d time.Duration) bool {
	handle := syscall.Handle(f.Fd())
	if handle == syscall.InvalidHandle {
		return false
	}
	ms := uint32(d / time.Millisecond)
	event, err := syscall.WaitForSingleObject(handle, ms)
	if err != nil {
		// A handle that cannot be waited on - a socket, for instance - fails
		// the wait immediately. Reporting "not readable" keeps Escape
		// immediate rather than turning the wait into an error path.
		return false
	}
	// WAIT_OBJECT_0 is "the object is signalled". WAIT_TIMEOUT is an ordinary
	// outcome meaning nothing arrived, not a failure, so it stays false.
	return event == syscall.WAIT_OBJECT_0
}
