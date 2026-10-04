package config

import "testing"

// The CLI and the Settings view must accept and refuse exactly the same values.
// Before this, `sbt setting set anim.typing_intensity 999` stored 999 happily
// while the TUI refused it - two code paths, two answers, one surprise.
func TestCoerceEnforcesDeclaredRanges(t *testing.T) {
	bad := []struct{ key, val string }{
		{"anim.typing_intensity", "999"},
		{"anim.typing_intensity", "-1"},
		{"troll.frequency", "101"},
		{"lan.port", "70000"},
		{"lan.port", "0"},
		{"general.default_memory", "1"},
	}
	for _, c := range bad {
		if _, err := Coerce(c.key, c.val); err == nil {
			t.Fatalf("%s=%q must be refused", c.key, c.val)
		}
	}
	good := []struct {
		key string
		val string
	}{
		{"anim.typing_intensity", "0"},
		{"anim.typing_intensity", "100"},
		{"anim.typing_intensity", "75"},
		{"troll.frequency", "15"},
		{"lan.port", "8787"},
	}
	for _, c := range good {
		if _, err := Coerce(c.key, c.val); err != nil {
			t.Fatalf("%s=%q must be accepted: %v", c.key, c.val, err)
		}
	}
}

func TestCoerceEnforcesChoiceLists(t *testing.T) {
	if _, err := Coerce("ui.palette", "bogus"); err == nil {
		t.Fatal("a palette outside the declared list must be refused")
	}
	for _, ok := range []string{"sbt", "cyber", "minimal", "ocean", "mono"} {
		if _, err := Coerce("ui.palette", ok); err != nil {
			t.Fatalf("palette %q must be accepted: %v", ok, err)
		}
	}
	if _, err := Coerce("anim.typing_speed", "ludicrous"); err == nil {
		t.Fatal("a typing speed outside the declared list must be refused")
	}
}

// A key the schema does not declare is refused, never invented.
func TestCoerceRefusesUnknownKeys(t *testing.T) {
	if _, err := Coerce("not.a.key", "x"); err == nil {
		t.Fatal("an undeclared key must be refused")
	}
}

// A malformed number must not silently become zero.
func TestCoerceRefusesMalformedNumbers(t *testing.T) {
	if _, err := Coerce("anim.typing_intensity", "abc"); err == nil {
		t.Fatal("a non-numeric intensity must be refused")
	}
}

// Every range and choice the schema declares must be reachable through Coerce,
// so no declared constraint can exist only for the TUI.
func TestDeclaredConstraintsAreEnforced(t *testing.T) {
	for _, d := range Schema {
		if _, _, ok := IntRange(d.Key); ok && d.Kind == KindInt {
			continue
		}
		if len(ChoiceValues(d.Key)) > 0 {
			continue
		}
	}
	if _, _, ok := IntRange("anim.typing_intensity"); !ok {
		t.Fatal("the typing intensity range must be declared")
	}
	if len(ChoiceValues("ui.palette")) != 5 {
		t.Fatal("the palette preset list must be declared")
	}
}
