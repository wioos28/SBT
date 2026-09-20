package ui

import (
	"fmt"
	"os"
	"strings"
)

// Frame draws the persistent SBT terminal chrome: a mode header on top and a
// status bar pinned to the bottom, with the interactive session scroll region
// in between. When stdout is not a terminal the frame degrades to a plain
// banner and prints nothing else.
type Frame struct {
	out     *os.File
	theme   *Theme
	enabled bool
	mode    ModeName
	sub     string
	items   []string
	width   int
	height  int
	header  int
	active  bool
}

// NewFrame builds a frame for the given mode.
func NewFrame(theme *Theme, out *os.File, mode ModeName) *Frame {
	f := &Frame{out: out, theme: theme, mode: mode, enabled: IsTerminal(out)}
	f.width, f.height = TerminalSize(out)
	return f
}

// SetSubtitle sets the short right-hand text in the header.
func (f *Frame) SetSubtitle(s string) { f.sub = s }

// SetStatusItems sets the footer items (e.g. sandbox state, network, cpu).
func (f *Frame) SetStatusItems(items []string) { f.items = items }

// SetMode switches the frame mode (used by `sbt mode`).
func (f *Frame) SetMode(m ModeName) { f.mode = m }

// Enabled reports whether the frame is drawing real terminal chrome.
func (f *Frame) Enabled() bool { return f.enabled }

// Begin draws the header and footer and enters the scroll region.
func (f *Frame) Begin() {
	if !f.enabled {
		f.printf("%s\n", f.theme.Leveled(ModeLevel(f.mode), "SBT "+string(f.mode)+" MODE - sandbox session"))
		return
	}
	f.width, f.height = TerminalSize(f.out)
	if f.height < 8 {
		f.enabled = false
		return
	}
	f.header = 2
	sep := strings.Repeat(f.theme.Symbols().HLine, f.width)
	f.printf("\x1b[2J\x1b[H")
	f.printf("\x1b[1;1H%s", Truncate(f.theme.ModeBanner(f.mode, f.sub, f.width), f.width))
	f.printf("\x1b[2;1H%s", f.theme.Leveled(ModeLevel(f.mode), Truncate(sep, f.width)))
	// Scroll region: rows 3 .. height-1, leaving the last row for the status bar.
	f.printf("\x1b[3;%dr", f.height-1)
	f.printf("\x1b[3;1H")
	f.drawFooter()
	f.active = true
}

func (f *Frame) drawFooter() {
	if !f.enabled {
		return
	}
	f.printf("\x1b7")                  // save cursor
	f.printf("\x1b[%d;1H", f.height-1) // hline separator row
	f.printf("\x1b[%d;1H%s", f.height-1, f.theme.Leveled(ModeLevel(f.mode), strings.Repeat(f.theme.Symbols().HLine, f.width)))
	f.printf("\x1b[%d;1H%s", f.height, f.theme.StatusLine(f.mode, f.items, f.width))
	f.printf("\x1b8") // restore cursor
}

// Refresh redraws the footer (used after the status items change).
func (f *Frame) Refresh() {
	if !f.enabled || !f.active {
		return
	}
	f.drawFooter()
}

// Resize re-reads the terminal geometry and redraws the frame.
func (f *Frame) Resize() {
	if !f.enabled {
		return
	}
	f.Begin()
}

// Print writes raw text inside the scroll region.
func (f *Frame) Print(format string, args ...any) {
	f.printf(format, args...)
}

// Line prints a line inside the scroll region.
func (f *Frame) Line(s string) { f.printf("%s\n", s) }

// End leaves the scroll region and prints a closing line.
func (f *Frame) End() {
	if !f.enabled || !f.active {
		return
	}
	f.printf("\x1b[r")
	f.printf("\x1b[%d;1H\n", f.height)
	f.active = false
}

func (f *Frame) printf(format string, args ...any) {
	fmt.Fprintf(f.out, format, args...)
}

// ClearLine clears the current terminal line.
func (f *Frame) ClearLine() {
	if f.enabled {
		f.printf("\x1b[2K")
	}
}

// PromptLines renders the informational preamble printed before a long
// running child command, so the user always knows where the command runs.
func (t *Theme) PromptLines(cwd string) string {
	return t.Yellow("[SBT LOW]") + " " + t.Gray("interactive shell - type") + " " + t.Text("help") + " " +
		t.Gray("for sandbox commands, ") + t.Text("sbt exit") + t.Gray(" to close the sandbox")
}
