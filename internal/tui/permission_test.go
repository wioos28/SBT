package tui

import (
	"strings"
	"testing"
	"time"
)

// A blocked run must ask, not refuse silently and not run quietly.
func TestBlockedRunAsksFirst(t *testing.T) {
	snap := baseSnapshot()
	snap.Sandbox.Unavailable = "no verified backend on this host"
	snap.ProbeFail = "unshare(CLONE_NEWNS) failed"
	snap.Permissions = PermissionReport{Rows: []PermissionRow{{
		Name: "Filesystem", Status: "DENIED", State: StateDanger,
		Reason: "mount namespace unavailable", Feature: "confining the sandbox filesystem",
	}}}
	st := newState(120, 40)
	st.Input = "echo hello"
	ev := st.handleKey(Key{Type: KeyEnter}, &snap)
	if !st.Permission.Open {
		t.Fatal("a run the host cannot isolate must raise the permission prompt")
	}
	if ev.kind == evRun {
		t.Fatal("the prompt must be answered before the command runs")
	}
	if st.Permission.Capability != "Filesystem" {
		t.Fatalf("the prompt must name the measured capability, got %q", st.Permission.Capability)
	}
	if st.Permission.Reason == "" || st.Permission.Risk == "" || st.Permission.Detail == "" {
		t.Fatal("the prompt must carry the capability, the reason, the risk and the evidence")
	}
}

// The prompt opens on Cancel, so a stray enter cannot run anything.
func TestPermissionPromptOpensOnCancel(t *testing.T) {
	snap := baseSnapshot()
	st := newState(120, 40)
	st.openPermission("Root", "running a command", "reduced isolation", "uid 0", []string{"ls"})
	if st.Permission.Choice != PermCancel {
		t.Fatal("the prompt must open on the safe answer")
	}
	ev := st.handleKey(Key{Type: KeyEnter}, &snap)
	if ev.kind == evRun {
		t.Fatal("enter on Cancel must not run anything")
	}
	if st.Permission.Open {
		t.Fatal("the prompt must close after an answer")
	}
}

// Allow must never be a grant: it carries the acknowledgement and the argv, and
// the session still decides whether it can run at all.
func TestAllowIsAnAcknowledgementNotAGrant(t *testing.T) {
	snap := baseSnapshot()
	st := newState(120, 40)
	st.openPermission("Root", "running a command", "reduced isolation", "uid 0", []string{"ls", "-la"})
	st.Permission.Choice = PermAllowOnce
	ev := st.handleKey(Key{Type: KeyEnter}, &snap)
	if ev.kind != evRun {
		t.Fatal("Allow once must reach the session as a run request")
	}
	if strings.Join(ev.argv, " ") != "ls -la" {
		t.Fatalf("the approval must carry the command the user typed, got %v", ev.argv)
	}
	if ev.scope != PermAllowOnce {
		t.Fatalf("the request must record the answer, got %v", ev.scope)
	}
	if st.PermAcknowledged {
		t.Fatal("allow once must not stop SBT asking again next time")
	}
}

// Allow for the session stops the asking. It must not change the cage.
func TestAllowForSessionOnlyStopsTheAsking(t *testing.T) {
	snap := baseSnapshot()
	snap.Sandbox.Unavailable = "no verified backend"
	before := snap.Cage
	st := newState(120, 40)
	st.openPermission("Root", "running a command", "reduced isolation", "uid 0", []string{"ls"})
	st.Permission.Choice = PermAllowSession
	ev := st.handleKey(Key{Type: KeyEnter}, &snap)
	if ev.kind != evRun || ev.scope != PermAllowSession {
		t.Fatal("allow for the session must reach the session as a run request")
	}
	if !st.PermAcknowledged {
		t.Fatal("allow for the session must stop the prompt repeating")
	}
	if snap.Cage != before {
		t.Fatal("answering the prompt must never change the cage verdict")
	}
	st.Input = "echo hi"
	if ev2 := st.handleKey(Key{Type: KeyEnter}, &snap); ev2.kind != evRun {
		t.Fatal("after the session acknowledgement the command must reach the session")
	}
}

// Configure goes to the Permissions centre rather than running anything.
func TestConfigureOpensThePermissionsCentre(t *testing.T) {
	snap := baseSnapshot()
	st := newState(120, 40)
	st.openPermission("Root", "running a command", "reduced isolation", "uid 0", []string{"ls"})
	st.Permission.Choice = PermConfigure
	ev := st.handleKey(Key{Type: KeyEnter}, &snap)
	if ev.kind == evRun {
		t.Fatal("Configure must not run the command")
	}
	if st.View != ViewPermissions {
		t.Fatalf("Configure must open the permissions centre, got %v", st.View)
	}
}

// Escape cancels, and the prompt is the most modal thing on screen.
func TestPermissionPromptIsModalAndCancelsOnEscape(t *testing.T) {
	snap := baseSnapshot()
	st := newState(120, 40)
	st.SetView(ViewFiles, timeAt())
	before := st.View
	st.openPermission("Root", "running a command", "risk", "evidence", []string{"ls"})
	st.handleKey(Key{Type: KeyDown}, &snap)
	if st.View != before {
		t.Fatal("the prompt must swallow navigation keys")
	}
	if st.Permission.Choice != PermCancel {
		t.Fatal("the prompt cursor must clamp on the safe answer, like every other list")
	}
	st.handleKey(Key{Type: KeyEsc}, &snap)
	if st.Permission.Open {
		t.Fatal("escape must close the prompt")
	}
	if st.View != before {
		t.Fatal("escape must not change the view")
	}
}

// A critical alert must stay up; a warning must fade.
func TestCriticalAlertHoldsAndWarningFades(t *testing.T) {
	now := timeAt()
	critical := AlertState{Kind: StateDanger, Title: "CAGE BROKEN", Since: now, Critical: true}
	if !critical.Active(now.Add(time.Hour)) {
		t.Fatal("a critical alert must stay up")
	}
	if critical.Fade(now) != 1 {
		t.Fatal("a critical alert must not fade out")
	}
	warn := AlertState{Kind: StateWarn, Title: "ISOLATION LIMITED", Since: now}
	if !warn.Active(now) {
		t.Fatal("a fresh warning must be visible")
	}
	if warn.Active(now.Add(alertLife + time.Second)) {
		t.Fatal("a warning must expire after its life")
	}
	if warn.Pulse(now, false) != 0 {
		t.Fatal("with motion off there must be no pulse")
	}
}

// The banner must not take the last row of the body.
func TestAlertBannerOnlyTakesARowWhenThereIsRoom(t *testing.T) {
	if l := computeLayout(40, 6, 1); l.HasAlert {
		t.Fatal("a banner must not be drawn when the terminal is too short for it")
	}
	l := computeLayout(80, 24, 1)
	if !l.HasAlert {
		t.Fatal("a banner must be drawn when there is room")
	}
	if l.Body.Y != l.AlertY+1 {
		t.Fatal("the body must start below the banner")
	}
}
