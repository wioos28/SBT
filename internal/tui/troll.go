package tui

import "time"

// The fun settings.
//
// Every rule here exists to keep this feature from ever getting in the way. It
// is off by default. It refuses to speak whenever anything important is on
// screen - a confirmation, a permission prompt, a warning, a running sandbox,
// any notice - because a joke that covers a security message is not a joke. It
// draws in the status line, which never overlaps content. And none of it is
// random: the messages come from a fixed list chosen by a counter, so a test
// can pin exactly what appears and when.

// trollLines are harmless, and none of them is ever about isolation.
var trollLines = []string{
	"the cage is checking on you",
	"nothing escapes this terminal",
	"sandbox accounting: still boring",
	"all quiet on the namespace front",
	"wasting cycles since the last commit",
	"a very polite jail",
	"the filesystem said nothing",
	"this line costs one keypress",
}

// Troll is the cosmetic-message state.
type Troll struct {
	Enabled   bool
	Frequency int
	Intensity int

	next time.Time
	step int
}

// Configure applies the fun settings. Frequency and Intensity are 0..100; zero
// frequency means the feature is off whatever the enabled flag says.
func (tr *Troll) Configure(enabled bool, frequency, intensity int) {
	switch {
	case frequency < 0:
		frequency = 0
	case frequency > 100:
		frequency = 100
	}
	switch {
	case intensity < 0:
		intensity = 0
	case intensity > 100:
		intensity = 100
	}
	tr.Enabled = enabled && frequency > 0
	tr.Frequency = frequency
	tr.Intensity = intensity
}

// quiet reports whether anything important is on screen. When it is, the fun
// settings stay silent - this is the guarantee that matters.
func quiet(st *UIState) bool {
	return st.Permission.Open ||
		st.Confirm.Kind != ConfirmNone ||
		st.Alert.Title != "" ||
		st.Busy.Active ||
		len(st.Toasts.Items) > 0
}

// Poll returns a cosmetic line for the status bar, or "" when nothing should be
// said. The caller decides whether the interface is quiet enough to show it.
//
// The schedule is deterministic: the gap between messages is derived from the
// frequency and a step counter, so there is no global random source and the
// behaviour is reproducible in a test.
func (tr *Troll) Poll(now time.Time, st *UIState) string {
	if !tr.Enabled || quiet(st) {
		return ""
	}
	if tr.next.IsZero() {
		tr.next = now.Add(tr.gap())
		return ""
	}
	if now.Before(tr.next) {
		return ""
	}
	tr.next = now.Add(tr.gap())
	tr.step++
	return trollLines[tr.step%len(trollLines)]
}

// gap is how long between messages. Frequency 100 is roughly one every twenty
// seconds and frequency 1 roughly one every half hour: slow enough that it
// cannot become noise, and it never fires more than once per interval no matter
// how often Poll is called.
func (tr *Troll) gap() time.Duration {
	f := tr.Frequency
	if f < 1 {
		f = 1
	}
	seconds := 1800 - (f-1)*1780/99
	return time.Duration(seconds) * time.Second
}
