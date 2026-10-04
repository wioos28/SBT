//go:build linux

package security

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func scanNamespaces(c *collector, base string) {
	areas := []struct{ file, label string }{
		{"mnt", "mount namespace"},
		{"pid", "pid namespace"},
		{"net", "network namespace"},
		{"user", "user namespace"},
		{"ipc", "ipc namespace"},
		{"uts", "uts namespace"},
	}
	for _, n := range areas {
		self, err := os.Readlink(filepath.Join(base, "ns", n.file))
		if err != nil {
			c.add(n.label, StatusNA, "cannot read "+n.file+" namespace: "+err.Error())
			continue
		}
		init, err := os.Readlink(filepath.Join("/proc/1/ns", n.file))
		if err != nil {
			c.add(n.label, StatusWarn, "namespace "+self+" (host namespace unreadable)")
			continue
		}
		if self == init {
			c.add(n.label, StatusWarn, "shares the host "+n.file+" namespace")
			if n.file == "mnt" {
				c.flag(issue(Warning, n.label, "shares the host mount namespace",
					"the target is not in a private mount namespace",
					"Enable mount namespace isolation.", "Remount the host tree read-only.", "Restart the sandbox."))
			}
		} else {
			c.add(n.label, StatusPass, "private "+n.file+" namespace ("+self+")")
		}
	}
}

func scanSeccompCaps(c *collector, base string) {
	status := readStatus(filepath.Join(base, "status"))
	switch status["Seccomp"] {
	case "2":
		c.add("seccomp", StatusPass, "seccomp filter mode 2 (filter) is active")
	case "1":
		c.add("seccomp", StatusWarn, "seccomp is in strict mode")
	case "":
		c.add("seccomp", StatusNA, "seccomp status unreadable")
	default:
		c.add("seccomp", StatusFail, "seccomp is disabled")
		c.flag(issue(Danger, "seccomp", "the syscall filter is not active", "",
			"Enable the seccomp deny list.", "Restart the sandbox."))
	}
	if eff := status["CapEff"]; eff != "" {
		if n, err := strconv.ParseUint(eff, 16, 64); err == nil && n != 0 {
			c.add("capabilities", StatusWarn, "effective capability mask is non-zero (0x"+eff+")")
			c.flag(issue(Warning, "capabilities", "process retains capabilities", "CapEff=0x"+eff,
				"Drop all capabilities before running the command."))
		} else {
			c.add("capabilities", StatusPass, "no effective capabilities")
		}
	}
	if v := status["NoNewPrivs"]; v == "1" {
		c.add("no_new_privs", StatusPass, "PR_SET_NO_NEW_PRIVS is set")
	} else if v != "" {
		c.add("no_new_privs", StatusWarn, "PR_SET_NO_NEW_PRIVS is not set")
	}
}

func scanEnvironment(c *collector, o Options) {
	if !o.ExpectEnvironmentIsolation {
		c.add("environment isolation", StatusNA, "environment isolation not requested")
		return
	}
	bad := unsafeEnv(os.Environ())
	if len(bad) == 0 {
		c.add("environment isolation", StatusPass, "no credential-bearing variables visible")
		return
	}
	c.add("environment isolation", StatusFail, "sensitive variables visible: "+strings.Join(bad, ", "))
	c.flag(issue(Danger, "environment isolation", "unsafe environment variables are visible",
		strings.Join(bad, ", "), "Scrub the environment before starting the sandbox."))
}

func scanMounts(c *collector, o Options, base string) {
	mounts := readMounts(filepath.Join(base, "mountinfo"))
	c.add("mount visibility", StatusPass, itoa(len(mounts))+" mounts visible")
	if sock, ok := hostSocketExposed(mounts); ok {
		c.add("host sockets", StatusFail, "host control socket exposed: "+sock)
		c.flag(issue(Critical, "host sockets", "a host control socket is visible inside the target", sock,
			"Do not expose host runtime sockets to the sandbox.", "Remove the mount.", "Restart the sandbox."))
	}
	for _, m := range mounts {
		if !m.writable || !isHostTree(m.target) || within(m.target, o.ExpectWorkspace) {
			continue
		}
		c.add("writable host path", StatusFail, "writable host mount: "+m.target)
		c.flag(issue(Danger, "filesystem isolation", "a host path is mounted writable", m.target,
			"Remount the host tree read-only.", "Use an isolated writable workspace.", "Restart the sandbox."))
	}
}

func scanNetwork(c *collector, o Options, base string) {
	if !o.ExpectNetworkBlocked {
		c.add("network isolation", StatusNA, "network isolation not requested")
		return
	}
	ifaces := nonLoopbackInterfaces(filepath.Join(base, "net", "dev"))
	if len(ifaces) == 0 {
		c.add("network isolation", StatusPass, "no non-loopback interface is present")
		return
	}
	c.add("network isolation", StatusFail, "unexpected interfaces: "+strings.Join(ifaces, ", "))
	c.flag(issue(Critical, "network isolation", "network interfaces are visible while the policy blocks the network",
		strings.Join(ifaces, ", "), "Enable the network namespace.", "Disable the interface.", "Restart the sandbox."))
}

func scanLimitsAndProc(c *collector, base string) {
	if lids, ok := readLimits(filepath.Join(base, "limits")); ok {
		if lids["Max address space"] {
			c.add("resource limits", StatusNA, "address space is unlimited")
		} else {
			c.add("resource limits", StatusPass, "address space limit is enforced")
		}
	}
	pids, err := countProc("/proc")
	if err != nil {
		return
	}
	c.add("proc exposure", StatusPass, itoa(pids)+" process(es) visible")
	if pids > 200 {
		c.add("process isolation", StatusWarn, "many processes visible ("+itoa(pids)+")")
		c.flag(issue(Warning, "process isolation", "a large number of host processes is visible",
			itoa(pids)+" processes", "Enable the pid namespace.", "Mount a private /proc."))
	}
}
