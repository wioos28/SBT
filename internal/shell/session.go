package shell

import (
	"context"
	"fmt"
	"os"

	"github.com/wioos28/sbt/internal/config"
	"github.com/wioos28/sbt/internal/journal"
	"github.com/wioos28/sbt/internal/platform"
	"github.com/wioos28/sbt/internal/security"
	"github.com/wioos28/sbt/internal/shared/policy"
	"github.com/wioos28/sbt/internal/tui"
	"github.com/wioos28/sbt/internal/version"
)

// RunSession starts the interactive SBT interface and returns the process exit
// code. It is what plain `sbt` runs.
//
// The wiring is the whole point of this function, and the order is deliberate:
//
//  1. The session workspace is created first, because the jail spec points at it
//     as both the seed and the capture directory. Creating it later would mean
//     the first run had nowhere to write.
//  2. The platform probe runs next, outside the interface. The probe forks
//     helpers and can take a moment; doing it before the screen opens means the
//     cage's first frame already shows a real verdict instead of an empty one
//     that fills in a second later.
//  3. Only then does the App take over the terminal, with the runner, the store
//     and the monitor all bound.
//
// The App and the Session share one goroutine each: the App reads keys and
// draws, the Session answers requests and fills the snapshot. Nothing else
// touches the terminal.
func RunSession() int {
	return RunSessionWith(context.Background(), os.Stdin, os.Stdout)
}

// RunSessionWith is RunSession with an explicit context and streams, so the
// loop can be cancelled and a caller can drive it from something other than the
// process's own standard streams.
func RunSessionWith(ctx context.Context, in, out *os.File) int {
	store, err := journal.Open()
	if err != nil {
		fmt.Fprintf(out, "sbt: %v\n", err)
		return 1
	}

	app, err := tui.NewApp(in, out)
	if err != nil {
		// A terminal SBT cannot drive is not a reason to fail the session: the
		// REPL below still works without an alternate screen.
		_ = store.Remove()
		fmt.Fprintf(out, "sbt: the interactive interface could not start: %v\n", err)
		return Run()
	}

	// The runner is the only component that starts sandboxes. It is bound to the
	// screen so a run can hand the real terminal to the command and take it
	// back, and to the store so captured files land in the session workspace.
	// Settings are read before the first frame so the startup sequence already
	// honours them: an animation the user turned off must not play for one
	// second first.
	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		cfg = config.DefaultSettings()
	}
	app.SetBootOptions(boolSetting(cfg, "anim.startup", true), boolSetting(cfg, "ui.welcome", true))
	if name, ok := cfg.GetString("ui.palette"); ok {
		app.Theme.SetPreset(name)
		app.Interp.Theme = app.Theme
		app.Screen.SetTheme(app.Theme)
	}
	app.BootStage("Initializing SBT", "workspace "+store.WorkspaceDir(), true)

	run := newRunner(store, in, out)
	run.SetScreen(app.Screen)

	sess := tui.NewSession(app)
	sess.WorkspaceDir = store.WorkspaceDir()
	sess.SetStore(store)
	sess.SetSampler(run.sampler)
	sess.SetRunner(run)
	sess.SetPreset(policy.Base())

	tui.SetVersionString(version.String())

	// The probe runs before the first frame so the cage opens with evidence
	// rather than with an empty panel. A failure here is not fatal: the session
	// opens showing exactly why nothing can run.
	sess.Refresh()
	caps := sess.Caps()
	run.SetCaps(caps)

	// Each stage below records work that has actually completed. The stage is
	// appended after the step, never before it, so the boot screen cannot show
	// a check for something SBT has not done yet.
	app.BootStage("Checking environment", bootCapsDetail(caps), caps != nil)
	app.BootStage("Checking sandbox", bootCageDetail(caps), bootCageOK(caps))
	rep := security.Permissions(caps)
	allowed, limited, denied := rep.Summary()
	app.BootStage("Checking permissions",
		fmt.Sprintf("%d allowed - %d limited - %d denied", allowed, limited, denied), true)
	app.BootStage("Loading configuration", "from "+config.Path(), cfgErr == nil)
	tw, th := app.Screen.Size()
	app.BootStage("Starting terminal", fmt.Sprintf("%dx%d", tw, th), true)
	app.BootStage("Ready", "", true)

	// Each frame re-reads the probe report, the journal and the monitor, so the
	// interface always shows the current state rather than a snapshot taken at
	// startup. It is driven by the draw loop instead of a timer so the numbers
	// the user sees are the ones that were just painted.
	app.OnFrame = func(snap *tui.Snapshot) {
		sess.SnapshotInto(snap)
	}

	// Requests raised by the UI are answered on their own goroutine. A sandbox
	// run blocks there - the terminal belongs to the sandboxed command while it
	// runs - which is exactly why it must not block the draw loop.
	//
	// The context is derived so cancelling the caller's context stops the
	// goroutine: an unanswered request after the interface closed would keep a
	// process alive with no one drawing.
	loopCtx, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-loopCtx.Done():
				return
			case req, ok := <-app.Out:
				if !ok {
					return
				}
				sess.Handle(loopCtx, req)
			}
		}
	}()

	runErr := app.Run(ctx)
	stop()
	<-done
	sess.Finish()

	// The session summary is printed after the alternate screen closes, so the
	// user's scrollback keeps a record of what happened inside the cage.
	app.Screen.PlainReport(sess.Report())

	if runErr != nil {
		fmt.Fprintf(out, "sbt: %v\n", runErr)
		return 1
	}
	return 0
}

// boolSetting reads a boolean preference, falling back to def when the key is
// missing or of the wrong type. A malformed setting must not stop the session
// from opening: the user can fix it from the Settings view.
func boolSetting(cfg *config.Settings, key string, def bool) bool {
	if v, ok := cfg.GetBool(key); ok {
		return v
	}
	return def
}

// bootCapsDetail is the evidence line for the environment stage.
func bootCapsDetail(caps *platform.Capabilities) string {
	if caps == nil {
		return "probe unavailable"
	}
	out := caps.OS + "/" + caps.Arch
	if caps.Kernel != "" {
		out += " " + caps.Kernel
	}
	return out
}

// bootCageDetail is the evidence line for the sandbox stage.
func bootCageDetail(caps *platform.Capabilities) string {
	if caps == nil || caps.Backend == "" {
		return "no verified backend"
	}
	return caps.Backend + " " + caps.OverallLevel().Label()
}

// bootCageOK reports whether a backend was actually verified. An unverified
// host is recorded as a failed stage, never as a passing one.
func bootCageOK(caps *platform.Capabilities) bool {
	return caps != nil && caps.Backend != ""
}
