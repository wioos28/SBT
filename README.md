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

### Startup

Typing `sbt` opens a startup screen: a large orange **sbt-wioos28** wordmark
with a travelling shine, a progress bar, and one row per check.

Every row is a **real** step SBT actually performed - the workspace open, the
environment probe, the sandbox backend verdict, the measured permission scan,
the configuration read, the terminal. A stage is recorded *after* the step
finishes, so the screen can never show a tick for work that has not happened,
and the percentage is the real fraction of stages that completed. A host with
no verified backend records the sandbox stage as **failed**, not passed.

The reveal is paced to 1.1s, any key skips it (and that key is consumed, so a
stray keystroke cannot run a command), and with `SBT_NO_MOTION` or
`anim.startup = off` it completes instantly with the same final frame. The
welcome screen that follows states the real cage verdict and is keyboard-first.

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
| `alt+1` … `alt+8` | terminal · files · changes · status · export · help · settings · permissions |
| `F10` / `alt+m` | open the menu bar |
| `ctrl+k` | command palette (type to filter) |
| `enter` | run the typed command; on a change, review its diff |
| `esc` | close the overlay, or go back |
| `ctrl+.` | stop the running sandbox |
| `ctrl+d` | leave — always asks first |

Commands can also be typed into the input line, so the common actions are
reachable by name as well as by key:

```
/help   /setting   /status   /files   /changes   /export
/security   /permissions   /session   /clear
/theme dark|light   /palette cyber   /language vi-VN   /destroy   /exit
```

An unknown name is reported and **never handed to a sandbox**, so a typo cannot
become a command that runs.

### Settings and permissions

**Settings** (`alt+7`, `/setting`) is driven from the same declared schema as
`sbt setting`, so the two can never disagree about what exists or what a legal
value is. Left/right moves between sections, up/down between rows, enter
toggles, cycles or edits inline, escape goes back. A typed value is validated
before it is stored - an out-of-range number, a malformed colour or a value
outside a choice list is refused and the reason is shown.

The view never writes the config file: it asks, and the session stores, saves
and re-applies, so the palette, the typing sweep and the startup options land on
the next frame instead of the next restart.

**Permissions** (`alt+8`, `/permissions`, or `sbt security permissions`) reports
what SBT can actually do here, and every row is measured rather than assumed:

```
Filesystem           ALLOWED   SBT can read and write its workspace
Network              DENIED    unavailable: unshare(CLONE_NEWNET) failed
PTY                  ALLOWED   /dev/ptmx opens read-write
Sudo                 LIMITED   sudo is installed at /usr/bin/sudo
Root                 DENIED    SBT is running as an unprivileged user
```

A denied row names the SBT feature that needs the capability and the
least-privilege step that would grant it. Nothing reports `ALLOWED` on an
assumption, and a capability that cannot be measured reports `UNKNOWN` rather
than being rounded up.

### When the host cannot isolate

Pressing enter on a command the host cannot isolate does not fail silently and
does not run quietly. SBT asks:

```
PERMISSION REQUIRED
Capability   Filesystem
Reason       confining the sandbox filesystem
Risk         the command runs with reduced isolation
Evidence     mount namespace unavailable
[ Allow once ]  [ Allow session ]  [ Configure ]  [ Cancel ]
```

**Allow is an acknowledgement, never a grant.** SBT cannot give itself a
privilege it does not have, so what "Allow" does is record that the user was told
what is missing, and let the session attempt the work - which still refuses
anything it cannot verify, and still says exactly why. "Allow for the session"
only stops SBT repeating the question; a test pins that it cannot change the cage
verdict. The prompt is the most modal layer in the interface and opens on
Cancel.

`/repair` (or the palette) re-runs the platform probe and re-derives the verdict.
If isolation still cannot be verified the answer is still **BROKEN**: repair never
disables a check to make the cage look healthy.

### Warnings and isolation

A probe that cannot verify isolation raises a full-width banner with an orange
glow and a border that **breathes rather than flashes**. It is keyed on the cage
verdict, so an unchanged verdict does not restart its clock every frame, and a
broken cage holds the banner up as critical rather than letting it fade while the
session is still open.

While a sandbox is running, an `ISOLATION ACTIVE` panel states the confined
process, its id, the reason, and what filesystem, network and permissions are
restricted to. Every field comes from measured state: the id from the sandbox,
the command from the journal, the policy from the preset in force.

### Destroying the sandbox

`/destroy`, or the palette and menu, opens a dialog that starts on the safe
choice and keeps the destructive button **locked** until you type
`DESTROY SBT SANDBOX` exactly. Escape always keeps the sandbox, even after the
phrase was typed.

When it does run, SBT stops the helper and unlinks the session workspace, and
the notice says *unlinked* rather than *securely erased*: a filesystem journal
may still hold blocks SBT cannot reach. For a tool whose job is to be honest
about what it cannot prove, overstating that would be the worst possible claim.

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

## Language

The interface text goes through one lookup with the English text as the fallback,
so a locale that does not cover a key shows readable English rather than a raw
key. **Vietnamese, English, Russian and Chinese** ship with translations for the
section titles, the permission prompt, the typed-confirmation states, the welcome
screen, the boot hints, the permission rows and the file strip; anything else
falls back.

Switch language from **Settings → Language** or `/language vi-VN`. It applies on
the next frame - no restart.

A custom language is a JSON file of `{"key": "text"}` dropped in
`~/.sbt/locales/`, or `sbt language import <file>`. The destroy phrase is
deliberately **not** translatable: it is a token the user types exactly, and
translating it would break the guard rather than help it.

## Appearance

The visual identity is **sbt-wioos28: orange + cyan**. The default dark ground
uses `#FF8A00` as its accent and `#00D9FF` as its information colour; the wordmark,
the startup logo, the typing sweep and the focus accents all follow from those two
values, so no colour is hardcoded at a call site.

Named presets - `sbt`, `cyber`, `minimal`, `ocean`, `mono` - live in one place
(`ui.palette`). Each preset is a complete palette rather than an accent tint,
because the ground and the three severity colours were tuned together; every
preset keeps PROTECTED / LIMITED / BROKEN distinct. `Primary`, `Secondary` and
`Info` are editable as `#RRGGBB` from Settings → Colors.

The typing animation follows typing speed: characters are stamped when they
arrive and tinted at draw time, so the input path does no extra work and there
is no timer running. `auto` mode shortens the trail for a fast typist. With the
animation off, the same call draws plain text.

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
