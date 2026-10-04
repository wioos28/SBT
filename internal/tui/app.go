package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/wioos28/sbt/internal/shared/policy"
)

// frameInterval is how often the cage repaints while something is animating.
const frameInterval = 100 * time.Millisecond

// idleInterval is the repaint cadence when nothing is visibly changing. Keeping
// a slow heartbeat lets the clock advance, but the terminal is left alone for
// most of it, so a still interface costs almost nothing.
const idleInterval = time.Second

// App is the top-level UI runtime. It owns the screen, the state machine and
// the key reader, and it is the only thing that turns keys into requests.
//
// The rule that shapes this type: the App never runs a command itself. It maps
// a key to a Request and hands it to the session, which is the only component
// allowed to make a security decision. The UI can ask; only the session can
// answer.
type App struct {
	Theme    *Theme
	Interp   *Interpreter
	Screen   *Screen
	Reader   *KeyReader
	State    *UIState
	Commands CommandSet

	// Out carries the requests the UI raises to the session. The App never
	// closes it: the session owns its lifetime.
	Out chan Request

	// StartupAnim and ShowWelcome come from user settings. They are decided once,
	// before the first frame, because a startup animation that changes midway is
	// just a flicker.
	StartupAnim bool
	ShowWelcome bool

	// OnFrame is called before every draw with the snapshot about to be drawn,
	// so a session can sample the monitor on the cadence the user actually
	// sees rather than on a timer that drifts away from the UI.
	OnFrame func(*Snapshot)

	// Snap is the current state. The session owns and mutates it; the App only
	// reads it while drawing.
	Snap Snapshot

	// keys carries decoded key presses from the single reader goroutine. One
	// goroutine is started for the life of the App, so timing out of a read
	// never leaks a goroutine and never loses a keystroke.
	keys chan readResult
}

// readResult is one decoded key or the error that ended the stream.
type readResult struct {
	key Key
	err error
}

// NewApp wires an App to a terminal.
func NewApp(in, out *os.File) (*App, error) {
	screen, err := OpenScreen(in, out)
	if err != nil {
		return nil, err
	}
	w, h := screen.Size()
	theme := NewThemeAuto()
	screen.SetTheme(theme)
	state := NewUIState(w, h)
	state.Commands = DefaultCommands()
	state.Transition = ViewTransition{To: ViewTerminal}
	app := &App{
		Theme:    theme,
		Interp:   NewInterpreter(theme),
		Screen:   screen,
		Reader:   NewKeyReader(in),
		State:    state,
		Commands: DefaultCommands(),
		Out:      make(chan Request, 8),
		keys:     make(chan readResult, 4),
	}
	// One reader goroutine for the life of the App. It parks in the terminal
	// read when the user is idle and always delivers a key, so the main loop
	// can wake on a timer without racing the input stream.
	go func() {
		for {
			k, err := app.Reader.ReadKey()
			app.keys <- readResult{k, err}
			if err != nil {
				return
			}
		}
	}()
	return app, nil
}

// Emit queues a request for the session. It never blocks the UI thread: a full
// channel means the session is behind, and dropping the oldest request is
// better than stalling the keyboard.
func (a *App) Emit(r Request) {
	select {
	case a.Out <- r:
	default:
		select {
		case <-a.Out:
		default:
		}
		select {
		case a.Out <- r:
		default:
		}
	}
}

// needsFrames reports whether the cage has something that visibly changes over
// time. When it does not, the loop slows to a heartbeat and the terminal is
// left alone.
//
// The check has to be honest in both directions. Reporting "always animating"
// whenever motion is enabled makes the loop repaint ten times a second on a
// completely idle session, which is wasted work and a measurable battery cost
// for an interface nobody is looking at; reporting "never animating" would
// freeze the spinner while a sandbox starts. So each moving part is named.
func (a *App) needsFrames() bool {
	if !a.State.Motion {
		return a.State.Busy.Active || len(a.State.Toasts.Live(a.Snap.Now)) > 0
	}
	if a.State.Busy.Active || len(a.State.Toasts.Live(a.Snap.Now)) > 0 {
		return true
	}
	// A character in the input line that is still lit is visibly animating.
	if a.State.Typing.Live(a.Snap.Now) {
		return true
	}
	// A view that is still easing in is visibly moving.
	if p := a.State.Transition.Progress(a.Snap.Now); p < 1 {
		return true
	}
	// An open menu eases its highlight in, so it is animating too.
	if a.State.Menu.Open {
		return true
	}
	// A meter that has not reached its target is still travelling.
	m := a.State.Meters
	now := a.Snap.Now
	if !m.CPU.Settled(now, true) || !m.Memory.Settled(now, true) || !m.Procs.Settled(now, true) {
		return true
	}
	// Everything else is still: a settled interface is left alone.
	return false
}

// runFrame advances the clocks, draws one frame and hands it to the screen.
func (a *App) runFrame(now time.Time) {
	a.Snap.Now = now
	a.State.FlashExpire(now)
	a.State.Toasts.Expire(now)
	if a.State.Busy.Since.IsZero() {
		a.State.Busy.Since = now
	}
	// The theme follows the snapshot's choice, so a toggle takes effect on the
	// very next frame instead of waiting for a restart. The screen is told the
	// theme changed too, because it caches the escape sequences it emits.
	if a.Theme != nil && a.Theme.Light != a.Snap.Light {
		a.Theme.SetLight(a.Snap.Light)
		a.Interp.Theme = a.Theme
		a.Screen.SetTheme(a.Theme)
		a.Screen.Invalidate()
	}
	if a.OnFrame != nil {
		a.OnFrame(&a.Snap)
	}
	// The companion's clock is advanced from the frame clock rather than a timer
	// of its own, so a still interface costs nothing extra.
	a.State.Pet = a.State.Pet.Settle(now)
	// The fun settings speak only when the interface is quiet; Poll returns
	// nothing at all when a warning, a dialog or a run is in progress.
	if line := a.State.Troll.Poll(now, a.State); line != "" {
		a.State.flash(line, StateMeta, now)
	}
	frame := a.Interp.Render(&a.Snap, a.State)
	a.Screen.cur.CopyFrom(frame)
	a.Screen.Present()
}

// Run is the main loop: read a key, update the state, draw, repeat until the
// session asks to close.
//
// The loop is deliberately not a busy timer. It waits for a key when the
// interface is still and only wakes on a timer when there is motion to show.
//
// A panic here is the worst failure this program has: the terminal is in raw
// mode on the alternate screen, so an unrecovered panic leaves the user with an
// unusable shell and no way back. The recovery therefore restores the terminal
// first and only then reports, and it is installed on every path out of Run
// rather than on the paths someone remembered.
func (a *App) Run(ctx context.Context) (err error) {
	defer a.Screen.Close()
	defer a.recoverPanic(&err)
	now := time.Now()
	a.Snap.Now = now
	a.State.InitRun = false
	a.Screen.Resized()
	a.State.Boot.Started = now
	if err := a.PlayBoot(ctx); err != nil {
		return err
	}
	if err := a.PlayWelcome(ctx); err != nil {
		return err
	}
	a.runFrame(now)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if a.Screen.Resized() {
			w, h := a.Screen.Size()
			a.State.Width, a.State.Height = w, h
		}
		now = time.Now()
		a.runFrame(now)

		r, ok := a.readKey(ctx)
		if !ok {
			continue // a repaint tick with no key pressed
		}
		if r.err != nil {
			return r.err
		}
		if r.key.Type == KeyRune && r.key.Ctrl && r.key.Rune == 'c' {
			return nil
		}
		if a.apply(a.State.handleKey(r.key, &a.Snap), now) {
			return nil
		}
	}
}

// readKey waits for a key, giving up early enough to repaint. The wait is the
// frame interval when something is animating and long enough to be
// indistinguishable from a blocking read when nothing is.
func (a *App) readKey(ctx context.Context) (readResult, bool) {
	wait := idleInterval
	if a.needsFrames() {
		wait = frameInterval
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return readResult{}, false
	case r := <-a.keys:
		return r, true
	case <-timer.C:
		return readResult{}, false
	}
}

// apply turns an event into state changes and session requests. It reports
// whether the session should close.
//
// The split matters: this method decides what the UI *says*, and the session
// decides what actually *happens*. A request is only ever queued here, never
// performed, so no key press can reach the kernel from this file.
func (a *App) apply(ev event, now time.Time) bool {
	switch ev.kind {
	case evRun:
		a.State.Busy = Busy{Active: true, Label: "starting sandbox", Since: now}
		a.State.flash("running "+joinArgs(ev.argv)+" in a fresh sandbox", StateOK, now)
		a.Emit(Request{Kind: ReqRun, Argv: ev.argv})
	case evStop:
		// Stopping is worth stating: the sandbox owns the terminal until it is
		// gone, and the user pressed a key to ask for that.
		a.State.flash("stopping the sandbox…", StateWarn, now)
		a.Emit(Request{Kind: ReqStop})
	case evSetPolicy:
		a.Emit(Request{Kind: ReqSetPolicy, Policy: policyFor(ev.preset)})
	case evOpenPolicy:
		a.Emit(Request{Kind: ReqOpenPolicy, Choice: PolicyHigh})
	case evExport:
		a.Emit(Request{Kind: ReqExport, Paths: ev.paths, Destination: ev.dest, Overwrite: ev.overwrite})
	case evDiscard:
		a.Emit(Request{Kind: ReqDiscard})
	case evRefresh:
		a.State.flash("re-checking the platform report…", StateMeta, now)
		a.Emit(Request{Kind: ReqRefresh})
	case evDiff:
		a.Emit(Request{Kind: ReqDiff, RunID: ev.runID, Entry: ev.entry})
	case evExit:
		a.Emit(Request{Kind: ReqExit})
		return true
	case evRepair:
		a.State.flash("re-initialising the cage…", StateMeta, now)
		a.Emit(Request{Kind: ReqRepairCage})
	case evDestroy:
		a.State.flash("destroying the sandbox…", StateWarn, now)
		a.Emit(Request{Kind: ReqDestroy})
	case evSetSetting:
		a.Emit(Request{Kind: ReqSetSetting, Key: ev.skey, Value: ev.sval})
	case evSetDest:
		a.State.Export.Destination = ev.dest
		a.State.flash("export destination set", StateOK, now)
	case evDropDiff, evNone, evHandled:
	}
	return false
}

// Settle reports a finished run back to the UI: it clears the busy indicator
// and raises one toast describing what the run did, so the outcome is reported
// in a single place instead of being implied by the absence of a spinner.
func (a *App) Settle(rs RunSummary, now time.Time) {
	a.State.Busy.Active = false
	kind := StateOK
	text := rs.Command + " finished cleanly"
	if rs.Exit != 0 {
		kind = StateDanger
		text = rs.Command + " exited " + itoa(rs.Exit)
	}
	if rs.Counts.Warnings > 0 {
		kind = StateWarn
	}
	a.State.Transcript.AddRun(rs)
	detail := ""
	if rs.Counts.Total() > 0 {
		detail = rs.Counts.Summary()
	}
	a.State.Toasts.Notify(kind, text, detail, now)
}

// policyFor maps the confirm dialog's choice onto a real policy preset.
func policyFor(c PolicyChoice) policy.Preset {
	if c == PolicyHigh {
		return policy.Relaxed()
	}
	return policy.Base()
}

// joinArgs renders an argv for the transcript and the flash message.
func joinArgs(argv []string) string { return strings.Join(argv, " ") }

// recoverPanic turns a panic in the interface into a reported error.
//
// The order of the two defers in Run is the whole point: Close runs before this
// one, because a deferred function runs last-in-first-out and Close was
// registered first. By the time this executes the terminal is already out of
// raw mode and off the alternate screen, so the user gets their shell back even
// if the report itself goes wrong.
//
// A recovered panic is reported rather than swallowed. Hiding it would leave the
// user staring at a cage that quietly stopped working, which is the failure mode
// this program exists to avoid.
func (a *App) recoverPanic(err *error) {
	r := recover()
	if r == nil {
		return
	}
	// Deliberately not fatal: a drawing fault in one view must not take down a
	// session that is holding a running sandbox.
	if *err == nil {
		*err = fmt.Errorf("the interface hit an internal error and was closed safely: %v", r)
	}
}
