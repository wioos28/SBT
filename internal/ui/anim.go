package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Step is one line of the sandbox initialization checklist.
type Step struct {
	Label  string
	Level  CapLevel
	Detail string
}

// StartupAnimation plays the short SBT boot animation. It only emits escape
// sequences on a terminal and never blocks longer than roughly one second; the
// animation is decoration layered on top of real detection results, never a
// substitute for them.
func (t *Theme) StartupAnimation(out *os.File, title string, steps []Step) {
	tty := IsTerminal(out)
	sym := t.Symbols()
	fmt.Fprintln(out, t.Bold(title))
	if tty {
		open := sym.Dot + " " + sym.Dot
		blink := " " + sym.Dot
		fmt.Fprintf(out, "  %s   %s", open, open)
		for i := 0; i < 2; i++ {
			time.Sleep(120 * time.Millisecond)
			fmt.Fprintf(out, "\r\x1b[2K  %s   %s", open, blink)
			time.Sleep(90 * time.Millisecond)
			fmt.Fprintf(out, "\r\x1b[2K  %s   %s", open, open)
		}
		fmt.Fprintf(out, "\r\x1b[2K")
	}
	fmt.Fprintln(out, t.Gray("Sandbox initializing..."))
	for _, st := range steps {
		glyph := t.ColorizeLevel(st.Level, GlyphOK)
		if st.Level != CapFull {
			glyph = t.Glyph(st.Level)
		}
		line := Pad(st.Label, 16) + glyph
		if st.Detail != "" {
			line += "  " + t.Gray(st.Detail)
		}
		fmt.Fprintln(out, line)
		if tty {
			time.Sleep(70 * time.Millisecond)
		}
	}
	fmt.Fprintln(out)
}

// Spinner shows progress for operations that can take a moment. On a
// non-terminal it prints only the start and end lines so logs stay readable.
type Spinner struct {
	out    *os.File
	theme  *Theme
	msg    string
	tty    bool
	stop   chan struct{}
	done   chan struct{}
	mu     sync.Mutex
	suffix string
}

// NewSpinner creates a spinner writing to out.
func NewSpinner(theme *Theme, out *os.File, msg string) *Spinner {
	return &Spinner{out: out, theme: theme, msg: msg, tty: IsTerminal(out)}
}

// Start begins rendering the spinner.
func (s *Spinner) Start() {
	if s.tty {
		s.stop = make(chan struct{})
		s.done = make(chan struct{})
		go func() {
			defer close(s.done)
			frames := []string{"|", "/", "-", "\\"}
			for i := 0; ; i++ {
				select {
				case <-s.stop:
					fmt.Fprintf(s.out, "\r\x1b[2K")
					return
				default:
				}
				s.mu.Lock()
				suffix := s.suffix
				s.mu.Unlock()
				fmt.Fprintf(s.out, "\r\x1b[2K%s %s %s", s.theme.Yellow(frames[i%len(frames)]), s.msg, s.theme.Gray(suffix))
				time.Sleep(120 * time.Millisecond)
			}
		}()
		return
	}
	fmt.Fprintf(s.out, "%s\n", s.msg)
}

// Update replaces the trailing detail text of the spinner.
func (s *Spinner) Update(suffix string) {
	s.mu.Lock()
	s.suffix = suffix
	s.mu.Unlock()
}

// Stop ends the spinner.
func (s *Spinner) Stop() {
	if s.tty && s.stop != nil {
		close(s.stop)
		<-s.done
		s.stop = nil
	}
}

// ProgressFunc returns a callback that renders a single line progress indicator.
func (t *Theme) ProgressFunc(out *os.File, prefix string) func(done, total int64) {
	tty := IsTerminal(out)
	last := time.Now()
	return func(done, total int64) {
		if !tty {
			return
		}
		if time.Since(last) < 100*time.Millisecond && done < total {
			return
		}
		last = time.Now()
		text := fmt.Sprintf("%s %s / %s", prefix, humanBytes(done), humanBytes(total))
		fmt.Fprintf(out, "\r\x1b[2K%s %s", t.Gray(text), strings.Repeat(".", 1))
		if done >= total {
			fmt.Fprintf(out, "\n")
		}
	}
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	v := float64(n)
	for _, u := range units {
		v /= unit
		if v < unit {
			return fmt.Sprintf("%.1f %s", v, u)
		}
	}
	return fmt.Sprintf("%.1f PB", v/unit)
}
