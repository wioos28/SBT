package shell

import (
	"context"
	"fmt"
	"os"

	"github.com/wioos28/sbt/internal/journal"
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
	run.SetCaps(sess.Caps())

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
