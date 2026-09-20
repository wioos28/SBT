//go:build !linux

package platform

import "runtime"

// probePlatform reports honestly that SBT cannot create Linux namespaces on this
// platform. The compatibility backend is selected instead.
func probePlatform() (map[string]Feature, string, error) {
	reason := "Linux namespaces are not available on " + runtime.GOOS +
		". SBT supports this platform in compatibility mode only; no filesystem, process or network isolation is enforced."
	out := map[string]Feature{}
	for _, key := range []string{
		"user_namespace", "mount_namespace", "filesystem_jail", "pid_namespace",
		"network_namespace", "capability_drop", "seccomp", "resource_limits", "cgroup_v2",
	} {
		out[key] = Feature{Level: Unavailable, Reason: reason}
	}
	return out, "compat", nil
}

// SelfPath returns the path of the running SBT executable.
func SelfPath() (string, error) {
	return execSelf()
}
