package tui

import (
	"strings"
	"time"
)

// The startup sequence.
//
// The rule this file exists to enforce: the boot screen only ever shows work
// that was actually done. A stage is a record of a real step - the workspace
// open, the platform probe, the permission scan, the configuration read - and
// the session creates it at the moment the step completes. A stage that has not
// run is drawn as pending, never as a tick, and the progress bar is the
// fraction of stages that genuinely finished. Nothing here fakes a check.

// Wordmark is the product name shown under the large logo.
const Wordmark = "sbt-wioos28"

// BootPhase tracks where the startup sequence is.
type BootPhase int

// Startup phases, in order.
const (
	// BootRunning is the animated startup screen.
	BootRunning BootPhase = iota
	// BootWelcome is the welcome panel shown once the checks are done.
	BootWelcome
	// BootCage is the normal interface.
	BootCage
)

// bootStageState is how far one startup step has got.
type bootStageState int

const (
	stagePending bootStageState = iota
	stageDone
	stageFailed
	stageSkipped
)

// BootStage is one recorded startup step.
type BootStage struct {
	Label string
	// Detail is the evidence the step produced, e.g. the kernel version.
	Detail string
	State  bootStageState
	At     time.Time
}

// BootSequence is the ordered record of startup work.
type BootSequence struct {
	Stages  []BootStage
	Started time.Time
	Phase   BootPhase
	// Reveal is how many stages the screen has shown. It trails len(Stages)
	// while the animation plays, so the progress bar eases toward the real
	// number instead of snapping to it.
	Reveal float64
}

// Add records a completed startup step. ok is the real outcome: a step that
// failed is recorded as failed, never hidden behind a tick.
func (b *BootSequence) Add(label, detail string, ok bool, now time.Time) {
	state := stageDone
	if !ok {
		state = stageFailed
	}
	b.Stages = append(b.Stages, BootStage{Label: label, Detail: detail, State: state, At: now})
}

// Done reports whether at least one stage ran and none are still pending.
func (b BootSequence) Done() bool {
	if len(b.Stages) == 0 {
		return false
	}
	for _, s := range b.Stages {
		if s.State == stagePending {
			return false
		}
	}
	return true
}

// Progress is the fraction of stages that actually ran.
func (b BootSequence) Progress() float64 {
	if len(b.Stages) == 0 {
		return 0
	}
	done := 0
	for _, s := range b.Stages {
		if s.State != stagePending {
			done++
		}
	}
	return float64(done) / float64(len(b.Stages))
}

// sbtLogo is the large wordmark drawn on the startup screen. It is a fixed
// block font rather than a font file so SBT keeps its zero-dependency build.
var sbtLogo = []string{
	" ███████╗ ██████╗ ████████╗",
	" ██╔════╝ ██╔══██╗╚══██╔══╝",
	" ███████╗ ██████╔╝   ██║   ",
	" ╚════██║ ██╔══██╗   ██║   ",
	" ███████║ ██████╔╝   ██║   ",
	" ╚══════╝ ╚═════╝    ╚═╝   ",
}

// sbtLogoASCII is the fallback for a terminal that cannot draw the block font.
var sbtLogoASCII = []string{
	"  +-------+ +------+ +--------+",
	"  |  +----+ | +--+ | +---+    ",
	"  +----+ |  +-+  +-+     |    ",
	"  +----+ |  +-+  +-+     |    ",
	"  +-------+ +------+     +    ",
	"  ..........................  ",
}

// Boot draws the startup screen. It is a pure function of the sequence and the
// clock: the shine sweep is derived from now, so a frame is reproducible.
func (i *Interpreter) Boot(b *Buffer, boot *BootSequence, now time.Time) {
	t := i.Theme
	p := t.Palette
	w, h := b.W, b.H
	if w < 24 || h < 8 {
		b.WriteClipped(0, 0, w, tr("boot.starting", "SBT starting"), Style{Fg: p.Primary, Bold: true})
		return
	}
	motion := t.Motion

	logo := sbtLogo
	if t.ASCII {
		logo = sbtLogoASCII
	}
	logoW := 0
	for _, row := range logo {
		if n := StringWidth(row); n > logoW {
			logoW = n
		}
	}

	total := len(logo) + 3 + len(boot.Stages) + 3
	top := (h - total) / 2
	if top < 1 {
		top = 1
	}

	sweepCol := -1
	if motion && boot.Phase == BootRunning {
		sweepCol = Sweep(now, boot.Started, SweepPeriod, logoW+8) - 4
	}
	for r, row := range logo {
		y := top + r
		if y >= h {
			break
		}
		x := (w - logoW) / 2
		if x < 0 {
			x = 0
		}
		col := x
		for _, ch := range row {
			lit := 0.0
			if sweepCol >= 0 {
				d := col - (x + sweepCol)
				if d < 0 {
					d = -d
				}
				if d < 6 {
					lit = 1 - float64(d)/6
				}
			}
			fg := p.Primary
			if lit > 0 {
				fg = Glow(p.Primary, lit*0.6)
			}
			b.Set(col, y, ch, Style{Fg: fg, Bold: lit > 0.15})
			col++
		}
	}

	barY := top + len(logo) + 1
	barW := logoW + 6
	if barW > w-2 {
		barW = w - 2
	}
	barX := (w - barW) / 2
	if barX < 0 {
		barX = 0
	}
	barColor := p.Primary
	if motion && sweepCol >= 0 {
		barColor = Glow(p.Primary, 0.2*Breath(now, boot.Started, SweepPeriod))
	}
	b.WriteClipped(barX, barY, barX+barW, strings.Repeat("▔", barW), Style{Fg: barColor})
	mark := Wordmark
	if t.ASCII {
		mark = strings.ToUpper(mark)
	}
	mx := (w - StringWidth(mark)) / 2
	if mx < 0 {
		mx = 0
	}
	b.WriteClipped(mx, barY+1, w, mark, Style{Fg: p.Primary, Bold: true})
	b.WriteClipped(barX, barY+2, barX+barW, strings.Repeat("▁", barW), Style{Fg: barColor})

	progY := barY + 4
	prog := boot.Progress()
	shown := boot.Reveal
	if shown > prog {
		shown = prog
	}
	if !motion {
		shown = prog
	}
	pw := w - 14
	if pw > 46 {
		pw = 46
	}
	if pw >= 8 {
		px := (w - pw) / 2
		if px < 0 {
			px = 0
		}
		b.WriteClipped(px, progY, px+pw, progressBar(shown, pw), Style{Fg: p.Primary})
		// The number tracks the bar, not the raw count, so the two never
		// disagree on screen. The raw count still decides which stage rows are
		// allowed to show a tick.
		b.WriteClipped(px+pw+1, progY, w, itoa(int(shown/float64(len(boot.Stages))*100+0.5))+"%", Style{Fg: p.Muted})
	}

	y := progY + 2
	for j, s := range boot.Stages {
		if y+j >= h-1 {
			break
		}
		revealed := float64(j+1) <= boot.Reveal+0.001 || !motion
		line, sty := bootStageLine(s, i.Theme, revealed)
		b.WriteClipped(2, y+j, w-2, line, sty)
	}
	if boot.Phase == BootRunning && h > 2 {
		b.WriteClipped(2, h-1, w-2, tr("boot.anyKey", "any key skips"), Style{Fg: p.Muted})
	}
}

// bootStageLine renders one stage row. A stage the animation has not reached
// yet is drawn as pending, never as done, so the screen cannot claim a step
// that has not happened on this frame.
func bootStageLine(s BootStage, t *Theme, revealed bool) (string, Style) {
	p := t.Palette
	g := t.Glyphs()
	if !revealed {
		s.State = stagePending
	}
	switch s.State {
	case stageDone:
		line := g.Check + " " + s.Label
		if s.Detail != "" {
			line += "  " + s.Detail
		}
		return line, Style{Fg: p.Text}
	case stageFailed:
		line := g.Cross_ + " " + s.Label
		if s.Detail != "" {
			line += "  " + s.Detail
		}
		return line, Style{Fg: p.Warning, Bold: true}
	case stageSkipped:
		return "~ " + s.Label, Style{Fg: p.Muted}
	default:
		return g.Ring + " " + s.Label, Style{Fg: p.Muted}
	}
}

// progressBar renders a filled/empty bar of the given ratio.
func progressBar(ratio float64, width int) string {
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := int(ratio*float64(width) + 0.5)
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}
