//go:build linux

package jail

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/wioos28/sbt/internal/platform/linux/rlimit"
	"github.com/wioos28/sbt/internal/platform/linux/sysnet"
	"github.com/wioos28/sbt/internal/shared/jailspec"
)

// setupError carries a structured result back to the parent so the CLI can show
// exactly which isolation feature failed and why.
type setupError struct{ Result jailspec.Result }

func (e *setupError) Error() string {
	return e.Result.Feature + ": " + e.Result.Reason
}

func fail(feature, reason string, err error) error {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	return &setupError{Result: jailspec.Result{Stage: "jail", OK: false, Feature: feature, Reason: reason, Errno: detail}}
}

// enterNamespaces unshares every remaining namespace and prepares loopback when
// the network policy isolates the sandbox.
func enterNamespaces(spec *jailspec.Spec) error {
	// The mount namespace is created here, after the user namespace, so that the
	// kernel treats this helper as the owner of the new mount namespace.
	if err := syscall.Unshare(syscall.CLONE_NEWNS); err != nil {
		return fail("mount_namespace", "Sandbox initialization failed. The current system does not allow the required isolation feature (mount namespace).", err)
	}
	// Never let a mount event reach the host mount namespace.
	if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		return fail("mount_namespace", "Sandbox initialization failed: mount propagation could not be made private.", err)
	}
	if spec.NetworkBlocked {
		if err := syscall.Unshare(syscall.CLONE_NEWNET); err != nil {
			return fail("network_namespace", "Sandbox initialization failed: the current network policy requires network isolation, which this host refuses.", err)
		}
		if err := sysnet.BringUpLoopback(); err != nil {
			// Loopback is a convenience, not a security property: report it and
			// keep running with the network still blocked.
			fmt.Fprintln(os.Stderr, "SBT: loopback could not be enabled inside the sandbox: "+err.Error())
		}
	}
	if err := syscall.Unshare(syscall.CLONE_NEWIPC); err != nil {
		return fail("ipc_namespace", "Sandbox initialization failed: IPC isolation is unavailable.", err)
	}
	if err := syscall.Unshare(syscall.CLONE_NEWUTS); err == nil && spec.Hostname != "" {
		_ = syscall.Sethostname([]byte(spec.Hostname))
	}
	return nil
}

func report(fd int, r jailspec.Result) {
	if fd < 0 {
		return
	}
	_, _ = syscall.Write(fd, jailspec.ResultJSON(r))
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok {
			if ws.Signaled() {
				return 128 + int(ws.Signal())
			}
			return ws.ExitStatus()
		}
		return ee.ExitCode()
	}
	return 2
}

func signalNote(err error) {
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return
	}
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		fmt.Fprintf(os.Stderr, "\nSBT: the command was terminated by signal %s (%d)\n", ws.Signal(), int(ws.Signal()))
	}
}

// reapOrphans collects processes that were reparented to this namespace init.
func reapOrphans() {
	deadline := time.Now().Add(1200 * time.Millisecond)
	for time.Now().Before(deadline) {
		var ws syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &ws, syscall.WNOHANG, nil)
		if pid <= 0 || err != nil {
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func joinUnenforced(a rlimit.Applied) string {
	u := a.Unenforced()
	out := ""
	for i, s := range u {
		if i > 0 {
			out += "; "
		}
		out += s
	}
	return out
}
