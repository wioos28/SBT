// Command cli-adapters is a leaf package of AI coding-CLI adapters. Each
// adapter knows how to detect, describe and launch one external coding CLI
// (OpenCode, Cline, Codex, ...). SBT never embeds their internals; it drives
// their published command lines inside the sandbox and records the run in the
// process manager.
package adapters

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// Adapter is the contract every AI coding CLI must satisfy to be driven by SBT.
type Adapter interface {
	// Name is the stable adapter id, e.g. "opencode".
	Name() string
	// Detect reports whether the CLI is installed on this host.
	Detect() bool
	// Install explains how to install the CLI (SBT does not fetch code).
	Install() string
	// Command returns the argv that launches the CLI. It is passed to
	// exec.Command, never to a shell.
	Command(args ...string) []string
	// Version returns the CLI version string.
	Version() (string, error)
}

type binaryAdapter struct {
	name     string
	binaries []string
	install  string
	extra    []string
}

func (a binaryAdapter) Name() string { return a.name }

func (a binaryAdapter) Detect() bool {
	for _, b := range a.binaries {
		if _, err := exec.LookPath(b); err == nil {
			return true
		}
	}
	return false
}

func (a binaryAdapter) Install() string { return a.install }

func (a binaryAdapter) Command(args ...string) []string {
	bin := a.name
	if p, err := exec.LookPath(a.binaries[0]); err == nil {
		bin = p
	}
	out := append([]string{bin}, a.extra...)
	return append(out, args...)
}

func (a binaryAdapter) Version() (string, error) {
	argv := a.Command("--version")
	out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if len(line) > 80 {
		line = line[:80]
	}
	return line, nil
}

// Known returns the built-in adapters. The implementations stay thin on
// purpose: detection is PATH lookup and launching is argv, so a change in the
// external CLI only affects its few lines here.
func Known() []Adapter {
	return []Adapter{
		binaryAdapter{name: "opencode", binaries: []string{"opencode"}, install: "install opencode from https://opencode.ai and ensure it is on PATH"},
		binaryAdapter{name: "cline", binaries: []string{"cline"}, install: "install the Cline CLI and ensure it is on PATH"},
		binaryAdapter{name: "codex", binaries: []string{"codex"}, install: "install the Codex CLI and ensure it is on PATH"},
	}
}

// ByName returns the adapter with the given name.
func ByName(name string) (Adapter, error) {
	for _, a := range Known() {
		if strings.EqualFold(a.Name(), name) {
			return a, nil
		}
	}
	names := []string{}
	for _, a := range Known() {
		names = append(names, a.Name())
	}
	sort.Strings(names)
	return nil, fmt.Errorf("unknown AI CLI %q (known: %s)", name, strings.Join(names, ", "))
}

// DetectAll returns the detection result for every known adapter.
func DetectAll() []Detection {
	out := []Detection{}
	for _, a := range Known() {
		d := Detection{Name: a.Name(), Installed: a.Detect()}
		if d.Installed {
			if v, err := a.Version(); err == nil {
				d.Version = v
			}
		} else {
			d.Hint = a.Install()
		}
		out = append(out, d)
	}
	return out
}

// Detection is one adapter's presence report.
type Detection struct {
	Name      string
	Installed bool
	Version   string
	Hint      string
}
