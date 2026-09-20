//go:build linux

package nsprobe

import (
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// dialOutside attempts a connection to a public address; it is expected to fail
// inside the sandbox network namespace.
func dialOutside() error {
	conn, err := net.DialTimeout("tcp", "1.1.1.1:53", 700*time.Millisecond)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

func probeOverlay(root string) (Feature, bool) {
	lower := filepath.Join(root, "ovl-lower")
	upperTmp := filepath.Join(root, "ovl-tmp")
	merged := filepath.Join(root, "ovl-merged")
	for _, d := range []string{lower, upperTmp, merged} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return Feature{Level: Unavailable, Reason: "cannot create overlay probe dirs: " + err.Error()}, true
		}
	}
	if err := os.WriteFile(filepath.Join(lower, "base.txt"), []byte("base\n"), 0o644); err != nil {
		return Feature{Level: Unavailable, Reason: "cannot write overlay lower probe file"}, true
	}
	if err := syscall.Mount("tmpfs", upperTmp, "tmpfs", 0, "size=4m"); err != nil {
		return Feature{Level: Unavailable, Reason: "memory-backed upper unavailable: " + err.Error()}, true
	}
	sub := filepath.Join(upperTmp, "u")
	subw := filepath.Join(upperTmp, "w")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		return Feature{Level: Unavailable, Reason: "cannot create overlay upper dir"}, true
	}
	if err := os.MkdirAll(subw, 0o755); err != nil {
		return Feature{Level: Unavailable, Reason: "cannot create overlay work dir"}, true
	}
	opts := "lowerdir=" + lower + ",upperdir=" + sub + ",workdir=" + subw
	if err := syscall.Mount("overlay", merged, "overlay", 0, opts); err != nil {
		return Feature{Level: Unavailable, Reason: "overlayfs with a memory-backed upper is unavailable here: " + err.Error()}, true
	}
	err := os.WriteFile(filepath.Join(merged, "new.txt"), []byte("x\n"), 0o644)
	_ = syscall.Unmount(merged, 0)
	if err != nil {
		return Feature{Level: Partial, Reason: "overlayfs mounted but writes failed: " + err.Error()}, true
	}
	return Feature{Level: Partial, Reason: "overlayfs available with a memory-backed upper; SBT v0.0.1 does not use it (see docs/LIMITATIONS.md)"}, true
}

func probeDevices(root string) Feature {
	devDir := filepath.Join(root, "dev")
	if err := os.MkdirAll(devDir, 0o755); err != nil {
		return Feature{Level: Unavailable, Reason: "cannot create /dev mount point"}
	}
	if err := syscall.Mount("tmpfs", devDir, "tmpfs", 0, "size=1m"); err != nil {
		return Feature{Level: Partial, Reason: "cannot create a private /dev (tmpfs mount failed): " + err.Error()}
	}
	for _, name := range []string{"null", "zero", "full", "random", "urandom", "tty", "ptmx"} {
		src := filepath.Join("/dev", name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		target := filepath.Join(devDir, name)
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, 0o666)
		if err != nil {
			continue
		}
		f.Close()
		if err := syscall.Mount(src, target, "", syscall.MS_BIND, ""); err != nil {
			return Feature{Level: Partial, Reason: "cannot bind host device nodes into the jail: " + err.Error()}
		}
	}
	if err := syscall.Mount("/dev/pts", filepath.Join(devDir, "pts"), "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return Feature{Level: Partial, Reason: "device nodes available but no pty support: " + err.Error()}
	}
	fd, err := syscall.Open(filepath.Join(devDir, "ptmx"), syscall.O_RDWR, 0)
	if err != nil {
		return Feature{Level: Partial, Reason: "pty allocation failed: " + err.Error()}
	}
	syscall.Close(fd)
	return Feature{Level: Partial, Reason: "device nodes are bind mounted from the host kernel rather than emulated"}
}
