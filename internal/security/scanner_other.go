//go:build !linux

package security

// platformScan reports that the scanner is not implemented on this platform
// rather than pretending the checks passed.
func platformScan(o Options) ([]Check, []Issue) {
	return []Check{{
		Name:   "platform scan",
		Status: StatusNA,
		Detail: "the SBT security scanner is implemented for Linux only",
	}}, nil
}
