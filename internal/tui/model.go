package tui

import (
	"os"
	"strings"
	"time"

	"github.com/wioos28/sbt/internal/journal"
	"github.com/wioos28/sbt/internal/monitor"
	"github.com/wioos28/sbt/internal/shared/policy"
	"github.com/wioos28/sbt/internal/shared/workspace"
)

// FileInfo is the UI-facing subset of journal.FileInfo used by export and file lists.
type FileInfo = journal.FileInfo

// PolicyChoice is the confirmation flow for policy changes.
type PolicyChoice int

const (
	PolicyLow PolicyChoice = iota
	PolicyHigh
)

// BootState tracks initial shell startup.
type BootState struct {
	Started time.Time
	Done    bool
}

// Cage is the session state the status strip reports.
type Cage struct {
	State  CageState
	Reason string
}

// modeWord is the trust level as the top bar shows it.
func modeWord(m policy.Mode) string {
	switch m {
	case policy.High:
		return "HIGH"
	case policy.Low:
		return "LOW"
	default:
		return "NORMAL"
	}
}

func policyWord(m policy.Mode, p policy.Preset) string {
	if p.Mode == policy.High || m == policy.High {
		return "HIGH"
	}
	if p.Mode == policy.Low || m == policy.Low {
		return "LOW"
	}
	return "NORMAL"
}

func fields(s string) []string { return strings.Fields(s) }

func osGetenv(k string) string { return os.Getenv(k) }

// HelpLines documents the cage's keys. It is built from the command registry so
// a binding and the help the user reads cannot drift apart.
var HelpLines = helpLines()

// helpViews is every view the rail and the menu can reach.
var helpViews = Views()

func helpLines() []string {
	lines := []string{
		"SBT cage",
		"  " + helpViewKeys() + "  switch view",
		"  f10 / alt+m        open the menu bar",
		"  ctrl+k             command palette",
		"  enter              run the command in the input line",
		"  enter (changes)    review the selected change",
		"  ctrl+.             stop the running sandbox",
		"  ctrl+d             leave (always asks first)",
		"  esc                close the overlay, or go back",
		"",
		"Look  menu switches the white / dark theme; SBT_THEME=light",
		"or SBT_THEME=dark picks one at startup.",
		"",
		"The status bar always states what the cage enforces, so the",
		"session never depends on a colour or a menu to be understood.",
	}
	return lines
}

// helpViewKeys renders the view shortcuts as one range.
func helpViewKeys() string {
	first, last := helpViews[0].Shortcut(), helpViews[len(helpViews)-1].Shortcut()
	return first + ".." + last[len("alt+"):]
}

// View is one screen inside the workspace area.
type View int

// The views reachable from the navigation rail and the command palette.
const (
	ViewTerminal View = iota
	ViewFiles
	ViewChanges
	ViewStatus
	ViewExport
	ViewHelp
	ViewSettings
	ViewPermissions
)

// Views lists every view in rail order. It is the single source of truth for the
// alt+<n> range, the rail and the switch-view help line, so a view can never be
// added to one of those and forgotten in the others.
func Views() []View {
	return []View{ViewTerminal, ViewFiles, ViewChanges, ViewStatus, ViewExport, ViewHelp, ViewSettings, ViewPermissions}
}

// Name is the rail label of a view.
func (v View) Name() string {
	switch v {
	case ViewFiles:
		return "Files"
	case ViewChanges:
		return "Changes"
	case ViewStatus:
		return "Status"
	case ViewExport:
		return "Export"
	case ViewHelp:
		return "Help"
	case ViewSettings:
		return "Settings"
	case ViewPermissions:
		return "Permissions"
	default:
		return "Terminal"
	}
}

// Shortcut is the key that selects a view directly.
func (v View) Shortcut() string {
	return "alt+" + itoa(int(v)+1)
}

// StateKind is the severity of one status line. It always travels with a word,
// so the interface keeps its meaning without colour.
type StateKind int

// Status severities.
const (
	StateOff StateKind = iota
	StateOK
	StateWarn
	StateDanger
	StateMeta
)

// Word is the severity as text.
func (s StateKind) Word() string {
	switch s {
	case StateOK:
		return "ok"
	case StateWarn:
		return "limited"
	case StateDanger:
		return "danger"
	default:
		return "off"
	}
}

// IsolationLine is one row of the security panel: what SBT actually enforces.
type IsolationLine struct {
	Label  string
	Value  string // ISOLATED / BLOCKED / RESTRICTED / UNAVAILABLE
	State  StateKind
	Reason string
}

// SandboxState describes the sandbox of the current run.
type SandboxState struct {
	// Running is true while a helper process is alive.
	Running bool
	// ID is the sandbox identifier of the running (or last) sandbox.
	ID string
	// LastExit is the exit code of the last finished command.
	LastExit int
	// Reason is the last note SBT produced about the sandbox.
	Reason string
	// Unavailable explains why a sandbox cannot start right now (empty when it
	// can).
	Unavailable string
}

// RunSummary is the terminal transcript's record of one finished command. The
// command's own output went to the real terminal while the cage was suspended,
// so the transcript carries the facts SBT measured: exit code, duration, the
// policy in force and what changed.
type RunSummary struct {
	Command  string
	Exit     int
	Duration time.Duration
	Counts   workspace.Counts
	Policy   string
	Note     string
}

// Line is one transcript line with its severity.
type Line struct {
	Text  string
	State StateKind
	Meta  bool // secondary line: indented, muted
}

// Snapshot is everything the UI is allowed to draw. The session fills it in
// from real sources; the UI never derives a value it was not given.
type Snapshot struct {
	// Identity.
	Version  string
	Platform string
	Backend  string
	Host     string
	Kernel   string
	OS       string
	Arch     string

	// Session policy and cage state.
	Mode   policy.Mode
	Policy policy.Preset
	Cage   Cage

	// Isolation evidence, straight from the platform probe.
	Isolation []IsolationLine
	Warnings  []string
	ProbeFail string

	// Sandbox state.
	Sandbox SandboxState

	// Session workspace.
	WorkspaceDir string
	Files        []journal.FileInfo
	Runs         []workspace.Run
	// RunIDs are the journal ids parallel to Runs. The UI needs them to ask
	// for a diff: a run is addressed by the directory it lives in, not by its
	// position in this list.
	RunIDs []string
	Notes  []string

	// Diff is the before/after content of the change the changes view has
	// selected. It is filled by the session from the journal, never computed
	// by the renderer.
	Diff DiffState

	// Resources.
	Stats monitor.Snapshot
	Peak  monitor.Snapshot

	// Export destination the user picked.
	Destination string

	// Now is the clock the UI renders; the session sets it so tests are stable.
	Now time.Time

	// Light records which ground the interface is drawn on, so the menu can
	// show the current choice as a tick rather than making the user remember.
	Light bool

	// Settings is the live configuration the Settings view edits, keyed by the
	// same keys the config schema declares. The UI shows what the session read
	// and asks the session to change it; it never writes the file itself.
	Settings map[string]any

	// Permissions is the measured capability report the Permissions view draws.
	Permissions PermissionReport

	// Confinement is the evidence panel shown while SBT has isolated a process
	// or session. It is named apart from Isolation, which is the older list of
	// what the probe enforces.
	Confinement IsolationState

	// Palette is the active dark-ground preset name. It is shown in the top bar
	// so the current look is never a guess.
	Palette string
}

// Counts sums the changes of every finished run.
func (s Snapshot) Counts() workspace.Counts {
	var c workspace.Counts
	for _, r := range s.Runs {
		rc := r.Counts()
		c.Added += rc.Added
		c.Modified += rc.Modified
		c.Deleted += rc.Deleted
		c.Warnings += rc.Warnings
	}
	return c
}

// LatestRun is the most recent finished run, if any.
func (s Snapshot) LatestRun() (workspace.Run, bool) {
	if len(s.Runs) == 0 {
		return workspace.Run{}, false
	}
	return s.Runs[len(s.Runs)-1], true
}

// RequestKind is what the UI asks the session to do. The UI never touches the
// sandbox, the policy or the filesystem itself: it asks, and the session - the
// only component allowed to make security decisions - answers.
type RequestKind int

// Requests the UI can raise.
const (
	// ReqRun runs argv inside a fresh sandbox.
	ReqRun RequestKind = iota
	// ReqStop stops the running sandbox.
	ReqStop
	// ReqSetPolicy replaces the session policy after a confirmation.
	ReqSetPolicy
	// ReqExport writes the selected workspace paths to a destination.
	ReqExport
	// ReqDiscard deletes the session workspace.
	ReqDiscard
	// ReqExit closes the session.
	ReqExit
	// ReqResize tells the session the terminal geometry changed.
	ReqResize
	// ReqRefresh asks the session to re-run the platform probe and re-sample
	// the monitor, so the isolation report cannot go stale while the session
	// is open.
	ReqRefresh
	// ReqDiff asks the session to load the before/after content of one path in
	// one run. Reading content is the session's job for the same reason running
	// a sandbox is: it is the only component that knows where runs are stored.
	ReqDiff
	// ReqOpenPolicy asks the session to change the policy of the next sandbox.
	ReqOpenPolicy
	// ReqRepairCage asks the session to re-initialise the cage: re-run the
	// probe and re-sample the monitor. It repairs, it never disables.
	ReqRepairCage
	// ReqDestroy is the explicit sandbox destruction. The session only ever
	// receives it after the user typed the exact confirmation phrase.
	ReqDestroy
	// ReqSetSetting changes one configuration value. The Settings view never
	// writes the file itself: it validates, asks, and the session stores and
	// applies the change so the running interface follows immediately.
	ReqSetSetting
)

// Request is one instruction from the UI to the session.
type Request struct {
	Kind        RequestKind
	Argv        []string
	Paths       []string
	Destination string
	Overwrite   bool
	Policy      policy.Preset
	// RunID and Entry address one changed path: the run directory it was
	// recorded in and the workspace-relative path inside it.
	RunID  string
	Entry  string
	Choice PolicyChoice
	// Key and Value carry a settings change.
	Key   string
	Value any
	// Scope records which answer the user gave to a permission prompt. It is
	// context for the session, not a grant: the cage is unchanged either way.
	Scope PermissionChoice
}

// RunRequest builds a run request.
func RunRequest(argv []string) *Request { return &Request{Kind: ReqRun, Argv: argv} }
