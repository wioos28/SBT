package tui

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/wioos28/sbt/internal/config"
	"github.com/wioos28/sbt/internal/journal"
	"github.com/wioos28/sbt/internal/monitor"
	"github.com/wioos28/sbt/internal/platform"
	"github.com/wioos28/sbt/internal/security"
	"github.com/wioos28/sbt/internal/shared/policy"
)

// Session owns the live state behind the cage and answers the App's requests.
//
// The division of labour is the important part. The App knows how to draw and
// how to read a keyboard; this type knows how to probe the host, run a
// sandbox and write an export. Neither makes a security decision for the
// other: the UI asks, and the reason a sandbox may not start lives here,
// where it can be measured rather than asserted.
type Session struct {
	// App is the UI this session answers.
	App *App

	// WorkspaceDir is where the session's files live on the host.
	WorkspaceDir string
	// Destination is where an export writes to. Empty means "not chosen yet".
	Destination string

	// caps is the last platform probe report.
	caps *platform.Capabilities
	// store is the journal of runs and files.
	store *journal.Store
	// sampler reads live resource usage for the sandbox tree.
	sampler *monitor.Sampler
	// runner performs the privileged work. It is an interface so the session
	// can be tested without a kernel.
	runner Runner
	// preset is the policy applied to the next sandbox.
	preset policy.Preset
	// diff is the most recently loaded review content.
	diff DiffState

	// host is the hostname shown in the top bar, resolved once.
	host string

	// settings is the live user configuration. The session owns it because the
	// Settings view must never write the file itself: it validates, asks, and
	// the session stores the change and applies it to the running interface.
	settings *config.Settings
	// perms is the measured capability report, refreshed with the probe.
	perms security.CapabilityReport
	// alertKey remembers which cage verdict produced the current banner, so a
	// verdict that repeats every frame does not restart its clock, and one that
	// changes does.
	alertKey string
}

// NewSession builds a session bound to an App. The probe is not run here:
// callers decide when the host is inspected, because on some machines the probe
// forks a helper and that must not happen on a hot path.
func NewSession(app *App) *Session {
	cfg, err := config.Load()
	if err != nil || cfg == nil {
		cfg = config.DefaultSettings()
	}
	s := &Session{App: app, preset: policy.Base(), host: hostname(), settings: cfg}
	s.applySettings()
	return s
}

// SetSettings replaces the configuration store, used when a caller has already
// loaded it and does not want the session reading the file again.
func (s *Session) SetSettings(cfg *config.Settings) {
	if cfg == nil {
		return
	}
	s.settings = cfg
	s.applySettings()
}

// settingBool reads a boolean preference with a fallback.
func (s *Session) settingBool(key string, def bool) bool {
	if s.settings == nil {
		return def
	}
	if v, ok := s.settings.GetBool(key); ok {
		return v
	}
	return def
}

// settingString reads a string preference with a fallback.
func (s *Session) settingString(key, def string) string {
	if s.settings == nil {
		return def
	}
	if v, ok := s.settings.GetString(key); ok && v != "" {
		return v
	}
	return def
}

// settingInt reads an int preference with a fallback.
func (s *Session) settingInt(key string, def int) int {
	if s.settings == nil {
		return def
	}
	if v, ok := s.settings.GetInt(key); ok {
		return v
	}
	return def
}

// applySettings pushes the current configuration into the running interface.
//
// This is what makes a settings edit land immediately rather than at the next
// restart: the palette, the typing animation and the motion switch are all read
// from here. It is safe to call repeatedly and touches no files.
func (s *Session) applySettings() {
	app := s.App
	if app == nil {
		return
	}
	app.Theme.SetPreset(s.settingString("ui.palette", "sbt"))
	if s.settingString("ui.theme", "dark") == "light" {
		app.Theme.SetLight(true)
	}
	colour, _ := ParseHex(s.settingString("anim.typing_color", "#00D9FF"))
	app.State.Typing.Configure(
		s.settingBool("anim.typing", true) && s.settingBool("ui.animations", true),
		s.settingString("anim.typing_speed", "auto"),
		s.settingInt("anim.typing_intensity", 70),
		colour,
	)
	app.StartupAnim = s.settingBool("anim.startup", true) && s.settingBool("ui.animations", true)
	app.ShowWelcome = s.settingBool("ui.welcome", true)
	if app.Interp != nil {
		app.Interp.Theme = app.Theme
	}
	app.Screen.SetTheme(app.Theme)
	app.Screen.Invalidate()
}

// setSetting validates, stores and applies one configuration change.
//
// Validation happens here as well as in the view: the CLI can reach this path
// too, and a bad value must never reach the file. A value that is refused is
// reported to the user instead of being silently dropped.
func (s *Session) setSetting(req Request, now time.Time) {
	if s.settings == nil {
		s.settings = config.DefaultSettings()
	}
	if err := s.settings.Set(req.Key, req.Value); err != nil {
		s.App.State.Toasts.Notify(StateWarn, "setting not saved", err.Error(), now)
		return
	}
	if err := s.settings.Save(); err != nil {
		s.App.State.Toasts.Notify(StateDanger, "setting not saved", err.Error(), now)
		return
	}
	s.applySettings()
}

// hostname is the short host label for the top bar. A name that cannot be read
// is simply absent; it is never a reason to fail the session.
func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	if i := strings.IndexByte(h, '.'); i > 0 {
		h = h[:i]
	}
	return h
}

// SetStore binds the journal so the file and change views have real data.
func (s *Session) SetStore(st *journal.Store) { s.store = st }

// SetSampler binds the monitor to the sandbox helper process so the resource
// panel measures the sandbox rather than SBT itself.
func (s *Session) SetSampler(smp *monitor.Sampler) { s.sampler = smp }

// SetRunner binds the component that performs privileged work.
func (s *Session) SetRunner(r Runner) { s.runner = r }

// SetPreset replaces the policy applied to the next sandbox.
//
// The policy is pushed to the runner as well as stored: the badge in the top bar
// describes the policy the next jail spec will actually carry, and a UI-only
// policy would be a claim about a sandbox that is never configured that way.
func (s *Session) SetPreset(p policy.Preset) {
	s.preset = p
	if ps, ok := s.runner.(PolicySetter); ok && ps != nil {
		ps.SetPreset(p)
	}
}

// Refresh re-runs the platform probe and rebuilds the isolation evidence.
//
// The probe is the only source of truth about what the cage can enforce, so
// the report is re-read rather than cached for the life of the session: a host
// can change (a namespace limit is lowered, a seccomp policy is denied) while
// SBT is open, and a stale "protected" claim would be a lie.
func (s *Session) Refresh() {
	s.caps = platform.Detect()
	s.perms = security.Permissions(s.caps)
}

// Caps returns the last platform probe report, or nil when the probe has not
// run yet. A front end uses it to give a runner the same evidence the session
// is displaying, so the two can never disagree.
func (s *Session) Caps() *platform.Capabilities { return s.caps }

// SnapshotInto rebuilds the Snapshot the interface is about to draw. It is the
// same refresh the draw loop performs, exposed so a front end can prime the
// first frame itself and so the wiring in one place decides what the interface
// sees.
func (s *Session) SnapshotInto(dst *Snapshot) { s.snapshotInto(dst) }

// snapshotInto rebuilds the Snapshot from the current session state. It is
// called before every frame, so it must stay cheap and must never fail a
// frame: a component that cannot be read leaves its field zero and the reason
// in the matching message, rather than being quietly omitted.
func (s *Session) snapshotInto(dst *Snapshot) {
	dst.Version = versionString()
	dst.Host = s.host
	dst.Policy = s.preset
	dst.Mode = s.preset.Mode
	dst.Destination = s.Destination
	dst.WorkspaceDir = s.WorkspaceDir
	dst.Now = time.Now()

	if s.caps != nil {
		dst.Kernel = s.caps.Kernel
		dst.Arch = s.caps.Arch
		dst.OS = s.caps.OS
		if s.caps.Distribution != "" {
			dst.Platform = s.caps.Distribution
		}
		dst.Backend = s.caps.Backend
		dst.ProbeFail = s.caps.ProbeError
		dst.Isolation = isolationLines(s.caps)
		dst.Warnings = s.caps.Warnings
	}
	// The cage verdict is derived from the probe, never assumed.
	dst.Cage = cageVerdict(s.caps, s.preset)
	s.updateAlert(dst, dst.Now)
	s.updateConfinement(dst)
	dst.Permissions = permissionRows(s.perms)
	if s.settings != nil {
		dst.Settings = s.settings.GetAll()
	}
	dst.Palette = s.App.Theme.Preset

	if s.sampler != nil {
		// Sample never fails: a value it could not read comes back zero with
		// Snapshot.Err explaining why, so a partial reading is still shown
		// rather than silently replaced by an empty panel.
		dst.Stats = s.sampler.Sample()
	}
	if s.store != nil {
		if files, err := s.store.Files(); err == nil {
			dst.Files = files
		}
		if runs, err := s.store.Runs(); err == nil {
			dst.Runs = runs
		}
		// The journal ids are read alongside the runs so the two stay parallel:
		// the review view addresses a run by its directory, never by its
		// position in the list.
		if ids, err := s.store.RunIDs(); err == nil {
			dst.RunIDs = ids
		}
	}
	// The live sandbox state comes from the runner when it can report it. A
	// runner that cannot leaves the field zero, which renders as "idle" - not
	// as a claim that a sandbox is running.
	if sr, ok := s.runner.(StateReader); ok && sr != nil {
		dst.Sandbox = sr.State()
	}
	// The diff is only replaced when the session was asked for one; otherwise
	// the previously loaded content stays on screen while the user moves the
	// cursor back to it.
	if s.diff.Loaded {
		dst.Diff = s.diff
	}
}

// isolationLines converts the probe report into the rows the security panel
// draws. Every probed feature gets a row even when it failed, because a
// missing feature that is not shown is a missing feature the user cannot act
// on.
func isolationLines(c *platform.Capabilities) []IsolationLine {
	lines := make([]IsolationLine, 0, len(c.Features))
	for _, f := range c.Features {
		line := IsolationLine{Label: f.Label, Value: f.Level.Label(), Reason: f.Reason}
		switch f.Level {
		case platform.Full:
			line.State = StateOK
		case platform.Partial:
			line.State = StateWarn
		case platform.Unavailable:
			line.State = StateDanger
		default:
			line.State = StateMeta
		}
		lines = append(lines, line)
	}
	return lines
}

// cageVerdict is the single place the overall state is decided. The worst
// verified feature wins, and a cage that could not be verified is never
// reported as protected - that is the whole promise of the tool.
func cageVerdict(c *platform.Capabilities, preset policy.Preset) Cage {
	if c == nil {
		return Cage{State: CageOff, Reason: "the platform probe has not run yet"}
	}
	if c.ProbeError != "" {
		return Cage{State: CageBroken, Reason: c.ProbeError}
	}
	// The two features the jail is built from. Without them there is no
	// filesystem isolation and no process isolation, so SBT refuses to start
	// rather than pretending to.
	verified := 0
	for _, key := range []string{"user_namespace", "mount_namespace"} {
		f := c.Feature(key)
		switch f.Level {
		case platform.Unavailable:
			return Cage{State: CageBroken, Reason: f.Label + ": " + f.Reason}
		case platform.Partial:
			return Cage{State: CageLimited, Reason: f.Label + " is partial: " + f.Reason}
		case platform.Full:
			verified++
		}
	}
	// A feature that the probe never reported is not a verified feature. An
	// empty report must land on "no evidence", never on "protected": this is
	// the one place where an optimistic default would be a safety bug.
	if verified < 2 {
		return Cage{State: CageLimited, Reason: "the probe did not verify every feature the cage needs"}
	}
	if preset.Mode == policy.High {
		return Cage{State: CageHigh, Reason: "high risk policy: the network is not blocked"}
	}
	return Cage{State: CageProtected, Reason: "isolation verified at runtime"}
}

// versionString is the build identity shown in the top bar. It is a variable
// rather than a direct import so the renderer stays a pure drawing layer and
// the binary tells it who it is at startup.
var versionString = func() string { return "" }

// SetVersionString installs the version reported by the binary.
//
// It exists so the renderer does not have to import internal/version: the
// interface can draw, but it should not need to know what it is. A front end
// that forgets to call it simply shows an empty identity rather than a wrong
// one.
func SetVersionString(v string) { versionString = func() string { return v } }

// Runner is what a Session needs in order to actually run a command. It is an
// interface so the session can be tested without a kernel, and so the security
// decision of "may this run" stays in one implementation.
type Runner interface {
	// Available reports whether a sandbox can start now, and why not if it
	// cannot. The reason is shown to the user verbatim.
	Available() (bool, string)
	// Run executes argv in a fresh sandbox and returns the finished run.
	Run(argv []string) (RunSummary, error)
	// Stop tears down a running sandbox.
	Stop() error
	// Export writes the selected paths to the destination.
	Export(paths []string, dest string, overwrite bool) error
	// Discard deletes the session workspace.
	Discard() error
}

// StateReader is implemented by a Runner that can report what its sandbox is
// doing right now.
//
// It is a separate, optional interface rather than part of Runner so the
// session still works with a minimal implementation - a test double needs no
// process state - while the real runner can report that a sandbox is running.
type StateReader interface {
	// State describes the sandbox the session is currently attached to.
	State() SandboxState
}

// DiffReader is implemented by a Runner that can read a finished run's content.
// The session needs it for the review view: the journal's before/after files
// are the only record of what a sandbox actually wrote.
type DiffReader interface {
	// Diff loads one path's before/after content from one run.
	Diff(runID, path string) (DiffState, error)
}

// StateSetter is implemented by a Runner that needs to be told when the
// platform report changes.
//
// It matters because the probe can be re-run while the session is open: a runner
// that kept the first report would keep starting sandboxes on a stale verdict.
type StateSetter interface {
	// SetCaps installs the current platform report.
	SetCaps(*platform.Capabilities)
}

// PolicySetter is implemented by a Runner that applies a policy to the sandbox
// it is about to start. Without it the UI's policy badge would describe a knob
// the jail spec never received.
type PolicySetter interface {
	// SetPreset installs the policy for the next sandbox.
	SetPreset(policy.Preset)
}

// Handle executes one Request from the App.
//
// Every branch here is the session's decision, not the UI's. The App asked for
// something; this type decides whether it happens and what the user is told
// when it does not. Nothing below trusts a request blindly: a run still has to
// pass the availability check, and an export still has to refuse an
// unacknowledged executable.
func (s *Session) Handle(ctx context.Context, req Request) {
	now := time.Now()
	ui := s.App.State
	switch req.Kind {
	case ReqRun:
		s.runRequest(ctx, req, now)
	case ReqStop:
		if s.runner == nil {
			return
		}
		if err := s.runner.Stop(); err != nil {
			ui.Toasts.Notify(StateWarn, "stop failed", err.Error(), now)
			return
		}
		ui.Toasts.Notify(StateMeta, "sandbox stopped", "", now)
	case ReqSetPolicy:
		s.SetPreset(req.Policy)
		ui.Toasts.Notify(StateWarn, "policy now "+string(req.Policy.Mode),
			"high risk mode does not block the network", now)
	case ReqExport:
		s.exportRequest(req, now)
	case ReqDiscard:
		if s.runner != nil {
			if err := s.runner.Discard(); err != nil {
				ui.Toasts.Notify(StateDanger, "discard failed", err.Error(), now)
				return
			}
		}
		ui.Toasts.Notify(StateMeta, "workspace discarded", "every change was deleted", now)
	case ReqRefresh:
		s.Refresh()
		if sr, ok := s.runner.(StateSetter); ok && sr != nil {
			sr.SetCaps(s.caps)
		}
		ui.Toasts.Notify(StateOK, "platform report refreshed",
			cageVerdict(s.caps, s.preset).State.Label(), now)
	case ReqDiff:
		s.diffRequest(req, now)
	case ReqOpenPolicy:
		s.SetPreset(policyFor(req.Choice))
	case ReqDestroy:
		s.destroyRequest(time.Now())
	case ReqSetSetting:
		s.setSetting(req, time.Now())
	case ReqResize, ReqExit:
		// Handled by the loop: resize is observed by the screen, exit ends Run.
	}
}

// diffRequest loads one path's before/after content for the review view.
//
// The UI never reads files: it asks for a run and a path, and the session - the
// only component that knows where runs are stored - answers. A load that fails
// still produces a DiffState, carrying the reason, because a pane that silently
// shows nothing reads as "this file did not change".
func (s *Session) diffRequest(req Request, now time.Time) {
	dr, ok := s.runner.(DiffReader)
	if !ok || dr == nil {
		s.diff = DiffState{RunID: req.RunID, Path: req.Entry, Loaded: true,
			Note: "this session cannot review file content"}
		return
	}
	if req.Entry == "" {
		return
	}
	diff, err := dr.Diff(req.RunID, req.Entry)
	if err != nil {
		s.diff = DiffState{RunID: req.RunID, Path: req.Entry, Loaded: true,
			Note: err.Error()}
		return
	}
	diff.Loaded = true
	s.diff = diff
}

// runRequest applies the policy checks and then runs the command.
func (s *Session) runRequest(ctx context.Context, req Request, now time.Time) {
	ui := s.App.State
	if s.runner == nil {
		ui.Toasts.Notify(StateDanger, "no runner is attached", "the session cannot start a sandbox", now)
		return
	}
	// The refusal check happens here, not in the UI: the UI already knows how
	// to display the reason, but only the session is allowed to decide.
	if ok, why := s.runner.Available(); !ok {
		ui.Toasts.Notify(StateDanger, "sandbox unavailable", why, now)
		return
	}
	if len(req.Argv) == 0 {
		return
	}
	ui.Busy.Active = false // the sandbox is running now, not starting
	summary, err := s.runner.Run(req.Argv)
	if err != nil {
		summary.Command = joinArgs(req.Argv)
		summary.Exit = -1
		summary.Note = err.Error()
	}
	s.App.Settle(summary, now)
}

// exportRequest validates an export before writing anything.
func (s *Session) exportRequest(req Request, now time.Time) {
	ui := s.App.State
	if s.runner == nil {
		ui.Toasts.Notify(StateDanger, "no runner is attached", "nothing can be written", now)
		return
	}
	if req.Destination == "" {
		ui.Toasts.Notify(StateWarn, "no destination chosen", "set one before exporting", now)
		return
	}
	if err := s.runner.Export(req.Paths, req.Destination, req.Overwrite); err != nil {
		ui.Toasts.Notify(StateDanger, "export failed", err.Error(), now)
		return
	}
	ui.Toasts.Notify(StateOK, "exported to "+req.Destination,
		itoa(len(req.Paths))+" path(s) written", now)
}

// Finish is called when the interface closes. It releases the session's
// resources and leaves the terminal usable.
func (s *Session) Finish() {
	if s.runner != nil {
		_ = s.runner.Stop()
	}
}

// Report is the plain-text summary printed after the cage closes, so the
// user's scrollback keeps a record of what happened inside it.
func (s *Session) Report() []string {
	var out []string
	s.snapshotInto(&s.App.Snap)
	snap := &s.App.Snap
	out = append(out, "")
	out = append(out, "SBT session summary")
	out = append(out, "  cage     "+snap.Cage.State.Label())
	if snap.Cage.Reason != "" {
		out = append(out, "  note     "+snap.Cage.Reason)
	}
	if c := snap.Counts(); c.Total() > 0 {
		out = append(out, "  changes  "+c.Summary())
		if c.Warnings > 0 {
			out = append(out, "  ! "+itoa(c.Warnings)+" path(s) carry a risk note - review before exporting")
		}
	} else if len(snap.Runs) > 0 {
		out = append(out, "  changes  none - every run left the workspace clean")
	}
	for _, w := range snap.Warnings {
		out = append(out, "  ! "+w)
	}
	out = append(out, "")
	return out
}

// permissionRows converts the measured capability report into the rows the
// Permissions view draws. Every row keeps its evidence and its least-privilege
// remedy; a verdict the host could not measure stays UNKNOWN.
func permissionRows(rep security.CapabilityReport) PermissionReport {
	out := PermissionReport{At: rep.At}
	for _, c := range rep.Caps {
		out.Rows = append(out.Rows, PermissionRow{
			Name:    c.Name,
			Status:  string(c.Status),
			State:   permissionState(string(c.Status)),
			Reason:  c.Reason,
			Feature: c.Feature,
			Remedy:  c.Remedy,
		})
	}
	return out
}

// destroyRequest tears the sandbox down and removes the session workspace.
//
// This is the only irreversible action in SBT, and it runs only after the user
// typed the exact confirmation phrase. The wording of the notice matters: SBT
// unlinks the workspace and stops the helper, but a filesystem journal may
// still hold blocks it cannot reach, so the notice says "removed" and not
// "securely erased". Overstating the guarantee here would be the worst possible
// lie for a tool whose job is to be honest about what it cannot prove.
func (s *Session) destroyRequest(now time.Time) {
	ui := s.App.State
	dir := s.WorkspaceDir
	if s.runner != nil {
		if err := s.runner.Stop(); err != nil {
			ui.Toasts.Notify(StateWarn, "sandbox stop reported a problem", err.Error(), now)
		}
	}
	removed := ""
	if s.store != nil {
		if err := s.store.Remove(); err != nil {
			ui.Toasts.Notify(StateWarn, "workspace removed with a problem",
				dir+" : "+err.Error(), now)
		} else {
			removed = dir
		}
	} else {
		removed = "the session workspace (nothing to remove)"
	}
	s.WorkspaceDir = ""
	detail := "the sandbox was stopped and " + removed + " was unlinked."
	detail += " Files were removed by an ordinary unlink: the filesystem may still hold copies in its journal."
	ui.Toasts.Notify(StateWarn, "sandbox destroyed", detail, now)
	ui.flash("sandbox destroyed", StateWarn, now)
}

// updateAlert raises or clears the warning banner from the cage verdict.
//
// The banner is derived from a real condition - a probe that could not verify
// isolation - and it is keyed on the verdict so an unchanged verdict does not
// restart the clock every frame. A broken cage holds the banner up as critical:
// that condition is not something to let fade while the session is still open.
func (s *Session) updateAlert(dst *Snapshot, now time.Time) {
	ui := s.App.State
	verdict := dst.Cage.State
	if verdict != CageBroken && verdict != CageLimited {
		s.alertKey = ""
		ui.Alert = AlertState{}
		return
	}
	key := verdict.Label() + "|" + dst.ProbeFail
	if key == s.alertKey {
		return
	}
	s.alertKey = key
	title := "ISOLATION LIMITED"
	body := []string{"some isolation could not be verified"}
	if verdict == CageBroken {
		title = "CAGE BROKEN"
		body = []string{"dangerous execution is refused until isolation is verified"}
	}
	if dst.ProbeFail != "" {
		body = append(body, dst.ProbeFail)
	}
	if dst.Cage.Reason != "" {
		body = append(body, dst.Cage.Reason)
	}
	ui.Alert = AlertState{
		Kind:     StateWarn,
		Title:    title,
		Body:     body,
		Since:    now,
		Critical: verdict == CageBroken,
	}
	if verdict == CageBroken {
		ui.Alert.Kind = StateDanger
	}
}

// updateConfinement fills the isolation evidence panel while a sandbox is
// running. Everything in it comes from state the session measured: the sandbox
// id, the command from the journal, and the policy that is actually in force.
func (s *Session) updateConfinement(dst *Snapshot) {
	if !dst.Sandbox.Running {
		dst.Confinement = IsolationState{}
		return
	}
	process := "sandbox"
	if last, ok := dst.LatestRun(); ok && len(last.Command) > 0 {
		process = strings.Join(last.Command, " ")
	}
	filesystem, network := "RESTRICTED", "BLOCKED"
	if s.preset.Mode == policy.High {
		filesystem, network = "RESTRICTED", "ALLOWED"
	}
	dst.Confinement = IsolationState{
		Active:      true,
		Process:     process,
		PID:         dst.Sandbox.ID,
		Reason:      policyWord(s.preset.Mode, s.preset) + " policy",
		Filesystem:  filesystem,
		Network:     network,
		Permissions: "MAPPED USER  " + policyWord(s.preset.Mode, s.preset),
		Since:       dst.Now,
	}
}
