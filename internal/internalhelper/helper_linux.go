//go:build linux

package internalhelper

import (
	"fmt"
	"os"

	"github.com/wioos28/sbt/internal/platform/linux/jail"
	"github.com/wioos28/sbt/internal/platform/linux/ns"
	"github.com/wioos28/sbt/internal/platform/linux/nsprobe"
)

func runHelper(mode string) (bool, int) {
	if dbg := os.Getenv("SBT_DEBUG_BARRIER"); dbg != "" {
		fmt.Fprintf(os.Stderr, "[barrier] mode=%q syncfd=%q ModeEnv=%q entering\n", mode, os.Getenv(ns.MappingSyncFDEnv), os.Getenv(ModeEnv))
	}
	// The parent writes /proc/<pid>/{setgroups,gid_map,uid_map} after this
	// helper signals readiness. Before that barrier this helper has no valid
	// identity mapping and must not touch the filesystem.
	ns.ReachMappingBarrier()
	switch mode {
	case ModeProbe:
		return true, nsprobe.Run(os.Stdout)
	case ModeNetProbe:
		return true, nsprobe.RunNetworkProbe(os.Stdout)
	case ModeJail:
		return true, jail.Run(os.Getenv(SpecEnv), controlFD())
	default:
		// Unknown helper mode: refuse instead of running something unexpected.
		return true, 127
	}
}

func controlFD() int {
	switch os.Getenv(ControlFDEnv) {
	case "", "3":
		return 3
	case "4":
		return 4
	default:
		return 3
	}
}
