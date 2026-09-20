// Package helpermode defines the environment contract between the SBT parent
// process and its hidden helper processes. It is a leaf package so that both the
// dispatcher and the platform helpers can depend on it without import cycles.
package helpermode

// Environment variables used to enter helper modes.
const (
	// ModeEnv selects a helper mode.
	ModeEnv = "SBT_INTERNAL_MODE"
	// SpecEnv points at the JSON jailspec file.
	SpecEnv = "SBT_INTERNAL_SPEC"
	// ControlFDEnv is the file descriptor number of the control pipe.
	ControlFDEnv = "SBT_INTERNAL_CONTROL_FD"
)

// Helper modes.
const (
	// ModeProbe verifies namespace/mount/capability/seccomp support.
	ModeProbe = "nsprobe"
	// ModeJail runs one command inside the sandbox jail.
	ModeJail = "jail"
	// ModeNetProbe verifies that the network policy really blocks connections.
	ModeNetProbe = "netprobe"
)
