//go:build linux

package nsprobe

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/wioos28/sbt/internal/platform/linux/caps"
	"github.com/wioos28/sbt/internal/platform/linux/sysnet"
)

// probeFilesystemJail verifies that a read-only recursive bind mount of a host
// system tree succeeds and that writes to it are refused afterwards. This is the
// mechanism SBT uses to expose the host toolchain inside the jail while keeping
// the host files unmodifiable.
func probeFilesystemJail(root string) Feature {
	target := filepath.Join(root, "jailusr")
	if err := os.MkdirAll(target, 0o755); err != nil {
		return Feature{Level: Unavailable, Reason: "cannot create jail target: " + err.Error()}
	}
	if err := syscall.Mount("/usr", target, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return Feature{Level: Unavailable, Reason: "recursive bind of /usr failed: " + err.Error()}
	}
	if err := syscall.Mount("", target, "", syscall.MS_BIND|syscall.MS_REMOUNT|syscall.MS_RDONLY, ""); err != nil {
		return Feature{Level: Partial, Reason: "bind succeeded but read-only remount failed: " + err.Error()}
	}
	if _, err := os.ReadDir(target); err != nil {
		return Feature{Level: Partial, Reason: "bound tree not readable: " + err.Error()}
	}
	probe := filepath.Join(target, ".sbt-write-probe")
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		f.Close()
		_ = os.Remove(probe)
		return Feature{Level: Unavailable,
			Reason: "read-only bind did not prevent writes; SBT refuses to claim filesystem isolation"}
	}
	return Feature{Level: Full, Reason: "read-only recursive bind verified (write refused: " + errText(err) + ")"}
}

// probeNetwork verifies that outbound traffic is refused while loopback can be
// raised inside the private network namespace.
func probeNetwork() Feature {
	loErr := sysnet.BringUpLoopback()
	dialErr := dialOutside()
	switch {
	case dialErr == nil:
		return Feature{Level: Unavailable, Reason: "network namespace did not block outbound connections"}
	case loErr != nil:
		return Feature{Level: Partial, Reason: "outbound traffic refused but loopback unavailable: " + loErr.Error()}
	default:
		return Feature{Level: Full, Reason: "private network namespace: outbound refused (" + dialErr.Error() + "), loopback up"}
	}
}

// probeCaps verifies that capabilities can be dropped from a user namespace root.
func probeCaps() Feature {
	before, err := caps.Current()
	if err != nil {
		return Feature{Level: Unavailable, Reason: "capget failed: " + err.Error()}
	}
	if before == 0 {
		return Feature{Level: Partial, Reason: "helper has no capabilities to drop; nothing to verify"}
	}
	if err := caps.DropAll(); err != nil {
		return Feature{Level: Unavailable, Reason: "capability drop failed: " + err.Error()}
	}
	after, err := caps.Current()
	if err != nil {
		return Feature{Level: Partial, Reason: "capabilities dropped but capget failed afterwards"}
	}
	if after != 0 {
		return Feature{Level: Partial, Reason: "capabilities were not fully cleared"}
	}
	return Feature{Level: Full, Reason: "capset + bounding set drop verified (cleared " + itoa(popcount(before)) + " capability bits)"}
}

// probeProc mounts a private /proc and counts the visible processes.
func probeProc(root string) Feature {
	procDir := filepath.Join(root, "proc")
	if err := os.MkdirAll(procDir, 0o555); err != nil {
		return Feature{Level: Unavailable, Reason: "cannot create /proc mount point: " + err.Error()}
	}
	if err := syscall.Mount("proc", procDir, "proc", 0, ""); err != nil {
		return Feature{Level: Unavailable, Reason: "mount proc failed: " + err.Error()}
	}
	entries, _ := os.ReadDir(procDir)
	visible := 0
	for _, e := range entries {
		if n := e.Name(); len(n) > 0 && n[0] >= '0' && n[0] <= '9' {
			visible++
		}
	}
	if visible <= 8 {
		return Feature{Level: Full, Reason: "private /proc mounted; " + itoa(visible) + " process(es) visible to the sandbox"}
	}
	return Feature{Level: Partial, Reason: "private /proc mounted but " + itoa(visible) + " processes visible"}
}

func trim(s string) string { return strings.TrimSpace(strings.ReplaceAll(s, "\n", " ")) }

func popcount(v uint32) int {
	n := 0
	for v != 0 {
		n += int(v & 1)
		v >>= 1
	}
	return n
}

func errText(err error) string {
	switch e := err.(type) {
	case *os.PathError:
		return e.Err.Error()
	case *os.LinkError:
		return e.Err.Error()
	default:
		return err.Error()
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
