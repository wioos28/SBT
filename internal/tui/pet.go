package tui

import "time"

// PetKind is which animal sits in the companion panel.
type PetKind int

// The companions. Each is drawn from a handful of frames and none of them can
// reach anything: a pet is a cosmetic, and it is never given a request channel.
const (
	PetCat PetKind = iota
	PetDog
	PetBird
	PetOff
)

// PetNames lists the companions in the order the settings cycle through them.
var PetNames = []string{"cat", "dog", "bird", "off"}

// Name renders the pet as the word shown in settings and the status line.
func (p PetKind) Name() string {
	switch p {
	case PetCat:
		return "cat"
	case PetDog:
		return "dog"
	case PetBird:
		return "bird"
	default:
		return "off"
	}
}

// Pet is the companion's state: which animal, and what it is doing.
//
// It carries no capability. There is deliberately no field here that could name
// a command or a path, because a feature that can be talked into acting is not a
// cosmetic - it is an action with extra steps, and the whole point of this
// program is that actions are visible and confirmed.
type Pet struct {
	Kind PetKind
	// Mood is the last reaction, shown as a word next to the animal.
	Mood string
	// MoodUntil is when the reaction expires, so a pet that was petted goes
	// back to being calm rather than staying excited forever.
	MoodUntil time.Time
	// Frame is the animation frame, derived from the clock by the caller.
	Frame int
}

// NewPet builds the companion named by kind.
func NewPet(kind PetKind) Pet { return Pet{Kind: kind} }

// Alive reports whether a pet is drawn at all. "off" and any unknown value draw
// nothing, so a bad setting cannot produce an empty panel with a border.
func (p Pet) Alive() bool {
	return p.Kind >= PetCat && p.Kind < PetOff
}

// Poke is the one interaction: clicking or pressing the pet key makes it react.
//
// It cannot run anything, so there is nothing here to confirm and nothing to
// guard. That is why it is safe for a stray click to reach: the worst a pet can
// do is change a word in a corner.
func (p *Pet) Poke(now time.Time) {
	if !p.Alive() {
		return
	}
	p.Mood = p.Kind.reaction()
	p.MoodUntil = now.Add(6 * time.Second)
	p.Frame = 0
}

// Settle expires a finished reaction and advances the animation frame.
//
// It is a pure function of the clock rather than a timer, so a still interface
// costs nothing and the animation cannot drift away from the frame that drew it.
func (p Pet) Settle(now time.Time) Pet {
	if p.Mood != "" && !p.MoodUntil.IsZero() && now.After(p.MoodUntil) {
		p.Mood = ""
		p.MoodUntil = time.Time{}
	}
	if p.Alive() {
		// Four frames at the idle cadence: enough to read as alive, slow enough
		// to be cheap.
		p.Frame = int(now.Unix()/2) % 4
	}
	return p
}

// reaction is the word the animal says when petted. Different per species so the
// companion is distinguishable by its behaviour and not only its shape.
func (p PetKind) reaction() string {
	switch p {
	case PetCat:
		return "purr"
	case PetDog:
		return "woof"
	case PetBird:
		return "tweet"
	default:
		return ""
	}
}

// art returns the companion's frame.
//
// The frames are ASCII first and box-drawing only where a terminal certainly
// has it, so the panel never shows a broken box on NO_COLOR or a plain terminal.
func (p Pet) art() []string {
	switch p.Kind {
	case PetCat:
		return catFrames[p.Frame%len(catFrames)]
	case PetDog:
		return dogFrames[p.Frame%len(dogFrames)]
	case PetBird:
		return birdFrames[p.Frame%len(birdFrames)]
	default:
		return nil
	}
}

var catFrames = [][]string{
	{" /\\___/\\ ", "(  o o  )", " >  ^  < "},
	{" /\\___/\\ ", "(  o o  )", " (  >  ) "},
	{" /\\___/\\ ", "(  ^ ^  )", " >  v  < "},
	{" /\\___/\\ ", "(  o o  )", " >  ^  < "},
}

var dogFrames = [][]string{
	{"  ,---,  ", " / ||| \\ ", " '---'  "},
	{"  ,---,  ", " / ||| \\ ", " '  ^  ' "},
	{"  ,---,  ", " / |v| \\ ", " '---'  "},
	{"  ,---,  ", " / ||| \\ ", " '  v  ' "},
}

var birdFrames = [][]string{
	{".---.", "( o )", " 'v'"},
	{".---.", "(o  )", " '|'"},
	{".---.", "( o )", " /|\\"},
	{".---.", "(  o)", " '|'"},
}
