package tui

import (
	"strings"
	"testing"

	"github.com/wioos28/sbt/internal/i18n"
)

// A missing translation must show readable English, never a raw key. That is the
// property that lets a language be added without translating everything first.
func TestMissingTranslationFallsBackToEnglish(t *testing.T) {
	if got := tr("definitely.not.a.key", "Readable English"); got != "Readable English" {
		t.Fatalf("a missing key must fall back, got %q", got)
	}
}

// Switching language must change the interface without a restart.
func TestLanguageSwitchTakesEffectImmediately(t *testing.T) {
	t.Cleanup(func() { _ = i18n.Use("en-US") })
	english := tr("settings.section.colors", "Colors")
	if err := i18n.Use("vi-VN"); err != nil {
		t.Fatalf("vi-VN must be a valid locale: %v", err)
	}
	viet := tr("settings.section.colors", "Colors")
	if viet == english {
		t.Fatalf("switching language must change the text, both read %q", english)
	}
	if err := i18n.Use("en-US"); err != nil {
		t.Fatal(err)
	}
	if back := tr("settings.section.colors", "Colors"); back != english {
		t.Fatalf("switching back must restore the text, got %q", back)
	}
}

// Every section, row and prompt label the interface shows must resolve, so no
// view is left showing a key because a translator missed it.
func TestEveryDeclaredKeyResolvesInEveryBuiltinLocale(t *testing.T) {
	t.Cleanup(func() { _ = i18n.Use("en-US") })
	keys := []string{"settings.section.appearance", "settings.section.colors",
		"settings.section.animations", "settings.section.terminal", "settings.section.security",
		"settings.section.language", "settings.section.troll", "settings.section.advanced",
		"permission.required", "permission.allowOnce", "permission.configure", "permission.cancel",
		"destroy.unlocked", "welcome.enter", "boot.anyKey", "perms.least", "files.none"}
	english := map[string]string{}
	for _, k := range keys {
		english[k] = tr(k, "fallback-"+k)
	}
	for _, locale := range i18n.BuiltinLocales() {
		if err := i18n.Use(locale); err != nil {
			t.Fatalf("%s must be selectable: %v", locale, err)
		}
		for _, k := range keys {
			got := tr(k, "fallback-"+k)
			if got == "" || got == k {
				t.Fatalf("%s shows a raw key for %q", locale, k)
			}
			if locale == i18n.FallbackLocale {
				// English is the fallback, so it must render the literal the
				// caller passed rather than depending on a catalogue entry.
				if got != "fallback-"+k {
					t.Fatalf("en-US must use the caller's literal, got %q", got)
				}
				continue
			}
			if v := i18n.T(k); v == "" || v == k {
				t.Fatalf("%s is missing the key %q", locale, k)
			}
		}
	}
}

// The four permission answers must stay in the order the product specifies,
// because the safe one is the last and escape always cancels.
func TestPermissionAnswerOrderIsStable(t *testing.T) {
	t.Cleanup(func() { _ = i18n.Use("en-US") })
	_ = i18n.Use("en-US")
	if len(PermissionLabels) != 4 {
		t.Fatalf("there must be exactly four answers, got %d", len(PermissionLabels))
	}
	if !strings.Contains(PermissionLabels[PermCancel], "Cancel") {
		t.Fatalf("the last answer must be Cancel, got %q", PermissionLabels[PermCancel])
	}
	if PermAllowOnce != 0 || PermConfigure != 2 || PermCancel != 3 {
		t.Fatal("the answer order must match the documented one")
	}
}

// The typed destroy phrase must survive translation: it is a fixed token the
// user must type exactly, not prose.
func TestDestroyPhraseIsNotTranslatable(t *testing.T) {
	t.Cleanup(func() { _ = i18n.Use("en-US") })
	_ = i18n.Use("vi-VN")
	if DestroyPhrase != "DESTROY SBT SANDBOX" {
		t.Fatalf("the destroy phrase must stay literal, got %q", DestroyPhrase)
	}
}
