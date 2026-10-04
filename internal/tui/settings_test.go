package tui

import (
	"strings"
	"testing"

	"github.com/wioos28/sbt/internal/config"
)

func snapWithSettings(over map[string]any) *Snapshot {
	snap := baseSnapshot()
	snap.Settings = map[string]any{}
	for k, v := range over {
		snap.Settings[k] = v
	}
	return &snap
}

// The destroy dialog must be unreachable without the exact phrase. This is the
// test that matters most in this file: everything else is convenience.
func TestDestroyNeedsTheExactPhrase(t *testing.T) {
	snap := baseSnapshot()
	sp := &snap
	st := newState(120, 40)
	st.askDestroy(sp)
	if st.Confirm.Kind != ConfirmDestroy {
		t.Fatal("askDestroy must open the destroy dialog")
	}
	if st.Confirm.Choice != 1 {
		t.Fatal("the destroy dialog must open on the safe choice")
	}
	if st.Confirm.RequirePhrase != DestroyPhrase {
		t.Fatalf("the dialog must require the phrase, got %q", st.Confirm.RequirePhrase)
	}

	// Enter on a fresh dialog must not destroy anything.
	st.handleKey(Key{Type: KeyEnter}, sp)
	if st.Confirm.Kind == ConfirmNone {
		t.Fatal("enter with an empty phrase must not close the destroy dialog")
	}

	// A near-miss phrase must also fail.
	for _, r := range DestroyPhrase[:len(DestroyPhrase)-1] {
		st.handleKey(Key{Type: KeyRune, Rune: r}, sp)
	}
	if st.confirmPhraseMatches() {
		t.Fatal("a truncated phrase must not unlock destroy")
	}
	st.handleKey(Key{Type: KeyEnter}, sp)
	if st.Confirm.Kind == ConfirmNone {
		t.Fatal("enter with a wrong phrase must not destroy the sandbox")
	}

	// The full phrase unlocks it, and only then does enter do the work.
	for _, r := range DestroyPhrase[len(DestroyPhrase)-1:] {
		st.handleKey(Key{Type: KeyRune, Rune: r}, sp)
	}
	if !st.confirmPhraseMatches() {
		t.Fatal("the exact phrase must unlock destroy")
	}
	ev := st.handleKey(Key{Type: KeyEnter}, sp)
	if ev.kind != evDestroy {
		t.Fatalf("the exact phrase followed by enter must destroy, got %v", ev.kind)
	}
}

// Escape must always keep the sandbox, even after the phrase was typed.
func TestDestroyEscapeAlwaysKeepsTheSandbox(t *testing.T) {
	snap := baseSnapshot()
	sp := &snap
	st := newState(120, 40)
	st.askDestroy(sp)
	for _, r := range DestroyPhrase {
		st.handleKey(Key{Type: KeyRune, Rune: r}, sp)
	}
	st.handleKey(Key{Type: KeyEsc}, sp)
	if st.Confirm.Kind != ConfirmNone {
		t.Fatal("escape must close the destroy dialog")
	}
	if anyConfirmOpen(st) {
		t.Fatal("escape must leave no confirmation pending")
	}
}

// A slash command must never reach a sandbox: an unknown name is reported and
// dropped, which is what stops a typo from executing something.
func TestUnknownSlashCommandIsNotRun(t *testing.T) {
	snap := baseSnapshot()
	sp := &snap
	st := newState(120, 40)
	for _, name := range []string{"/nope", "/", "/runme", "/destroyy"} {
		ev := st.slashCommand(name, sp)
		if ev.kind == evRun {
			t.Fatalf("%q must not become a run event", name)
		}
	}
}

// The documented slash commands must reach the view they name.
func TestSlashCommandsSwitchViews(t *testing.T) {
	cases := map[string]View{
		"/help":        ViewHelp,
		"/setting":     ViewSettings,
		"/status":      ViewStatus,
		"/files":       ViewFiles,
		"/permissions": ViewPermissions,
		"/security":    ViewPermissions,
		"/session":     ViewTerminal,
		"/export":      ViewExport,
	}
	for cmd, want := range cases {
		snap := baseSnapshot()
		sp := &snap
		st := newState(120, 40)
		st.slashCommand(cmd, sp)
		if st.View != want {
			t.Fatalf("%q must switch to %v, got %v", cmd, want, st.View)
		}
	}
}

// A slash command with a bad argument must be refused rather than stored.
func TestSlashCommandArgumentIsValidated(t *testing.T) {
	snap := baseSnapshot()
	sp := &snap
	st := newState(120, 40)
	ev := st.slashCommand("/palette nonsense", sp)
	if ev.kind != evSetSetting {
		t.Fatal("/palette with an unknown preset must not request a change")
	}
	st2 := newState(120, 40)
	ev2 := st2.slashCommand("/palette ocean", sp)
	if ev2.kind != evSetSetting || ev2.sval != "ocean" {
		t.Fatalf("/palette ocean must request the ocean preset, got %v", ev2.sval)
	}
}

// Every setting the view offers must exist in the schema, or the row would be a
// key the CLI and the config file have never heard of.
func TestEverySettingRowIsInTheSchema(t *testing.T) {
	for _, sec := range settingSections {
		for _, row := range sec.Rows {
			if row.Kind == settingAction {
				continue
			}
			if _, ok := config.SchemaKey(row.Key); !ok {
				t.Fatalf("settings row %q is not declared in the schema", row.Key)
			}
		}
	}
}

// Editing a value refuses everything the schema does not allow.
func TestSettingValidationRefusesBadValues(t *testing.T) {
	bad := []struct{ key, val string }{
		{"anim.typing_intensity", "101"},
		{"anim.typing_intensity", "-1"},
		{"anim.typing_intensity", "fast"},
		{"anim.typing_speed", "ludicrous"},
		{"anim.typing_color", "not-a-colour"},
		{"anim.typing_color", "#12345"},
		{"lan.port", "70000"},
		{"ui.palette", "neon"},
	}
	for _, c := range bad {
		if _, err := validateSetting(c.key, c.val); err == nil {
			t.Fatalf("%s=%q must be refused", c.key, c.val)
		}
	}
	good := []struct {
		key string
		val string
	}{
		{"anim.typing_intensity", "75"},
		{"anim.typing_intensity", "0"},
		{"anim.typing_speed", "slow"},
		{"anim.typing_color", "#00D9FF"},
		{"lan.port", "8080"},
		{"ui.palette", "cyber"},
	}
	for _, c := range good {
		if _, err := validateSetting(c.key, c.val); err != nil {
			t.Fatalf("%s=%q must be accepted: %v", c.key, c.val, err)
		}
	}
	if _, err := validateSetting("not.a.key", "x"); err == nil {
		t.Fatal("an unknown key must be refused")
	}
}

// A range violation must not reach the session: the row keeps editing and the
// message says why.
func TestOutOfRangeValueIsNotStored(t *testing.T) {
	sp := snapWithSettings(map[string]any{"anim.typing_intensity": 70})
	st := newState(120, 40)
	st.SetView(ViewSettings, timeAt())
	for i, sec := range settingSections {
		if sec.Title != "Animations" {
			continue
		}
		st.Settings.Section = i
	}
	for j, row := range settingSections[st.Settings.Section].Rows {
		if row.Key == "anim.typing_intensity" {
			st.Settings.Cursor = j
		}
	}
	st.handleKey(Key{Type: KeyEnter}, sp)
	if !st.Settings.Editing {
		t.Fatal("enter on a number must open the editor")
	}
	for _, r := range "500" {
		st.handleKey(Key{Type: KeyRune, Rune: r}, sp)
	}
	ev := st.handleKey(Key{Type: KeyEnter}, sp)
	if ev.kind == evSetSetting {
		t.Fatal("an out-of-range value must not be sent to the session")
	}
	if !strings.Contains(st.Settings.Message, "not saved") {
		t.Fatalf("the refusal must be reported, got %q", st.Settings.Message)
	}
}

// Left and right must move between sections and stay inside the list.
func TestSettingsSectionNavigation(t *testing.T) {
	snap := baseSnapshot()
	sp := &snap
	st := newState(120, 40)
	st.SetView(ViewSettings, timeAt())
	st.handleKey(Key{Type: KeyRight}, sp)
	if st.Settings.Section != 1 {
		t.Fatalf("right must advance the section, got %d", st.Settings.Section)
	}
	st.handleKey(Key{Type: KeyLeft}, sp)
	if st.Settings.Section != 0 {
		t.Fatalf("left must go back, got %d", st.Settings.Section)
	}
	for i := 0; i < len(settingSections)+3; i++ {
		st.handleKey(Key{Type: KeyLeft}, sp)
	}
	if st.Settings.Section != 0 {
		t.Fatal("the section cursor must clamp at the first section")
	}
}
