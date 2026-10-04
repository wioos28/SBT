// Package security is SBT's central security layer: warning levels, the
// security timeline, the sandbox scanner, the self-test harness and the fix
// recommendation engine.
//
// Everything here is fail-closed. A check that cannot be performed returns
// Unknown or NOT AVAILABLE rather than a pass, and a CRITICAL finding is meant
// to stop or freeze the affected sandbox, never to be silently ignored.
package security

import (
	"strings"

	"github.com/wioos28/sbt/internal/i18n"
)

// Level is the severity of a security warning or event. The numeric order is the
// severity order: a higher value is more severe.
type Level int

// Severity levels, lowest first.
const (
	// Info is informational.
	Info Level = iota
	// Notice is worth knowing but not a problem.
	Notice
	// Warning is a policy-relevant fact that should be reviewed.
	Warning
	// Danger is a problem that must block the risky operation.
	Danger
	// Critical is an isolation failure that must stop or freeze the sandbox.
	Critical
)

// String renders the level.
func (l Level) String() string {
	switch l {
	case Info:
		return "INFO"
	case Notice:
		return "NOTICE"
	case Warning:
		return "WARNING"
	case Danger:
		return "DANGER"
	case Critical:
		return "CRITICAL"
	default:
		return "INFO"
	}
}

// Rank returns the severity order of a level.
func (l Level) Rank() int { return int(l) }

// AtLeast reports whether l is at least as severe as other.
func (l Level) AtLeast(other Level) bool { return l >= other }

// ParseLevel decodes a level name (case-insensitive), returning ok=false for an
// unknown name so callers can report a typo instead of guessing.
func ParseLevel(s string) (Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "info":
		return Info, true
	case "notice":
		return Notice, true
	case "warning", "warn":
		return Warning, true
	case "danger", "dangerous":
		return Danger, true
	case "critical", "crit":
		return Critical, true
	default:
		return Info, false
	}
}

// Translate returns the localised name of a level.
func (l Level) Translate() string {
	switch l {
	case Info:
		return i18n.T(i18n.KeyLevelInfo)
	case Notice:
		return i18n.T(i18n.KeyLevelNotice)
	case Warning:
		return i18n.T(i18n.KeyLevelWarning)
	case Danger:
		return i18n.T(i18n.KeyLevelDanger)
	case Critical:
		return i18n.T(i18n.KeyLevelCritical)
	default:
		return l.String()
	}
}
