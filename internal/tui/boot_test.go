package tui

import (
	"strings"
	"testing"
)

// The boot screen must never claim work that has not happened. That is the one
// property that makes the loading bar trustworthy, so it is tested directly
// rather than inferred from the drawing code.

// A stage only counts once it has a terminal state.
func TestBootProgressCountsOnlyFinishedStages(t *testing.T) {
	now := timeAt()
	var b BootSequence
	b.Add("one", "", true, now)
	b.Add("two", "", true, now)
	if got := b.Progress(); got != 1 {
		t.Fatalf("two finished stages must read 100%%, got %v", got)
	}
	b.Stages = append(b.Stages, BootStage{Label: "three", State: stagePending})
	if got := b.Progress(); got != 2.0/3.0 {
		t.Fatalf("a pending stage must not count as done, got %v", got)
	}
	if b.Done() {
		t.Fatal("a sequence with a pending stage is not done")
	}
}

// A failed step is recorded as failed, never as a tick.
func TestBootRecordsFailureAsFailure(t *testing.T) {
	now := timeAt()
	var b BootSequence
	b.Add("probe", "no verified backend", false, now)
	line, sty := bootStageLine(b.Stages[0], NewTheme(DefaultPalette), true)
	if strings.Contains(line, "✓") {
		t.Fatalf("a failed stage must not render as done, got %q", line)
	}
	if sty.Fg != NewTheme(DefaultPalette).Palette.Warning {
		t.Fatal("a failed stage must be drawn in the warning colour")
	}
}

// An unrevealed stage draws as pending even when its real state is done.
func TestBootUnrevealedStageNeverRendersAsDone(t *testing.T) {
	now := timeAt()
	th := NewTheme(DefaultPalette)
	var b BootSequence
	b.Add("environment", "linux", true, now)
	line, _ := bootStageLine(b.Stages[0], th, false)
	if !strings.Contains(line, th.Glyphs().Ring) {
		t.Fatalf("an unrevealed stage must render as pending, got %q", line)
	}
	if strings.Contains(line, th.Glyphs().Check) {
		t.Fatalf("an unrevealed stage must not render a tick, got %q", line)
	}
}

// With no stages at all there is nothing to claim: progress must be zero, not
// one, or an empty boot screen would show a full bar.
func TestEmptyBootReportsNoProgress(t *testing.T) {
	var b BootSequence
	if got := b.Progress(); got != 0 {
		t.Fatalf("an empty sequence must report 0, got %v", got)
	}
	if b.Done() {
		t.Fatal("an empty sequence is not a completed one")
	}
}

// The boot screen and the welcome panel have to survive every terminal SBT can
// be dropped into, including the ones too small for the art.
func TestBootAndWelcomeRenderAtEverySize(t *testing.T) {
	th := NewTheme(DefaultPalette)
	th.Motion = true
	in := NewInterpreter(th)
	now := timeAt()
	var seq BootSequence
	seq.Started = now
	seq.Add("Initializing SBT", "workspace /tmp/ws", true, now)
	seq.Add("Checking sandbox", "namespace full", true, now)
	seq.Add("Checking sandbox", "no verified backend", false, now)
	seq.Reveal = 3

	for _, sz := range [][2]int{{20, 6}, {24, 8}, {40, 10}, {60, 16}, {80, 24}, {120, 40}, {200, 60}} {
		buf := NewBuffer(sz[0], sz[1])
		in.Boot(buf, &seq, now)

		snap := baseSnapshot()
		snap.Now = now
		st := newState(sz[0], sz[1])
		wbuf := NewBuffer(sz[0], sz[1])
		in.Welcome(wbuf, &snap, st, now)
	}
}

// A wide terminal must actually show the product name, not just a border.
func TestBootShowsTheWordmarkOnAWideTerminal(t *testing.T) {
	th := NewTheme(DefaultPalette)
	in := NewInterpreter(th)
	now := timeAt()
	var seq BootSequence
	seq.Started = now
	seq.Add("Ready", "", true, now)
	seq.Reveal = 1
	buf := NewBuffer(80, 24)
	in.Boot(buf, &seq, now)
	var sb strings.Builder
	for y := 0; y < buf.H; y++ {
		for x := 0; x < buf.W; x++ {
			if r := buf.Cells[y*buf.W+x].R; r != 0 {
				sb.WriteRune(r)
			}
		}
		sb.WriteByte('\n')
	}
	if !strings.Contains(sb.String(), Wordmark) {
		t.Fatal("the boot screen must show the sbt-wioos28 wordmark")
	}
}

// Motion off must still produce a readable, complete screen.
func TestBootWithMotionOffShowsTheFinalState(t *testing.T) {
	th := NewTheme(DefaultPalette)
	th.Motion = false
	in := NewInterpreter(th)
	now := timeAt()
	var seq BootSequence
	seq.Started = now
	seq.Add("Ready", "", true, now)
	seq.Reveal = 0
	buf := NewBuffer(80, 24)
	in.Boot(buf, &seq, now)
	seen := false
	for y := 0; y < buf.H; y++ {
		for x := 0; x < buf.W; x++ {
			if buf.Cells[y*buf.W+x].R == 'R' {
				seen = true
			}
		}
	}
	if !seen {
		t.Fatal("with motion off the boot screen must still show the finished stage")
	}
}
