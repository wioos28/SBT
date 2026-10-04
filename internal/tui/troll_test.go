package tui

import (
	"testing"
	"time"
)

// The guarantee that matters: the fun settings never speak over anything
// security related. Each of these is a separate way the interface can be busy,
// and none of them may produce a cosmetic line.
func TestFunSettingsNeverInterfereWithSecurityUI(t *testing.T) {
	now := timeAt()
	blocked := map[string]func(st *UIState){
		"permission prompt": func(st *UIState) { st.Permission.Open = true },
		"confirmation":      func(st *UIState) { st.Confirm.Kind = ConfirmDestroy },
		"critical alert":    func(st *UIState) { st.Alert = AlertState{Title: "CAGE BROKEN", Critical: true} },
		"running sandbox":   func(st *UIState) { st.Busy.Active = true },
		"a pending notice":  func(st *UIState) { st.Toasts.Notify(StateWarn, "isolation limited", "why", now) },
	}
	for name, block := range blocked {
		tr := &Troll{Enabled: true, Frequency: 100, Intensity: 100}
		st := newState(80, 24)
		block(st)
		for i := 0; i < 5; i++ {
			if line := tr.Poll(now.Add(time.Duration(i)*time.Hour), st); line != "" {
				t.Fatalf("%s: the fun settings must stay silent, got %q", name, line)
			}
		}
	}
}

// Off by default, and a zero frequency keeps it off whatever the flag says.
func TestFunSettingsAreOffUnlessAsked(t *testing.T) {
	now := timeAt()
	st := newState(80, 24)
	tr := &Troll{}
	if line := tr.Poll(now, st); line != "" {
		t.Fatalf("a default session must be quiet, got %q", line)
	}
	tr.Configure(true, 0, 50)
	if line := tr.Poll(now, st); line != "" {
		t.Fatalf("zero frequency must keep it off, got %q", line)
	}
	tr.Configure(true, 100, 50)
	spoke := false
	for i := 0; i < 200; i++ {
		if tr.Poll(now.Add(time.Duration(i)*time.Hour), st) != "" {
			spoke = true
			break
		}
	}
	if !spoke {
		t.Fatal("the fun settings must actually speak once they are switched on")
	}
}

// It must not fire twice inside its own interval, so a repainting interface
// cannot turn one message into a stream of them.
func TestFunSettingsRespectTheirInterval(t *testing.T) {
	now := timeAt()
	st := newState(80, 24)
	tr := &Troll{}
	tr.Configure(true, 100, 50)
	gap := tr.gap()
	last := time.Time{}
	spoken := 0
	// Twenty polls a second for an hour.
	for i := 0; i < 72000; i++ {
		at := now.Add(time.Duration(i) * 50 * time.Millisecond)
		if tr.Poll(at, st) == "" {
			continue
		}
		spoken++
		if !last.IsZero() && at.Sub(last) < gap {
			t.Fatalf("two lines %v apart is faster than the %v interval", at.Sub(last), gap)
		}
		last = at
	}
	if spoken == 0 {
		t.Fatal("an hour at the highest frequency should still say something")
	}
}

// Out-of-range values are clamped rather than trusted.
func TestFunSettingsClampTheirInput(t *testing.T) {
	tr := &Troll{}
	tr.Configure(true, 5000, -20)
	if tr.Frequency != 100 || tr.Intensity != 0 {
		t.Fatalf("out-of-range values must clamp, got frequency %d intensity %d", tr.Frequency, tr.Intensity)
	}
}
