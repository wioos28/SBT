package platform

import "os"

// execSelf returns the absolute path of the running executable.
func execSelf() (string, error) {
	return os.Executable()
}
