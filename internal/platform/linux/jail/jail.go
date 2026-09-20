//go:build linux

// Package jail implements SBT's Linux isolation backend.
//
// The helper process is started by the parent with CLONE_NEWUSER|CLONE_NEWPID,
// so it is already pid 1 of a private pid namespace when this code runs (see
// package ns for why). The helper then:
//
//  1. unshares the mount, network, ipc and uts namespaces (in that order),
//  2. makes mount propagation private so nothing can reach the host,
//  3. builds a read-only rootfs out of recursive bind mounts of the host
//     toolchain plus writable sandbox directories,
//  4. mounts a private /proc, /dev, /tmp and /var,
//  5. chroots into the jail,
//  6. applies rlimits, drops every capability and installs the seccomp deny list,
//  7. forks the user command as pid 2 and supervises it (reaping orphans),
//  8. reports exactly what failed to the parent over a control pipe.
//
// Every step is mandatory unless the spec marks it optional: SBT never continues
// with a partially enforced policy, because a half-built jail would be a security
// claim it cannot back up.
package jail

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/wioos28/sbt/internal/platform/linux/caps"
	"github.com/wioos28/sbt/internal/platform/linux/rlimit"
	"github.com/wioos28/sbt/internal/platform/linux/seccomp"
	"github.com/wioos28/sbt/internal/shared/jailspec"
)

// Run is the helper entry point. It returns the process exit code.
func Run(specPath string, controlFD int) int {
	spec, err := jailspec.Load(specPath)
	if err != nil {
		report(controlFD, jailspec.Result{Stage: "jail", OK: false, Reason: err.Error()})
		return 2
	}
	if len(spec.Command) == 0 {
		report(controlFD, jailspec.Result{Stage: "jail", OK: false, Reason: "no command given"})
		return 2
	}

	if err := enterNamespaces(spec); err != nil {
		var se *setupError
		if errors.As(err, &se) {
			report(controlFD, se.Result)
		} else {
			report(controlFD, jailspec.Result{Stage: "jail", OK: false, Reason: err.Error()})
		}
		return 3
	}
	if err := buildRootfs(spec); err != nil {
		reason := "Sandbox initialization failed while building the isolated filesystem. " + err.Error()
		report(controlFD, jailspec.Result{Stage: "jail", OK: false, Feature: "filesystem_jail", Reason: reason, Errno: err.Error()})
		return 3
	}
	// The workspace handoff directories live on the host, so their descriptors
	// must be opened before the jail is entered. They are O_CLOEXEC and are
	// never passed to the sandboxed command: capture happens after it exits.
	hand, err := openHandoff(spec)
	if err != nil {
		reason := "Sandbox initialization failed: the session workspace could not be opened. " + err.Error()
		report(controlFD, jailspec.Result{Stage: "jail", OK: false, Feature: "workspace_handoff", Reason: reason, Errno: err.Error()})
		return 3
	}
	defer hand.close()
	if err := syscall.Chroot(spec.Rootfs); err != nil {
		reason := "Sandbox initialization failed: chroot into the sandbox rootfs was refused. SBT has NOT started an unsafe fallback."
		report(controlFD, jailspec.Result{Stage: "jail", OK: false, Feature: "filesystem_jail", Reason: reason, Errno: err.Error()})
		return 3
	}
	if err := syscall.Chdir(spec.Cwd); err != nil {
		if cerr := syscall.Chdir("/workspace"); cerr != nil {
			if rerr := syscall.Chdir("/"); rerr != nil {
				report(controlFD, jailspec.Result{Stage: "jail", OK: false, Reason: "cannot enter the sandbox working directory: " + err.Error()})
				return 3
			}
		}
	}

	// Restore the session workspace into the sandbox tmpfs. This happens before
	// the security policy is applied and before any command exists: it only
	// reads host data and writes into the sandbox's own tmpfs.
	if err := hand.seedInto(workspacePath(spec)); err != nil {
		reason := "Sandbox initialization failed: the session workspace could not be restored. " + err.Error()
		report(controlFD, jailspec.Result{Stage: "jail", OK: false, Feature: "workspace_handoff", Reason: reason, Errno: err.Error()})
		return 3
	}

	if spec.NoNewPrivs {
		if err := seccomp.NoNewPrivs(); err != nil {
			report(controlFD, jailspec.Result{Stage: "jail", OK: false, Feature: "no_new_privs",
				Reason: "Sandbox initialization failed: the kernel refused PR_SET_NO_NEW_PRIVS.", Errno: err.Error()})
			return 3
		}
	}
	if spec.DropAllCaps {
		if err := caps.DropAll(); err != nil {
			report(controlFD, jailspec.Result{Stage: "jail", OK: false, Feature: "capability_drop",
				Reason: "Sandbox initialization failed: privileges could not be dropped, so SBT refuses to run the command.",
				Errno:  err.Error()})
			return 3
		}
	}
	if spec.Seccomp {
		if err := seccomp.InstallDenylist(); err != nil {
			report(controlFD, jailspec.Result{Stage: "jail", OK: false, Feature: "seccomp",
				Reason: "Sandbox initialization failed: the syscall filter could not be installed.", Errno: err.Error()})
			return 3
		}
	}

	applied := rlimit.Apply(spec.Limits)

	argv := spec.Command
	child := exec.Command(argv[0], argv[1:]...)
	child.Env = spec.Env
	child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
	started := time.Now()
	if err := child.Start(); err != nil {
		reason := "Cannot execute the command inside the sandbox: " + err.Error()
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			reason = "The sandbox does not provide " + argv[0] +
				". Run `sbt doctor` to see which host tools are exposed inside the sandbox."
		}
		report(controlFD, jailspec.Result{Stage: "jail", OK: false, Reason: reason, Errno: err.Error()})
		fmt.Fprintln(os.Stderr, reason)
		return 127
	}
	report(controlFD, jailspec.Result{
		Stage:    "jail",
		OK:       true,
		ChildPid: child.Process.Pid,
		Reason:   applied.Summary(),
		Errno:    joinUnenforced(applied),
	})

	werr := child.Wait()
	code := exitCode(werr)
	// Copy the changed files out of the sandbox workspace. The command is gone,
	// so this cannot be influenced by it beyond the content it left behind; a
	// failure here is reported and never silently ignored.
	if err := hand.captureRun(spec, code, started); err != nil {
		fmt.Fprintln(os.Stderr, "SBT: the sandbox changes could not be captured: "+err.Error())
	}
	reapOrphans()
	signalNote(werr)
	return code
}
