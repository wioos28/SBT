//go:build linux

package shell

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wioos28/sbt/internal/journal"
	"github.com/wioos28/sbt/internal/monitor"
	"github.com/wioos28/sbt/internal/platform"
	"github.com/wioos28/sbt/internal/shared/jailspec"
	"github.com/wioos28/sbt/internal/shared/policy"
	"github.com/wioos28/sbt/internal/shared/workspace"
	"github.com/wioos28/sbt/internal/tui"
)

// runner implements tui.Runner against the real jail helper.
//
// It is the only place in SBT that starts a sandbox. The UI asks for a run; this
// type decides whether one is allowed, builds the spec, spawns the helper,
// reaps it and records what it changed. Keeping that here rather than in the
// renderer is what preserves the rule that the interface can never grant itself
// permissions.
type runner struct {
	// store is the session workspace and run history.
	store *journal.Store
	// sampler measures the sandbox tree. It is shared with the session so the
	// resource panel and the runner measure the same pids.
	sampler *monitor.Sampler
	// preset is the policy applied to the next sandbox.
	preset policy.Preset

	in, out *os.File
	// screen is suspended around a run so the sandboxed command owns the real
	// terminal. SBT never proxies a command's tty.
	screen *tui.Screen

	// caps is the last probe report, consulted before every run so a host that
	// changed its mind cannot be started on a stale "protected".
	caps *platform.Capabilities

	mu sync.Mutex
	// live is the running sandbox, nil when idle.
	live *liveSandbox
	// lastExit, lastID and lastNote describe the most recent finished run.
	lastExit int
	lastID   string
	lastNote string
}

// liveSandbox is a running sandbox handle.
type liveSandbox struct {
	id       string
	dir      string
	specPath string
	cmd      *exec.Cmd
	started  time.Time
	// done is closed once the helper has been reaped, so a concurrent Stop can
	// wait for the process to actually be gone instead of racing it.
	done chan struct{}
}

// newRunner wires a runner to a session workspace.
func newRunner(store *journal.Store, in, out *os.File) *runner {
	r := &runner{store: store, in: in, out: out, preset: policy.Base()}
	r.sampler = monitor.New(0)
	r.sampler.SetWorkspace(store.WorkspaceDir(), policy.StdWorkspaceMB<<20)
	r.sampler.SetLimits(r.preset.Processes, r.preset.MemoryMB<<20)
	return r
}

// SetScreen binds the screen so a run can hand the terminal to the sandboxed
// command and take it back afterwards.
func (r *runner) SetScreen(s *tui.Screen) { r.screen = s }

// SetPreset records the policy applied to the next sandbox.
func (r *runner) SetPreset(p policy.Preset) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.preset = p
	// The monitor's limits describe the sandbox that will actually start, so
	// they follow the policy rather than the last run's numbers.
	r.sampler.SetWorkspace(r.store.WorkspaceDir(), p.WorkspaceMB<<20)
	r.sampler.SetLimits(p.Processes, p.MemoryMB<<20)
}

// SetCaps records the platform report consulted by Available.
func (r *runner) SetCaps(c *platform.Capabilities) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.caps = c
}

// Diff loads one path's before/after content from a finished run and converts it
// into the shape the review view draws.
//
// It reports a failure rather than an empty diff: "this file was not captured"
// and "this file is unchanged" must not look the same on screen.
func (r *runner) Diff(runID, path string) (tui.DiffState, error) {
	fd, err := r.store.Diff(runID, path)
	if err != nil {
		return tui.DiffState{}, err
	}
	out := tui.DiffState{
		RunID:     runID,
		Path:      fd.Path,
		Kind:      string(fd.Kind),
		Binary:    fd.Binary,
		Truncated: fd.Truncated,
		Note:      fd.Note,
		Loaded:    true,
	}
	if fd.Binary {
		return out, nil
	}
	out.Lines = fd.Unified(diffLineBudget)
	for _, l := range out.Lines {
		switch {
		case len(l) > 0 && l[0] == '+':
			out.Added++
		case len(l) > 0 && l[0] == '-':
			out.Removed++
		}
	}
	return out, nil
}

// diffLineBudget bounds how many lines of one file the review view renders. It
// is deliberately generous but finite: a diff pane that tried to draw a
// multi-megabyte file would stall the frame loop.
const diffLineBudget = 2000

// Available reports whether a sandbox can start now, and why not if it cannot.
//
// This is the runner's security gate. It repeats the REPL's check verbatim: a
// host that cannot provide the two namespaces the jail is built from is
// refused, because a sandbox SBT cannot verify is not a sandbox.
func (r *runner) Available() (bool, string) {
	r.mu.Lock()
	caps := r.caps
	r.mu.Unlock()
	if caps == nil {
		return false, "the platform probe has not run yet"
	}
	if caps.ProbeError != "" {
		return false, "platform probe failed: " + caps.ProbeError
	}
	uns := caps.Feature("user_namespace")
	if uns.Level != platform.Full {
		return false, "this host cannot create a user namespace (" + uns.Reason +
			"); SBT refuses to start a pretend sandbox"
	}
	if m := caps.Feature("mount_namespace"); m.Level == platform.Unavailable {
		return false, "mount namespaces are refused on this host (" + m.Reason +
			"); the filesystem jail cannot be built"
	}
	return true, ""
}

// Run executes argv in a fresh sandbox and returns the finished run.
//
// The sequence is: verify the isolation, reserve a run directory, build the
// spec, hand the terminal to the helper, wait for it, then fold the captured
// manifest into the session workspace. Nothing is reported as changed until the
// journal has actually recorded it.
func (r *runner) Run(argv []string) (tui.RunSummary, error) {
	cmdline := strings.Join(argv, " ")
	fail := func(err error) (tui.RunSummary, error) {
		return tui.RunSummary{Command: cmdline, Exit: -1, Note: err.Error()}, err
	}
	if ok, why := r.Available(); !ok {
		return fail(fmt.Errorf("%s", why))
	}
	if len(argv) == 0 {
		return fail(fmt.Errorf("no command given"))
	}
	resolved, err := resolveCommand(argv)
	if err != nil {
		return fail(err)
	}

	r.mu.Lock()
	busy := r.live != nil
	preset := r.preset
	r.mu.Unlock()
	if busy {
		return fail(fmt.Errorf("a sandbox is already running; stop it first"))
	}

	runID, err := r.store.BeginRun(argv)
	if err != nil {
		return fail(err)
	}
	spec, specPath, err := r.buildSpec(resolved, preset)
	if err != nil {
		return fail(err)
	}
	live, err := r.spawn(spec, specPath)
	if err != nil {
		_ = os.RemoveAll(filepath.Dir(specPath))
		return fail(err)
	}

	r.mu.Lock()
	r.live = live
	r.mu.Unlock()
	// The sampler follows the helper, so the resource panel measures the
	// sandbox rather than SBT itself.
	r.sampler.SetRoot(live.cmd.Process.Pid)

	// The command gets the real terminal, so the cage steps aside for it. This
	// is the whole reason a sandboxed command can use a tty at all.
	if r.screen != nil {
		r.screen.Suspend()
	}
	waitErr := live.cmd.Wait()
	if r.screen != nil {
		r.screen.Resume()
	}
	close(live.done)
	_ = os.RemoveAll(live.dir)

	exit := exitStatus(waitErr)
	started := spec.CreatedAt
	if live.started.After(started) {
		started = live.started
	}
	// Fold the helper's manifest into the session workspace. A missing manifest
	// is recorded as a note: "no changes" and "we could not tell" are very
	// different claims, and only one of them is true.
	run, ferr := r.store.FinishRun(runID, workspaceFallback(live.id, resolved, exit, started))
	if ferr != nil {
		run.Note = joinNote(run.Note, ferr.Error())
	}

	r.sampler.SetRoot(0)
	r.sampler.ResetPeak()
	r.mu.Lock()
	r.live = nil
	r.lastExit, r.lastID, r.lastNote = exit, live.id, run.Note
	r.mu.Unlock()

	return tui.RunSummary{
		Command:  cmdline,
		Exit:     exit,
		Duration: time.Since(started),
		Counts:   run.Counts(),
		Policy:   preset.Summary(),
		Note:     run.Note,
	}, nil
}

// spawn starts the helper for this run and returns once the jail is confirmed
// up, before the command ends. The isolation itself happens in startHelper; this
// wrapper exists to hold the scratch directory bookkeeping the session owns.
func (r *runner) spawn(spec *jailspec.Spec, specPath string) (*liveSandbox, error) {
	cmd, err := startHelper(spec, specPath, r.in, r.out)
	if err != nil {
		return nil, err
	}
	return &liveSandbox{
		id:       spec.SandboxID,
		dir:      filepath.Dir(specPath),
		specPath: specPath,
		cmd:      cmd,
		started:  time.Now(),
		done:     make(chan struct{}),
	}, nil
}

// Stop tears down the running sandbox. The helper is pid 1 of its pid
// namespace, so killing it takes every process inside down with it.
func (r *runner) Stop() error {
	r.mu.Lock()
	live := r.live
	r.mu.Unlock()
	if live == nil {
		return nil
	}
	err := killSandboxProcess(live.cmd)
	<-live.done
	_ = os.RemoveAll(live.dir)
	r.sampler.SetRoot(0)
	return err
}

// Export writes the selected workspace paths to the destination.
func (r *runner) Export(paths []string, dest string, overwrite bool) error {
	report, err := r.store.Export(paths, journal.ExportOptions{
		Destination: dest,
		Overwrite:   overwrite,
	})
	if err != nil {
		return err
	}
	// Refused paths are reported, never dropped: an export that quietly wrote
	// three of five files is worse than one that says what it skipped.
	if len(report.Skipped) == 0 && len(report.Dropped) == 0 {
		return nil
	}
	parts := make([]string, 0, 2)
	if n := len(report.Skipped); n > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", n))
	}
	if n := len(report.Dropped); n > 0 {
		parts = append(parts, fmt.Sprintf("%d refused as unsafe paths", n))
	}
	detail := strings.Join(parts, ", ")
	if n := len(report.Skipped); n > 0 {
		for i, s := range report.Skipped {
			if i == 3 {
				detail += "\n  and more"
				break
			}
			detail += "\n  " + s
		}
	}
	return fmt.Errorf("exported %d path(s) but %s", len(report.Written), detail)
}

// Discard deletes the session workspace and everything in it.
func (r *runner) Discard() error { return r.store.Remove() }

// State reports the sandbox state the UI shows. It is read on every frame, so
// it must stay cheap and must never claim a sandbox is running when it is not.
func (r *runner) State() tui.SandboxState {
	r.mu.Lock()
	live := r.live
	st := tui.SandboxState{LastExit: r.lastExit, ID: r.lastID, Reason: r.lastNote}
	r.mu.Unlock()
	if live != nil {
		st.Running = true
		st.ID = live.id
	}
	if ok, why := r.Available(); !ok {
		st.Unavailable = why
	}
	return st
}

// buildSpec assembles the jail spec for one run, wiring the session workspace in
// as both the seed and the capture directory.
func (r *runner) buildSpec(argv []string, preset policy.Preset) (*jailspec.Spec, string, error) {
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

	workspaceMB := preset.WorkspaceMB
	if workspaceMB <= 0 {
		workspaceMB = policy.StdWorkspaceMB
	}
	spec := &jailspec.Spec{
		SandboxID:      id,
		Rootfs:         root,
		Command:        argv,
		Cwd:            "/workspace",
		Env:            baseEnv(),
		Binds:          defaultBinds(),
		NetworkBlocked: preset.NetworkBlocked,
		Hostname:       id,
		DropAllCaps:    true,
		NoNewPrivs:     true,
		Seccomp:        true,
		Limits: jailspec.Limits{
			MemoryBytes:     preset.MemoryMB << 20,
			CPUQuotaSeconds: preset.CPUSeconds,
			OpenFiles:       preset.OpenFiles,
			Processes:       uint64(preset.Processes),
		},
		Interactive:         isTerminal(r.in),
		LogPath:             filepath.Join(dir, "helper.log"),
		WorkspaceDir:        "/workspace",
		WorkspaceIn:         r.store.WorkspaceDir(),
		WorkspaceLimitBytes: workspaceMB << 20,
		CaptureLimitBytes:   captureLimit(workspaceMB),
		CaptureMaxFiles:     defaultCaptureFiles,
		CreatedAt:           time.Now().UTC(),
	}
	specPath := filepath.Join(dir, "spec.json")
	if err := jailspec.Save(specPath, spec); err != nil {
		_ = os.RemoveAll(dir)
		return nil, "", fmt.Errorf("cannot write the jail spec: %w", err)
	}
	return spec, specPath, nil
}

// defaultCaptureFiles bounds how many files one run may copy onto the host.
const defaultCaptureFiles = 4096

// captureLimit bounds how much content a single run may copy out. It is never
// larger than the workspace itself, so the workspace tmpfs stays the tighter
// bound and the capture limit only ever matters on a host with plenty of disk.
func captureLimit(workspaceMB int64) int64 {
	const maxCapture = 64 << 20
	limit := workspaceMB << 20
	if limit <= 0 || limit > maxCapture {
		return maxCapture
	}
	return limit
}

// workspaceFallback is the run record used when the helper wrote no manifest. It
// carries the facts SBT measured itself and records plainly that the change
// report is missing, so an empty entry list is never read as "nothing changed".
func workspaceFallback(id string, argv []string, exit int, started time.Time) workspace.Run {
	return workspace.Run{
		SandboxID: id,
		Command:   argv,
		ExitCode:  exit,
		Started:   started.UTC(),
		Finished:  time.Now().UTC(),
	}
}

// resolveCommand resolves argv[0] on the host, so the helper is never asked to
// search a PATH it may not have and the error the user sees names the real
// problem.
func resolveCommand(argv []string) ([]string, error) {
	resolved := make([]string, len(argv))
	for i, a := range argv {
		if i != 0 {
			resolved[i] = a
			continue
		}
		p, err := exec.LookPath(a)
		if err != nil {
			return nil, fmt.Errorf("command not found on this host: %s", a)
		}
		resolved[i] = p
	}
	return resolved, nil
}

// exitStatus turns a wait error into the command's exit code. A helper that was
// killed has no exit status of its own; that is reported as -1 rather than as a
// success, because "the sandbox vanished" is not "the command succeeded".
func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if asExitError(err, &ee) {
		if code := ee.ExitCode(); code >= 0 {
			return code
		}
	}
	return -1
}

// joinNote merges two operational notes without dropping either.
func joinNote(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "; " + b
	}
}
