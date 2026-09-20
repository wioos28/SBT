//go:build linux

// Package ns knows how SBT starts helper processes inside new namespaces.
//
// Two details are load bearing and were verified by probing the kernel:
//
//  1. CLONE_NEWUSER and CLONE_NEWPID are requested together by the *parent*.
//     The helper therefore starts life as pid 1 of a private pid namespace. If a
//     multithreaded process called unshare(CLONE_NEWPID) itself, a runtime thread
//     could become pid 1 and, when it exits, the kernel tears the namespace down
//     and every later fork fails with ENOMEM.
//
//  2. CLONE_NEWNS is *not* requested by the parent. A mount namespace created in
//     the same clone call as the user namespace belongs to the parent's user
//     namespace, and mounts inside it are then refused with EPERM. The helper
//     unshares the mount namespace itself instead.
package ns

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// Options describes a helper process to spawn.
type Options struct {
	// Mode is the hidden helper mode (helpermode constants).
	Mode string
	// SpecPath is the JSON spec passed through SBT_INTERNAL_SPEC.
	SpecPath string
	// Control is an optional pipe handed to the helper as file descriptor 3.
	Control *os.File
	// Stdin, Stdout, Stderr default to the process standard streams. The
	// sandboxed command inherits them, so terminal output is not proxied.
	Stdin  *os.File
	Stdout *os.File
	Stderr *os.File
	// ExtraEnv is appended to the helper environment.
	ExtraEnv []string
}

// ControlFD is the file descriptor the helper reads the control pipe from.
const ControlFD = 3

// SysProcAttr returns the attributes used for every helper process.
//
// The uid/gid mapping is NOT written here (no UidMappings/GidMappings fields):
// the parent writes /proc/<pid>/{setgroups,gid_map,uid_map} itself after the
// helper signals readiness, following libcontainer. Letting exec.Command write
// the maps from a fork helper thread deadlocked this process against the child
// on containerized kernels (the child stayed a zombie and the parent blocked in
// waitid forever), so the handshake in SpawnMapped avoids that path entirely.
func SysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWPID,
		// If SBT dies, the sandbox must not survive it.
		Pdeathsig: syscall.SIGKILL,
	}
}

// Spawn starts a helper process. The caller owns Wait on the returned command.
func Spawn(o Options) (*exec.Cmd, error) {
	self, err := SelfPath()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(self)
	env := append(os.Environ(),
		helpermodeModeEnv+"="+o.Mode,
	)
	if o.SpecPath != "" {
		env = append(env, helpermodeSpecEnv+"="+o.SpecPath)
	}
	if o.Control != nil {
		env = append(env, helpermodeControlEnv+"="+itoa(ControlFD))
	}
	env = append(env, o.ExtraEnv...)
	cmd.Env = env
	cmd.SysProcAttr = SysProcAttr()
	cmd.Stdin = pick(o.Stdin, os.Stdin)
	cmd.Stdout = pick(o.Stdout, os.Stdout)
	cmd.Stderr = pick(o.Stderr, os.Stderr)
	if o.Control != nil {
		cmd.ExtraFiles = []*os.File{o.Control}
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cannot start sandbox helper: %w", err)
	}
	return cmd, nil
}

func pick(a, b *os.File) *os.File {
	if a != nil {
		return a
	}
	return b
}

// SelfPath returns the binary to re-execute for helpers.
func SelfPath() (string, error) {
	if _, err := os.Stat("/proc/self/exe"); err == nil {
		return "/proc/self/exe", nil
	}
	return os.Executable()
}

// Started is the result of SpawnMapped: a running helper whose uid/gid maps are
// already written by the parent.
type Started struct {
	// Cmd is the started helper command; the caller owns Wait on it.
	Cmd *exec.Cmd
	// Report is the file the helper writes its JSON report to (probe modes) or
	// the control pipe it reports jail results over.
	StdoutFile *os.File
}

// SpawnMapped starts a helper process in fresh user+pid namespaces and writes
// the uid/gid mapping from this (parent) process once the helper signals that
// it reached the mapping barrier.
//
// The helper must call ReachMappingBarrier (same package) as its very first
// namespaced action: before the maps are written the helper is still an
// unmapped user and must not touch the filesystem.
func SpawnMapped(o Options) (*Started, error) {
	self, err := SelfPath()
	if err != nil {
		return nil, err
	}
	// Bidirectional handshake socket: the helper writes one readiness byte
	// after clone; the parent answers with one byte once the uid/gid maps are
	// written. A single socketpair is enough because the helper both sends
	// and receives; a pipe write end cannot be read back.
	parentSock, childSock, err := socketpair()
	if err != nil {
		return nil, fmt.Errorf("cannot create mapping handshake socket: %w", err)
	}
	defer func() {
		_ = parentSock.Close()
	}()

	cmd := exec.Command(self)
	env := append(os.Environ(),
		helpermodeModeEnv+"="+o.Mode,
	)
	if o.SpecPath != "" {
		env = append(env, helpermodeSpecEnv+"="+o.SpecPath)
	}
	env = append(env, o.ExtraEnv...)
	// The handshake socket is handed to the helper right after the optional
	// control pipe, so it lands on fd 4 with a control pipe and fd 3 without.
	// The env var naming that descriptor must be part of cmd.Env before Start.
	syncFD := ControlFD
	extra := make([]*os.File, 0, 2)
	if o.Control != nil {
		env = append(env, helpermodeControlEnv+"="+itoa(ControlFD))
		extra = append(extra, o.Control)
		syncFD = ControlFD + 1
	}
	env = append(env, mappingSyncFDEnv+"="+itoa(syncFD))
	cmd.Env = env
	cmd.SysProcAttr = SysProcAttr()
	cmd.Stdin = pick(o.Stdin, os.Stdin)
	cmd.Stdout = pick(o.Stdout, os.Stdout)
	cmd.Stderr = pick(o.Stderr, os.Stderr)
	cmd.ExtraFiles = append(extra, childSock)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cannot start sandbox helper: %w", err)
	}
	// The parent no longer needs the child end; keeping it open would hide a
	// peer disconnect from the handshake and could wedge the helper.
	_ = childSock.Close()

	// Wait for the helper's readiness byte, write the maps, then acknowledge
	// over the same socket: the helper blocks on the read until the maps are
	// guaranteed to be in place.
	ready := make(chan byte, 1)
	go func() {
		buf := make([]byte, 1)
		n, rerr := parentSock.Read(buf)
		if rerr != nil || n == 0 {
			ready <- 0
			return
		}
		ready <- buf[0]
	}()
	if err := WaitForHelperReady(cmd.Process.Pid, ready); err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return nil, fmt.Errorf("sandbox helper did not report for mapping (env has %s=%s): %w", mappingSyncFDEnv, itoa(syncFD), err)
	}
	if _, err := parentSock.Write([]byte{MappedSyncByte}); err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return nil, fmt.Errorf("cannot acknowledge mapping to sandbox helper: %w", err)
	}
	return &Started{Cmd: cmd}, nil
}

// MappingSyncFDEnv names the environment variable that tells the helper which
// descriptor carries the mapping handshake socket.
const MappingSyncFDEnv = "SBT_INTERNAL_MAPPING_SYNC_FD"

var mappingSyncFDEnv = MappingSyncFDEnv

// socketpair returns a connected bidirectional AF_UNIX socket pair. Both ends
// are usable for reading and writing, which is what the mapping handshake
// needs: the helper sends readiness, the parent answers with one byte.
func socketpair() (*os.File, *os.File, error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, nil, err
	}
	return os.NewFile(uintptr(fds[0]), "mapping-parent"), os.NewFile(uintptr(fds[1]), "mapping-child"), nil
}

// ReachMappingBarrier is called by the helper: it writes the readiness byte and
// then blocks until the parent confirms that the maps are written. It must be
// the first namespaced action of a helper.
func ReachMappingBarrier() {
	fdStr := os.Getenv(MappingSyncFDEnv)
	if fdStr == "" {
		return
	}
	fd, err := strconv.Atoi(fdStr)
	if err != nil {
		return
	}
	f := os.NewFile(uintptr(fd), "mapping-sync")
	if f == nil {
		return
	}
	// Tell the parent we are ready for the maps.
	if _, err := f.Write([]byte{MappedSyncByte}); err != nil {
		return
	}
	// Block until the parent confirms the maps are written; anything else is
	// treated as a failure so the helper never runs unmapped by accident.
	buf := make([]byte, 1)
	if n, rerr := f.Read(buf); rerr != nil || n != 1 || buf[0] != MappedSyncByte {
		os.Exit(126)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
