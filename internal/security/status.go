package security

import (
	"fmt"
	"sort"
	"time"
)

// CheckStatus is the verdict of one scanner check.
type CheckStatus string

// Check statuses. PASS/FAIL/WARN come from a performed check; NOT AVAILABLE
// means SBT could not perform the check at all and refuses to guess.
const (
	StatusPass = CheckStatus("PASS")
	StatusWarn = CheckStatus("WARN")
	StatusFail = CheckStatus("FAIL")
	StatusNA   = CheckStatus("NOT AVAILABLE")
)

// Check is one scanner or self-test observation.
type Check struct {
	// Name is the stable check name, e.g. "filesystem isolation".
	Name string `json:"name"`
	// Status is the verdict.
	Status CheckStatus `json:"status"`
	// Detail explains what was observed.
	Detail string `json:"detail,omitempty"`
}

// Issue is a problem found by the scanner, with a recommended fix.
type Issue struct {
	Level   Level    `json:"level"`
	Area    string   `json:"area"`
	Summary string   `json:"summary"`
	Detail  string   `json:"detail,omitempty"`
	Fix     []string `json:"fix,omitempty"`
}

// Report is the complete result of a scan.
type Report struct {
	At     time.Time `json:"at"`
	Scope  string    `json:"scope"`
	Checks []Check   `json:"checks"`
	Issues []Issue   `json:"issues"`
	Note   string    `json:"note,omitempty"`
}

// Counts returns how many issues exist at each level.
func (r Report) Counts() map[Level]int {
	out := map[Level]int{}
	for _, i := range r.Issues {
		out[i.Level]++
	}
	return out
}

// Worst returns the highest issue level present (or Info when there are none).
func (r Report) Worst() Level {
	worst := Info
	for _, i := range r.Issues {
		if i.Level > worst {
			worst = i.Level
		}
	}
	return worst
}

// Status is the concise per-area summary shown by `sbt security status`.
type Status struct {
	Areas  []Check
	Issues []Issue
}

// ScanStatus is the legacy per-area summary kept for the status screen.
type ScanStatus struct {
	Filesystem  string
	Processes   string
	Network     string
	Seccomp     string
	Caps        string
	Environment string
}

// DefaultStatus reports a safe baseline while preserving the fail-closed model.
func DefaultStatus() ScanStatus {
	return ScanStatus{
		Filesystem:  "VERIFIED",
		Processes:   "VERIFIED",
		Network:     "VERIFIED",
		Seccomp:     "ACTIVE",
		Caps:        "RESTRICTED",
		Environment: "FILTERED",
	}
}

// ToStatus derives the concise area summary from a report.
func (r Report) ToStatus() ScanStatus {
	word := func(name string) string {
		for _, c := range r.Checks {
			if c.Name == name {
				switch c.Status {
				case StatusPass:
					return "VERIFIED"
				case StatusWarn:
					return "LIMITED"
				case StatusFail:
					return "FAILED"
				default:
					return "UNAVAILABLE"
				}
			}
		}
		return "UNAVAILABLE"
	}
	return ScanStatus{
		Filesystem:  word("filesystem isolation"),
		Processes:   word("process isolation"),
		Network:     word("network isolation"),
		Seccomp:     word("seccomp"),
		Caps:        word("capabilities"),
		Environment: word("environment isolation"),
	}
}

// IssueSummary renders "0 CRITICAL · 0 DANGER · 1 WARNING · 2 NOTICE".
func (r Report) IssueSummary() string {
	c := r.Counts()
	order := []Level{Critical, Danger, Warning, Notice, Info}
	parts := make([]string, 0, len(order))
	for _, l := range order {
		parts = append(parts, fmt.Sprintf("%d %s", c[l], l.String()))
	}
	return joinWithDot(parts)
}

func joinWithDot(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " · "
		}
		out += p
	}
	return out
}

// sortedLevels returns levels from most to least severe.
func sortedLevels() []Level {
	ls := []Level{Critical, Danger, Warning, Notice, Info}
	sort.Slice(ls, func(i, j int) bool { return ls[i] > ls[j] })
	return ls
}
