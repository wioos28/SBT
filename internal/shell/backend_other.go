//go:build !linux

package shell

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/wioos28/sbt/internal/shared/jailspec"
)

// The one platform-specific seam in the session.
//
// Everything about a run - building the spec, seeding the workspace, reaping the
// helper, folding the manifest into the journal - is shared code. The single
// step that cannot be shared is starting the isolation itself, because that is
// the step each operating system does differently. `startHelper` is that step,
// and the only reason this package has per-OS files at all.

// startHelper launches the isolation backend for spec and blocks until it has
// either come up or refused to.
//
// It returns the helper command so the caller owns Wait on it. The helper is
// pid 1 of the sandbox's process namespace wherever the platform provides one,
// which is what makes killing it tear the whole sandbox down with it.
//
// An implementation must never return a running helper together with an error,
// and must never return success for isolation it could not establish: the whole
// promise of this program is that a sandbox that is not real says so.
func startHelper(spec *jailspec.Spec, specPath string, in, out *os.File) (*exec.Cmd, error) {
	return nil, fmt.Errorf("%s", backendUnavailable())
}

// backendUnavailable is the user-facing reason a platform has no sandbox.
//
// It is deliberately specific about *what* is missing rather than saying "not
// supported": a user who reads this should be able to tell the difference
// between "this program does not know how to isolate here" and "this machine
// is configured in a way that stops isolation".
func backendUnavailable() string {
	switch runtime.GOOS {
	case "darwin":
		return "SBT has no verified sandbox backend for macOS. The interface runs, " +
			"but it refuses to run commands rather than pretending they are isolated."
	case "windows":
		return "SBT has no verified sandbox backend for Windows. The interface runs, " +
			"but it refuses to run commands rather than pretending they are isolated. " +
			"See docs/LIMITATIONS.md."
	default:
		return "SBT has no verified sandbox backend for " + runtime.GOOS + ". " +
			"The interface runs, but it refuses to run commands rather than " +
			"pretending they are isolated."
	}
}
