package adapters

import "testing"

func TestByNameKnownAndUnknown(t *testing.T) {
	for _, n := range []string{"opencode", "cline", "codex"} {
		a, err := ByName(n)
		if err != nil || a.Name() != n {
			t.Fatalf("ByName(%q) = %v,%v", n, a, err)
		}
		argv := a.Command("--help")
		if len(argv) == 0 || argv[len(argv)-1] != "--help" {
			t.Fatalf("Command() should append args: %v", argv)
		}
		// Command must be argv, never a shell string.
		for _, part := range argv {
			if part == "sh" || part == "-c" {
				t.Fatalf("adapter command must not route through a shell: %v", argv)
			}
		}
	}
	if _, err := ByName("evil; rm -rf /"); err == nil {
		t.Fatal("injected adapter name should fail")
	}
}

func TestDetectAllReports(t *testing.T) {
	got := DetectAll()
	if len(got) != 3 {
		t.Fatalf("expected 3 adapters, got %d", len(got))
	}
}
