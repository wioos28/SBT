package tui

import (
	"strings"
	"testing"
	"time"
)

// A meter rises smoothly rather than jumping, and a fall is immediate.
func TestMeterEasesUpAndFallsImmediately(t *testing.T) {
	now := time.Now()
	var m Meter
	m.Set(0, now, true)
	m.Set(100, now.Add(time.Second), true)

	start := m.Value(now.Add(time.Second), true)
	mid := m.Value(now.Add(time.Second+meterRise/2), true)
	end := m.Value(now.Add(time.Second+meterRise), true)

	if start >= 100 {
		t.Fatalf("the meter must not be at target on the first frame, got %v", start)
	}
	if mid <= start {
		t.Fatal("the meter must be moving while it rises")
	}
	if end < 99.9 {
		t.Fatalf("the meter must reach its target, got %v", end)
	}
	// Falling is a fact, not a transition.
	m.Set(0, now.Add(4*time.Second), true)
	if v := m.Value(now.Add(4*time.Second), true); v != 0 {
		t.Fatalf("a falling meter must be immediate, got %v", v)
	}
}

// With motion off, every animation collapses to its still frame.
func TestMotionOffFreezesAnimation(t *testing.T) {
	now := time.Now()
	var m Meter
	m.Set(0, now, false)
	m.Set(80, now, false)
	if v := m.Value(now.Add(time.Millisecond), false); v != 80 {
		t.Fatalf("a still meter must show the target, got %v", v)
	}
	th := NewTheme(DefaultPalette)
	th.Motion = false
	if got := th.Spinner(now, now); got != SpinnerFrames[0] {
		t.Fatalf("a still spinner must hold one frame, got %q", got)
	}
	snap := baseSnapshot()
	snap.Now = now
	st := newState(80, 24)
	st.Motion = false
	if strings.Contains(renderText(80, 24, &snap, st), SpinnerFrames[1]) {
		t.Fatal("no animated spinner glyph may appear with motion off")
	}
}

// The animation clock must actually advance between frames.
//
// The samples step across a whole spinner cycle, because any single fixed
// offset lands on one phase forever: sampling every interval lands on the
// boundary, sampling every half-interval lands on the midpoint.
func TestSpinnerAdvancesWithTime(t *testing.T) {
	th := NewTheme(DefaultPalette)
	th.Motion = true
	start := time.Now()
	seen := map[string]bool{}
	steps := 20
	cycle := SpinnerInterval * time.Duration(len(SpinnerFrames))
	for i := 0; i < steps; i++ {
		at := start.Add(cycle * time.Duration(i) / time.Duration(steps))
		seen[th.Spinner(at, start)] = true
	}
	if len(seen) < 2 {
		t.Fatalf("the spinner must cycle through frames over time, saw %d distinct frames", len(seen))
	}
}

// formatBytes must pick the right unit. The earlier version overshot by one
// and reported 188KiB as "184320MiB", which is not a formatting nit: a wrong
// number next to a resource meter is a false statement about the sandbox.
func TestFormatBytesPicksTheRightUnit(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0B"},
		{512, "512B"},
		{1024, "1KiB"},
		{1536, "1KiB"},
		{180 << 20, "180MiB"},
		{180 * 1024 * 1024, "180MiB"},
		{2 << 30, "2GiB"},
		{4096, "4KiB"},
	}
	for _, c := range cases {
		if got := formatBytes(c.in); got != c.want {
			t.Errorf("formatBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// A meter suffix must fit the panel; a runaway label would push the bar out.
func TestRenderSurvivesLargeValues(t *testing.T) {
	snap := baseSnapshot()
	snap.Now = time.Now()
	snap.Sandbox.Running = true
	snap.Stats.RSSBytes = 900 << 30
	snap.Stats.MemLimitBytes = 1 << 40
	snap.Stats.CPUPercent = 100
	st := newState(120, 40)
	text := renderText(120, 40, &snap, st)
	if !strings.Contains(text, "GiB") {
		t.Fatalf("a large value must render as GiB, got:\n%s", text)
	}
}
func TestMeterAdoptsFirstReadingImmediately(t *testing.T) {
	now := time.Now()
	var m Meter
	m.Set(180<<20, now, true)
	if v := m.Value(now, true); v != 180<<20 {
		t.Fatalf("the first reading must be shown at once, got %v", v)
	}
	// A later jump eases: it starts from where the bar was, so it is still low
	// just after the change and reaches the target by the end of the rise.
	m.Set(400<<20, now.Add(time.Second), true)
	if v := m.Value(now.Add(time.Second), true); v >= 400<<20 {
		t.Fatalf("a later change must ease rather than jump, got %v", v)
	}
	if v := m.Value(now.Add(time.Second+meterRise), true); v != 400<<20 {
		t.Fatalf("the eased value must reach the target, got %v", v)
	}
}

// A 120x30 frame must contain the whole chrome. The badge word is checked
// rather than the internal state enum, because what the user reads is the part
// that has to be right.
func TestFullFrameHasChrome(t *testing.T) {
	snap := baseSnapshot()
	snap.WorkspaceDir = "/tmp/ws"
	text := renderText(120, 30, &snap, newState(120, 30))
	for _, want := range []string{"SBT", "READY", "isolation", "resources", "session terminal"} {
		if !strings.Contains(text, want) {
			t.Fatalf("frame must contain %q, got:\n%s", want, text)
		}
	}
}

// High risk mode must be visible in the badge. This is the state where a
// fall-through to the default would tell the user everything is fine.
func TestHighRiskIsNeverReportedAsReady(t *testing.T) {
	snap := baseSnapshot()
	snap.Now = time.Now()
	snap.Cage = Cage{State: CageHigh, Reason: "network is not blocked"}
	text := renderText(120, 30, &snap, newState(120, 30))
	if !strings.Contains(text, "HIGH RISK") {
		t.Fatalf("high risk must be named in the badge, got:\n%s", text)
	}
	if strings.Contains(text, "● READY") {
		t.Fatal("high risk must never render as READY")
	}
}

// A busy session must name what it is doing rather than just spinning.
func TestBusyStateIsNamed(t *testing.T) {
	snap := baseSnapshot()
	snap.Now = time.Now()
	st := newState(120, 30)
	st.Busy = Busy{Active: true, Label: "starting sandbox", Since: snap.Now}
	if text := renderText(120, 30, &snap, st); !strings.Contains(strings.ToUpper(text), "STARTING SANDBOX") {
		t.Fatalf("a busy session must name what it is doing, got:\n%s", text)
	}
}

// Rendering must never panic, at any geometry a terminal can report. A crash
// inside the draw loop takes the whole session with it, so this sweeps the
// range rather than trusting one size.
func TestRenderNeverPanics(t *testing.T) {
	sizes := [][2]int{
		{20, 5}, {24, 8}, {40, 10}, {62, 20}, {80, 24}, {100, 30}, {120, 40}, {200, 60},
	}
	for _, sz := range sizes {
		for _, view := range []View{ViewTerminal, ViewFiles, ViewChanges, ViewStatus, ViewExport, ViewHelp} {
			snap := baseSnapshot()
			snap.WorkspaceDir = "/tmp/ws"
			st := newState(sz[0], sz[1])
			st.View = view
			renderText(sz[0], sz[1], &snap, st)
		}
	}
}

// An overlay must draw on top of the content rather than replace it.
func TestOverlaysDrawAboveContent(t *testing.T) {
	snap := baseSnapshot()
	snap.Now = time.Now()
	st := newState(120, 40)
	st.Palette.Open = true
	withPalette := renderText(120, 40, &snap, st)

	st.Palette.Open = false
	st.Confirm = Confirm{
		Kind: ConfirmExit, Title: "Exit",
		Body: []string{"leave SBT?"}, Choice: 1,
	}
	withConfirm := renderText(120, 40, &snap, st)

	if withPalette == withConfirm {
		t.Fatal("the palette and the confirm dialog must not draw identically")
	}
	if !strings.Contains(withConfirm, "leave SBT?") {
		t.Fatalf("the confirm body must be on screen, got:\n%s", withConfirm)
	}
}

// An export that includes an executable must say so before writing anything.
func TestExportWarnsAboutExecutables(t *testing.T) {
	snap := baseSnapshot()
	snap.Files = []FileInfo{
		{Path: "notes.txt", Size: 10},
		{Path: "tool", Size: 4096, Executable: true},
	}
	st := newState(120, 40)
	st.Export = NewExportSel(snap.Files, "/tmp/out")
	st.Export.Toggle(1)
	st.askExport(&snap)
	if st.Confirm.Kind != ConfirmExport {
		t.Fatal("export must open a confirmation")
	}
	joined := strings.Join(st.Confirm.Body, " ")
	if !strings.Contains(joined, "executable") {
		t.Fatalf("the dialog must name the executable risk, got %q", joined)
	}
	if st.Confirm.Choice != 1 {
		t.Fatal("export must default to the safe choice")
	}
}

// A toast must be visible with its own text, not implied by a colour.
func TestToastIsRenderedWithItsText(t *testing.T) {
	snap := baseSnapshot()
	snap.Now = time.Now()
	st := newState(100, 30)
	st.Toasts.Notify(StateDanger, "CAGE BROKEN", "user namespace denied", snap.Now)
	if text := renderText(100, 30, &snap, st); !strings.Contains(text, "CAGE BROKEN") {
		t.Fatalf("the warning text must be on screen, got:\n%s", text)
	}
}

// A degraded feature is named in words, so the meaning survives no colour.
func TestDegradedIsolationShowsAWord(t *testing.T) {
	snap := baseSnapshot()
	snap.Now = time.Now()
	if text := renderText(120, 40, &snap, newState(120, 40)); !strings.Contains(text, "PARTIAL") {
		t.Fatalf("a partial feature must be named in words, got:\n%s", text)
	}
}

// A terminal too small for the cage says so instead of drawing garbage.
func TestSmallTerminalExplainsItself(t *testing.T) {
	snap := baseSnapshot()
	text := renderText(20, 5, &snap, newState(20, 5))
	if !strings.Contains(text, "too small") {
		t.Fatalf("a too-small terminal must explain itself, got:\n%s", text)
	}
}
