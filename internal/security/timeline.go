package security

import (
	"sync"
	"time"
)

// Timeline is the ordered record of security events, shown by `sbt security
// logs` and the Logs view. It is safe for concurrent use because TUI, web and
// sandbox goroutines all append to it.
type Timeline struct {
	mu     sync.Mutex
	alerts []Alert
	cap    int
}

// NewTimeline returns a timeline that keeps at most max recent alerts.
func NewTimeline(max int) *Timeline {
	if max <= 0 {
		max = 4096
	}
	return &Timeline{cap: max}
}

// Add records an alert, stamping it with the current time when unset.
func (t *Timeline) Add(a Alert) Alert {
	if a.At.IsZero() {
		a.At = time.Now()
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.alerts = append(t.alerts, a)
	if len(t.alerts) > t.cap {
		t.alerts = t.alerts[len(t.alerts)-t.cap:]
	}
	return a
}

// Record is a convenience for adding an event at a level.
func (t *Timeline) Record(level Level, sandbox, event string) Alert {
	return t.Add(Alert{Level: level, Sandbox: sandbox, Event: event})
}

// All returns a copy of every alert, oldest first.
func (t *Timeline) All() []Alert {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Alert, len(t.alerts))
	copy(out, t.alerts)
	return out
}

// Recent returns the last n alerts, oldest first.
func (t *Timeline) Recent(n int) []Alert {
	t.mu.Lock()
	defer t.mu.Unlock()
	if n <= 0 || n > len(t.alerts) {
		n = len(t.alerts)
	}
	out := make([]Alert, n)
	copy(out, t.alerts[len(t.alerts)-n:])
	return out
}

// CountsByLevel returns how many alerts exist at each severity.
func (t *Timeline) CountsByLevel() map[Level]int {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := map[Level]int{}
	for _, a := range t.alerts {
		out[a.Level]++
	}
	return out
}

// Critical returns the alerts at CRITICAL level.
func (t *Timeline) Critical() []Alert {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []Alert
	for _, a := range t.alerts {
		if a.Level >= Critical {
			out = append(out, a)
		}
	}
	return out
}

// Len returns the number of recorded alerts.
func (t *Timeline) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.alerts)
}

// Policy decides what SBT must do when it meets a finding of a given level.
type Policy struct{}

// Verdict tells the caller whether an operation may continue.
type Verdict int

// Verdicts, ordered from allowed to forbidden.
const (
	// Allow lets the operation proceed.
	Allow Verdict = iota
	// ContinueWithPolicy lets the operation proceed but records a warning.
	ContinueWithPolicy
	// BlockRisk forbids the specific risky operation.
	BlockRisk
	// FreezeSandbox stops or freezes the whole sandbox.
	FreezeSandbox
)

// Decide maps a severity to the required response. This is the single place the
// fail-closed policy lives, so the TUI, CLI and web UI cannot disagree.
func Decide(level Level) Verdict {
	switch {
	case level >= Critical:
		return FreezeSandbox
	case level == Danger:
		return BlockRisk
	case level == Warning:
		return ContinueWithPolicy
	default:
		return Allow
	}
}
