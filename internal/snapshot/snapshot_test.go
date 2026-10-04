package snapshot

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateListRestoreDelete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SBT_HOME", home)

	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "sub", "b.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := Create("coder", ws, "before risky agent")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if meta.Files != 2 || meta.RootHash == "" {
		t.Fatalf("bad meta: %+v", meta)
	}
	list, err := List("coder")
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %+v %v", list, err)
	}

	// Corrupt the workspace, then restore.
	_ = os.WriteFile(filepath.Join(ws, "a.txt"), []byte("changed"), 0o644)
	if _, err := Restore("coder", meta.ID, ws); err != nil {
		t.Fatalf("restore: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(ws, "a.txt"))
	if string(data) != "hello" {
		t.Fatalf("restore did not bring back the original: %q", data)
	}

	if err := Delete("coder", meta.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n, _ := Count("coder"); n != 0 {
		t.Fatalf("count after delete = %d", n)
	}
}

func TestInvalidNamesRejected(t *testing.T) {
	t.Setenv("SBT_HOME", t.TempDir())
	if _, err := Create("../evil", t.TempDir(), ""); err == nil {
		t.Fatal("unsafe sandbox name must be rejected")
	}
	if err := Delete("coder", "../../x"); err == nil {
		t.Fatal("unsafe snapshot id must be rejected")
	}
}

func TestDeleteOnlyInsideManagedTree(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SBT_HOME", home)
	// Sanity: the base exists and a stray delete outside it is refused.
	_ = fmt.Sprint()
	bad := filepath.Join(home, "snapshots", "..", "snapshots-evil")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Delete("..", "x"); err == nil {
		t.Fatal("expected invalid sandbox name")
	}
}
