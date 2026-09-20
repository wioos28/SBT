//go:build linux

package ns

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Sync byte written by the helper over the control pipe once its own half of
// the startup handshake is done. The parent waits for it before writing the
// uid/gid maps into /proc/<pid>/*. 0x53 ('S') is arbitrary but must match the
// helper side.
const mappedSyncByte = 0x53

// MappedSyncByte is exported for the helper packages.
const MappedSyncByte = mappedSyncByte

// WriteMappings is called by the parent after the helper announces itself.
// It writes the identity mappings that turn the helper into namespace root.
// The Go runtime's UidMappings support is not used on purpose: writing the maps
// from the parent before the helper touches the filesystem keeps the fork and
// the map writes independent, which avoids a fork/wait deadlock observed on
// containerized kernels when exec.Command writes the maps itself.
func WriteMappings(pid int) error {
	uid, gid := os.Getuid(), os.Getgid()

	setgroups := fmt.Sprintf("/proc/%d/setgroups", pid)
	if err := os.WriteFile(setgroups, []byte("deny"), 0o644); err != nil {
		return fmt.Errorf("cannot write setgroups: %w", err)
	}
	gidMap := fmt.Sprintf("/proc/%d/gid_map", pid)
	mapping := "0 " + strconv.Itoa(gid) + " 1\n"
	if err := os.WriteFile(gidMap, []byte(mapping), 0o644); err != nil {
		return fmt.Errorf("cannot write gid_map: %w", err)
	}
	uidMap := fmt.Sprintf("/proc/%d/uid_map", pid)
	mapping = "0 " + strconv.Itoa(uid) + " 1\n"
	if err := os.WriteFile(uidMap, []byte(mapping), 0o644); err != nil {
		return fmt.Errorf("cannot write uid_map: %w", err)
	}
	return nil
}

// WaitForHelperReady waits until the helper signals that it can safely receive
// its identity mapping, then writes the mapping.
func WaitForHelperReady(pid int, ready <-chan byte) error {
	select {
	case b := <-ready:
		if b != mappedSyncByte {
			return fmt.Errorf("helper sent an unexpected startup byte 0x%x", b)
		}
	case <-time.After(helperReadyTimeout):
		return fmt.Errorf("helper did not signal readiness within %s", helperReadyTimeout)
	}
	return WriteMappings(pid)
}

// helperReadyTimeout bounds how long the parent waits for the helper handshake.
const helperReadyTimeout = 10 * time.Second
