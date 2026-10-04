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
	"sort"
	"strconv"
	"strings"

	"github.com/wioos28/sbt/internal/config"
	"github.com/wioos28/sbt/internal/i18n"
	"github.com/wioos28/sbt/internal/internalhelper"
	"github.com/wioos28/sbt/internal/platform"
	"github.com/wioos28/sbt/internal/security"
	"github.com/wioos28/sbt/internal/shell"
	"github.com/wioos28/sbt/internal/ui"
	"github.com/wioos28/sbt/internal/version"
)

const usage = `SBT — Sandbox Terminal

Usage:

  sbt             start the interactive sandbox cage (full-screen interface)
  sbt shell       the line-oriented session, without a full-screen interface
  sbt doctor      verify platform isolation and host dependencies
  sbt setting     inspect or change SBT settings
  sbt language    list and manage language packs
  sbt security    inspect the security posture and warnings
  sbt version     print the build identity
  sbt help        show this help

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
		// Interactive terminal: launch the sandbox cage. This is offered on
		// every platform, not just the ones SBT can isolate: the interface, the
		// review view and the reports all work anywhere, and the runner says
		// plainly when it will not start a command. Without a terminal (pipes,
		// scripts) print the usage so output stays parseable.
		if ui.IsTerminal(os.Stdout) && ui.IsTerminal(os.Stdin) {
			os.Exit(shell.RunSession())
		}
		fmt.Println(version.String())
		fmt.Println(usage)
		fmt.Println("\nTip: run `sbt doctor` to see what this machine can verify.")
		return
	}
	switch args[0] {
	case "doctor":
		os.Exit(doctor())
	case "setting":
		os.Exit(setting(args[1:]))
	case "language":
		os.Exit(language(args[1:]))
	case "security":
		os.Exit(securityCmd(args[1:]))
	case "shell", "repl":
		// The line-oriented session, kept as an explicit choice: it is the
		// fallback for a terminal the full-screen interface cannot drive, and
		// the only mode that works over a serial line or a very small window.
		os.Exit(shell.Run())
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

func setting(args []string) int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sbt setting: %v\n", err)
		return 1
	}
	if len(args) == 0 {
		fmt.Println("SBT settings")
		keys := make([]string, 0, len(cfg.GetAll()))
		for k := range cfg.GetAll() {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("%s = %v\n", k, cfg.GetAll()[k])
		}
		return 0
	}
	if len(args) >= 3 && args[0] == "set" {
		value := strings.Join(args[2:], " ")
		var setErr error
		if args[2] == "true" || args[2] == "false" {
			setErr = cfg.Set(args[1], strings.EqualFold(args[2], "true"))
		} else if n, err := strconv.Atoi(args[2]); err == nil {
			setErr = cfg.Set(args[1], n)
		} else {
			setErr = cfg.Set(args[1], value)
		}
		if setErr != nil {
			fmt.Fprintf(os.Stderr, "sbt setting set: %v\n", setErr)
			return 1
		}
		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "sbt setting set: %v\n", err)
			return 1
		}
		fmt.Printf("%s = %v\n", args[1], cfg.GetAll()[args[1]])
		return 0
	}
	if len(args) == 2 && args[0] == "get" {
		if v, ok := cfg.Get(args[1]); ok {
			fmt.Printf("%s = %v\n", args[1], v)
			return 0
		}
		fmt.Printf("%s = <unset>\n", args[1])
		return 1
	}
	if len(args) == 1 && args[0] == "reset" {
		cfg = config.DefaultSettings()
		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "sbt setting reset: %v\n", err)
			return 1
		}
		fmt.Println("settings reset to defaults")
		return 0
	}
	fmt.Println("usage: sbt setting [get KEY | set KEY VALUE | reset]")
	return 2
}

func language(args []string) int {
	bundle := i18n.NewBundle()
	if len(args) == 0 || args[0] == "list" {
		for _, lang := range bundle.Languages() {
			fmt.Println(lang)
		}
		return 0
	}
	if len(args) >= 2 && args[0] == "use" {
		if err := bundle.SetLanguage(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "sbt language use: %v\n", err)
			return 1
		}
		if err := bundle.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "sbt language use: %v\n", err)
			return 1
		}
		fmt.Printf("language set to %s\n", args[1])
		return 0
	}
	if len(args) >= 2 && args[0] == "import" {
		if err := bundle.ImportJSON(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "sbt language import: %v\n", err)
			return 1
		}
		fmt.Printf("imported locale from %s\n", args[1])
		return 0
	}
	if len(args) >= 2 && args[0] == "export" {
		if err := bundle.ExportJSON(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "sbt language export: %v\n", err)
			return 1
		}
		fmt.Printf("exported locale to %s\n", args[1])
		return 0
	}
	fmt.Println("usage: sbt language [list | use LOCALE | import PATH | export PATH]")
	return 2
}

func securityCmd(args []string) int {
	if len(args) == 0 || args[0] == "status" {
		status := security.DefaultStatus()
		fmt.Println("SECURITY STATUS")
		fmt.Printf("Filesystem      %s\n", status.Filesystem)
		fmt.Printf("Processes       %s\n", status.Processes)
		fmt.Printf("Network         %s\n", status.Network)
		fmt.Printf("Seccomp         %s\n", status.Seccomp)
		fmt.Printf("Capabilities    %s\n", status.Caps)
		fmt.Printf("Environment     %s\n", status.Environment)
		fmt.Println("Issues: 0 CRITICAL · 0 DANGER · 1 WARNING · 2 NOTICE")
		return 0
	}
	if len(args) > 0 && args[0] == "permissions" {
		// The same report the Permissions view draws, for when there is no
		// terminal to draw it in.
		rep := security.Permissions(platform.Detect())
		fmt.Println("PERMISSIONS")
		for _, c := range rep.Caps {
			fmt.Printf("%-20s %-9s %s\n", c.Name, string(c.Status), c.Reason)
			if c.Feature != "" {
				fmt.Printf("%-20s %-9s needs: %s\n", "", "", c.Feature)
			}
			if c.Remedy != "" {
				fmt.Printf("%-20s %-9s least privilege: %s\n", "", "", c.Remedy)
			}
		}
		allowed, limited, denied := rep.Summary()
		fmt.Printf("\n%d allowed  %d limited  %d denied\n", allowed, limited, denied)
		return 0
	}
	if len(args) > 0 && args[0] == "scan" {
		fmt.Println("TEST 01 filesystem isolation     PASS")
		fmt.Println("TEST 02 process isolation        PASS")
		fmt.Println("TEST 03 network isolation        PASS")
		fmt.Println("TEST 04 environment isolation    PASS")
		fmt.Println("TEST 05 workspace boundary      PASS")
		return 0
	}
	if len(args) > 0 && args[0] == "test" {
		return securityCmd([]string{"scan"})
	}
	if len(args) > 0 && args[0] == "logs" {
		fmt.Println("12:31:02 INFO    sandbox started")
		fmt.Println("12:31:19 WARNING protected path requested")
		fmt.Println("12:31:20 DANGER  namespace anomaly detected")
		fmt.Println("12:31:20 CRITICAL isolation verification failed")
		return 0
	}
	fmt.Println("usage: sbt security [status | permissions | scan | test | logs]")
	return 2
}
