package tui

import (
	"context"
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
	theme := NewTheme(DefaultPalette)
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
func (a *App) needsFrames() bool {
	if !a.State.Motion {
		return a.State.Busy.Active || len(a.State.Toasts.Live(a.Snap.Now)) > 0
	}
	return true
}

// runFrame advances the clocks, draws one frame and hands it to the screen.
func (a *App) runFrame(now time.Time) {
	a.Snap.Now = now
	a.State.FlashExpire(now)
	a.State.Toasts.Expire(now)
	if a.State.Busy.Since.IsZero() {
		a.State.Busy.Since = now
	}
	if a.OnFrame != nil {
		a.OnFrame(&a.Snap)
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
func (a *App) Run(ctx context.Context) error {
	defer a.Screen.Close()
	now := time.Now()
	a.Snap.Now = now
	a.State.InitRun = false
	a.Screen.Resized()
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
func (a *App) apply(ev event, now time.Time) bool {
	switch ev.kind {
	case evRun:
		a.State.Busy = Busy{Active: true, Label: "starting sandbox", Since: now}
		a.State.flash("running "+joinArgs(ev.argv)+" in a fresh sandbox", StateOK, now)
		a.Emit(Request{Kind: ReqRun, Argv: ev.argv})
	case evSetPolicy:
		a.Emit(Request{Kind: ReqSetPolicy, Policy: policyFor(ev.preset)})
	case evExport:
		a.Emit(Request{Kind: ReqExport, Paths: ev.paths, Destination: ev.dest, Overwrite: ev.overwrite})
	case evDiscard:
		a.Emit(Request{Kind: ReqDiscard})
	case evRefresh:
		a.State.flash("re-checking the platform report…", StateMeta, now)
		a.Emit(Request{Kind: ReqRefresh})
	case evExit:
		a.Emit(Request{Kind: ReqExit})
		return true
	case evStop, evNone, evHandled:
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
