package security

import (
	"time"
)

// Options configures a scan. The scanner only reads operating-system state; it
// never writes, mounts, or launches an exploit.
type Options struct {
	// TargetPID is the process whose namespace context is inspected. 0 means
	// the current process. For a running sandbox this is the helper pid, which
	// is pid 1 of the sandbox's pid namespace, so its /proc entries describe
	// what the sandbox can actually see.
	TargetPID int
	// SandboxID labels the report.
	SandboxID string
	// ExpectWorkspace is the path inside the sandbox that is meant to be the
	// only writable tree (usually /workspace).
	ExpectWorkspace string
	// ExpectNetworkBlocked is the network policy that is supposed to be in force.
	ExpectNetworkBlocked bool
	// ExpectEnvironmentIsolation is true when the environment should be scrubbed.
	ExpectEnvironmentIsolation bool
	// ExpectReadOnlyHost is true when host trees should be read-only.
	ExpectReadOnlyHost bool
	// AllowedHostPaths lists host paths that are intentionally visible.
	AllowedHostPaths []string
}

// Scan runs every check and returns a report. It is safe to call at any time and
// leaves no trace on the host: a check that cannot be performed is reported as
// NOT AVAILABLE rather than assumed to pass.
func Scan(o Options) Report {
	rep := Report{At: time.Now(), Scope: scope(o)}
	checks, issues := platformScan(o)
	rep.Checks = checks
	rep.Issues = issues
	rep.Note = "SBT verifies observable state only; a passing scan is not a proof of absolute security."
	return rep
}

func scope(o Options) string {
	switch {
	case o.SandboxID != "":
		return "sandbox:" + o.SandboxID
	case o.TargetPID > 0:
		return "pid:" + itoa(o.TargetPID)
	default:
		return "current-process"
	}
}

// issue is a small builder used by the platform checks.
func issue(level Level, area, summary, detail string, fix ...string) Issue {
	return Issue{Level: level, Area: area, Summary: summary, Detail: detail, Fix: fix}
}

// itoa is a local integer formatter (kept dependency-free on purpose).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
