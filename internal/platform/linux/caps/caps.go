//go:build linux

// Package caps drops Linux capabilities from the sandbox helper before it runs
// user commands. Inside the user namespace the helper holds a full capability
// set; the sandboxed command must not inherit it.
package caps

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	prCapbsetDrop  = 22
	prCapbsetClear = 23
	prCapAmbClear  = 24
	capVersion3    = 0x20080522
	linCapLastCap  = 41
)

type capHeader struct {
	Version uint32
	Pid     int32
}

type capData struct {
	Effective   uint32
	Permitted   uint32
	Inheritable uint32
}

func capsetClear() error {
	hdr := capHeader{Version: capVersion3, Pid: 0}
	data := [2]capData{}
	if _, _, errno := syscall.Syscall(syscall.SYS_CAPSET,
		uintptr(unsafe.Pointer(&hdr)), uintptr(unsafe.Pointer(&data[0])), 0); errno != 0 {
		return fmt.Errorf("capset(clear) failed: %w", errno)
	}
	return nil
}

// DropAll removes capabilities from the bounding set, clears ambient caps and
// clears the effective/permitted/inheritable sets.
//
// This is the last security step before executing a sandboxed command; a
// failure is reported to the caller so the sandbox refuses to start instead of
// running with privileges it claimed to have dropped.
func DropAll() error {
	// Drop everything from the bounding set first: it cannot be re-added.
	for cap := 0; cap <= linCapLastCap; cap++ {
		_, _, errno := syscall.Syscall6(syscall.SYS_PRCTL, prCapbsetDrop, uintptr(cap), 0, 0, 0, 0)
		if errno != 0 && errno != syscall.EINVAL {
			// EINVAL means the capability does not exist on this kernel.
			continue
		}
	}
	if _, _, errno := syscall.Syscall6(syscall.SYS_PRCTL, prCapbsetClear, 0, 0, 0, 0, 0); errno != 0 {
		return fmt.Errorf("prctl(PR_CAPBSET_CLEAR) failed: %w", errno)
	}
	if _, _, errno := syscall.Syscall6(syscall.SYS_PRCTL, prCapAmbClear, 0, 0, 0, 0, 0); errno != 0 {
		return fmt.Errorf("prctl(PR_CAP_AMBIENT_CLEAR_ALL) failed: %w", errno)
	}
	if err := capsetClear(); err != nil {
		return err
	}
	return nil
}

// Current returns the effective capability mask of the calling process.
func Current() (uint32, error) {
	var hdr = capHeader{Version: capVersion3, Pid: 0}
	var data [2]capData
	if _, _, errno := syscall.Syscall(syscall.SYS_CAPGET,
		uintptr(unsafe.Pointer(&hdr)), uintptr(unsafe.Pointer(&data[0])), 0); errno != 0 {
		return 0, errno
	}
	return data[0].Effective, nil
}
