package tui

import (
	"math"
	"time"
)

// The animation engine.
//
// Every animation in SBT is a pure function of a clock the caller supplies.
// Nothing in this file reads the wall clock and nothing keeps state between
// frames: drawing at time T always produces the same pixels, whatever happened
// in between. That is what makes motion testable, and it is why the renderer
// can stay a pure function of (Snapshot, UIState).
//
// Accessibility is built in rather than bolted on. Every effect here has a
// static form, and one Motion flag selects between them, so honouring
// NO_MOTION means "draw the still frame", not "draw a different interface".

// Cycle lengths of the built in effects. They are deliberately unhurried:
// motion that pulls the eye interrupts reading, and SBT is mostly read.
const (
	// SpinnerInterval is the delay between spinner frames.
	SpinnerInterval = 90 * time.Millisecond
	// PulsePeriod is one full breath of a pulsing element.
	PulsePeriod = 1700 * time.Millisecond
	// SweepPeriod is one pass of a travelling highlight.
	SweepPeriod = 2400 * time.Millisecond
	// BlinkPeriod is the on/off cycle of a blinking warning.
	BlinkPeriod = 850 * time.Millisecond
	// ViewFade is how long a view takes to fade in after a switch.
	ViewFade = 170 * time.Millisecond
	// ToastFade is the leading and trailing fade of a notification.
	ToastFade = 180 * time.Millisecond
)

// Phase is the position within a repeating cycle, 0..1, measured from start.
func Phase(now, start time.Time, period time.Duration) float64 {
	if period <= 0 {
		return 0
	}
	return PhaseFrom(now.Sub(start), period)
}

// PhaseFrom is Phase for a plain elapsed duration.
func PhaseFrom(elapsed, period time.Duration) float64 {
	if period <= 0 || elapsed < 0 {
		return 0
	}
	// The division is done in float: an integer quotient would collapse every
	// sub-interval to 0 and a spinner would sit on its first frame forever.
	return float64(elapsed%period) / float64(period)
}

// EaseOutCubic decelerates: quick to start, settling gently into place.
func EaseOutCubic(t float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	u := 1 - t
	return 1 - u*u*u
}

// EaseInOutCubic accelerates then decelerates.
func EaseInOutCubic(t float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	if t < 0.5 {
		return 4 * t * t * t
	}
	u := -2*t + 2
	return 1 - u*u*u/2
}

// Breath is a smooth 0..1 sine over one cycle. It is the curve behind every
// glow, so a pulsing element brightens and dims rather than snapping.
func Breath(now, start time.Time, period time.Duration) float64 {
	return 0.5 + 0.5*math.Sin(2*math.Pi*Phase(now, start, period))
}

// Blink reports whether a blinking element is lit at this instant.
func Blink(now, start time.Time, period time.Duration) bool {
	return Phase(now, start, period) < 0.5
}

// Sweep is the column of a highlight travelling across a bar of the given
// width. The caller clamps it, which is what makes it read as a bar that fills
// rather than an indicator that wanders off the end.
func Sweep(now, start time.Time, period time.Duration, width int) int {
	if width <= 0 {
		return 0
	}
	return int(Phase(now, start, period) * float64(width))
}

// SpinnerFrame is the index of the spinner frame for an n frame set.
func SpinnerFrame(now, start time.Time, frames int) int {
	if frames <= 0 {
		return 0
	}
	return int(Phase(now, start, SpinnerInterval) * float64(frames))
}

// FadeIn is 0..1 over the first part of a duration: an element that appears.
//
// It does not wrap. A one-shot transition must reach 1 and stay there, which is
// why this is not Phase: wrapping would send a completed fade back to 0 and an
// animation would never finish.
func FadeIn(now, start time.Time, d time.Duration) float64 {
	if d <= 0 {
		return 1
	}
	elapsed := now.Sub(start)
	if elapsed >= d {
		return 1
	}
	if elapsed <= 0 {
		return 0
	}
	return float64(elapsed) / float64(d)
}

// ViewTransition remembers where a view switch came from and when it happened,
// so the incoming view eases in rather than snapping into place.
type ViewTransition struct {
	From View
	To   View
	At   time.Time
}

// Progress is how far through the view fade we are, 0..1. A transition that was
// never started is finished, which keeps a still UI from being treated as
// mid-animation forever.
func (v ViewTransition) Progress(now time.Time) float64 {
	if v.At.IsZero() {
		return 1
	}
	return FadeIn(now, v.At, ViewFade)
}

// Fade is the eased opacity of the incoming view.
func (v ViewTransition) Fade(now time.Time) float64 { return EaseOutCubic(v.Progress(now)) }

// DiffState is the before/after content of one changed path, as the session
// loaded it from the journal.
//
// The renderer never diffs anything itself: it draws these lines. That keeps
// the "SBT shows what it measured, not what it assumed" rule intact - the diff
// is produced from the content the helper actually captured, and when it could
// not be produced the reason is in Note rather than a silently empty review.
type DiffState struct {
	// RunID and Path identify what is shown, so the view can tell whether the
	// loaded diff still matches the cursor.
	RunID string
	Path  string
	// Kind is the change the run recorded for this path.
	Kind string
	// Lines are the rendered diff rows, already prefixed.
	Lines []string
	// Added and Removed are the line counts, shown in the header.
	Added   int
	Removed int
	// Binary is set when the content cannot be reviewed as text.
	Binary bool
	// Truncated is set when the file was larger than the review budget.
	Truncated bool
	// Note carries any reason the diff is incomplete.
	Note string
	// Loaded reports whether a diff was read at all. It is false before
	// anything is selected and when the load failed, which is what lets the
	// view show the failure instead of an empty review.
	Loaded bool
}

// Matches reports whether this loaded diff is the one a cursor points at. The
// changes view uses it to avoid showing run 1's diff next to run 2's entry.
func (d DiffState) Matches(runID, path string) bool {
	return d.Loaded && d.RunID == runID && d.Path == path
}

// Busy is the indeterminate progress indicator shown while the session is doing
// something the UI cannot yet report. It is deliberately label-first: the user
// is told what is happening before they are told it is happening fast.
type Busy struct {
	Active bool
	Label  string
	Since  time.Time
}

// Mix blends from a to b by t in 0..1.
func Mix(a, b RGB, t float64) RGB {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	return RGB{R: mix8(a.R, b.R, t), G: mix8(a.G, b.G, t), B: mix8(a.B, b.B, t)}
}

func mix8(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t)
}

// Glow lifts a colour toward white without changing its hue, which is how a
// live element reads as lit rather than merely differently coloured.
func Glow(c RGB, amount float64) RGB { return Mix(c, RGB{255, 255, 255}, amount) }

// Mute pushes a colour toward the background, for de-emphasised text.
func Mute(c, toward RGB, amount float64) RGB { return Mix(c, toward, amount) }

// meterRise is the time a meter takes to catch up when the value goes up. It is
// long enough that a spike is legible as a movement, short enough that the
// number still feels current.
const meterRise = 320 * time.Millisecond

// Meter is one animated gauge. The renderer holds these between frames so a
// value can be approached over time instead of jumping.
//
// Rise is eased and fall is not, deliberately: a growing bar should be seen
// growing, but a shrinking bar should be believed immediately.
type Meter struct {
	// Shown is the value currently on screen.
	Shown float64
	// Target is the value most recently measured.
	Target float64
	// Since is when Target last changed.
	Since time.Time
}

// Set records a new measurement and starts the ease toward it. The currently
// shown value is captured as the new origin, so a measurement arriving mid
// animation continues from where the bar is rather than jumping back.
//
// The very first measurement is adopted immediately instead of eased: there is
// nothing to ease from, and animating up from zero would show an empty gauge
// for the first third of a second even though the value was already known.
func (m *Meter) Set(v float64, now time.Time, motion bool) {
	if v == m.Target && !m.Since.IsZero() {
		return
	}
	first := m.Since.IsZero()
	m.Shown = m.Value(now, motion)
	m.Target = v
	m.Since = now
	if first || !motion {
		m.Shown = v
	}
}

// Value is what the meter should draw right now.
func (m *Meter) Value(now time.Time, motion bool) float64 {
	if !motion || m.Since.IsZero() {
		return m.Target
	}
	if m.Target <= m.Shown {
		// Falling is immediate: it is a fact, not a transition.
		return m.Target
	}
	return m.Shown + (m.Target-m.Shown)*EaseOutCubic(FadeIn(now, m.Since, meterRise))
}

// Settled reports whether the meter has reached its target, which is what lets
// the renderer stop repainting once nothing is moving.
func (m *Meter) Settled(now time.Time, motion bool) bool {
	return m.Value(now, motion) >= m.Target
}

// Meters is the named gauge set one frame of the cage uses.
type Meters struct {
	CPU    Meter
	Memory Meter
	Procs  Meter
}

// Eased is a convenience for callers that only want the value.
func (m Meters) Eased(now time.Time, motion bool) (cpu, mem, procs float64) {
	return m.CPU.Value(now, motion), m.Memory.Value(now, motion), m.Procs.Value(now, motion)
}
