//go:build unix

package sandbox

import "syscall"

// suspendProcess pauses a helper so it can be inspected without destroying it.
//
// It is best effort by design: a sandbox that could not be paused must still be
// marked FROZEN in the registry, because leaving it marked "running" would be
// a worse lie than a freeze that did not take.
func suspendProcess(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGSTOP)
}
