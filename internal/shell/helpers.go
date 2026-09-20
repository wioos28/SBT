//go:build linux

package shell

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/wioos28/sbt/internal/shared/jailspec"
	"github.com/wioos28/sbt/internal/ui"
)

// uiTheme aliases the ui theme so shell stays readable.
type uiTheme = ui.Theme

func newTheme(out *os.File) *uiTheme { return ui.NewTheme(out) }

func isTerminal(f *os.File) bool { return ui.IsTerminal(f) }

// spinner wraps ui.NewSpinner with start/stop convenience. On stop the
// message is printed again with the final word so the context is not lost.
type spinner struct {
	s   *ui.Spinner
	msg string
	out *os.File
}

func newSpinner(t *uiTheme, out *os.File, msg string) *spinner {
	return &spinner{s: ui.NewSpinner(t, out, msg), msg: msg, out: out}
}

func (sp *spinner) start() { sp.s.Start() }

func (sp *spinner) stop(final string) {
	sp.s.Stop()
	fmt.Fprintf(sp.out, "%s — %s\n", sp.msg, final)
}

// baseEnv is the scrubbed environment handed to sandbox commands.
func baseEnv() []string {
	env := []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/tmp",
	}
	for _, k := range []string{"TERM", "LANG", "LC_ALL", "LC_CTYPE"} {
		if v := os.Getenv(k); v != "" {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// defaultBinds describes the sandbox filesystem: read-only host toolchain,
// writable tmpfs scratch spaces, synthetic /proc and the device nodes the
// command legitimately needs.
func defaultBinds() []jailspec.Bind {
	binds := []jailspec.Bind{
		{Source: "tmpfs", Target: "/tmp", FSType: "tmpfs", SizeBytes: 64 << 20, ModeBits: 0o1777, NoSuid: true, NoDev: true},
		{Source: "tmpfs", Target: "/workspace", FSType: "tmpfs", SizeBytes: 128 << 20, NoSuid: true, NoDev: true},
		{Source: "tmpfs", Target: "/var", FSType: "tmpfs", SizeBytes: 32 << 20, NoSuid: true, NoDev: true},
		{Source: "tmpfs", Target: "/run", FSType: "tmpfs", SizeBytes: 16 << 20, ModeBits: 0o755, NoSuid: true, NoDev: true},
		{Source: "proc", Target: "/proc", FSType: "proc"},
	}
	for _, tree := range []string{"/usr", "/etc", "/bin", "/sbin"} {
		if _, err := os.Stat(tree); err != nil {
			continue
		}
		binds = append(binds, jailspec.Bind{Source: tree, Target: tree, ReadOnly: true, Recursive: true, Optional: true})
	}
	for _, node := range []string{"/dev/null", "/dev/zero", "/dev/urandom", "/dev/random"} {
		if _, err := os.Stat(node); err != nil {
			continue
		}
		binds = append(binds, jailspec.Bind{Source: node, Target: node, ReadOnly: true, Optional: true})
	}
	return binds
}

// killSandboxProcess terminates the sandbox helper (pid 1 of the sandbox pid
// namespace) and waits for it so nothing is left behind.
func killSandboxProcess(sb *sandbox) error {
	if sb == nil || sb.cmd == nil || sb.cmd.Process == nil {
		return nil
	}
	_ = sb.cmd.Process.Kill()
	_, werr := sb.cmd.Process.Wait()
	return werr
}

// exitSummary turns a helper exit error into a short user-facing note.
func exitSummary(err error) string {
	if err == nil {
		return "command finished"
	}
	var ee *exec.ExitError
	if ok := asExitError(err, &ee); ok {
		return fmt.Sprintf("command exited with %s", strings.TrimSpace(err.Error()))
	}
	return "sandbox terminated: " + err.Error()
}

func asExitError(err error, target **exec.ExitError) bool {
	if e, ok := err.(*exec.ExitError); ok {
		*target = e
		return true
	}
	return false
}
