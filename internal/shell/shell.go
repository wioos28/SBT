//go:build linux

// Package shell implements the interactive SBT terminal session: the startup
// banner with the verified capability report, the REPL, and the commands that
// start, inspect and stop a sandbox.
package shell

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/wioos28/sbt/internal/platform"
	"github.com/wioos28/sbt/internal/platform/linux/ns"
	"github.com/wioos28/sbt/internal/shared/helpermode"
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
	controlR, controlW, err := os.Pipe()
	if err != nil {
		_ = os.RemoveAll(filepath.Dir(specPath))
		sp.stop("failed")
		return fmt.Errorf("cannot create the sandbox control pipe: %w", err)
	}
	started, err := ns.SpawnMapped(ns.Options{
		Mode:     helpermode.ModeJail,
		SpecPath: specPath,
		Control:  controlW,
		Stdin:    s.in,
		Stdout:   s.out,
		Stderr:   os.Stderr,
	})
	_ = controlW.Close()
	if err != nil {
		_ = controlR.Close()
		_ = os.RemoveAll(filepath.Dir(specPath))
		sp.stop("failed")
		return fmt.Errorf("cannot start the sandbox helper: %w", err)
	}
	result, rerr := readControlResult(controlR)
	_ = controlR.Close()
	if rerr != nil {
		_ = started.Cmd.Process.Kill()
		_, _ = started.Cmd.Process.Wait()
		_ = os.RemoveAll(filepath.Dir(specPath))
		sp.stop("failed")
		return fmt.Errorf("sandbox did not report startup: %w", rerr)
	}
	sp.stop("ready")
	if !result.OK {
		_, _ = started.Cmd.Process.Wait()
		_ = os.RemoveAll(filepath.Dir(specPath))
		s.sbxState = "FAILED"
		s.reason = result.Reason
		return fmt.Errorf("%s", result.Reason)
	}
	sb := &sandbox{
		id:       spec.SandboxID,
		dir:      filepath.Dir(specPath),
		specPath: specPath,
		cmd:      started.Cmd,
	}
	s.sbx = sb
	s.sbxState = "RUNNING"
	s.reason = result.Reason
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

// readControlResult reads one newline terminated JSON report from the control
// pipe, bounded by controlReadTimeout.
func readControlResult(r *os.File) (jailspec.Result, error) {
	var res jailspec.Result
	_ = r.SetReadDeadline(time.Now().Add(controlReadTimeout))
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil {
		return res, err
	}
	if err := json.Unmarshal([]byte(line), &res); err != nil {
		return res, fmt.Errorf("invalid sandbox report: %w", err)
	}
	return res, nil
}

// stop kills the sandbox. The helper is pid 1 of its pid namespace, so killing
// it tears the whole namespace down with it.
func (s *Session) stop() {
	if s.sbx == nil {
		return
	}
	sb := s.sbx
	s.sbx = nil
	s.sbxState = "STOPPED"
	s.reason = "sandbox stopped by user"
	_ = killSandboxProcess(sb)
	_ = os.RemoveAll(sb.dir)
}

func newSandboxID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("cannot generate a sandbox id: %w", err)
	}
	return "sbx-" + hex.EncodeToString(b[:]), nil
}
