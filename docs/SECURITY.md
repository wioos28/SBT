# SBT security model

SBT runs commands inside an isolation it verifies at runtime. This document
describes what is enforced, how it is verified, and what SBT refuses to do.

For what SBT deliberately does *not* do, see [LIMITATIONS.md](LIMITATIONS.md).

## The core rule

> SBT never claims isolation it has not measured.

Every capability the interface displays comes from a runtime probe, not from a
build tag, a distro guess or a configuration file. A feature that cannot be
verified is reported as `Unavailable` or `Unknown`, and the corresponding
isolation is disabled rather than assumed.

`internal/platform` is the only source of truth. When a host's answer changes
while a session is open - a namespace limit is lowered, a policy is denied - the
report is re-read rather than cached for the life of the process. A stale
"protected" claim would be a lie, so it is never cached.

## What the probe actually does

`platform.Detect()` does not read `/proc` and guess. It re-executes the SBT
binary as a short-lived helper in a fresh user namespace and pid namespace and
*attempts the operations it is about to claim*. Namespace creation, mounting,
capability dropping, seccomp installation and a network isolation check are all
performed for real, and the outcome is what gets reported.

The probe refuses to report a feature it did not exercise. An empty report can
never render as "protected"; it renders as `Limited` with the reason "the probe
did not verify every feature the cage needs".

## The two features SBT will not start without

The jail is built from a user namespace and a mount namespace. Without both there
is no filesystem isolation and no process isolation. If either is `Unavailable`,
`start` refuses:

```
this host cannot create a user namespace (...); SBT refuses to start a pretend sandbox
```

There is no degraded mode, no "best effort" run and no warning-then-continue.
A command that was not isolated would be indistinguishable, in the interface,
from one that was - and that is precisely the failure this project exists to
prevent.

## What a sandbox enforces

For each run, the SBT helper (`internal/platform/linux/jail`) does the following
as pid 1 of the sandbox's pid namespace:

1. Unshares the mount, network, ipc and uts namespaces.
2. Makes mount propagation private, so no mount can propagate back to the host.
3. Builds a rootfs from read-only binds of the host toolchain plus tmpfs scratch
   mounts at `/tmp`, `/workspace`, `/var` and `/run`, and a private `/proc`.
4. `chroot`s into it.
5. Applies rlimits (`RLIMIT_AS`, `RLIMIT_CPU`, `RLIMIT_NOFILE`, `RLIMIT_NPROC`,
   `RLIMIT_FSIZE`).
6. Drops every capability from the bounding and effective sets.
7. Sets `PR_SET_NO_NEW_PRIVS`.
8. Installs the seccomp denylist.
9. Runs the command and reaps orphans.

Every one of these steps is mandatory. A failure at any step reports which
feature failed and why, and the command does not run. SBT never continues with a
partially enforced policy.

### Two details that are load bearing

**`CLONE_NEWUSER` and `CLONE_NEWPID` are requested by the *parent*, together.**
If a multithreaded process called `unshare(CLONE_NEWPID)` itself, a runtime
thread could become pid 1 of the new namespace; when that thread exited, the
kernel would tear the namespace down and every later `fork` would fail with
`ENOMEM`. Requesting both in the clone call makes the helper pid 1 from the
start.

**`CLONE_NEWNS` is *not* requested by the parent.** A mount namespace created in
the same clone call as a user namespace belongs to the *parent's* user
namespace, and mounts inside it are then refused with `EPERM`. The helper
unshares the mount namespace itself, after it is already namespace root.

### The uid/gid handshake

The Go runtime's `UidMappings` are deliberately not used. Writing the maps from
the parent, after the helper signals readiness over a socketpair, keeps the fork
and the map writes independent. Letting `exec.Command` write the maps from a
fork helper thread deadlocked the process against the child on containerized
kernels: the child stayed a zombie and the parent blocked in `waitid` forever.
## How files leave the sandbox

A sandboxed command writes into a tmpfs mounted at `/workspace`. It never holds
a descriptor to a host directory.

When the command exits, the SBT helper - trusted code, running outside the
sandboxed process - compares the tmpfs against the baseline it recorded while
seeding, and copies the changed files into a host directory. The copy is bounded
by `CaptureLimitBytes` and `CaptureMaxFiles`; hitting either bound marks the run
truncated. SBT never silently drops a change.

The workspace is seeded in the same way before the command starts, so a session
accumulates files across runs.

### Path handling

Every path that comes from a manifest or from the interface is validated with
`workspace.ValidPath` and resolved through an `os.Root`. Anything that is not
already a clean, relative, traversal-free path is rejected outright rather than
normalised - rewriting `../x` into `x` would hide a hostile manifest instead of
reporting it.

Exports open the destination through an `os.Root`, create files with `O_EXCL`,
and remove an existing file before replacing it. A symlink at the destination is
replaced, never followed. Non-regular sources are refused. Execute bits are
preserved and nothing is ever executed.

### Executables get a warning, not a block

Exporting a binary is a legitimate thing to want, so SBT does not forbid it. It
does require the user to acknowledge it: the export confirmation names the
number of executable files in the selection before anything is written.

## Hidden helper modes

SBT re-executes itself with `SBT_INTERNAL_MODE` set to reach code that must run
in a particular namespace context. These modes are not part of the CLI surface
and are not reachable by passing an argument:

| Mode | Purpose |
| --- | --- |
| `probe` | Verify namespace, mount, capability and seccomp support. |
| `jail` | Build the jail and run one command inside it. |
| `netprobe` | Verify that the network policy really blocks connections. |

The spec is passed as a JSON file rather than through the environment, because
the environment is visible from inside the sandbox.

An unknown mode exits `127` rather than running anything unexpected.

## The interface cannot grant itself permissions

The rule that shapes the code: **the UI asks, the session decides.**

- `internal/tui` renders snapshots and queues `Request` values. It never starts a
  process, reads a file outside the buffers it draws, or changes a policy.
- `internal/tui/session.go` answers those requests. It is the only component that
  decides whether a run is allowed.
- `internal/shell/runner.go` performs the work: it checks the probe, builds the
  jail spec, spawns the helper, reaps it and records what changed.

This is enforced structurally, not by convention: the runner is an interface the
session holds, the interface's keyboard layer is a pure function from key to
event, and no key handler can reach the kernel.

## Every destructive action asks first

Discarding the workspace, exiting with unexported changes, switching to the high
risk policy, and exporting executables all pass through a confirmation.

The dialog's dangerous choice is always the **left** button and the safe choice
always the right one, so position carries the meaning whether or not colour is
available. Escape always resolves to the safe side. A user who is unsure cannot
destroy anything by reflex.

The high risk confirmation lists the concrete knobs that change - "network access
is no longer blocked", "memory limit raised to 2048 MB" - rather than showing the
label. A user confirms facts, not a word.

## What the interface states about itself

- Every state carries a **word**, never only a colour. `PROTECTED`, `LIMITED`,
  `BROKEN`, `HIGH RISK`, `SANDBOX ACTIVE`.
- Degraded isolation is named in the status bar, in the security panel and in the
  session summary, so dismissing a toast cannot make a warning disappear.
- A load that failed says so. A pane that silently shows nothing reads as "this
  file did not change", which is a different and wrong claim.
- `NO_COLOR`, `TERM=dumb` and `SBT_ASCII` degrade colour and glyphs, never
  meaning.
- `SBT_NO_MOTION`, `NO_MOTION` and `SBT_REDUCED_MOTION` collapse every animation
  to its still frame. Honouring them means "draw the still frame", not "draw a
  different interface".

## Reporting a problem

If SBT claims isolation the host does not provide, that is a security bug and
the most serious one this project can have. Please include `sbt doctor` output:
it contains the evidence for every claim SBT makes.

Before the maps are written the helper is an unmapped user with no valid
identity. It therefore touches nothing at all until the parent confirms the maps
are in place; if the confirmation does not arrive, it exits `126` rather than
continuing unmapped.