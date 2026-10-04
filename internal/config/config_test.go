package config

import (
	"path/filepath"
	"testing"
)

func TestDefaultsAndSet(t *testing.T) {
	cfg := DefaultSettings()
	if v, ok := cfg.GetBool("ui.show_prefix"); !ok || !v {
		t.Fatalf("ui.show_prefix default should be true, got ok=%v value=%v", ok, v)
	}
	if err := cfg.Set("ui.show_prefix", false); err != nil {
		t.Fatalf("set: %v", err)
	}
	if v, ok := cfg.GetBool("ui.show_prefix"); !ok || v {
		t.Fatalf("ui.show_prefix should be false after set, got ok=%v value=%v", ok, v)
	}
	if err := cfg.Set("general.default_memory", 2048); err != nil {
		t.Fatalf("set: %v", err)
	}
	if n, ok := cfg.GetInt("general.default_memory"); !ok || n != 2048 {
		t.Fatalf("general.default_memory should be 2048, got ok=%v value=%v", ok, n)
	}
}

func TestSetRejectsUnknownKey(t *testing.T) {
	cfg := DefaultSettings()
	if err := cfg.Set("does.not.exist", 1); err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestCoerce(t *testing.T) {
	if v, err := Coerce("lan.port", "9000"); err != nil || v != 9000 {
		t.Fatalf("coerce int: %v %v", v, err)
	}
	if v, err := Coerce("lan.enabled", "true"); err != nil || v != true {
		t.Fatalf("coerce bool: %v %v", v, err)
	}
	if _, err := Coerce("lan.port", "not-a-number"); err == nil {
		t.Fatal("expected error for bad int")
	}
}

func TestSecretRedaction(t *testing.T) {
	if !IsSecret("lan.token") {
		t.Fatal("lan.token should be treated as secret")
	}
	if got := Redact("lan.token", "abc123"); got == "abc123" {
		t.Fatalf("secret value leaked: %q", got)
	}
	if got := Redact("lan.port", 8787); got != "8787" {
		t.Fatalf("non-secret render = %q", got)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SBT_HOME", dir)
	cfg := DefaultSettings()
	if err := cfg.Set("ui.show_prefix", false); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Set("lan.port", 9310); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if Path() != filepath.Join(dir, "config.toml") {
		t.Fatalf("unexpected config path %q", Path())
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if v, _ := got.GetBool("ui.show_prefix"); v {
		t.Fatal("ui.show_prefix did not persist")
	}
	if n, _ := got.GetInt("lan.port"); n != 9310 {
		t.Fatalf("lan.port did not persist: %d", n)
	}
}

func TestSchemaCoverage(t *testing.T) {
	for _, cat := range Categories() {
		if len(ByCategory(cat)) == 0 {
			t.Fatalf("category %s has no settings", cat)
		}
	}
	for _, key := range []string{
		"general.default_memory", "general.default_network", "general.workspace",
		"ai.default_model", "ai.default_runtime", "ai.sandbox",
		"cli.auto_detect", "cli.sandbox",
		"security.require_isolation", "security.read_only_host",
		"security.environment_isolation", "security.confirm_dangerous",
		"ui.show_prefix", "ui.theme", "ui.compact", "ui.animations",
		"lan.enabled", "lan.port", "lan.authentication",
		"notifications.enabled", "notifications.minimum_level",
	} {
		if _, ok := SchemaKey(key); !ok {
			t.Fatalf("missing setting in schema: %s", key)
		}
	}
}
