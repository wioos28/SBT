//go:build !linux && !darwin

package tui

import (
	"os"
	"time"
)

// waitReadable is not implementable without a platform poll primitive, so SBT
// treats Escape as immediate on this platform and does not offer Alt+<key>
// bindings there. It says so in the help view instead of pretending.
func waitReadable(*os.File, time.Duration) bool { return false }
