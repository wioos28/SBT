package tui

import (
	"context"
	"time"
)

// The startup driver.
//
// These are the App methods that own the boot and welcome phases. They live in
// their own file because the sequence is a phase of the UI loop rather than part
// of the loop itself, and keeping it apart makes the rule easy to see: the
// session records what it did, this file paces the reveal, and neither one
// invents a stage.

// bootDuration is how long the startup reveal takes. It is deliberately short:
// it paces work that has already finished, so it must never become a delay the
// user has to sit through.
const bootDuration = 1100 * time.Millisecond

// welcomeMaxWait bounds the welcome screen so a stray paste cannot hold it open
// forever.
const welcomeMaxWait = 30 * time.Second

// SetBootOptions configures the startup sequence from user settings.
func (a *App) SetBootOptions(animation, welcome bool) {
	a.StartupAnim = animation
	a.ShowWelcome = welcome
}

// BootStage records one real startup step. The session calls it as each check
// finishes, so the boot screen can only show work that already happened: there
// is no path that adds a stage without doing the step behind it.
func (a *App) BootStage(label, detail string, ok bool) {
	a.State.Boot.Add(label, detail, ok, time.Now())
}

// Reveal advances the animation to this instant and reports whether every stage
// has been revealed.
func (b *BootSequence) RevealStep(elapsed time.Duration) bool {
	if len(b.Stages) == 0 {
		return true
	}
	n := float64(len(b.Stages))
	b.Reveal = n * float64(elapsed) / float64(bootDuration)
	if b.Reveal > n {
		b.Reveal = n
	}
	return b.Reveal >= n
}

// drawBootFrame paints one startup frame.
func (a *App) drawBootFrame(now time.Time) {
	if a.Screen.Resized() {
		w, h := a.Screen.Size()
		a.State.Width, a.State.Height = w, h
	}
	a.State.Boot.Phase = BootRunning
	b := NewBuffer(a.State.Width, a.State.Height)
	a.Interp.Boot(b, &a.State.Boot, now)
	a.Screen.cur.CopyFrom(b)
	a.Screen.Present()
}

// PlayBoot reveals the stages the session recorded.
//
// With motion off, or the startup animation disabled, it completes instantly
// and leaves the final state to be drawn: the still frame is the same interface
// rather than a different one. Any key press skips it, and that key is consumed
// rather than queued, so a user who types during startup does not run a command
// they never finished.
func (a *App) PlayBoot(ctx context.Context) error {
	boot := &a.State.Boot
	if !a.StartupAnim || !a.State.Motion || len(boot.Stages) == 0 {
		boot.Reveal = float64(len(boot.Stages))
		boot.Phase = BootCage
		return nil
	}
	boot.Phase = BootRunning
	start := time.Now()
	skip := false
	for {
		now := time.Now()
		if boot.RevealStep(now.Sub(start)) {
			skip = true
		}
		a.drawBootFrame(now)
		if skip {
			break
		}
		select {
		case <-ctx.Done():
			boot.Reveal = float64(len(boot.Stages))
			boot.Phase = BootCage
			return nil
		case r := <-a.keys:
			if r.err != nil {
				boot.Phase = BootCage
				return r.err
			}
			skip = true
		case <-time.After(frameInterval):
		}
	}
	boot.Reveal = float64(len(boot.Stages))
	boot.Phase = BootCage
	a.Screen.Invalidate()
	return nil
}

// drawWelcomeFrame paints the welcome panel over the ground colour.
func (a *App) drawWelcomeFrame(now time.Time) {
	if a.Screen.Resized() {
		w, h := a.Screen.Size()
		a.State.Width, a.State.Height = w, h
	}
	a.Snap.Now = now
	if a.OnFrame != nil {
		a.OnFrame(&a.Snap)
	}
	t := a.Theme
	b := NewBuffer(a.State.Width, a.State.Height)
	b.Fill(0, 0, b.W, b.H, ' ', Style{Bg: t.Palette.Bg, HasBg: true, Fg: t.Palette.Text})
	a.Interp.Welcome(b, &a.Snap, a.State, now)
	a.Screen.cur.CopyFrom(b)
	a.Screen.Present()
}

// PlayWelcome shows the welcome panel until the user presses a key.
func (a *App) PlayWelcome(ctx context.Context) error {
	if !a.ShowWelcome {
		return nil
	}
	a.State.Boot.Phase = BootWelcome
	a.drawWelcomeFrame(time.Now())
	timer := time.NewTimer(welcomeMaxWait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil
	case r := <-a.keys:
		if r.err != nil {
			return r.err
		}
	case <-timer.C:
	}
	a.Screen.Invalidate()
	return nil
}
