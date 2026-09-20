//go:build linux

package jail

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/wioos28/sbt/internal/shared/jailspec"
)

// buildRootfs performs every mount described by the spec inside the sandbox
// mount namespace. Mount failures for non-optional entries abort the sandbox:
// SBT never continues with a partially applied security policy.
func buildRootfs(spec *jailspec.Spec) error {
	root, err := filepath.Abs(spec.Rootfs)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("cannot create sandbox rootfs %s: %w", root, err)
	}
	for _, b := range spec.Binds {
		if err := applyBind(b); err != nil {
			if b.Optional {
				continue
			}
			return fmt.Errorf("mount %s: %w", b.Target, err)
		}
	}
	return nil
}

func applyBind(b jailspec.Bind) error {
	target := b.Target
	if b.FSType == "" {
		// Bind mounts need the mount point to exist.
		if _, err := os.Stat(target); err != nil {
			if _, serr := os.Stat(b.Source); serr != nil {
				return fmt.Errorf("source %s does not exist: %w", b.Source, serr)
			}
			fi, serr := os.Stat(b.Source)
			if serr != nil {
				return serr
			}
			if fi.IsDir() {
				if err := os.MkdirAll(target, 0o755); err != nil {
					return err
				}
			} else {
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
					return err
				}
				f, cerr := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, 0o666)
				if cerr != nil {
					return cerr
				}
				_ = f.Close()
			}
		}
		flags := uintptr(syscall.MS_BIND)
		if b.Recursive {
			flags |= syscall.MS_REC
		}
		if err := syscall.Mount(b.Source, target, "", flags, ""); err != nil {
			return err
		}
		if !b.ReadOnly && !b.NoSuid && !b.NoDev && !b.NoExec {
			return nil
		}
		remount := uintptr(syscall.MS_BIND | syscall.MS_REMOUNT)
		if b.ReadOnly {
			remount |= syscall.MS_RDONLY
		}
		if b.NoSuid {
			remount |= syscall.MS_NOSUID
		}
		if b.NoDev {
			remount |= syscall.MS_NODEV
		}
		if b.NoExec {
			remount |= syscall.MS_NOEXEC
		}
		return syscall.Mount("", target, "", remount, "")
	}

	data := mountData(b)
	flags := uintptr(0)
	if b.ReadOnly {
		flags |= syscall.MS_RDONLY
	}
	if b.NoSuid {
		flags |= syscall.MS_NOSUID
	}
	if b.NoDev {
		flags |= syscall.MS_NODEV
	}
	if b.NoExec {
		flags |= syscall.MS_NOEXEC
	}
	if b.FSType == "proc" {
		flags |= syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC
	}
	return syscall.Mount(b.Source, target, b.FSType, flags, data)
}

func mountData(b jailspec.Bind) string {
	if b.FSType != "tmpfs" {
		return ""
	}
	parts := []string{}
	if b.SizeBytes > 0 {
		parts = append(parts, fmt.Sprintf("size=%d", b.SizeBytes))
	}
	if b.ModeBits > 0 {
		parts = append(parts, fmt.Sprintf("mode=%#o", b.ModeBits))
	}
	return strings.Join(parts, ",")
}

// EnsureDir creates a directory inside the sandbox rootfs, creating parents.
func EnsureDir(root, rel string, mode os.FileMode) error {
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(p, mode); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return nil
}
