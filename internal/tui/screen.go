package tui

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/wioos28/sbt/internal/ui"
)

// Screen owns the terminal: raw mode, the alternate screen, and the double
// buffer that turns a redraw into the smallest possible write.
//
// The low level termios and window-size code lives in internal/ui, which the
// non-interactive fallback shell also uses; SBT keeps exactly one copy of it.
type Screen struct {
	in, out  *os.File
	restore  func()
	theme    *Theme
	prev     *Buffer
	cur      *Buffer
	w, h     int
	started  bool
	suspend  int
	lastDraw bytes.Buffer
}

// OpenScreen puts the terminal into the interactive SBT mode.
func OpenScreen(in, out *os.File) (*Screen, error) {
	s := &Screen{in: in, out: out}
	s.w, s.h = ui.TerminalSize(out)
	s.cur = NewBuffer(s.w, s.h)
	if err := s.enter(); err != nil {
		return nil, err
	}
	return s, nil
}

// enter switches to raw mode, the alternate screen and hides the cursor.
func (s *Screen) enter() error {
	restore, err := ui.RawMode(s.in)
	if err != nil {
		return fmt.Errorf("the terminal cannot be put into raw mode: %w", err)
	}
	s.restore = restore
	// 1049: alternate screen with a saved cursor; 25: hide cursor; 2026: make
	// one redraw atomic so a slow terminal never shows a half drawn frame.
	fmt.Fprint(s.out, "\x1b[?1049h\x1b[?25l\x1b[2J\x1b[H")
	s.started = true
	return nil
}

// Size returns the current terminal geometry.
func (s *Screen) Size() (int, int) { return s.w, s.h }

// Frame returns the buffer views draw into. Its content is undefined until the
// view has drawn every cell it cares about.
func (s *Screen) Frame() *Buffer { return s.cur }

// Resized re-reads the terminal size and reports whether it changed. The frame
// buffer is reset when it did.
func (s *Screen) Resized() bool {
	w, h := ui.TerminalSize(s.out)
	if w == s.w && h == s.h {
		return false
	}
	if w < 20 || h < 6 {
		// Refuse to draw into a geometry that cannot hold the cage: keep the
		// last good size so the UI stays coherent until the terminal recovers.
		return false
	}
	s.w, s.h = w, h
	s.cur.Resize(w, h)
	s.prev = nil
	return true
}

// Present writes the difference between the current frame and the last one.
func (s *Screen) Present() {
	if !s.started || s.suspend > 0 {
		return
	}
	s.lastDraw.Reset()
	s.lastDraw.WriteString("\x1b[?2026h")
	same := s.prev != nil && s.prev.W == s.cur.W && s.prev.H == s.cur.H
	drew := false
	for y := 0; y < s.cur.H; y++ {
		if same && s.rowEqual(y) {
			continue
		}
		drew = true
		s.lastDraw.WriteString("\x1b[" + itoa(y+1) + ";1H")
		s.writeRow(y)
	}
	s.lastDraw.WriteString("\x1b[0m\x1b[?2026l")
	if drew {
		_, _ = s.out.Write(s.lastDraw.Bytes())
	}
	if !same {
		s.prev = NewBuffer(s.cur.W, s.cur.H)
	}
	copy(s.prev.Cells, s.cur.Cells)
}

// rowEqual reports whether one row of the current frame matches the last one.
func (s *Screen) rowEqual(y int) bool {
	start := y * s.cur.W
	end := start + s.cur.W
	for i := start; i < end; i++ {
		a, b := s.cur.Cells[i], s.prev.Cells[i]
		if a.R != b.R || a.S != b.S {
			return false
		}
	}
	return true
}

// writeRow renders one row, emitting a style sequence only when it changes.
func (s *Screen) writeRow(y int) {
	theme := s.theme
	var style Style
	haveStyle := false
	for x := 0; x < s.cur.W; x++ {
		c := s.cur.Cells[y*s.cur.W+x]
		if c.R == 0 {
			continue // padding cell of a wide rune
		}
		if theme != nil && (!haveStyle || c.S != style) {
			s.lastDraw.WriteString(theme.Style(c.S))
			style, haveStyle = c.S, true
		}
		s.lastDraw.WriteRune(c.R)
	}
	if theme != nil {
		s.lastDraw.WriteString(theme.Reset())
	}
}

// theme is set by the App so the screen can emit style sequences. A nil theme
// renders plain text.
func (s *Screen) SetTheme(t *Theme) { s.theme = t }

// Suspend hands the terminal back for a child process: the alternate screen is
// left, the cursor restored and the terminal returned to cooked mode. SBT never
// proxies a sandbox command's terminal, so this is how a sandboxed command gets
// a real tty.
func (s *Screen) Suspend() {
	if !s.started || s.suspend > 0 {
		s.suspend++
		return
	}
	s.suspend++
	fmt.Fprint(s.out, "\x1b[0m\x1b[?25h\x1b[?1049l")
	if s.restore != nil {
		s.restore()
	}
}

// Resume takes the terminal back after a child process has finished.
func (s *Screen) Resume() {
	if s.suspend == 0 {
		return
	}
	s.suspend--
	if s.suspend > 0 {
		return
	}
	restore, err := ui.RawMode(s.in)
	if err == nil {
		s.restore = restore
	}
	fmt.Fprint(s.out, "\x1b[?1049h\x1b[?25l\x1b[2J\x1b[H")
	s.prev = nil
}

// Printf writes raw text while the screen is suspended. It is used for the few
// messages SBT prints outside the cage, such as a capture failure warning.
func (s *Screen) Printf(format string, args ...any) {
	fmt.Fprintf(s.out, format, args...)
}

// Close restores the terminal.
func (s *Screen) Close() {
	if s.restore != nil {
		s.restore()
		s.restore = nil
	}
	if s.started {
		fmt.Fprint(s.out, "\x1b[0m\x1b[?25h\x1b[?1049l")
		s.started = false
	}
}

// PlainReport prints the session summary after the screen is closed, so the
// user's scrollback keeps a record of what happened inside the cage.
func (s *Screen) PlainReport(lines []string) {
	for _, l := range lines {
		fmt.Fprintln(s.out, strings.TrimRight(l, " \t"))
	}
}
