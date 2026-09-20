// Package internalhelper implements the hidden helper modes that SBT re-executes
// with. The helper modes are documented in docs/SECURITY.md but are not part of
// the public CLI surface.
package internalhelper

import (
	"os"

	"github.com/wioos28/sbt/internal/shared/helpermode"
)

// Environment variables used to enter helper modes.
const (
	// ModeEnv selects a helper mode.
	ModeEnv = helpermode.ModeEnv
	// SpecEnv points at the JSON jailspec file.
	SpecEnv = helpermode.SpecEnv
	// ControlFDEnv is the file descriptor of the control pipe.
	ControlFDEnv = helpermode.ControlFDEnv
)

// Helper modes.
const (
	// ModeProbe verifies namespace/mount/capability/seccomp support.
	ModeProbe = helpermode.ModeProbe
	// ModeJail runs one command inside the sandbox jail.
	ModeJail = helpermode.ModeJail
	// ModeNetProbe verifies that the network policy really blocks connections.
	ModeNetProbe = helpermode.ModeNetProbe
)

// MaybeRun executes a helper mode when one was requested. It returns true when
// the process acted as a helper and the caller must exit with the given code.
//
// The helper dispatch also runs from init() so that re-executed helpers work in
// any binary that links this package, including Go test binaries (the capability
// probe re-executes the *test* binary when tests call platform.Detect).
func MaybeRun() (bool, int) {
	mode := os.Getenv(ModeEnv)
	if mode == "" {
		return false, 0
	}
	return runHelper(mode)
}

func init() {
	if mode := os.Getenv(ModeEnv); mode != "" {
		handled, code := runHelper(mode)
		if handled {
			os.Exit(code)
		}
	}
}
