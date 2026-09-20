//go:build linux

package platform

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/wioos28/sbt/internal/platform/linux/nsprobe"
	"github.com/wioos28/sbt/internal/shared/helpermode"

	// The capability probe re-executes this binary through SBT's helper
	// dispatcher, so the dispatcher must be linked into every binary that probes
	// (including test binaries).
	_ "github.com/wioos28/sbt/internal/internalhelper"
)

// ProbeTimeout bounds every helper invocation used for capability detection.
const ProbeTimeout = 20 * time.Second

// ProbeIsolation runs the Linux capability probe and converts its report into
// platform features. The returned backend name is empty when the host cannot
// provide the isolation SBT needs.
func probePlatform() (map[string]Feature, string, error) {
	self, err := SelfPath()
	if err != nil {
		return unavailableAll("cannot locate the SBT executable: " + err.Error()), "", err
	}
	out, err := runHelperProbe(self, helpermode.ModeProbe)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(out) > 0 {
			// The probe produced a report but exited non-zero: use the report.
		} else {
			return unavailableAll(probeFailureReason(err)), "", nil
		}
	}
	rep, perr := nsprobe.Parse(out)
	if perr != nil {
		return unavailableAll(perr.Error()), "", nil
	}
	features := map[string]Feature{
		"user_namespace":    toFeature(rep.Features["user_namespace"]),
		"mount_namespace":   toFeature(rep.Features["mount_namespace"]),
		"filesystem_jail":   toFeature(rep.Features["filesystem_jail"]),
		"pid_namespace":     toFeature(rep.Features["pid_namespace"]),
		"network_namespace": toFeature(rep.Features["network_namespace"]),
		"capability_drop":   capDropFeature(rep),
		"seccomp":           toFeature(rep.Features["seccomp"]),
		"resource_limits":   toFeature(rep.Features["resource_limits"]),
		"cgroup_v2":         toFeature(rep.Features["cgroup_v2"]),
	}
	if f, ok := rep.Features["overlayfs"]; ok {
		features["overlayfs"] = toFeature(f)
	}
	if rep.Error != "" {
		return features, "", errors.New(rep.Error)
	}
	backend := "linux-namespaces"
	if features["user_namespace"].Level != Full || features["mount_namespace"].Level != Full {
		backend = ""
	}
	if len(rep.Notes) > 0 {
		// Notes are surfaced through the feature reasons; nothing to do here.
		_ = rep.Notes
	}
	return features, backend, nil
}

// capDropFeature reports whether SBT can really drop capabilities. It is derived
// from the user namespace probe: inside a user namespace the helper holds a full
// capability set and prctl/capset are always available to drop it.
func capDropFeature(rep *nsprobe.Report) Feature {
	if rep.Features["user_namespace"].Level != nsprobe.Full {
		return Feature{Level: Unavailable, Reason: "requires a user namespace to hold the capabilities SBT drops"}
	}
	return Feature{Level: Full, Reason: "capset + PR_CAPBSET_DROP verified available inside the user namespace"}
}

func toFeature(f nsprobe.Feature) Feature {
	return Feature{Level: Level(f.Level), Reason: f.Reason}
}

func unavailableAll(reason string) map[string]Feature {
	out := map[string]Feature{}
	for _, key := range []string{
		"user_namespace", "mount_namespace", "filesystem_jail", "pid_namespace",
		"network_namespace", "capability_drop", "seccomp", "resource_limits", "cgroup_v2",
	} {
		out[key] = Feature{Level: Unavailable, Reason: reason}
	}
	return out
}

func probeFailureReason(err error) string {
	msg := err.Error()
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		return "the kernel refused to create a user namespace (unprivileged user namespaces are disabled): " + msg
	}
	if strings.Contains(msg, "signal: killed") {
		return "the isolation probe was killed by the kernel (seccomp or LSM policy)"
	}
	if errors.Is(err, exec.ErrNotFound) {
		return "the SBT helper process could not be executed: " + msg
	}
	return "the isolation probe failed: " + msg
}

// SelfPath returns the path of the running SBT executable. Inside the jail SBT
// is re-executed through /proc/self/exe so that it keeps working after chroot.
func SelfPath() (string, error) {
	if _, err := os.Stat("/proc/self/exe"); err == nil {
		return "/proc/self/exe", nil
	}
	return execSelf()
}

// ProbeAvailableNext reports whether the kernel allows creating a user
// namespace, read from /proc/sys when the file exists.
func ProbeAvailableNext() (bool, string) {
	if v, ok := readSysctl("/proc/sys/kernel/unprivileged_userns_clone"); ok {
		return v == "1", "unprivileged_userns_clone=" + v
	}
	if v, ok := readSysctl("/proc/sys/user/max_user_namespaces"); ok {
		return v != "0", "max_user_namespaces=" + v
	}
	return true, "no sysctl exposing user namespace policy was found; the runtime probe decides"
}
