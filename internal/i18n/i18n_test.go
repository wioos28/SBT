package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTranslationLookup(t *testing.T) {
	b := NewBundle()
	b.LoadJSON("vi-VN", map[string]string{KeyWarningCritical: "Cảnh báo quan trọng"})
	if err := b.SetLanguage("vi-VN"); err != nil {
		t.Fatal(err)
	}
	if got := b.T(KeyWarningCritical); got != "Cảnh báo quan trọng" {
		t.Fatalf("wrong translation: %q", got)
	}
}

func TestFallbackToEnglish(t *testing.T) {
	b := NewBundle()
	if err := b.SetLanguage("vi-VN"); err != nil {
		t.Fatal(err)
	}
	// vi-VN does not define a made-up key; it must fall back to English.
	b.LoadJSON("en-US", map[string]string{"only.en": "Only English"})
	if got := b.T("only.en"); got != "Only English" {
		t.Fatalf("fallback failed: %q", got)
	}
	if got := b.T("totally.missing"); got != "totally.missing" {
		t.Fatalf("missing key should return the key itself, got %q", got)
	}
}

func TestBuiltinLanguagesPresent(t *testing.T) {
	b := NewBundle()
	for _, want := range []string{"en-US", "vi-VN", "ru-RU", "zh-CN"} {
		if got := b.T(KeyNavDashboard); got == "" {
			t.Fatal("empty translation")
		}
		found := false
		for _, l := range b.Languages() {
			if l == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("builtin locale %s missing", want)
		}
	}
}

func TestImportExportRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SBT_HOME", dir)

	src := filepath.Join(dir, "xx-XX.json")
	if err := os.WriteFile(src, []byte(`{"nav.dashboard":"Board"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	b := NewBundle()
	if err := b.ImportJSON(src); err != nil {
		t.Fatalf("import: %v", err)
	}
	if err := b.SetLanguage("xx-XX"); err != nil {
		t.Fatal(err)
	}
	if got := b.T(KeyNavDashboard); got != "Board" {
		t.Fatalf("custom language not active: %q", got)
	}
	out := filepath.Join(dir, "out.json")
	if err := b.ExportJSON(out); err != nil {
		t.Fatalf("export: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("exported file missing: %v", err)
	}
}

func TestInvalidLocaleRejected(t *testing.T) {
	b := NewBundle()
	if err := b.SetLanguage("not a locale!"); err == nil {
		t.Fatal("expected invalid locale error")
	}
}

func TestGlobalBundle(t *testing.T) {
	SetDefault(NewBundle())
	if err := Use("ru-RU"); err != nil {
		t.Fatal(err)
	}
	if Current() != "ru-RU" {
		t.Fatalf("global current = %q", Current())
	}
	if T(KeyWarningCritical) == "" {
		t.Fatal("global T returned empty")
	}
	// Restore English for other tests.
	_ = Use("en-US")
}
