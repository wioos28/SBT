//go:build linux

// Package rlimit applies the resource limits of a sandbox spec with setrlimit.
//
// Limits are enforced by the kernel per process, which means SBT can limit
// memory, cpu, file size, open files, processes and core dumps without cgroup
// delegation. Limits that the kernel refuses are reported back to the caller so
// the sandbox status never claims a limit that is not active.
package rlimit

import (
	"fmt"
	"syscall"

	"github.com/wioos28/sbt/internal/shared/jailspec"
)

// Applied describes which limits were really accepted by the kernel.
type Applied struct {
	Memory  bool
	CPU     bool
	File    bool
	NoFiles bool
	Procs   bool
	Core    bool
	Errors  map[string]string
}

// Apply installs every limit in the spec. It never returns an error for a limit
// that could not be lowered; those are recorded in Applied so that the caller
// can display "not enforced" honestly.
func Apply(l jailspec.Limits) Applied {
	a := Applied{Errors: map[string]string{}}
	set := func(res int, name string, cur, max uint64, mark *bool) {
		lim := syscall.Rlimit{Cur: cur, Max: max}
		if cur == 0 {
			lim = syscall.Rlimit{Cur: unlimited(), Max: unlimited()}
		}
		if err := syscall.Setrlimit(res, &lim); err != nil {
			a.Errors[name] = err.Error()
			return
		}
		var got syscall.Rlimit
		if err := syscall.Getrlimit(res, &got); err != nil {
			a.Errors[name] = "set but unreadable: " + err.Error()
			return
		}
		if cur != 0 && got.Cur != cur {
			a.Errors[name] = "kernel adjusted the requested value"
			return
		}
		*mark = true
	}

	if l.MemoryBytes > 0 {
		set(syscall.RLIMIT_AS, "memory", uint64(l.MemoryBytes), uint64(l.MemoryBytes), &a.Memory)
	}
	if l.CPUQuotaSeconds > 0 {
		set(syscall.RLIMIT_CPU, "cpu", uint64(l.CPUQuotaSeconds), uint64(l.CPUQuotaSeconds+5), &a.CPU)
	}
	if l.FileBytes > 0 {
		set(syscall.RLIMIT_FSIZE, "file_size", uint64(l.FileBytes), uint64(l.FileBytes), &a.File)
	}
	if l.OpenFiles > 0 {
		set(syscall.RLIMIT_NOFILE, "open_files", l.OpenFiles, l.OpenFiles, &a.NoFiles)
	}
	if l.Processes > 0 {
		set(rlimitNproc, "processes", l.Processes, l.Processes, &a.Procs)
	}
	if l.CoreDumpBytes >= 0 {
		set(syscall.RLIMIT_CORE, "core_dump", uint64(l.CoreDumpBytes), uint64(l.CoreDumpBytes), &a.Core)
	}
	return a
}

// rlimitNproc is RLIMIT_NPROC (6 on Linux). The Go syscall package does not
// expose this constant, and it is a stable kernel ABI value.
const rlimitNproc = 6

func unlimited() uint64 { return ^uint64(0) }

// Summary renders the applied limits as "memory=on cpu=on ..." for logs.
func (a Applied) Summary() string {
	mark := func(b bool) string {
		if b {
			return "on"
		}
		return "off"
	}
	return fmt.Sprintf("memory=%s cpu=%s file_size=%s open_files=%s processes=%s core_dump=%s",
		mark(a.Memory), mark(a.CPU), mark(a.File), mark(a.NoFiles), mark(a.Procs), mark(a.Core))
}

// Unenforced returns the human readable reasons for limits that are not active.
func (a Applied) Unenforced() []string {
	out := make([]string, 0, len(a.Errors))
	for k, v := range a.Errors {
		out = append(out, k+": "+v)
	}
	return out
}
