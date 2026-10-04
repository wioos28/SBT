package runtime

import "testing"

func TestResolveKnownAndUnknown(t *testing.T) {
	if _, err := Resolve("llama.cpp"); err != nil {
		t.Fatalf("resolve llama.cpp: %v", err)
	}
	if _, err := Resolve("ollama"); err != nil {
		t.Fatalf("resolve ollama: %v", err)
	}
	if _, err := Resolve("mystery-engine"); err == nil {
		t.Fatal("unknown runtime should fail")
	}
}

func TestDetectNeverExecutesModels(t *testing.T) {
	for _, r := range All() {
		if r.Available && r.Binary == "" {
			t.Fatalf("available runtime %s has no binary", r.Name)
		}
	}
}
