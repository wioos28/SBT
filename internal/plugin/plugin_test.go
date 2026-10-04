package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func writeManifest(t *testing.T, dir, name string, caps ...Capability) {
	t.Helper()
	m := Manifest{Name: name, Version: "0.1.0", Capabilities: caps}
	data := []byte(`{"name":"` + m.Name + `","version":"` + m.Version + `","capabilities":` + capsJSON(caps) + `}`)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func capsJSON(caps []Capability) string {
	if len(caps) == 0 {
		return "[]"
	}
	out := "["
	for i, c := range caps {
		if i > 0 {
			out += ","
		}
		out += `"` + string(c) + `"`
	}
	return out + "]"
}

func TestInstallListRemove(t *testing.T) {
	t.Setenv("SBT_HOME", t.TempDir())
	src := t.TempDir()
	writeManifest(t, src, "greeter", CapFilesystem)
	m, err := Install(src)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(m.Capabilities) != 1 {
		t.Fatalf("capabilities lost: %+v", m)
	}
	if len(List()) != 1 {
		t.Fatal("expected one plugin")
	}
	if err := Remove("greeter"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(List()) != 0 {
		t.Fatal("expected no plugins")
	}
}

func TestUnknownCapabilityRejected(t *testing.T) {
	t.Setenv("SBT_HOME", t.TempDir())
	src := t.TempDir()
	writeManifest(t, src, "bad", Capability("root-everything"))
	if _, err := Install(src); err == nil {
		t.Fatal("unknown capability must be rejected")
	}
}

func TestBadNamesRejected(t *testing.T) {
	if err := (Manifest{Name: "../x", Version: "1"}).Validate(); err == nil {
		t.Fatal("path traversal name must be rejected")
	}
	if err := Remove("../x"); err == nil {
		t.Fatal("path traversal remove must be rejected")
	}
}
