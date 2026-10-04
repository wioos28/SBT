package tui

import (
	"strings"
	"time"
)

// Alerts and isolation.
//
// Two ideas live here. An Alert is a prominent banner for a warning or a
// critical event, drawn with the border pulse the product asks for and without
// any infinite flash. IsolationState is the evidence panel shown while SBT has
// confined a process or session, so the user can always inspect why it happened
// and what is restricted - an isolation the user cannot inspect is an isolation
// they cannot trust.

// AlertState is a banner shown for a warning or a critical event.
//
// A critical alert stays up until the situation changes: a critical condition is
// not something to scroll past, and the cage keeps reporting it in the status
// bar regardless of whether the banner is on screen.
type AlertState struct {
	Kind  StateKind
	Title string
	Body  []string
	Since time.Time
	// Critical holds the banner up instead of letting it fade out.
	Critical bool
}

// alertLife is how long a non-critical banner stays up. Long enough to read,
// short enough not to become wallpaper.
const alertLife = 6 * time.Second

// Active reports whether the banner is currently shown.
func (a AlertState) Active(now time.Time) bool {
	if a.Title == "" {
		return false
	}
	if a.Critical {
		return true
	}
	return now.Sub(a.Since) < alertLife
}

// Fade is the banner opacity: critical alerts hold at full, warnings ease out.
func (a AlertState) Fade(now time.Time) float64 {
	if a.Critical {
		return 1
	}
	if a.Title == "" {
		return 0
	}
	elapsed := now.Sub(a.Since)
	if elapsed < ViewFade {
		return 0.6 + 0.4*EaseOutCubic(float64(elapsed)/float64(ViewFade))
	}
	remain := alertLife - elapsed
	if remain <= 0 {
		return 0
	}
	if remain < ViewFade {
		return float64(remain) / float64(ViewFade)
	}
	return 1
}

// Pulse is the border-pulse amount for the banner, 0..1. It shares the Breath
// curve, so with motion off it is simply zero: the still frame is the same
// banner, not a different one.
func (a AlertState) Pulse(now time.Time, motion bool) float64 {
	if !motion || !a.Active(now) {
		return 0
	}
	period := PulsePeriod
	if a.Critical {
		period = PulsePeriod / 2
	}
	return Breath(now, a.Since, period)
}

// alertBanner draws the full-width banner and returns the row after it. The
// pulse lives in the colour, never in the text: the wording must stay readable
// at every frame, which is also why a critical event breathes faster but never
// strobes.
func (i *Interpreter) alertBanner(b *Buffer, a AlertState, w, y int, now time.Time, anim bool) int {
	if !a.Active(now) || y < 0 || y >= b.H {
		return y
	}
	t := i.Theme
	p := t.Palette
	fade := a.Fade(now)
	accent := p.Warning
	if a.Kind == StateDanger {
		accent = p.Red
	}
	accent = Mix(p.Surface2, accent, 0.55+0.45*fade)
	if pulse := a.Pulse(now, t.Motion && anim); pulse > 0 {
		accent = Glow(accent, 0.25*pulse)
	}
	b.Fill(0, y, w, 1, ' ', Style{Bg: accent, HasBg: true, Fg: p.Text})
	mark := "! "
	if a.Kind == StateDanger {
		mark = "!! "
	}
	col := b.Write(1, y, mark, Style{Bg: accent, HasBg: true, Fg: p.Text, Bold: true})
	col = b.Write(col, y, a.Title, Style{Bg: accent, HasBg: true, Fg: p.Text, Bold: true})
	if len(a.Body) > 0 && col+2 < w {
		b.WriteClipped(col+2, y, w-1, strings.Join(a.Body, "  "),
			Style{Bg: accent, HasBg: true, Fg: p.Text})
	}
	return y + 1
}

// IsolationState describes an active confinement. Every field is filled from
// what the session actually did, never from what it intended.
type IsolationState struct {
	Active      bool
	Process     string
	PID         string
	Reason      string
	Filesystem  string
	Network     string
	Permissions string
	Since       time.Time
}

// isolationPanel draws the ISOLATION ACTIVE evidence box.
//
// It is deliberately factual: what is confined, under which id, why, and what
// each dimension is restricted to. The user is meant to be able to read this
// and understand the confinement without opening a second screen.
func (i *Interpreter) isolationPanel(b *Buffer, iso IsolationState, st *UIState, now time.Time, glow bool) {
	if !iso.Active || st.Width < 34 || st.Height < 10 {
		return
	}
	t := i.Theme
	p := t.Palette
	w := 46
	if st.Width-4 < w {
		w = st.Width - 4
	}
	h := 9
	x := st.Width - w - 1
	if x < 0 {
		x = 0
	}
	y := st.Height - h - 1
	if y < 2 {
		y = 2
	}
	accent := p.Primary
	if t.Motion && glow {
		if pulse := Breath(now, iso.Since, PulsePeriod); pulse > 0 {
			accent = Glow(accent, 0.2*pulse)
		}
	}
	inner := t.Panel(b, x, y, w, h, "isolation active", "alt+4 status", accent, true)
	if inner.Empty() {
		return
	}
	row := 0
	put := func(label, value string, sty Style) {
		if inner.Y+row >= inner.Bottom() {
			return
		}
		b.WriteClipped(inner.X, inner.Y+row, inner.Right(), Pad(label, 13), Style{Fg: p.Muted})
		b.WriteClipped(inner.X+13, inner.Y+row, inner.Right(), value, sty)
		row++
	}
	put("Process", iso.Process, Style{Fg: p.Text, Bold: true})
	put("PID", iso.PID, Style{Fg: p.Text})
	put("Reason", iso.Reason, Style{Fg: p.Warning})
	put("Filesystem", iso.Filesystem, Style{Fg: p.Text})
	put("Network", iso.Network, Style{Fg: p.Text})
	put("Permissions", iso.Permissions, Style{Fg: p.Text})
}
