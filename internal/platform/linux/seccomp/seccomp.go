//go:build linux

// Package seccomp installs a small syscall deny list inside the sandbox.
//
// SBT uses a deny list rather than a whitelist on purpose: a whitelist that is
// strict enough to be useful breaks ordinary toolchains (npm, python, compilers,
// git) in ways users cannot debug. The deny list removes the syscalls that would
// let a sandbox escape, load kernel code, debug the host, or add kernel keys.
//
// After installation the filter cannot be removed by the sandboxed process.
package seccomp

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	prSetNoNewPrivs   = 38
	prSetSeccomp      = 22
	seccompModeFilter = 2

	auditArchX86_64  = 0xc000003e
	auditArchAArch64 = 0xc00000b7

	retAllow       = 0x7fff0000
	retKillProcess = 0x80000000
	retErrnoPerm   = 0x00050000 | 1 // SECCOMP_RET_ERRNO | EPERM
)

// syscallNames is the documented deny list, with per-architecture numbers.
type entry struct {
	name  string
	amd64 int
	arm64 int
}

var denylist = []entry{
	{"mount", 165, 40},
	{"umount2", 166, 39},
	{"pivot_root", 155, 41},
	{"chroot", 161, 51},
	{"unshare", 272, 97},
	{"setns", 308, 268},
	{"ptrace", 101, 117},
	{"process_vm_readv", 310, 270},
	{"process_vm_writev", 311, 271},
	{"kexec_load", 246, 104},
	{"kexec_file_load", 320, 294},
	{"init_module", 175, 105},
	{"finit_module", 313, 273},
	{"delete_module", 176, 106},
	{"reboot", 169, 142},
	{"swapon", 167, 224},
	{"swapoff", 168, 225},
	{"acct", 163, 89},
	{"quotactl", 179, 60},
	{"nfsservctl", 180, 42},
	{"add_key", 248, 217},
	{"request_key", 249, 218},
	{"keyctl", 250, 219},
	{"open_by_handle_at", 304, 265},
	{"name_to_handle_at", 303, 264},
	{"bpf", 321, 280},
	{"userfaultfd", 323, 282},
	{"perf_event_open", 298, 241},
	{"iopl", 172, -1},
	{"ioperm", 173, -1},
	{"syslog", 103, 116},
}

// Names returns the documented deny list names.
func Names() []string {
	out := make([]string, 0, len(denylist))
	for _, e := range denylist {
		out = append(out, e.name)
	}
	return out
}

func archNumbers() ([]int, uint32, error) {
	switch runtime.GOARCH {
	case "amd64":
		nums := make([]int, 0, len(denylist))
		for _, e := range denylist {
			if e.amd64 >= 0 {
				nums = append(nums, e.amd64)
			}
		}
		return nums, auditArchX86_64, nil
	case "arm64":
		nums := make([]int, 0, len(denylist))
		for _, e := range denylist {
			if e.arm64 >= 0 {
				nums = append(nums, e.arm64)
			}
		}
		return nums, auditArchAArch64, nil
	default:
		return nil, 0, fmt.Errorf("seccomp deny list is not implemented for %s", runtime.GOARCH)
	}
}

// build constructs the BPF program: architecture check, deny list, allow rest.
func build(nums []int, arch uint32) []syscall.SockFilter {
	f := []syscall.SockFilter{
		{Code: syscall.BPF_LD | syscall.BPF_W | syscall.BPF_ABS, K: 4},            // arch
		{Code: syscall.BPF_JMP | syscall.BPF_JEQ | syscall.BPF_K, Jt: 1, K: arch}, // if arch == expected -> continue
		{Code: syscall.BPF_RET | syscall.BPF_K, K: retKillProcess},                // otherwise kill
		{Code: syscall.BPF_LD | syscall.BPF_W | syscall.BPF_ABS, K: 0},            // syscall nr
	}
	for _, n := range nums {
		f = append(f,
			syscall.SockFilter{Code: syscall.BPF_JMP | syscall.BPF_JEQ | syscall.BPF_K, Jf: 1, K: uint32(n)},
			syscall.SockFilter{Code: syscall.BPF_RET | syscall.BPF_K, K: retErrnoPerm},
		)
	}
	f = append(f, syscall.SockFilter{Code: syscall.BPF_RET | syscall.BPF_K, K: retAllow})
	return f
}

// InstallDenylist sets PR_SET_NO_NEW_PRIVS and installs the deny list filter.
func InstallDenylist() error {
	nums, arch, err := archNumbers()
	if err != nil {
		return err
	}
	if err := noNewPrivs(); err != nil {
		return err
	}
	prog := build(nums, arch)
	fprog := &syscall.SockFprog{Len: uint16(len(prog)), Filter: &prog[0]}
	if _, _, errno := syscall.Syscall6(syscall.SYS_PRCTL, prSetSeccomp, seccompModeFilter,
		uintptr(unsafe.Pointer(fprog)), 0, 0, 0); errno != 0 {
		return fmt.Errorf("prctl(PR_SET_SECCOMP, FILTER) failed: %w", errno)
	}
	return nil
}

// InstallTestFilter installs a filter that denies unshare(2) so the probe can
// verify that the filter is actually enforced.
func InstallTestFilter() error {
	nums, arch, err := archNumbers()
	if err != nil {
		return err
	}
	probeNum := -1
	for _, e := range denylist {
		if e.name == "unshare" {
			probeNum = e.amd64
			if runtime.GOARCH == "arm64" {
				probeNum = e.arm64
			}
		}
	}
	if probeNum < 0 {
		return fmt.Errorf("no probe syscall available")
	}
	if err := noNewPrivs(); err != nil {
		return err
	}
	prog := build([]int{probeNum}, arch)
	fprog := &syscall.SockFprog{Len: uint16(len(prog)), Filter: &prog[0]}
	if _, _, errno := syscall.Syscall6(syscall.SYS_PRCTL, prSetSeccomp, seccompModeFilter,
		uintptr(unsafe.Pointer(fprog)), 0, 0, 0); errno != 0 {
		return fmt.Errorf("prctl(PR_SET_SECCOMP, FILTER) failed: %w", errno)
	}
	_ = nums
	return nil
}

// noNewPrivs sets PR_SET_NO_NEW_PRIVS, which the kernel requires before an
// unprivileged process may install a seccomp filter.
func noNewPrivs() error {
	if _, _, errno := syscall.Syscall6(syscall.SYS_PRCTL, prSetNoNewPrivs, 1, 0, 0, 0, 0); errno != 0 {
		return fmt.Errorf("prctl(PR_SET_NO_NEW_PRIVS) failed: %w", errno)
	}
	return nil
}

// NoNewPrivs is the exported variant used by the jail before running a command.
func NoNewPrivs() error { return noNewPrivs() }

// ProbeBlocked calls unshare(2), which the test filter denies, to verify that
// the filter is enforced by the kernel.
func ProbeBlocked() bool {
	err := syscall.Unshare(0)
	return err == syscall.EPERM || err == syscall.EACCES
}
