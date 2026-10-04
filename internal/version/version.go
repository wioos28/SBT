// Package version holds the SBT build identity.
package version

// These values are overridable at build time with -ldflags.
var (
	// Version is the semantic version of this SBT build.
	Version = "0.1.0"
	// Commit is the git revision the binary was built from.
	Commit = "dev"
	// Date is the build date (RFC3339) if provided by the build system.
	Date = "unknown"
)

// Name is the product name.
const Name = "SBT"

// Tagline is the product one-liner used in help output.
const Tagline = "sbt-wioos28 - secure sandbox terminal: isolate, observe, review, then export or discard."

// Brand is the visual identity shown at startup.
const Brand = "sbt-wioos28"

// String returns a single-line version description.
func String() string {
	s := Name + " v" + Version
	if Commit != "" && Commit != "dev" {
		s += " (" + Commit + ")"
	}
	return s
}
