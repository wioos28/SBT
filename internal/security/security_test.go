package security

import (
	"strings"
	"testing"
	"time"

	"github.com/wioos28/sbt/internal/i18n"
)

func TestLevelOrdering(t *testing.T) {
	if Critical.Rank() <= Danger.Rank() {
		t.Fatal("critical must rank above danger")
	}
	if Warning.Rank() <= Info.Rank() {
		t.Fatal("warning must rank above info")
	}
	if Notice.Rank() <= Info.Rank() {
		t.Fatal("notice must rank above info")
	}
}

func TestParseLevel(t *testing.T) {
	for name, want := range map[string]Level{
		"info": Info, "notice": Notice, "warning": Warning, "warn": Warning,
		"danger": Danger, "critical": Critical, "CRIT": Critical,
	} {
		got, ok := ParseLevel(name)
		if !ok || got != want {
			t.Fatalf("ParseLevel(%q) = %v,%v want %v", name, got, ok, want)
		}
	}
	if _, ok := ParseLevel("bogus"); ok {
		t.Fatal("unknown level should not parse")
	}
}

func TestDecideIsFailClosed(t *testing.T) {
	if Decide(Critical) != FreezeSandbox {
		t.Fatal("critical must freeze the sandbox")
	}
	if Decide(Danger) != BlockRisk {
		t.Fatal("danger must block the risky operation")
	}
	if Decide(Warning) != ContinueWithPolicy {
		t.Fatal("warning must continue under policy")
	}
	if Decide(Info) != Allow {
		t.Fatal("info must be allowed")
	}
}

func TestCriticalWarningModalHasNoIgnore(t *testing.T) {
	w := NewWarningModal(Alert{Level: Critical, Sandbox: "ai-coder", Reason: "mount namespace unavailable"})
	if w.AllowIgnore {
		t.Fatal("critical warning must not offer Ignore")
	}
	if !w.Blocking {
		t.Fatal("critical warning must block")
	}
	modal := w.Modal()
	if !strings.Contains(modal, "ai-coder") {
		t.Fatal("modal should name the sandbox")
	}
	if strings.Contains(modal, "[ Ignore ]") {
		t.Fatal("critical modal must not offer an Ignore button")
	}
	for _, b := range w.Buttons {
		if strings.EqualFold(b, "Ignore") {
			t.Fatal("critical warning must not list an Ignore button")
		}
	}
	if !strings.Contains(modal, "[ Inspect ]") || !strings.Contains(modal, "[ Destroy ]") {
		t.Fatalf("critical modal should offer Inspect/Destroy, got:\n%s", modal)
	}
}

func TestWarningLevelsButtons(t *testing.T) {
	if w := NewWarningModal(Alert{Level: Danger}); w.AllowIgnore {
		t.Fatal("danger must not be ignorable")
	}
	if w := NewWarningModal(Alert{Level: Warning}); !w.AllowIgnore {
		t.Fatal("warning should be ignorable under policy")
	}
}

func TestTimelineRecordsAndCounts(t *testing.T) {
	tl := NewTimeline(10)
	tl.Record(Info, "sbx", "sandbox started")
	tl.Record(Warning, "sbx", "protected path requested")
	tl.Record(Critical, "sbx", "isolation verification failed")
	c := tl.CountsByLevel()
	if c[Critical] != 1 || c[Warning] != 1 || c[Info] != 1 {
		t.Fatalf("counts wrong: %+v", c)
	}
	if len(tl.Critical()) != 1 {
		t.Fatalf("expected 1 critical alert")
	}
	if len(tl.Recent(2)) != 2 {
		t.Fatalf("recent(2) wrong")
	}
}

func TestTimelineCaps(t *testing.T) {
	tl := NewTimeline(3)
	for i := 0; i < 10; i++ {
		tl.Record(Info, "s", "e")
	}
	if tl.Len() != 3 {
		t.Fatalf("timeline should cap at 3, got %d", tl.Len())
	}
}

func TestScanIsNonDestructiveAndReports(t *testing.T) {
	// The scan must never write to the host; it only reads /proc. We assert it
	// returns a report with checks and never panics.
	rep := Scan(Options{ExpectEnvironmentIsolation: true, ExpectNetworkBlocked: true})
	if len(rep.Checks) == 0 {
		t.Fatal("scan produced no checks")
	}
	if rep.Note == "" {
		t.Fatal("scan should state its limits")
	}
	// The scanner must not claim absolute security.
	if !strings.Contains(strings.ToLower(rep.Note), "not") {
		t.Fatalf("scan note should disclaim absolute security: %q", rep.Note)
	}
}

func TestSelfTestOrderAndRendering(t *testing.T) {
	rep := SelfTest(Options{ExpectEnvironmentIsolation: true})
	if len(rep.Checks) != len(selfTestOrder) {
		t.Fatalf("expected %d self-test checks, got %d", len(selfTestOrder), len(rep.Checks))
	}
	for i, name := range selfTestOrder {
		if rep.Checks[i].Name != name {
			t.Fatalf("check %d = %q want %q", i, rep.Checks[i].Name, name)
		}
	}
	out := RenderSelfTest(rep)
	if !strings.Contains(out, "TEST 01 filesystem isolation") {
		t.Fatalf("self-test rendering wrong:\n%s", out)
	}
}

func TestFixesRequireConfirmation(t *testing.T) {
	rep := Report{Issues: []Issue{{
		Level: Danger, Area: "filesystem isolation", Summary: "Host filesystem is writable.",
		Fix: []string{"Enable mount namespace.", "Remount host tree read-only."},
	}}}
	fixes := Fixes(rep)
	if len(fixes) != 1 {
		t.Fatalf("expected 1 fix, got %d", len(fixes))
	}
	if !fixes[0].RequireConfirmation {
		t.Fatal("fix must require confirmation")
	}
	// Applying without confirmation must be refused.
	if _, err := ApplyFix(fixes[0], false, func(string) error { return nil }); err == nil {
		t.Fatal("unconfirmed destructive fix must be refused")
	}
	applied, err := ApplyFix(fixes[0], true, func(string) error { return nil })
	if err != nil || !applied.Applied {
		t.Fatalf("confirmed fix should apply: %v %v", applied, err)
	}
}

func TestReportSummaryAndWorst(t *testing.T) {
	rep := Report{Issues: []Issue{{Level: Warning}, {Level: Notice}}}
	if rep.Worst() != Warning {
		t.Fatalf("worst = %v", rep.Worst())
	}
	if !strings.Contains(rep.IssueSummary(), "1 WARNING") {
		t.Fatalf("summary = %q", rep.IssueSummary())
	}
}

func TestAlertStringStable(t *testing.T) {
	a := Alert{At: time.Date(2026, 1, 2, 12, 31, 2, 0, time.UTC), Level: Critical, Sandbox: "sbx", Event: "isolation verification failed", Action: "process frozen"}
	s := a.String()
	if !strings.Contains(s, "12:31:02") || !strings.Contains(s, "CRITICAL") || !strings.Contains(s, "process frozen") {
		t.Fatalf("alert string = %q", s)
	}
}

func TestLocalizedLevelNames(t *testing.T) {
	i18n.SetDefault(i18n.NewBundle())
	_ = i18n.Use("en-US")
	if Info.Translate() != "INFO" {
		t.Fatalf("level translate = %q", Info.Translate())
	}
}
