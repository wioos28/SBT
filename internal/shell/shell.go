// Package shell implements the interactive SBT session: the startup banner
// with the verified capability report, the REPL, the full-screen cage, and the
// runner that starts sandboxes.
//
// Most of this package is platform neutral. The one step that is not - actually
// starting the isolation - lives behind startHelper in backend_linux.go and
// backend_other.go. That split is what lets the interface run on a machine SBT
// cannot isolate: the cage, the review view and the reports all work, and the
// runner says plainly that it will not start a command.
package shell

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/wioos28/sbt/internal/platform"
	"github.com/wioos28/sbt/internal/shared/jailspec"
)

// Defaults applied to every sandbox unless overridden in the session.
const (
	defaultMemoryMB   = 512
	defaultCPUSeconds = 60
	defaultOpenFiles  = 256
	defaultProcesses  = 64
)

// controlReadTimeout bounds how long the parent waits for the jail helper's
// startup report before giving up.
const controlReadTimeout = 20 * time.Second

// sandbox is a running sandbox handle. It holds the helper command: the helper
// is pid 1 of its pid namespace, so killing it tears the whole sandbox down.
type sandbox struct {
	id       string
	dir      string
	specPath string
	cmd      *exec.Cmd
}

// Session is one interactive SBT terminal session.
type Session struct {
	theme      *uiTheme
	in, out    *os.File
	caps       *platform.Capabilities
	sbx        *sandbox
	sbxState   string // NOT STARTED / RUNNING / STOPPED / FAILED / UNAVAILABLE
	reason     string
	mode       string
	networkOff bool
	memoryMB   int64
	cpuSeconds int64
	startCwd   string
}

func newSession() *Session {
	return &Session{
		theme:      newTheme(os.Stdout),
		in:         os.Stdin,
		out:        os.Stdout,
		sbxState:   "NOT STARTED",
		mode:       "low",
		networkOff: true,
		memoryMB:   defaultMemoryMB,
		cpuSeconds: defaultCPUSeconds,
		startCwd:   "/workspace",
	}
}

func (s *Session) line(format string, args ...any) {
	fmt.Fprintf(s.out, format+"\n", args...)
}

// probe runs the platform capability probe (the helper round trip) and stores
// the report for the banner and for sandbox decisions.
func (s *Session) probe() {
	sp := newSpinner(s.theme, s.out, "verifying platform isolation")
	sp.start()
	s.caps = platform.Detect()
	sp.stop("done")
}

// sandboxAvailable reports whether the host can provide the isolation SBT
// refuses to work without, with the user-facing refusal reason otherwise.
func (s *Session) sandboxAvailable() (bool, string) {
	if s.caps == nil {
		return false, "the platform probe has not run yet"
	}
	if s.caps.ProbeError != "" {
		return false, "platform probe failed: " + s.caps.ProbeError
	}
	uns := s.caps.Feature("user_namespace")
	if uns.Level != platform.Full {
		return false, "this host cannot create a user namespace (" + uns.Reason + "); SBT refuses to start a pretend sandbox"
	}
	if m := s.caps.Feature("mount_namespace"); m.Level == platform.Unavailable {
		return false, "mount namespaces are refused on this host (" + m.Reason + "); the filesystem jail cannot be built"
	}
	return true, ""
}

// buildSpec assembles the jail spec for one command execution.
func (s *Session) buildSpec(argv []string) (*jailspec.Spec, string, error) {
	id, err := newSandboxID()
	if err != nil {
		return nil, "", err
	}
	dir, err := os.MkdirTemp("", "sbt-")
	if err != nil {
		return nil, "", fmt.Errorf("cannot create the sandbox scratch directory: %w", err)
	}
	root := filepath.Join(dir, "rootfs")
	for _, d := range []string{"usr", "etc", "bin", "sbin", "dev", "proc", "var", "run", "tmp", "workspace"} {
		_ = os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	_ = os.Chmod(filepath.Join(root, "tmp"), 0o1777)

	spec := &jailspec.Spec{
		SandboxID:      id,
		Rootfs:         root,
		Command:        argv,
		Cwd:            s.startCwd,
		Env:            baseEnv(),
		Binds:          defaultBinds(),
		NetworkBlocked: s.networkOff,
		Hostname:       id,
		DropAllCaps:    true,
		NoNewPrivs:     true,
		Seccomp:        true,
		Limits: jailspec.Limits{
			MemoryBytes:     s.memoryMB << 20,
			CPUQuotaSeconds: s.cpuSeconds,
			OpenFiles:       defaultOpenFiles,
			Processes:       defaultProcesses,
		},
		Interactive: isTerminal(s.in),
		LogPath:     filepath.Join(dir, "helper.log"),
		CreatedAt:   time.Now().UTC(),
	}
	specPath := filepath.Join(dir, "spec.json")
	if err := jailspec.Save(specPath, spec); err != nil {
		_ = os.RemoveAll(dir)
		return nil, "", fmt.Errorf("cannot write the jail spec: %w", err)
	}
	return spec, specPath, nil
}

// start runs argv inside a fresh sandbox and reports the outcome.
func (s *Session) start(argv []string) error {
	if ok, why := s.sandboxAvailable(); !ok {
		s.sbxState = "UNAVAILABLE"
		s.reason = why
		return fmt.Errorf("%s", why)
	}
	resolved := make([]string, len(argv))
	for i, a := range argv {
		if i == 0 {
			p, err := exec.LookPath(a)
			if err != nil {
				return fmt.Errorf("command not found on this host: %s", a)
			}
			resolved[i] = p
			continue
		}
		resolved[i] = a
	}

	sp := newSpinner(s.theme, s.out, "starting sandbox")
	sp.start()
	spec, specPath, err := s.buildSpec(resolved)
	if err != nil {
		sp.stop("failed")
		return err
	}
	// The platform-specific step. Everything above and below this call is
	// shared; only the isolation itself differs per operating system.
	cmd, err := startHelper(spec, specPath, s.in, s.out)
	if err != nil {
		_ = os.RemoveAll(filepath.Dir(specPath))
		sp.stop("failed")
		s.sbxState = "UNAVAILABLE"
		s.reason = err.Error()
		return err
	}
	sp.stop("ready")
	sb := &sandbox{
		id:       spec.SandboxID,
		dir:      filepath.Dir(specPath),
		specPath: specPath,
		cmd:      cmd,
	}
	s.sbx = sb
	s.sbxState = "RUNNING"
	s.reason = "sandbox ready"
	// Reap the helper in the background: when it exits the command finished
	// (or the sandbox was stopped) and the session state must move on.
	go func() {
		werr := sb.cmd.Wait()
		_ = os.RemoveAll(sb.dir)
		if s.sbx == sb {
			s.sbx = nil
			if s.sbxState == "RUNNING" {
				s.sbxState = "STOPPED"
				s.reason = exitSummary(werr)
			}
		}
	}()
	return nil
}

// stop kills the sandbox. The helper is pid 1 of its pid namespace wherever the
// platform provides one, so killing it tears the whole namespace down.
func (s *Session) stop() {
	if s.sbx == nil {
		return
	}
	sb := s.sbx
	s.sbx = nil
	s.sbxState = "STOPPED"
	s.reason = "sandbox stopped by user"
	_ = killSandboxProcess(sb.cmd)
	_ = os.RemoveAll(sb.dir)
}

func newSandboxID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("cannot generate a sandbox id: %w", err)
	}
	return "sbx-" + hex.EncodeToString(b[:]), nil
}
