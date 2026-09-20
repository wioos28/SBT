// Package version holds the SBT build identity.
package version

// These values are overridable at build time with -ldflags.
var (
	// Version is the semantic version of this SBT build.
	Version = "0.0.1"
	// Commit is the git revision the binary was built from.
	Commit = "dev"
	// Date is the build date (RFC3339) if provided by the build system.
	Date = "unknown"
)

// Name is the product name.
const Name = "SBT"

// Tagline is the product one-liner used in help output.
const Tagline = "Sandbox Terminal - run with isolation, observe, review, then export or discard."

// String returns a single-line version description.
func String() string {
	s := Name + " v" + Version
	if Commit != "" && Commit != "dev" {
		s += " (" + Commit + ")"
	}
	return s
}
