package models

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSource(t *testing.T) {
	cases := map[string]Kind{
		"owner/repo":                 KindHFRepo,
		"hf://owner/repo/model.gguf": KindHFURI,
		"https://example.com/m.gguf": KindURL,
	}
	for raw, want := range cases {
		got, err := ParseSource(raw)
		if err != nil || got.Kind != want {
			t.Fatalf("ParseSource(%q) = %+v,%v want %s", raw, got, err, want)
		}
	}
	for _, bad := range []string{"", "http://example.com/m.gguf", "https://example.com/m.bin", "hf://a/b", "a/b/c", "ok/../../evil"} {
		if _, err := ParseSource(bad); err == nil {
			t.Fatalf("ParseSource(%q) should fail", bad)
		}
	}
}

func TestImportUseRemove(t *testing.T) {
	t.Setenv("SBT_HOME", t.TempDir())
	dir := t.TempDir()
	gguf := filepath.Join(dir, "tiny.gguf")
	if err := os.WriteFile(gguf, []byte("GGUF-fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := Import(gguf)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if info.SHA256 == "" || info.Size == 0 {
		t.Fatalf("bad import record: %+v", info)
	}
	active, err := Use(info.Name)
	if err != nil || !active.Active {
		t.Fatalf("use: %+v %v", active, err)
	}
	if err := Remove(info.Name); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(List()) != 0 {
		t.Fatal("registry should be empty")
	}
}
