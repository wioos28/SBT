package security

import (
	"fmt"
	"strings"
	"time"
)

// SelfTest names, in order, the checks the harness reports. They are the checks
// a user expects to see from `sbt security test`.
var selfTestOrder = []string{
	"filesystem isolation",
	"process isolation",
	"network isolation",
	"environment isolation",
	"workspace boundary",
	"proc exposure",
	"capability restrictions",
	"mount visibility",
	"resource limits",
	"seccomp",
}

// SelfTest runs the harness against the target described by o and returns a
// report whose checks are the self-test names, in order.
//
// IMPORTANT: a passing self-test shows that the observable isolation SBT relies
// on is present. It is NOT a proof of absolute security and SBT never claims it
// is. The harness deliberately performs no destructive exploit.
func SelfTest(o Options) Report {
	scan := Scan(o)
	byName := map[string]Check{}
	for _, c := range scan.Checks {
		byName[c.Name] = c
	}
	// The self-test order uses friendlier names that map onto scanner checks.
	alias := map[string]string{
		"filesystem isolation":    "mount namespace",
		"process isolation":       "pid namespace",
		"network isolation":       "network namespace",
		"environment isolation":   "environment isolation",
		"workspace boundary":      "writable host path",
		"proc exposure":           "proc exposure",
		"capability restrictions": "capabilities",
		"mount visibility":        "mount visibility",
		"resource limits":         "resource limits",
		"seccomp":                 "seccomp",
	}
	rep := Report{At: time.Now(), Scope: scan.Scope}
	for _, name := range selfTestOrder {
		src, ok := byName[alias[name]]
		if !ok {
			rep.Checks = append(rep.Checks, Check{Name: name, Status: StatusNA, Detail: "no evidence produced"})
			continue
		}
		status := src.Status
		// A writable host path means the workspace boundary failed.
		if name == "workspace boundary" {
			if src.Status == StatusFail {
				status = StatusFail
			} else if src.Status == StatusPass {
				status = StatusPass
			}
		}
		rep.Checks = append(rep.Checks, Check{Name: name, Status: status, Detail: src.Detail})
	}
	rep.Issues = scan.Issues
	rep.Note = "A passing self-test is evidence, not proof: it verifies observable isolation only."
	return rep
}

// RenderSelfTest formats a self-test report as numbered "TEST NN name STATUS"
// lines, the form shown by `sbt security test`.
func RenderSelfTest(r Report) string {
	var b strings.Builder
	for i, c := range r.Checks {
		fmt.Fprintf(&b, "TEST %02d %-26s %s\n", i+1, c.Name, c.Status)
	}
	b.WriteString("\n")
	b.WriteString(r.IssueSummary())
	b.WriteString("\n")
	if r.Note != "" {
		b.WriteString(r.Note)
		b.WriteString("\n")
	}
	return b.String()
}
