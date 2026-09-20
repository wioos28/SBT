//go:build linux

package ns

import "github.com/wioos28/sbt/internal/shared/helpermode"

const (
	helpermodeModeEnv    = helpermode.ModeEnv
	helpermodeSpecEnv    = helpermode.SpecEnv
	helpermodeControlEnv = helpermode.ControlFDEnv
)
