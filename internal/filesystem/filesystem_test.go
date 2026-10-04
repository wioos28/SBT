package filesystem

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeRelRejectsTraversal(t *testing.T) {
	for _, bad := range []string{"../etc/passwd", "/etc/passwd", "a/../../b", `a\b`} {
		if _, err := SafeRel(bad); err == nil {
			t.Fatalf("SafeRel(%q) should fail", bad)
		}
	}
	good := map[string]string{"a/b": "a/b", "./a": "a", ".": "."}
	for in, want := range good {
		got, err := SafeRel(in)
		if err != nil || got != want {
			t.Fatalf("SafeRel(%q) = %q,%v want %q", in, got, err, want)
		}
	}
}

func TestScopeReadOnly(t *testing.T) {
	if !Host.ReadOnly() {
		t.Fatal("HOST scope must be read-only by default")
	}
	if Sandbox.ReadOnly() || Workspace.ReadOnly() || Model.ReadOnly() {
		t.Fatal("sandbox/workspace/model scopes must be writable")
	}
	if DefaultScope() != Sandbox {
		t.Fatal("file manager default scope must be SANDBOX")
	}
}

func TestHostScopeRefusesWrites(t *testing.T) {
	m := NewManager()
	if err := m.Write(Host, "tmp/xx", []byte("x"), 0o600); err == nil {
		t.Fatal("write to HOST scope should be refused")
	}
}

func TestManagerListWriteReadDelete(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetRoot(Workspace, dir)

	if err := m.Write(Workspace, "sub/file.txt", []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	data, err := m.Read(Workspace, "sub/file.txt")
	if err != nil || string(data) != "hello" {
		t.Fatalf("read: %q %v", data, err)
	}
	entries, err := m.List(Workspace, "sub")
	if err != nil || len(entries) != 1 || entries[0].Name != "file.txt" {
		t.Fatalf("list: %+v %v", entries, err)
	}
	st, err := m.Stat(Workspace, "sub/file.txt")
	if err != nil || st.SHA256 == "" {
		t.Fatalf("stat/hash: %+v %v", st, err)
	}
	if err := m.Delete(Workspace, "sub/file.txt"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub", "file.txt")); !os.IsNotExist(err) {
		t.Fatal("file should be gone")
	}
}

func TestManagerResolveEscapeBlocked(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetRoot(Workspace, dir)
	if _, err := m.Resolve(Workspace, "../../etc/passwd"); err == nil {
		t.Fatal("escaped path must be refused")
	}
}

func TestSHA256Integrity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	h, err := HashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(h, "encrypt") {
		t.Fatal("hash must be hex, not a claim of encryption")
	}
	res, err := VerifyFile(path, h)
	if err != nil || !res.Match {
		t.Fatalf("integrity should match: %+v %v", res, err)
	}
	res2, _ := VerifyFile(path, "deadbeef")
	if res2.Match {
		t.Fatal("wrong expected hash should not match")
	}
	if !strings.Contains(res2.String(), "INTEGRITY FAILED") {
		t.Fatal("integrity failure should be reported")
	}
}

func TestCompareSnapshots(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetRoot(Workspace, dir)
	if err := m.Write(Workspace, "a.txt", []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	recorded := map[string]string{"a.txt": HashBytes([]byte("one")), "b.txt": HashBytes([]byte("two"))}
	changed, missing, err := m.CompareSnapshots(Workspace, recorded)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatalf("nothing should have changed: %v", changed)
	}
	if len(missing) != 1 || missing[0] != "b.txt" {
		t.Fatalf("b.txt should be missing: %v", missing)
	}
}
