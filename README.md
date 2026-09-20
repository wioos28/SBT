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
sbt          # launch the interactive sandbox terminal
sbt doctor   # verify platform isolation and host dependencies
sbt version
```

### Interactive terminal

Typing `sbt` in a terminal starts the session: it verifies the platform
isolation with the runtime probe and drops you into the `sbt>` prompt.

```
  sbt> start echo hello
  sbt> status
  sbt> mem 256            # memory limit for the next sandbox
  sbt> net off            # allow network in the next sandbox
  sbt> stop               # stop the running sandbox
  sbt> exit
```

Inside a sandbox the command runs as the mapped unprivileged user in private
user/pid/mount namespaces with a scrubbed environment, read-only host trees,
tmpfs scratch mounts, an applied seccomp filter and rlimits. `start` refuses to
run anything if the platform probe cannot verify the isolation the host
pretends to offer (`sbt` never fakes a sandbox).

SBT verifies every isolation claim at runtime with a short-lived helper process
in a fresh user+pid namespace (see `internal/platform`). Features the host
cannot verify are reported as unavailable and disabled, never assumed.
