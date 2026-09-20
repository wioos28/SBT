//go:build linux

// Package nsprobe verifies at runtime which Linux isolation primitives actually
// work on this host.
//
// The probe runs as a helper process that the parent started with
// CLONE_NEWUSER|CLONE_NEWPID, so by the time this code runs it is already pid 1
// of a private pid namespace and holds full capabilities inside its own user
// namespace. It tests the real kernel behaviour - mounts, forks, limits, seccomp
// - instead of guessing from kernel versions, and it never touches the host: all
// mounts happen in its own private mount namespace.
package nsprobe

import (
	"encoding/json"
	"io"
	"os"
	"syscall"
)

// Level mirrors platform.Level (0 full, 1 partial, 2 unavailable, 3 unknown).
type Level int

// Capability levels.
const (
	Full        Level = 0
	Partial     Level = 1
	Unavailable Level = 2
	Unknown     Level = 3
)

// Feature is one probed capability.
type Feature struct {
	Level  Level  `json:"level"`
	Reason string `json:"reason,omitempty"`
}

// Report is the probe result consumed by internal/platform.
type Report struct {
	Features map[string]Feature `json:"features"`
	Backend  string             `json:"backend"`
	Notes    []string           `json:"notes,omitempty"`
	Error    string             `json:"error,omitempty"`
}

// Run executes the probe and writes its JSON report to w.
func Run(w io.Writer) int {
	rep := &Report{Features: map[string]Feature{}, Backend: "linux-namespaces"}
	// The probe root is created and owned by the parent, which removes it after
	// this helper exits. SBT never walks a directory tree that has a procfs or
	// tmpfs mounted inside it, because that would recurse into the mounted
	// filesystem instead of the directory.
	root := os.Getenv("SBT_PROBE_ROOT")
	if root == "" {
		dir, derr := os.MkdirTemp("", "sbt-probe-")
		if derr != nil {
			rep.Error = "cannot create probe directory: " + derr.Error()
			emit(w, rep)
			return 1
		}
		root = dir
	} else if err := os.MkdirAll(root, 0o700); err != nil {
		rep.Error = "cannot create probe directory " + root + ": " + err.Error()
		emit(w, rep)
		return 1
	}

	// Optional debug gating (used only by developer tests): with
	// SBT_DEBUG_PROBE_SUBSET set, the probe stops after the given tier so that a
	// hang can be attributed to a specific group of checks. Unset means: run all.
	limit := 0
	switch os.Getenv("SBT_DEBUG_PROBE_SUBSET") {
	case "1":
		limit = 1
	case "2":
		limit = 2
	case "3":
		limit = 3
	case "4":
		limit = 4
	}
	include := func(tier int) bool { return limit == 0 || tier <= limit }

	// Process isolation: the parent created the pid namespace, so this helper
	// must be pid 1. Everything else here depends on it being alive.
	if os.Getpid() == 1 {
		rep.Features["pid_namespace"] = Feature{Level: Full,
			Reason: "helper runs as pid 1 of a private pid namespace created by the parent"}
	} else {
		rep.Features["pid_namespace"] = Feature{Level: Unavailable,
			Reason: "helper is pid " + itoa(os.Getpid()) + ", so no private pid namespace was created"}
	}

	// User namespace: the parent created it with an explicit uid map.
	if data, err := os.ReadFile("/proc/self/uid_map"); err == nil {
		rep.Features["user_namespace"] = Feature{Level: Full,
			Reason: "uid_map " + trim(string(data)) + " (namespace root maps to the calling user)"}
	} else {
		rep.Features["user_namespace"] = Feature{Level: Unavailable,
			Reason: "cannot read /proc/self/uid_map: " + err.Error()}
	}

	// Mount namespace + propagation.
	if include(1) {
		if err := syscall.Unshare(syscall.CLONE_NEWNS); err != nil {
			rep.Features["mount_namespace"] = Feature{Level: Unavailable,
				Reason: "unshare(CLONE_NEWNS) failed: " + err.Error()}
		} else if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
			rep.Features["mount_namespace"] = Feature{Level: Partial,
				Reason: "mount namespace created but propagation could not be made private: " + err.Error()}
		} else {
			target := root + "/mnt"
			if err := os.MkdirAll(target, 0o755); err != nil {
				rep.Features["mount_namespace"] = Feature{Level: Partial, Reason: "cannot create probe mount point: " + err.Error()}
			} else if err := syscall.Mount("tmpfs", target, "tmpfs", 0, "size=1m"); err != nil {
				rep.Features["mount_namespace"] = Feature{Level: Partial, Reason: "unshare works but tmpfs mount failed: " + err.Error()}
			} else {
				rep.Features["mount_namespace"] = Feature{Level: Full,
					Reason: "private mount namespace with private propagation; tmpfs mount verified"}
			}
		}
	}

	// Filesystem jail: read-only recursive bind of a host tree that really
	// refuses writes.
	if include(1) {
		rep.Features["filesystem_jail"] = probeFilesystemJail(root)
	}

	// Network isolation must be tested before seccomp is installed, because the
	// deny list includes unshare(2).
	if include(2) {
		if err := syscall.Unshare(syscall.CLONE_NEWNET); err != nil {
			rep.Features["network_namespace"] = Feature{Level: Unavailable, Reason: "unshare(CLONE_NEWNET) failed: " + err.Error()}
		} else {
			rep.Features["network_namespace"] = probeNetwork()
		}
		_ = syscall.Unshare(syscall.CLONE_NEWIPC)
		if err := syscall.Unshare(syscall.CLONE_NEWUTS); err == nil {
			rep.Features["uts_namespace"] = Feature{Level: Full, Reason: "hostname namespace verified"}
		}

		// Privilege drop: inside the user namespace the helper holds capabilities and
		// must be able to remove them.
		rep.Features["capability_drop"] = probeCaps()
		rep.Features["proc_mount"] = probeProc(root)
	}
	if include(3) {
		rep.Features["resource_limits"] = probeResourceLimits()
		rep.Features["cgroup_v2"] = probeCgroupV2()
		rep.Features["devices"] = probeDevices(root)
		if f, ok := probeOverlay(root); ok {
			rep.Features["overlayfs"] = f
		}
	}
	if include(4) {
		// Seccomp last: it blocks unshare(2) for the rest of this process.
		rep.Features["seccomp"] = probeSeccomp()
	}

	emit(w, rep)
	return 0
}

// Parse decodes a probe report.
func Parse(data []byte) (*Report, error) {
	var rep Report
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil, err
	}
	if rep.Features == nil {
		rep.Features = map[string]Feature{}
	}
	return &rep, nil
}

func emit(w io.Writer, rep *Report) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	_ = enc.Encode(rep)
}
