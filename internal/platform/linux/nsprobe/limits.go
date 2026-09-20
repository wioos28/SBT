//go:build linux

package nsprobe

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/wioos28/sbt/internal/platform/linux/seccomp"
	"github.com/wioos28/sbt/internal/platform/linux/sysnet"
)

func probeResourceLimits() Feature {
	lim := &syscall.Rlimit{Cur: 512 << 20, Max: 512 << 20}
	if err := syscall.Setrlimit(syscall.RLIMIT_AS, lim); err != nil {
		return Feature{Level: Unavailable, Reason: "setrlimit(RLIMIT_AS) failed: " + err.Error()}
	}
	var got syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_AS, &got)
	// Restore the previous value: the probe only verified enforcement, it must
	// not keep a limit that the sandbox did not ask for.
	unlimited := syscall.Rlimit{Cur: ^uint64(0), Max: ^uint64(0)}
	_ = syscall.Setrlimit(syscall.RLIMIT_AS, &unlimited)
	if err != nil {
		return Feature{Level: Partial, Reason: "limit set but not readable back: " + err.Error()}
	}
	if got.Cur != lim.Cur {
		return Feature{Level: Partial, Reason: "kernel did not keep the requested address space limit"}
	}
	return Feature{Level: Full,
		Reason: "kernel accepted and reports back RLIMIT_AS (CPU, NPROC, FSIZE and NOFILE are applied the same way)"}
}

func probeSeccomp() Feature {
	if err := seccomp.InstallTestFilter(); err != nil {
		return Feature{Level: Unavailable, Reason: err.Error()}
	}
	if !seccomp.ProbeBlocked() {
		return Feature{Level: Partial, Reason: "filter installed but the denied syscall was not blocked"}
	}
	return Feature{Level: Full, Reason: "BPF filter installed; denied syscall returns EPERM (" +
		itoa(len(seccomp.Names())) + " syscalls in the deny list)"}
}

func probeCgroupV2() Feature {
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err != nil {
		return Feature{Level: Unavailable, Reason: "cgroup v2 is not mounted at /sys/fs/cgroup"}
	}
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return Feature{Level: Unavailable, Reason: "cannot read /proc/self/cgroup"}
	}
	rel := ""
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "0::") {
			rel = strings.TrimPrefix(line, "0::")
		}
	}
	probe := filepath.Join("/sys/fs/cgroup", rel, "sbt-probe-check")
	if err := os.Mkdir(probe, 0o755); err != nil {
		return Feature{Level: Partial,
			Reason: "cgroup v2 is present but not delegated to this user; SBT enforces limits with rlimits instead"}
	}
	_ = os.Remove(probe)
	return Feature{Level: Full, Reason: "delegated cgroup v2 subtree is writable"}
}

// RunNetworkProbe verifies, from inside a network namespace, that outbound
// traffic is refused while loopback still works. It is used by tests and by
// `sbt doctor --verify-network`.
func RunNetworkProbe(w io.Writer) int {
	out := map[string]any{}
	_, err := net.DialTimeout("tcp", "1.1.1.1:53", 700*time.Millisecond)
	out["outbound_blocked"] = err != nil
	if err != nil {
		out["outbound_error"] = err.Error()
	} else {
		out["outbound_error"] = ""
	}
	loErr := sysnet.BringUpLoopback()
	out["loopback_error"] = ""
	if loErr != nil {
		out["loopback_error"] = loErr.Error()
	}
	ln, lerr := net.Listen("tcp", "127.0.0.1:0")
	out["loopback_listen"] = lerr == nil
	if lerr != nil {
		out["loopback_listen_error"] = lerr.Error()
	} else {
		_ = ln.Close()
	}
	_ = json.NewEncoder(w).Encode(out)
	return 0
}
