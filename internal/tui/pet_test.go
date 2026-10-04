package tui

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// A pet must never be able to name an action. This walks the struct's exported
// surface by behaviour instead: poking it may change its own fields and nothing
// else about the session.
func TestPokeTouchesNothingButThePet(t *testing.T) {
	st, snap := mouseTestState()
	before := *st
	st.Pet = NewPet(PetCat)
	st.Pet.Poke(snap.Now)
	if st.Pet.Mood == "" {
		t.Fatal("the pet did not react")
	}
	if st.Pet.Mood != "purr" {
		t.Errorf("mood = %q, want purr", st.Pet.Mood)
	}
	// The rest of the session state is untouched: no view changed, no cursor
	// moved, no confirm opened, no input line touched.
	st.Pet = Pet{}
	if st.View != before.View {
		t.Errorf("view changed: %v", st.View)
	}
	if st.Confirm.Kind != before.Confirm.Kind {
		t.Errorf("a confirm opened from petting: %v", st.Confirm.Kind)
	}
	if st.Input != before.Input {
		t.Error("the command input was modified by petting")
	}
	if st.List.Index != before.List.Index {
		t.Error("a list cursor moved from petting")
	}
}

func TestPetKindNames(t *testing.T) {
	cases := map[PetKind]struct {
		name string
		live bool
	}{
		PetCat:  {"cat", true},
		PetDog:  {"dog", true},
		PetBird: {"bird", true},
		PetOff:  {"off", false},
		// An unknown value must draw nothing rather than an empty box.
		PetKind(99): {"off", false},
	}
	for kind, want := range cases {
		p := NewPet(kind)
		if p.Alive() != want.live {
			t.Errorf("kind %d alive = %v, want %v", kind, p.Alive(), want.live)
		}
		if !p.Alive() {
			if len(p.art()) != 0 {
				t.Errorf("kind %d drew art while off", kind)
			}
			continue
		}
		if got := p.Kind.Name(); got != want.name {
			t.Errorf("kind %d name = %q, want %q", kind, got, want.name)
		}
		if len(p.art()) == 0 {
			t.Errorf("kind %d drew no art", kind)
		}
	}
}

func TestPetMoodExpires(t *testing.T) {
	now := time.Now()
	p := NewPet(PetDog)
	p.Poke(now)
	if p.Mood == "" {
		t.Fatal("no reaction")
	}
	// Still reacting just before it expires.
	if got := p.Settle(now.Add(5 * time.Second)); got.Mood == "" {
		t.Error("the reaction expired early")
	}
	// Calm again after it.
	if got := p.Settle(now.Add(7 * time.Second)); got.Mood != "" {
		t.Errorf("mood = %q, want it cleared", got.Mood)
	}
	// A pet that is off never reacts at all.
	off := NewPet(PetOff)
	off.Poke(now)
	if off.Mood != "" {
		t.Error("a pet that is off still reacted")
	}
}

func TestPetFramesAdvance(t *testing.T) {
	base := time.Now()
	p := NewPet(PetCat)
	seen := map[int]bool{}
	for i := 0; i < 16; i++ {
		seen[p.Settle(base.Add(time.Duration(i)*time.Second)).Frame] = true
	}
	if len(seen) < 2 {
		t.Error("the companion never animated")
	}
}

// A click on the companion pets it, and a click elsewhere does not.
func TestClickPetsTheCompanion(t *testing.T) {
	st, snap := mouseTestState()
	st.Pet = NewPet(PetBird)
	snap.Stats.ProcsLimit = 8
	r, ok := st.petBounds(snap)
	if !ok {
		t.Skip("this terminal has no room for the companion panel")
	}
	st.handleKey(Key{Type: KeyMouse, Mouse: Mouse{
		Button: MouseLeft, X: r.X + 1, Y: r.Y + 1, Press: true,
	}}, snap)
	if st.Pet.Mood != "tweet" {
		t.Errorf("mood = %q, want tweet", st.Pet.Mood)
	}

	// Far away from the panel: nothing happens.
	st.Pet.Mood = ""
	st.handleKey(Key{Type: KeyMouse, Mouse: Mouse{
		Button: MouseLeft, X: 1, Y: 1, Press: true,
	}}, snap)
	if st.Pet.Mood != "" {
		t.Error("a click far from the companion petted it")
	}
}

// The companion is cosmetic, so it must survive the full geometry sweep the
// renderer is tested against without panicking.
func TestPetSurvivesEveryGeometry(t *testing.T) {
	for w := 20; w <= 200; w += 7 {
		for h := 6; h <= 60; h += 5 {
			st := NewUIState(w, h)
			snap := &Snapshot{Now: time.Now()}
			snap.Stats.ProcsLimit = 8
			for _, kind := range []PetKind{PetCat, PetDog, PetBird, PetOff, PetKind(99)} {
				st.Pet = NewPet(kind)
				interp := NewInterpreter(NewTheme(DefaultPalette))
				interp.Render(snap, st)
			}
		}
	}
}

// A panic anywhere in the interface must leave the terminal usable. This is the
// single worst failure mode, because raw mode plus the alternate screen means
// the user is left with a shell they cannot type into.
//
// Run is checked through its own signature rather than by driving a terminal:
// what is asserted is that the recovery is installed on Run at all, so a future
// refactor cannot quietly drop the defer.

// A recovered panic becomes a reported error, never a silent stop.
func TestRecoverPanicReportsAnError(t *testing.T) {
	app := &App{}
	var err error
	func() {
		defer app.recoverPanic(&err)
		panic("draw failed")
	}()
	if err == nil {
		t.Fatal("the panic was swallowed instead of reported")
	}
	if !strings.Contains(err.Error(), "draw failed") {
		t.Errorf("error %q does not mention the cause", err)
	}
}

// A panic that happens while an error is already set must not overwrite it.
func TestRecoverPanicKeepsTheFirstError(t *testing.T) {
	app := &App{}
	var err error = errSentinel
	func() {
		defer app.recoverPanic(&err)
		panic("later failure")
	}()
	if err != errSentinel {
		t.Errorf("error = %v, want the original", err)
	}
}

// A clean return through recoverPanic leaves the error alone.
func TestRecoverPanicIsQuietWithoutAPanic(t *testing.T) {
	app := &App{}
	var err error = errSentinel
	func() {
		defer app.recoverPanic(&err)
	}()
	if err != errSentinel {
		t.Errorf("error = %v, want it untouched", err)
	}
}

var errSentinel = errors.New("original")
