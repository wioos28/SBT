# SBT
Sandbox terminal

## Install

```sh
go install github.com/wioos28/sbt@latest   # module builds
# or from a checkout:
go build -o /usr/local/bin/sbt .
```

## Usage

```sh
sbt          # launch the interactive sandbox cage
sbt shell    # the line-oriented session, without a full-screen interface
sbt doctor   # verify platform isolation and host dependencies
sbt version
```

### The cage

Typing `sbt` in a terminal starts the full-screen interface. It verifies the
platform isolation with the runtime probe, then drops you into the cage: a top
bar, a menu bar, the workspace, the live monitor and a status line that always
states what the cage enforces right now.

```
  sbt> echo hello > notes.txt
  sbt> (review it in Workspace → Changes, or alt+3)
  sbt> (export with Workspace → Export changes…)
```

**Keys**

| Key | Does |
| --- | --- |
| `alt+1` … `alt+6` | terminal · files · changes · status · export · help |
| `F10` / `alt+m` | open the menu bar |
| `ctrl+k` | command palette (type to filter) |
| `enter` | run the typed command; on a change, review its diff |
| `esc` | close the overlay, or go back |
| `ctrl+.` | stop the running sandbox |
| `ctrl+d` | leave — always asks first |

The menu bar carries **Session · View · Security · Workspace · Look · Help**.
Every row goes through the same action dispatcher the palette and the key
bindings use, so the three ways of doing a thing can never disagree.

## Platform support

| | Interface | Sandbox |
| --- | --- | --- |
| Linux | full | full, verified at runtime |
| macOS | full | refused — no verified backend |
| Windows | full (falls back to `sbt shell`; no raw console mode in stdlib) | refused — no verified backend |

`sbt` builds for all three. On a platform with no verified backend the cage
still opens, the reports still work, and starting a command is **refused with
the reason shown** — SBT will not run a command it cannot prove is isolated.
See [docs/LIMITATIONS.md](docs/LIMITATIONS.md).

## Appearance

The interface leads with white. On the dark ground the primary text is pure
white (`#FFFFFF`) and the greys are cool and low-contrast, so it reads as text on
a ground rather than as coloured blocks.

A light theme is one keystroke away: **Look → Light / dark theme**, or
`SBT_THEME=light` / `SBT_THEME=dark` at startup. The light palette is not an
inversion - each colour is chosen against a white background, because a
mechanically flipped dark scheme produces pastels that are unreadable in
daylight. The current theme is named in the top bar so it is never a guess.

**Severity colours are the deliberate exception.** Green, red and amber answer
"is this safe?", which is the one question this interface exists for. They are
never flattened to white, and never merged with each other - doing so would make
`PROTECTED` and `BROKEN` look alike at a glance. Every one of them is paired with
a word, so the meaning survives `NO_COLOR` regardless.

## Isolation

Inside a sandbox the command runs as the mapped unprivileged user in private
user/pid/mount namespaces with a scrubbed environment, read-only host trees,
tmpfs scratch mounts, an applied seccomp filter and rlimits. `start` refuses to
run anything if the platform probe cannot verify the isolation the host
pretends to offer (`sbt` never fakes a sandbox).

Files leave the sandbox only after the command that wrote them has exited, and
only through SBT. Files you keep land in a session workspace whose path is shown
in the interface and printed in the closing summary; export what you need, then
discard the workspace.

SBT verifies every isolation claim at runtime with a short-lived helper process
in a fresh user+pid namespace (see `internal/platform`). Features the host
cannot verify are reported as unavailable and disabled, never assumed.

- [docs/SECURITY.md](docs/SECURITY.md) — what is enforced and how it is verified
- [docs/LIMITATIONS.md](docs/LIMITATIONS.md) — what SBT deliberately does not do
