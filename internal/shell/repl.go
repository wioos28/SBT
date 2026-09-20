//go:build linux

package shell

import (
	"errors"
	"io"
	"strings"

	"github.com/wioos28/sbt/internal/ui"
	"github.com/wioos28/sbt/internal/version"
)

// Run probes the platform and then hands the terminal to the interactive
// session loop.
func Run() int {
	s := newSession()
	s.probe()
	printBanner(s)
	return s.loop()
}

// printBanner draws the startup screen: identity, verified isolation and the
// command list. This is what the user sees when they type `sbt`.
func printBanner(s *Session) {
	t := s.theme
	sym := t.Symbols()
	s.line("")
	s.line("  %s  %s", t.Yellow(version.Name+" — Sandbox Terminal"), version.String())
	s.line("  %s", t.Gray(strings.Repeat(sym.HLine, 76)))
	for _, f := range s.caps.Features {
		mark, lvl := sym.OK, t.Green
		switch f.Level {
		case 1:
			mark, lvl = sym.Warn, t.Yellow
		case 2, 3:
			mark, lvl = sym.Dot, t.Gray
		}
		s.line("  %s %-20s %s", lvl(mark), f.Label, t.Gray(f.Reason))
	}
	s.line("  %s", t.Gray(strings.Repeat(sym.HLine, 76)))
	s.line("  %s %s   %s %s   %s %s",
		t.Green("start <cmd>"), t.Gray("run in a verified sandbox"),
		t.Yellow("status"), t.Gray("state & limits"),
		t.Text("help"), t.Gray("all commands · exit to leave"))
	s.line("")
}

// loop reads commands from the terminal until the user exits.
func (s *Session) loop() int {
	editor := ui.NewEditor(s.theme, s.in, s.out)
	prompt := s.theme.Yellow("sbt") + s.theme.Gray("> ")
	for {
		line, err := editor.ReadLine(prompt)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, ui.ErrInterrupted) {
				s.line("")
				s.exitShell()
				return 0
			}
			s.line("%s %s", s.theme.Red("input error:"), err.Error())
			return 1
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if code := s.dispatch(line); code >= 0 {
			if code > 0 {
				s.exitShell()
			}
			return code
		}
	}
}

// dispatch runs one command line. It returns -1 to keep the loop running, or
// a non-negative exit code to leave.
func (s *Session) dispatch(line string) int {
	cmd, args := splitCommand(line)
	switch cmd {
	case "exit", "quit":
		return 0
	case "help", "?":
		s.cmdHelp()
	case "status":
		s.cmdStatus()
	case "start", "run":
		s.cmdStart(args)
	case "stop":
		s.cmdStop()
	case "net":
		s.cmdNet(args)
	case "mem":
		s.cmdMem(args)
	case "cpu":
		s.cmdCPU(args)
	case "clear", "cls":
		s.out.WriteString("\x1b[2J\x1b[H")
	default:
		// Bare words are treated as a sandbox command for convenience.
		s.cmdStart(strings.Fields(line))
	}
	return -1
}

func splitCommand(line string) (string, []string) {
	f := strings.Fields(line)
	if len(f) == 0 {
		return "", nil
	}
	return f[0], f[1:]
}
