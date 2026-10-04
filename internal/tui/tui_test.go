package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/wioos28/sbt/internal/platform"
	"github.com/wioos28/sbt/internal/shared/policy"
)

// baseSnapshot is a snapshot with enough filled in that every view has
// something to draw. Tests build on it so a new field does not break twenty
// cases at once.
func baseSnapshot() Snapshot {
	now := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	return Snapshot{
		Version:  "v0.0.2",
		Host:     "testhost",
		Kernel:   "6.1.0",
		Platform: "debian",
		Backend:  "userns",
		Now:      now,
		Cage:     Cage{State: CageProtected, Reason: "isolation verified at runtime"},
		Isolation: []IsolationLine{
			{Label: "user namespace", Value: "FULL", State: StateOK},
			{Label: "mount namespace", Value: "FULL", State: StateOK},
			{Label: "seccomp", Value: "PARTIAL", State: StateWarn, Reason: "filtered"},
		},
	}
}

func newState(w, h int) *UIState {
	st := NewUIState(w, h)
	st.Commands = DefaultCommands()
	return st
}

// renderText flattens a buffer to plain text, so a test can assert on what the
// user sees rather than on cell styles.
func renderText(w, h int, snap *Snapshot, st *UIState) string {
	b := NewInterpreter(NewTheme(DefaultPalette)).Render(snap, st)
	var sb strings.Builder
	for y := 0; y < b.H; y++ {
		for x := 0; x < b.W; x++ {
			c := b.Cells[y*b.W+x]
			if c.R == 0 {
				continue
			}
			sb.WriteRune(c.R)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// A cage that was never probed must not claim to be protected.
func TestCageVerdictNeverClaimsProtectionWithoutEvidence(t *testing.T) {
	if got := cageVerdict(nil, policy.Base()); got.State == CageProtected {
		t.Fatal("an unprobed cage must not report PROTECTED")
	}
}

// A probe with no verified feature must never be reported as protected.
func TestCageVerdictNeedsVerifiedFeatures(t *testing.T) {
	c := &platform.Capabilities{}
	if got := cageVerdict(c, policy.Base()); got.State == CageProtected {
		t.Fatalf("a probe with nothing verified must not be protected, got %v", got.State)
	}
}

// A flash message expires so it cannot sit on the status bar forever.
func TestFlashExpires(t *testing.T) {
	now := time.Now()
	st := newState(80, 24)
	st.flash("something happened", StateWarn, now)
	st.FlashExpire(now.Add(time.Second))
	if st.Flash.Text == "" {
		t.Fatal("flash must survive 1s")
	}
	st.FlashExpire(now.Add(7 * time.Second))
	if st.Flash.Text != "" {
		t.Fatal("flash must expire after its life")
	}
}

// The palette must do something on enter, and it must match what the
// equivalent key binding does.
func TestPaletteEnterOpensConfirmation(t *testing.T) {
	snap := baseSnapshot()
	snap.WorkspaceDir = "/tmp/ws"
	st := newState(100, 30)
	st.Palette.Open = true
	st.Palette.Query = "discard"
	if len(st.Palette.entries(st.Commands)) == 0 {
		t.Fatal("discard must be findable in the palette")
	}
	ev := st.handleKey(Key{Type: KeyEnter}, &snap)
	if ev.kind == evDiscard {
		t.Fatal("discard must ask first, not act directly")
	}
	if st.Confirm.Kind != ConfirmDiscard {
		t.Fatalf("expected a discard confirmation, got %v", st.Confirm.Kind)
	}
	if st.Confirm.Choice != 1 {
		t.Fatal("a destructive dialog must default to the safe choice")
	}
}

// Escape always resolves a confirmation to the safe side.
func TestConfirmEscapeChoosesSafe(t *testing.T) {
	snap := baseSnapshot()
	snap.WorkspaceDir = "/tmp/ws"
	st := newState(100, 30)
	st.askDiscard(&snap)
	st.Confirm.Choice = 0 // the user had moved onto the dangerous button
	if ev := st.handleKey(Key{Type: KeyEsc}, &snap); ev.kind == evDiscard {
		t.Fatal("escape must never perform the destructive action")
	}
	if st.Confirm.Kind != ConfirmNone {
		t.Fatal("escape must close the dialog")
	}
}

// Enter on the dangerous choice emits the destructive event exactly once.
func TestConfirmEnterEmitsEventOnce(t *testing.T) {
	snap := baseSnapshot()
	snap.WorkspaceDir = "/tmp/ws"
	st := newState(100, 30)
	st.askDiscard(&snap)
	st.Confirm.Choice = 0
	if ev := st.handleKey(Key{Type: KeyEnter}, &snap); ev.kind != evDiscard {
		t.Fatalf("expected evDiscard, got %v", ev.kind)
	}
	if st.Confirm.Kind != ConfirmNone {
		t.Fatal("the dialog must close after it resolves")
	}
	if ev2 := st.handleKey(Key{Type: KeyEnter}, &snap); ev2.kind == evDiscard {
		t.Fatal("a resolved dialog must not fire a second time")
	}
}

// A warning toast outranks a success toast from the same moment.
func TestToastSeverityOrdering(t *testing.T) {
	now := time.Now()
	var ts Toasts
	ts.Notify(StateOK, "finished cleanly", "", now)
	ts.Notify(StateDanger, "isolation broke", "user namespace denied", now)
	if len(ts.Items) == 0 || ts.Items[0].Kind != StateDanger {
		t.Fatalf("the danger notice must come first, got %+v", ts.Items)
	}
	if got := ts.Worst(now); got != StateDanger {
		t.Fatalf("Worst must report danger, got %v", got)
	}
}

// Toasts expire and the stack is capped, so the corner cannot fill up.
func TestToastExpiryAndCap(t *testing.T) {
	now := time.Now()
	var ts Toasts
	for i := 0; i < 6; i++ {
		ts.Notify(StateMeta, "note", "", now)
	}
	if len(ts.Items) > MaxToasts {
		t.Fatalf("the stack must be capped, got %d", len(ts.Items))
	}
	ts.Expire(now.Add(10 * time.Second))
	if len(ts.Items) != 0 {
		t.Fatalf("expired notices must be dropped, got %d", len(ts.Items))
	}
}
