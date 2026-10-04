//go:build !unix

package sandbox

// suspendProcess is a no-op where the OS offers no freeze signal.
//
// The registry is still updated to FROZEN by the caller: recording the user's
// intent is what stops the normal path from continuing the sandbox, and it is
// better to record that honestly than to skip it because the platform cannot
// pause a process.
func suspendProcess(int) {}
