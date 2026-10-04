package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// The shipped command line must work. A v0.1 that prints a usage error for a
// documented subcommand, or stores a value the UI would refuse, is not a
// release.
func TestDocumentedCommandsAllWork(t *testing.T) {
	t.Setenv("SBT_HOME", t.TempDir())
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"version"}, "v0.1.0"},
		{[]string{"help"}, "Usage:"},
		{[]string{"doctor"}, "kernel:"},
		{[]string{"security"}, "SECURITY STATUS"},
		{[]string{"security", "permissions"}, "PERMISSIONS"},
		{[]string{"security", "scan"}, "SECURITY SCAN"},
		{[]string{"setting"}, "SBT settings"},
		{[]string{"setting", "list"}, "SBT settings"},
		{[]string{"language"}, "en-US"},
		{[]string{"language", "list"}, "en-US"},
	}
	for _, c := range cases {
		out := capture(t, c.args)
		if !strings.Contains(out, c.want) {
			t.Errorf("sbt %s must print %q, got:\n%s", strings.Join(c.args, " "), c.want, out)
		}
	}
}

// Every locale SBT advertises must actually be selectable and listed.
func TestEveryAdvertisedLocaleIsSelectable(t *testing.T) {
	t.Setenv("SBT_HOME", t.TempDir())
	list := capture(t, []string{"language", "list"})
	for _, locale := range []string{"en-US", "vi-VN", "ru-RU", "zh-CN"} {
		if !strings.Contains(list, locale) {
			t.Errorf("%s must be listed by `sbt language list`", locale)
		}
		if out := capture(t, []string{"language", "use", locale}); !strings.Contains(out, locale) {
			t.Errorf("sbt language use %s must succeed, got:\n%s", locale, out)
		}
	}
}

// A value outside the declared range must be refused by the CLI exactly as the
// Settings view refuses it, and must not be written.
func TestSettingSetRefusesOutOfRange(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SBT_HOME", dir)
	out := capture(t, []string{"setting", "set", "anim.typing_intensity", "999"})
	if !strings.Contains(out, "between 0 and 100") {
		t.Fatalf("an out-of-range value must be refused, got:\n%s", out)
	}
	if got := capture(t, []string{"setting", "get", "anim.typing_intensity"}); strings.Contains(got, "999") {
		t.Fatal("a refused value must not be stored")
	}
	if out := capture(t, []string{"setting", "set", "anim.typing_intensity", "75"}); !strings.Contains(out, "75") {
		t.Fatalf("a legal value must be accepted, got:\n%s", out)
	}
}

func TestSettingSetRefusesUnknownPalette(t *testing.T) {
	t.Setenv("SBT_HOME", t.TempDir())
	out := capture(t, []string{"setting", "set", "ui.palette", "bogus"})
	if !strings.Contains(out, "must be one of") {
		t.Fatalf("an unknown preset must be refused, got:\n%s", out)
	}
}

// `sbt security scan` used to print a fixed list of PASS lines for checks that
// were never run. It must now report what it measured, and must not claim a
// capability it could not verify.
func TestSecurityScanReportsMeasuredChecks(t *testing.T) {
	t.Setenv("SBT_HOME", t.TempDir())
	out := capture(t, []string{"security", "scan"})
	for _, want := range []string{"TEST 01", "TEST 11", "passed"} {
		if !strings.Contains(out, want) {
			t.Errorf("the scan must report %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "TEST 12") {
		t.Error("the scan must not invent rows beyond the measured capabilities")
	}
}

// capture runs the CLI with args and returns stdout and stderr together.
func capture(t *testing.T, args []string) string {
	t.Helper()
	old := os.Args
	os.Args = append([]string{"sbt"}, args...)
	defer func() { os.Args = old }()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	done := make(chan string, 1)
	go func() {
		out, _ := io.ReadAll(r)
		done <- string(out)
	}()
	code := dispatch(args)
	_ = w.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	out := <-done
	_ = r.Close()
	_ = code
	return out
}
