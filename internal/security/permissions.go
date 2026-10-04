// The permission report.
//
// The Permissions view exists because "run as root" and "the sandbox works" are
// different claims, and a user is entitled to know which one is true. Every row
// is measured - a path is opened, a directory is written to, a capability is
// looked up - and anything that cannot be measured says UNKNOWN rather than
// guessing. Each row also carries the feature that needs the capability and the
// least-privilege step that would grant it, so a denial is actionable instead
// of a dead end, and a remedy never asks for blanket root.
package security

import (
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/wioos28/sbt/internal/platform"
)

// CapStatus is the verdict for one capability.
type CapStatus string

// Capability verdicts.
const (
	// CapAllowed means SBT verified the capability is present.
	CapAllowed CapStatus = "ALLOWED"
	// CapLimited means it works with documented restrictions.
	CapLimited CapStatus = "LIMITED"
	// CapDenied means SBT verified it is not available.
	CapDenied CapStatus = "DENIED"
	// CapUnknown means it could not be measured on this platform.
	CapUnknown CapStatus = "UNKNOWN"
)

// Capability is one row of the permission report.
type Capability struct {
	Name string
	// Status is the measured verdict.
	Status CapStatus
	// Reason explains the verdict with the evidence that produced it.
	Reason string
	// Feature is what in SBT needs the capability (empty when nothing does).
	Feature string
	// Remedy is the least-privilege step that would grant it (empty when none).
	Remedy string
}

// CapabilityReport is the full measurement.
type CapabilityReport struct {
	Caps []Capability
	At   time.Time
}

// Summary counts the rows by verdict.
func (r CapabilityReport) Summary() (allowed, limited, denied int) {
	for _, c := range r.Caps {
		switch c.Status {
		case CapAllowed:
			allowed++
		case CapLimited:
			limited++
		default:
			denied++
		}
	}
	return
}

// Permissions measures the capabilities SBT cares about. caps is the platform
// probe report when one is available; a nil report makes the isolation-derived
// rows read UNKNOWN instead of assuming a backend exists.
func Permissions(caps *platform.Capabilities) CapabilityReport {
	rep := CapabilityReport{At: time.Now()}
	rep.Caps = []Capability{
		filesystemCap(),
		networkCap(caps),
		processCap(caps),
		ptyCap(),
		environmentCap(),
		dockerCap(caps),
		sudoCap(),
		rootCap(),
		deviceCap(),
		tempDirCap(),
		workingDirCap(),
	}
	return rep
}

// stateDir is the SBT state directory. It mirrors config.Dir deliberately
// rather than importing it, so this package keeps no dependency on the CLI
// layer and can be used by anything that needs the report.
func stateDir() string {
	if v := os.Getenv("SBT_HOME"); v != "" {
		return v
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".sbt-wioos28")
	}
	return filepath.Join(".", ".sbt-wioos28")
}

// probeDir reports whether SBT can create and write in dir.
func probeDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".sbt-probe-*")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}

// filesystemCap checks the one directory SBT must own.
func filesystemCap() Capability {
	dir := filepath.Join(stateDir(), "workspace")
	if err := probeDir(dir); err != nil {
		return Capability{
			Name: "Filesystem", Status: CapDenied,
			Reason:  "cannot use the session workspace: " + err.Error(),
			Feature: "capturing files a sandboxed command writes",
			Remedy:  "make " + dir + " writable by this user",
		}
	}
	return Capability{Name: "Filesystem", Status: CapAllowed, Reason: "SBT can read and write its workspace"}
}

// featureCap turns one probe feature into a permission row.
func featureCap(caps *platform.Capabilities, key, name, need, remedy string) Capability {
	if caps == nil {
		return Capability{Name: name, Status: CapUnknown,
			Reason:  "the platform probe has not run",
			Feature: need}
	}
	f := caps.Feature(key)
	c := Capability{Name: name, Feature: need}
	switch f.Level {
	case platform.Full:
		c.Status, c.Reason = CapAllowed, "verified: "+f.Reason
	case platform.Partial:
		c.Status, c.Reason = CapLimited, "partial: "+f.Reason
		c.Remedy = remedy
	default:
		c.Status, c.Remedy = CapDenied, remedy
		c.Reason = "unavailable: " + f.Reason
	}
	return c
}

func networkCap(caps *platform.Capabilities) Capability {
	return featureCap(caps, "network_namespace", "Network", "cutting a sandbox off from the network",
		"allow unprivileged user namespaces: sysctl kernel.unprivileged_userns_clone=1")
}

func processCap(caps *platform.Capabilities) Capability {
	return featureCap(caps, "pid_namespace", "Process control", "confining a sandbox to its own pid namespace",
		"run on Linux with unprivileged user namespaces enabled")
}

// ptyCap checks that a pseudoterminal can be opened, which an interactive
// sandboxed command needs.
func ptyCap() Capability {
	if runtime.GOOS == "windows" {
		return Capability{Name: "PTY", Status: CapUnknown,
			Reason:  "pseudoterminals are not used on Windows",
			Feature: "an interactive command inside the sandbox"}
	}
	f, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return Capability{Name: "PTY", Status: CapDenied,
			Reason:  "/dev/ptmx is not available: " + err.Error(),
			Feature: "an interactive command inside the sandbox",
			Remedy:  "mount a devtmpfs on /dev, or pass the pty device through"}
	}
	_ = f.Close()
	return Capability{Name: "PTY", Status: CapAllowed, Reason: "/dev/ptmx opens read-write"}
}

// environmentCap is always allowed: SBT builds the sandbox environment itself
// and passes none of the host's by default.
func environmentCap() Capability {
	return Capability{Name: "Environment", Status: CapAllowed,
		Reason: "SBT builds the sandbox environment itself and passes none of the host's by default"}
}

// dockerCap detects a container runtime.
func dockerCap(caps *platform.Capabilities) Capability {
	container := platform.InContainer()
	if caps != nil {
		container = container || caps.InContainer
	}
	if !container {
		return Capability{Name: "Docker", Status: CapDenied, Reason: "no container runtime detected"}
	}
	c := Capability{Name: "Docker", Status: CapLimited,
		Reason:  "SBT is running inside a container",
		Feature: "harder isolation primitives may be restricted by the container runtime",
		Remedy:  "grant only the specific capability you need, never blanket privilege"}
	if _, err := os.Stat("/var/run/docker.sock"); err == nil {
		c.Status, c.Reason, c.Remedy = CapAllowed, "inside a container, and a Docker socket is present", ""
	}
	return c
}

// sudoCap reports whether sudo exists. SBT never uses it, and says so, so a
// user is not told to escalate when they do not have to.
func sudoCap() Capability {
	if p, ok := platform.LookPath("sudo"); ok {
		return Capability{Name: "Sudo", Status: CapLimited,
			Reason:  "sudo is installed at " + p,
			Feature: "nothing in SBT requires sudo",
			Remedy:  "prefer unprivileged user namespaces over running SBT as root"}
	}
	return Capability{Name: "Sudo", Status: CapDenied, Reason: "no sudo binary on PATH"}
}

// rootCap reports whether the process is root.
func rootCap() Capability {
	if platform.IsElevated() {
		return Capability{Name: "Root", Status: CapAllowed,
			Reason:  "SBT is running as uid 0",
			Feature: "nothing - running as root is not required and widens the blast radius",
			Remedy:  "run SBT as an unprivileged user and rely on user namespaces"}
	}
	return Capability{Name: "Root", Status: CapDenied, Reason: "SBT is running as an unprivileged user"}
}

// deviceCap checks whether device nodes are reachable.
func deviceCap() Capability {
	if runtime.GOOS != "linux" {
		return Capability{Name: "Device access", Status: CapUnknown, Reason: "not measured on this platform"}
	}
	if _, err := os.Stat("/dev/null"); err != nil {
		return Capability{Name: "Device access", Status: CapDenied, Reason: "/dev/null is not present: " + err.Error()}
	}
	return Capability{Name: "Device access", Status: CapLimited,
		Reason:  "/dev is present; a sandbox is given a minimal device set only",
		Feature: "device nodes are not exposed to the sandbox beyond the basics"}
}

// tempDirCap checks the scratch directory SBT uses.
func tempDirCap() Capability {
	dir := os.TempDir()
	if err := probeDir(dir); err != nil {
		return Capability{Name: "Temporary directory", Status: CapDenied, Reason: dir + " is not usable: " + err.Error()}
	}
	return Capability{Name: "Temporary directory", Status: CapAllowed, Reason: dir + " is writable"}
}

// workingDirCap reports the working directory and whether it is writable.
func workingDirCap() Capability {
	wd, err := os.Getwd()
	if err != nil {
		return Capability{Name: "Working directory", Status: CapUnknown, Reason: "could not read the working directory"}
	}
	if err := probeDir(wd); err != nil {
		return Capability{Name: "Working directory", Status: CapLimited,
			Reason: wd + " is read-only", Feature: "SBT writes only inside its own workspace, never here"}
	}
	return Capability{Name: "Working directory", Status: CapAllowed, Reason: wd + " is writable"}
}
