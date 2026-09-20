// Command sbt is the Sandbox Terminal entry point.
//
// SBT verifies what the host kernel really supports at runtime (platform
// probe), runs commands inside that verified isolation, and dispatches its
// hidden helper modes through internal/internalhelper: a re-executed SBT with
// SBT_INTERNAL_MODE set never reaches the CLI below.
package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/wioos28/sbt/internal/internalhelper"
	"github.com/wioos28/sbt/internal/platform"
	"github.com/wioos28/sbt/internal/shell"
	"github.com/wioos28/sbt/internal/ui"
	"github.com/wioos28/sbt/internal/version"
)

const usage = `SBT — Sandbox Terminal

Usage:

  sbt doctor     verify platform isolation and host dependencies
  sbt version    print the build identity
  sbt help       show this help

Hidden helper modes are not part of the CLI surface; they are selected by the
SBT_INTERNAL_MODE environment variable when SBT re-executes itself.`

func main() {
	// Hidden helper dispatch: when this process was re-executed as a helper
	// (probe, jail, net probe) run that mode and exit with its status.
	if handled, code := internalhelper.MaybeRun(); handled {
		os.Exit(code)
	}

	args := os.Args[1:]
	if len(args) == 0 {
		// Interactive terminal: launch the sandbox shell. Without a terminal
		// (pipes, scripts) print the usage so output stays parseable.
		if runtime.GOOS == "linux" && ui.IsTerminal(os.Stdout) && ui.IsTerminal(os.Stdin) {
			os.Exit(shell.Run())
		}
		fmt.Println(version.String())
		fmt.Println(usage)
		fmt.Println("\nTip: run `sbt doctor` to see what this machine can verify.")
		return
	}
	switch args[0] {
	case "doctor":
		os.Exit(doctor())
	case "version", "--version", "-v":
		fmt.Println(version.String())
	case "help", "--help", "-h":
		fmt.Println(usage)
	default:
		fmt.Fprintf(os.Stderr, "sbt: unknown command %q\n\n%s\n", args[0], usage)
		os.Exit(2)
	}
}

// doctor runs the platform capability probe and the dependency check, prints
// every claim with its evidence, and exits non-zero when SBT could not verify
// what it needs.
func doctor() int {
	fmt.Println(version.String())
	caps := platform.Detect()
	fmt.Printf("kernel: %s (%s/%s)\n", caps.Kernel, caps.OS, caps.Arch)
	if caps.Distribution != "" {
		fmt.Printf("distro: %s\n", caps.Distribution)
	}
	if caps.Backend != "" {
		fmt.Printf("backend: %s\n", caps.Backend)
	}
	fmt.Println()

	status := map[platform.Level]string{
		platform.Full:        "[+]",
		platform.Partial:     "[~]",
		platform.Unavailable: "[ ]",
		platform.Unknown:     "[?]",
	}
	for _, f := range caps.Features {
		fmt.Printf("  %s %-18s %-12s %s\n", status[f.Level], f.Label, f.Level.Label(), f.Reason)
	}

	for _, w := range caps.Warnings {
		fmt.Printf("  warning: %s\n", w)
	}

	fmt.Println("\ndependencies:")
	missingRequired, missingOptional := 0, 0
	for _, d := range caps.Dependencies {
		mark := "[+]"
		if !d.Found {
			mark = "[ ]"
			if d.Required {
				missingRequired++
			} else {
				missingOptional++
			}
		}
		note := d.Version
		if note == "" && d.Note != "" {
			note = d.Note
		}
		if note != "" {
			note = " (" + note + ")"
		}
		fmt.Printf("  %s %-8s %s%s\n", mark, d.Name, map[bool]string{true: "found", false: "missing"}[d.Found], note)
	}

	if caps.ProbeError != "" {
		fmt.Fprintf(os.Stderr, "\nsbt doctor: platform probe failed: %s\n", caps.ProbeError)
		return 1
	}
	if missingRequired > 0 {
		fmt.Fprintf(os.Stderr, "\nsbt doctor: %d required dependenc%s missing\n", missingRequired, map[bool]string{true: "y", false: "ies"}[missingRequired == 1])
		return 1
	}
	if missingOptional > 0 {
		fmt.Printf("\n%d optional dependenc%s missing; SBT does not require %s.\n", missingOptional, map[bool]string{true: "y", false: "ies"}[missingOptional == 1], map[bool]string{true: "it", false: "them"}[missingOptional == 1])
	}
	return 0
}
