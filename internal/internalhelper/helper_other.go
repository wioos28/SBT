//go:build !linux

package internalhelper

// On non-Linux platforms SBT has no namespace helper modes: isolation is not
// available and the compatibility backend is used instead.
func runHelper(mode string) (bool, int) {
	return true, 127
}
