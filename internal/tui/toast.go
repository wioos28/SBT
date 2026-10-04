package tui

import "time"

// A toast is a transient, non-blocking notice. SBT uses toasts for anything the
// user must know but did not have to interrupt them for: a finished run, a
// change that needs review, an isolation feature that dropped to partial.
//
// Two rules keep toasts honest. They never hide state that only exists in a
// toast - the cage verdict also lives in the status bar, so dismissing a toast
// cannot make a warning disappear. And a danger toast outranks an ok toast of
// the same moment, so a success never buries a warning.
type Toast struct {
	ID     int
	Text   string
	Detail string
	Kind   StateKind
	// Icon is the glyph shown before the text; always paired with a word.
	Icon  string
	SetAt time.Time
	// Life is how long the toast stays fully visible before it fades out.
	Life time.Duration
}

// Expired reports whether the toast has outlived its life plus its fade.
func (t Toast) Expired(now time.Time) bool {
	if t.Life <= 0 {
		return true
	}
	return now.Sub(t.SetAt) > t.Life+ToastFade
}

// Fade is the toast opacity at this instant: it rises, holds, then falls. A
// danger toast is never faded all the way out while it is the newest notice,
// because a warning that vanishes unread is a warning that was not given.
//
// A toast that was created at this very instant reads as fully visible: the
// fade-in eases away from zero rather than starting there, so the frame that
// raises the notice is the frame that shows it.
func (t Toast) Fade(now time.Time) float64 {
	if t.Life <= 0 {
		return 0
	}
	elapsed := now.Sub(t.SetAt)
	if elapsed < 0 {
		return 0
	}
	if elapsed < ToastFade {
		// Ease from a visible floor to full, so the first frame is legible.
		return 0.6 + 0.4*EaseOutCubic(float64(elapsed)/float64(ToastFade))
	}
	if elapsed > t.Life {
		remain := t.Life + ToastFade - elapsed
		if remain < 0 {
			return 0
		}
		return float64(remain) / float64(ToastFade)
	}
	return 1
}

// defaultLife is how long each severity stays up. A warning is deliberately
// given longer than a confirmation: the user has to be able to read it and then
// look up what it refers to.
func defaultLife(kind StateKind) time.Duration {
	switch kind {
	case StateDanger:
		return 9 * time.Second
	case StateWarn:
		return 7 * time.Second
	case StateOK:
		return 4 * time.Second
	default:
		return 3 * time.Second
	}
}

// Toasts is the ordered stack of notices shown in the corner of the cage. It
// holds no clock of its own: the caller passes now, which keeps it testable.
type Toasts struct {
	Items []Toast
	next  int
}

// Max is how many toasts are shown at once. Older notices are dropped rather
// than queued, so the corner of the screen never grows into a wall of text.
const MaxToasts = 3

// Notify records a new toast and returns the one that was stored.
func (ts *Toasts) Notify(kind StateKind, text, detail string, now time.Time) Toast {
	ts.next++
	t := Toast{
		ID:     ts.next,
		Text:   text,
		Detail: detail,
		Kind:   kind,
		Icon:   glyphForKind(kind),
		SetAt:  now,
		Life:   defaultLife(kind),
	}
	// A newer notice of equal or higher severity goes first: the corner shows
	// the most urgent news, not the most recent trivia.
	ts.Items = append(ts.Items, t)
	ts.sort()
	ts.trim(now)
	return t
}

// sort orders newest first, breaking ties by severity so a danger notice is
// never pushed below an ok one from the same moment.
func (ts *Toasts) sort() {
	for i := 1; i < len(ts.Items); i++ {
		for j := i; j > 0; j-- {
			a, b := ts.Items[j-1], ts.Items[j]
			if a.SetAt.After(b.SetAt) || (a.SetAt.Equal(b.SetAt) && a.Kind >= b.Kind) {
				break
			}
			ts.Items[j-1], ts.Items[j] = ts.Items[j], ts.Items[j-1]
		}
	}
}

// trim drops notices that have already faded out, then caps the stack.
func (ts *Toasts) trim(now time.Time) {
	live := ts.Items[:0]
	for _, t := range ts.Items {
		if !t.Expired(now) {
			live = append(live, t)
		}
	}
	ts.Items = live
	if len(ts.Items) > MaxToasts {
		ts.Items = ts.Items[:MaxToasts]
	}
}

// Expire removes notices that have finished.
func (ts *Toasts) Expire(now time.Time) { ts.trim(now) }

// Live returns the visible toasts, newest first.
func (ts *Toasts) Live(now time.Time) []Toast {
	ts.trim(now)
	return ts.Items
}

// Worst is the highest severity currently on screen, which is what the status
// bar reports so that a danger toast also shows up in the always-present state
// line.
func (ts *Toasts) Worst(now time.Time) StateKind {
	worst := StateOff
	for _, t := range ts.Items {
		if t.Kind > worst && !t.Expired(now) {
			worst = t.Kind
		}
	}
	return worst
}

// Clear drops every toast.
func (ts *Toasts) Clear() { ts.Items = nil }

// glyphForKind picks the mark for a severity. The glyph is a hint; the text
// always carries the meaning.
func glyphForKind(kind StateKind) string {
	switch kind {
	case StateOK:
		return "✓"
	case StateWarn:
		return "!"
	case StateDanger:
		return "✕"
	default:
		return "·"
	}
}
