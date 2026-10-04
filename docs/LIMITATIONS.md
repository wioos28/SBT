# Known limitations

SBT reports its isolation claims by measuring the host, not by assuming a
kernel feature exists. This file records the places where SBT deliberately does
less than it could, and the places where it cannot do what the documentation
elsewhere might suggest.

Read this as a list of reasons SBT will refuse to run something, or will run it
with less protection than you asked for. Anything not listed here is described
in [SECURITY.md](SECURITY.md).

## A sandbox command sees the host filesystem read-only

The jail rootfs is assembled from recursive, read-only binds of `/usr`, `/etc`,
`/bin` and `/sbin`, plus writable tmpfs mounts at `/tmp`, `/workspace`, `/var`
and `/run`. Consequences:

- A command cannot install a package, write to `/etc`, or replace a binary.
- Only the host paths SBT names are visible. A toolchain installed somewhere
  unusual is not in the jail.
- The writable areas are tmpfs: they live in memory and vanish with the sandbox.
  A workspace larger than its configured limit fails rather than spilling onto
  the host disk.

## Files leave the sandbox only after the command exits

A sandboxed command writes into a tmpfs mounted at `/workspace`. The SBT helper
- which runs as pid 1 inside the sandbox, not as the sandboxed process - copies
the changed files onto the host *after* the command has exited.

This is the isolation model, not an optimisation. The sandboxed process never
holds a descriptor to any host directory, so it cannot write to the host at any
point in its life, including while it is running.

The cost is that a long-running command produces no visible output in SBT until
it finishes. The interface shows live CPU, memory and process counts while it
runs, and the file list appears afterwards.

## overlayfs is probed but not used

`internal/platform/linux/nsprobe` mounts a real overlay with a memory-backed
upper layer to find out whether the host supports one. `sbt doctor` reports the
result.

SBT does not use it. A copy-on-write overlay would be a natural fit for the
workspace, and it is deliberately declined:

- **The probe's answer is not the run's guarantee.** Overlay support varies by
  filesystem, mount options and kernel patch level in ways a one-shot probe at
  startup cannot fully capture. SBT would rather use the mechanism it can verify
  by construction.
- **The capture step still has to happen.** The sandboxed process has to be
  unable to reach the host regardless of which filesystem backs its workspace.
  An overlay lowers the cost of the copy; it does not remove the requirement that
  a trusted component perform it.
- **Memory-backed uppers are bounded and can fail silently under pressure.**
  A workspace that exhausts its upper layer mid-run produces an incomplete
  result, which is exactly the failure mode SBT refuses to produce.

SBT uses tmpfs plus an explicit copy instead. The copy is bounded
(`CaptureLimitBytes`, `CaptureMaxFiles`) and hitting either bound is reported as
a truncated run - never as a clean one.

## Resource limits are per process, not per sandbox

`RLIMIT_AS`, `RLIMIT_CPU`, `RLIMIT_NOFILE` and `RLIMIT_NPROC` are applied inside
the jail. `RLIMIT_NPROC` counts per uid, which inside a user namespace with a
single mapped user is effectively per sandbox; the memory and CPU limits are
per process. A sandboxed command that forks many children is bounded by
`RLIMIT_NPROC` and by the pid namespace, not by `RLIMIT_AS` in aggregate.

## cgroup limits are probed, not enforced

`cgroup v2` support is detected and reported. SBT does not write to cgroups.
Enforcement is left to rlimits, which are verifiable per process and require no
privileges. A sandbox that needs whole-cgroup accounting should be run under an
external runtime.

## The seccomp filter is a denylist

`internal/platform/linux/seccomp` installs a filter that denies the syscalls
SBT knows are dangerous in a jail (`mount`, `pivot_root`, `ptrace`, the module
and keyring interfaces, and so on). It is not an allowlist.

This is a real limitation and is reported as such: `sbt doctor` shows the seccomp
feature as `Limited`, never as fully isolated. A denylist cannot catch a syscall
nobody thought of, and the number of syscalls a modern libc uses keeps growing.

## No GPU, no devices beyond the basics

Only `/dev/null`, `/dev/zero`, `/dev/urandom` and `/dev/random` are provided.
There is no `/dev/shm`, no `/dev/fd`, no GPU device and no `/sys`. A command
that needs a PTY, a DRI device or a shared-memory segment will fail.

## Interactive programs work, but with a caveat

A command that reads from the terminal gets the real terminal: the interface
suspends the alternate screen, the sandbox inherits stdin, stdout and stderr,
and the interface redraws when the command exits. Programs that redraw heavily,
or that assume a controlling terminal with specific capabilities, may behave
differently than they would in a plain shell.

## Non-Linux hosts: the interface runs, the sandbox does not

SBT builds and its interface runs on Windows and macOS. What does not exist is
a verified sandbox backend for those platforms, so **starting a command is
refused** there with the reason spelled out in the interface.

This is a deliberate limit, not an oversight:

- **SBT has never measured isolation on those platforms, so it does not claim
  any.** A green "PROTECTED" badge over a command that ran without confinement
  would be the worst possible failure mode: the user would have every reason to
  trust it.
- **A working sandbox backend for Windows or macOS is a real project**, not a
  build tag. Windows would mean Job Objects, restricted tokens and a filesystem
  filter; macOS would mean a seatbelt profile or a VM. Both have to be *probed*
  the way the Linux backend is, or the guarantee is decorative.
- **Shipping an unverified backend would make SBT less safe, not more.** Every
  claim in the interface is supposed to be backed by a measurement.

What *does* work everywhere: the whole interface, the menu, the workspace and
change review, the diff viewer, the export flow and `sbt doctor`. The journal
stores before/after copies on the host, so reviewing a session produced
elsewhere needs no isolation.

On a platform with no backend, the cage renders an explicit "sandbox
unavailable" verdict rather than a quiet "ready", and `sbt doctor` explains
which platform it could not build a jail for.

### What a backend would have to do

`internal/shell` has exactly one platform-specific seam: `startHelper`. A new
backend is a file that implements it, plus a probe that establishes - at
runtime - that the isolation it claims is real. Everything else in the session
is already shared.

## Raw terminal mode is unavailable off Unix

On Windows the interface cannot switch the console into raw mode through the
standard library, so the full-screen cage does not start and SBT falls back to
the line-oriented session (`sbt shell`). This is stated rather than hidden: the
session prints why it degraded. A Windows build with real console mode support
would need either a platform-specific termios equivalent or a dependency the
project currently does not take.

Escape handling is portable: Windows waits on the console handle itself, so
`alt+<key>` bindings work there rather than being silently dropped.

## The workspace is temporary

A session's files live in a directory under the system temporary directory,
created with mode `0700`. It is **not** cleaned up when SBT exits, because the
files may be the only copy of a command's output. The path is printed in the
session summary and shown in the interface's navigation rail.

Clean it up yourself once you have exported what you need, or use
**Workspace → Discard workspace…** inside the session.