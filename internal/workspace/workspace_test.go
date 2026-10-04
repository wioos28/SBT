package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateFromCopiesProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SBT_HOME", home)

	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst, err := CreateFrom("proj", src)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "main.go"))
	if err != nil || string(data) != "package main" {
		t.Fatalf("copy missing: %q %v", data, err)
	}
	// The original must be untouched: changes in the copy do not leak back.
	_ = os.WriteFile(filepath.Join(dst, "main.go"), []byte("changed"), 0o644)
	orig, _ := os.ReadFile(filepath.Join(src, "main.go"))
	if string(orig) != "package main" {
		t.Fatal("original project was modified")
	}

	list, err := List()
	if err != nil || len(list) != 1 || list[0] != "proj" {
		t.Fatalf("list: %v %v", list, err)
	}
	if err := ExportChanges("proj", filepath.Join(home, "exported")); err != nil {
		t.Fatalf("export: %v", err)
	}
	if err := Discard("proj"); err != nil {
		t.Fatalf("discard: %v", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("workspace copy should be gone")
	}
}

func TestInvalidWorkspaceNamesRejected(t *testing.T) {
	t.Setenv("SBT_HOME", t.TempDir())
	for _, bad := range []string{"", ".", "..", "../evil", "a/b", `a\b`} {
		if _, err := Root(bad); err == nil {
			t.Fatalf("Root(%q) should fail", bad)
		}
		if err := Discard(bad); err == nil {
			t.Fatalf("Discard(%q) should fail", bad)
		}
	}
}
