//go:build !linux

package shell

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/wioos28/sbt/internal/journal"
	"github.com/wioos28/sbt/internal/monitor"
	"github.com/wioos28/sbt/internal/platform"
	"github.com/wioos28/sbt/internal/shared/policy"
	"github.com/wioos28/sbt/internal/tui"
)

// runner is the cage's Runner on a platform SBT cannot isolate.
//
// It is not a stub that panics or a degraded "run it anyway" mode. It is a
// complete implementation of every method whose honest answer is "no": the
// interface runs, the journal works, an export can be produced from a session
// workspace, and starting a command is refused with the reason spelled out.
//
// The alternative - a runner that executed commands without isolation - would
// be the single most dangerous thing this program could do. A user who sees a
// green "PROTECTED" badge has every reason to trust that their command was
// confined. Refusing is the whole point.
type runner struct {
	store   *journal.Store
	sampler *monitor.Sampler
	preset  policy.Preset
	caps    *platform.Capabilities

	mu        sync.Mutex
	lastExit  int
	lastID    string
	lastNote  string
	workspace int64
}

// newRunner wires a runner to a session workspace.
func newRunner(store *journal.Store, in, out *os.File) *runner {
	r := &runner{store: store, preset: policy.Base()}
	r.sampler = monitor.New(0)
	r.sampler.SetWorkspace(store.WorkspaceDir(), policy.StdWorkspaceMB<<20)
	r.sampler.SetLimits(r.preset.Processes, r.preset.MemoryMB<<20)
	return r
}

// SetScreen is a no-op here: no sandbox ever owns the terminal, so there is
// nothing to hand over and take back.
func (r *runner) SetScreen(*tui.Screen) {}

// SetPreset records the policy that would have been applied.
func (r *runner) SetPreset(p policy.Preset) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.preset = p
	r.sampler.SetWorkspace(r.store.WorkspaceDir(), p.WorkspaceMB<<20)
	r.sampler.SetLimits(p.Processes, p.MemoryMB<<20)
}

// SetCaps records the platform report.
func (r *runner) SetCaps(c *platform.Capabilities) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.caps = c
}

// Available always refuses, and says exactly why.
//
// The reason is not "unsupported platform" thrown away with the details: it
// names what is missing and what SBT did instead, so a user is never left
// wondering whether the command ran somewhere.
func (r *runner) Available() (bool, string) { return false, backendUnavailable() }

// Run refuses. Nothing is spawned, nothing is written.
func (r *runner) Run(argv []string) (tui.RunSummary, error) {
	why := backendUnavailable()
	cmdline := strings.Join(argv, " ")
	err := fmt.Errorf("%s", why)
	return tui.RunSummary{Command: cmdline, Exit: -1, Note: why}, err
}

// Stop has nothing to stop: no sandbox was ever started.
func (r *runner) Stop() error { return nil }

// Export still works. A session workspace can be produced on any platform - by
// the journal itself, or by a previous run elsewhere - and exporting it is
// plain file copying with no isolation involved.
func (r *runner) Export(paths []string, dest string, overwrite bool) error {
	report, err := r.store.Export(paths, journal.ExportOptions{
		Destination: dest,
		Overwrite:   overwrite,
	})
	if err != nil {
		return err
	}
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
	return fmt.Errorf("exported %d path(s) but %s", len(report.Written), strings.Join(parts, ", "))
}

// Discard deletes the session workspace.
func (r *runner) Discard() error { return r.store.Remove() }

// State reports the sandbox state. Unavailable is always set, which is what
// makes the cage render an explicit "sandbox unavailable" verdict instead of a
// quiet "ready".
func (r *runner) State() tui.SandboxState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return tui.SandboxState{
		LastExit:    r.lastExit,
		ID:          r.lastID,
		Reason:      r.lastNote,
		Unavailable: backendUnavailable(),
	}
}

// Diff reads a recorded run's content. The journal stores before/after copies
// on the host, so reviewing them needs no isolation and works on every
// platform.
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

// diffLineBudget bounds how many lines of one file the review view renders.
const diffLineBudget = 2000
