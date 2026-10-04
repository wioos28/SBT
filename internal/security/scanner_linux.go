//go:build linux

package security

import (
	"path/filepath"
)

// collector accumulates checks and issues while the scanner runs.
type collector struct {
	checks []Check
	issues []Issue
}

func (c *collector) add(name string, s CheckStatus, detail string) {
	c.checks = append(c.checks, Check{Name: name, Status: s, Detail: detail})
}

func (c *collector) flag(i Issue) { c.issues = append(c.issues, i) }

// platformScan performs the Linux checks. Every check reads state under /proc;
// none of them writes to the host, mounts anything, or runs exploit code.
func platformScan(o Options) ([]Check, []Issue) {
	base := "/proc"
	if o.TargetPID > 0 {
		base = filepath.Join("/proc", itoa(o.TargetPID))
	}
	c := &collector{}
	scanNamespaces(c, base)
	scanSeccompCaps(c, base)
	scanEnvironment(c, o)
	scanMounts(c, o, base)
	scanNetwork(c, o, base)
	scanLimitsAndProc(c, base)
	return c.checks, c.issues
}
