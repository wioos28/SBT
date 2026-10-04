package tui

import "time"

// The typing animation.
//
// The constraint that shapes this file is that it must never add input
// latency. So it does no work on the input path and starts no timer. When the
// input line changes, the character that arrived is stamped with the time; when
// a frame is drawn, each cell's brightness is a pure function of that stamp and
// the current clock. With the animation off the renderer draws the plain text
// and none of this is consulted, so there is no second code path to drift.

// typingFade maps a configured speed to how long a freshly typed character
// stays lit.
var typingFade = map[string]time.Duration{
	"slow":   1100 * time.Millisecond,
	"normal": 650 * time.Millisecond,
	"fast":   320 * time.Millisecond,
	"auto":   650 * time.Millisecond,
}

// typeMark records when one character of the input line was typed.
type typeMark struct {
	At time.Time
}

// TypingState tracks the recent keystrokes of the command input so the renderer
// can draw a light sweep behind them.
type TypingState struct {
	Enabled   bool
	Speed     string
	Intensity int
	Color     RGB

	marks       []typeMark
	lastText    []rune
	lastAt      time.Time
	autoShorten bool
}

// Configure applies the animation settings. The session calls it when the
// configuration is read or changed, so a settings edit takes effect at once
// rather than at the next restart.
func (ty *TypingState) Configure(enabled bool, speed string, intensity int, color RGB) {
	if _, ok := typingFade[speed]; !ok {
		speed = "auto"
	}
	ty.Enabled = enabled
	ty.Speed = speed
	switch {
	case intensity < 0:
		intensity = 0
	case intensity > 100:
		intensity = 100
	}
	ty.Intensity = intensity
	ty.Color = color
}

// Fade is how long a character stays lit. In auto mode a fast typist gets a
// shorter trail, so the light does not smear into a block behind the caret.
func (ty *TypingState) Fade() time.Duration {
	base, ok := typingFade[ty.Speed]
	if !ok {
		base = typingFade["auto"]
	}
	if ty.autoShorten {
		base = base * 2 / 3
	}
	return base
}

// Sync reconciles the marks with the new text of the input line.
//
// It only needs to understand the two edits this prompt supports - a character
// appended, and a character removed from the end. Anything else (a paste that
// reshapes the middle) re-stamps the whole line, which is honest: those
// characters did just appear.
func (ty *TypingState) Sync(now time.Time, text string) {
	next := []rune(text)
	prev := ty.lastText
	ty.lastText = next
	interval := now.Sub(ty.lastAt)
	ty.lastAt = now
	if !ty.Enabled {
		ty.marks = ty.marks[:0]
		return
	}

	switch {
	case len(next) == len(prev)+1 && startsWith(next, prev):
		ty.marks = append(ty.marks, typeMark{At: now})
	case len(next) < len(prev) && startsWith(prev, next):
		if len(ty.marks) > len(next) {
			ty.marks = ty.marks[:len(next)]
		}
	case len(next) == len(prev):
	default:
		ty.marks = ty.marks[:0]
		for range next {
			ty.marks = append(ty.marks, typeMark{At: now})
		}
	}
	if ty.Speed == "auto" {
		ty.autoShorten = interval > 0 && interval < 120*time.Millisecond
	}
	ty.trim(now)
}

// trim drops marks that have fully faded, so the slice does not grow with every
// keystroke over a long session.
func (ty *TypingState) trim(now time.Time) {
	fade := ty.Fade()
	keep := 0
	for _, m := range ty.marks {
		if now.Sub(m.At) < fade {
			keep++
		}
	}
	if keep == len(ty.marks) {
		return
	}
	ty.marks = ty.marks[len(ty.marks)-keep:]
}

// Brightness is how lit the character at rune index i is right now, 0..1.
func (ty *TypingState) Brightness(i int, now time.Time) float64 {
	if !ty.Enabled || i < 0 || i >= len(ty.marks) {
		return 0
	}
	fade := ty.Fade()
	elapsed := now.Sub(ty.marks[i].At)
	if elapsed < 0 || elapsed > fade {
		return 0
	}
	return 1 - EaseOutCubic(float64(elapsed)/float64(fade))
}

// Live reports whether any character is still glowing, which is what tells the
// draw loop it has something to animate.
func (ty *TypingState) Live(now time.Time) bool {
	if !ty.Enabled {
		return false
	}
	fade := ty.Fade()
	for _, m := range ty.marks {
		if e := now.Sub(m.At); e >= 0 && e < fade {
			return true
		}
	}
	return false
}

// Reset clears the marks, called when the input line is submitted.
func (ty *TypingState) Reset() {
	ty.marks = ty.marks[:0]
	ty.lastText = nil
	ty.autoShorten = false
}

// DrawInput draws the input line, tinting each character toward the sweep
// colour by how recently it was typed.
func (ty *TypingState) DrawInput(b *Buffer, x, y, limit int, text string, now time.Time, base Style) {
	if !ty.Enabled {
		b.WriteClipped(x, y, limit, text, base)
		return
	}
	col := x
	for i, r := range []rune(text) {
		if col >= limit {
			break
		}
		sty := base
		if lit := ty.Brightness(i, now); lit > 0 {
			amt := lit * float64(ty.Intensity) / 100.0
			sty.Fg = Mix(base.Fg, ty.Color, amt*0.85)
			if amt > 0.5 {
				sty.Bold = true
			}
		}
		b.WriteClipped(col, y, limit, string(r), sty)
		col += runeWidth(r)
	}
}

// startsWith reports whether a begins with all of b.
func startsWith(a, b []rune) bool {
	if len(b) > len(a) {
		return false
	}
	for i := range b {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
