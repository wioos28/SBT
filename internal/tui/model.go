package tui

import (
	"time"

	"github.com/wioos28/sbt/internal/journal"
	"github.com/wioos28/sbt/internal/monitor"
	"github.com/wioos28/sbt/internal/shared/policy"
	"github.com/wioos28/sbt/internal/shared/workspace"
)

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
)

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

	// Session policy and cage state.
	Mode   policy.Mode
	Policy policy.Preset
	Cage   CageState

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
	Notes        []string

	// Resources.
	Stats monitor.Snapshot
	Peak  monitor.Snapshot

	// Export destination the user picked.
	Destination string

	// Now is the clock the UI renders; the session sets it so tests are stable.
	Now time.Time
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
)

// Request is one instruction from the UI to the session.
type Request struct {
	Kind        RequestKind
	Argv        []string
	Paths       []string
	Destination string
	Overwrite   bool
	Policy      policy.Preset
}

// RunRequest builds a run request.
func RunRequest(argv []string) *Request { return &Request{Kind: ReqRun, Argv: argv} }
