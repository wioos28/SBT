package ui

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Confirm asks a yes/no question. The default is used when the user just hits
// Enter and when input is not available (EOF, non-interactive stdin). SBT never
// treats a missing answer as consent for a destructive operation because the
// security-relevant call sites pass false as the default.
func (t *Theme) Confirm(in *os.File, question string, def bool) (bool, error) {
	suffix := "[y/N]"
	if def {
		suffix = "[Y/n]"
	}
	prompt := t.Leveled(LevelWarn, GlyphWarn+" ") + question + " " + t.Gray(suffix) + " "
	ans, err := ReadLinePlain(in, prompt)
	if err != nil && ans == "" {
		return def, err
	}
	return parseYesNo(ans, def), nil
}

func parseYesNo(ans string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(ans)) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return def
	}
}

// Select renders a numbered menu and returns the chosen index (1-based).
func (t *Theme) Select(in *os.File, title string, options []string, def int) (int, error) {
	if title != "" {
		fmt.Fprintln(os.Stdout, t.Bold(title))
	}
	for i, opt := range options {
		fmt.Fprintf(os.Stdout, " %s %s\n", t.Gray("["+strconv.Itoa(i+1)+"]"), opt)
	}
	for {
		ans, err := ReadLinePlain(in, t.Gray("> "))
		if err != nil && ans == "" {
			return def, err
		}
		ans = strings.TrimSpace(ans)
		if ans == "" {
			return def, nil
		}
		n, cerr := strconv.Atoi(ans)
		if cerr == nil && n >= 1 && n <= len(options) {
			return n, nil
		}
		fmt.Fprintln(os.Stdout, t.Red("Invalid selection."))
	}
}

// InputLine asks for a free-form line, returning def on empty input.
func (t *Theme) InputLine(in *os.File, question, def string) (string, error) {
	hint := ""
	if def != "" {
		hint = " [" + def + "]"
	}
	ans, err := ReadLinePlain(in, question+t.Gray(hint)+" ")
	if err != nil && ans == "" {
		return def, err
	}
	ans = strings.TrimSpace(ans)
	if ans == "" {
		return def, nil
	}
	return ans, nil
}

// PressEnter waits for a newline so that output is not lost.
func PressEnter(in *os.File, hint string) {
	if !IsTerminal(in) {
		return
	}
	_, _ = ReadLinePlain(in, "\n"+hint)
}

// PrintErr writes a message to stderr.
func PrintErr(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

// Fprintln writes a line to an arbitrary writer.
func Fprintln(w io.Writer, s string) { fmt.Fprintln(w, s) }

// Fprintf writes formatted output to an arbitrary writer.
func Fprintf(w io.Writer, format string, args ...any) { fmt.Fprintf(w, format, args...) }
