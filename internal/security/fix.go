package security

import (
	"fmt"
	"strings"
)

// Fix is a human-readable remediation for one issue found by the scanner.
type Fix struct {
	// Issue is the issue this fix addresses.
	Issue Issue
	// Steps are the ordered remediation instructions.
	Steps []string
	// Destructive is true when applying the fix destroys sandbox state.
	Destructive bool
	// RequireConfirmation is true for every fix that changes the sandbox; SBT
	// never applies a fix silently.
	RequireConfirmation bool
}

// Fixes derives remediation steps for every issue in a report.
func Fixes(r Report) []Fix {
	out := make([]Fix, 0, len(r.Issues))
	for _, i := range r.Issues {
		steps := i.Fix
		if len(steps) == 0 {
			steps = []string{"Review the finding and re-run `sbt security scan`."}
		}
		out = append(out, Fix{
			Issue:               i,
			Steps:               steps,
			Destructive:         i.Level >= Critical,
			RequireConfirmation: true,
		})
	}
	return out
}

// RenderFix formats one fix as the text shown in the "View Fix" dialog.
func RenderFix(f Fix) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", f.Issue.Level.String())
	fmt.Fprintf(&b, "%s\n\n", f.Issue.Summary)
	if f.Issue.Detail != "" {
		fmt.Fprintf(&b, "Detail: %s\n\n", f.Issue.Detail)
	}
	b.WriteString("Recommended fix:\n")
	for i, s := range f.Steps {
		fmt.Fprintf(&b, "%d. %s\n", i+1, s)
	}
	if f.RequireConfirmation {
		b.WriteString("\n[ View Fix ]   [ Apply Fix ]   [ Cancel ]\n")
	}
	if f.Destructive {
		b.WriteString("\nThis fix destroys sandbox state and requires explicit confirmation.\n")
	}
	return b.String()
}

// ApplyResult records the outcome of applying a fix.
type ApplyResult struct {
	Applied bool
	Steps   []string
	Note    string
}

// ApplyFix applies a fix's steps through the supplied executor. The executor is
// provided by the caller (the sandbox manager) so the security package never
// touches the filesystem itself. A destructive fix that is not confirmed is
// refused.
func ApplyFix(f Fix, confirmed bool, exec func(step string) error) (ApplyResult, error) {
	if f.RequireConfirmation && !confirmed {
		return ApplyResult{Applied: false, Note: "confirmation required"}, fmt.Errorf("fix requires explicit confirmation")
	}
	if exec == nil {
		return ApplyResult{Applied: false, Note: "no executor"}, fmt.Errorf("no executor supplied")
	}
	res := ApplyResult{Applied: true}
	for _, step := range f.Steps {
		if err := exec(step); err != nil {
			res.Applied = false
			res.Note = err.Error()
			return res, err
		}
		res.Steps = append(res.Steps, step)
	}
	return res, nil
}
