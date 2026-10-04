package ai

import (
	"strings"
	"testing"

	"github.com/wioos28/sbt/internal/runtime"
)

func TestConstraintsAreFailClosed(t *testing.T) {
	c := strings.Join(Constraints(), " ")
	for _, want := range []string{"network=OFF", "host=INVISIBLE", "environment=FILTERED"} {
		if !strings.Contains(c, want) {
			t.Fatalf("constraints missing %s: %s", want, c)
		}
	}
}

func TestDefaultsRequiresModel(t *testing.T) {
	t.Setenv("SBT_HOME", t.TempDir())
	if _, err := Defaults(Session{}); err == nil {
		t.Fatal("expected an error with no registered model")
	}
}

func TestLaunchCommandNoShell(t *testing.T) {
	t.Setenv("SBT_HOME", t.TempDir())
	s := Session{Model: "tiny", Runtime: runtime.LlamaCpp, Sandbox: true, Prompt: []string{"hi"}}
	argv, err := LaunchCommand(s, "/models/tiny.gguf")
	if err == nil {
		for _, part := range argv {
			if part == "sh" || part == "-c" {
				t.Fatalf("launch must not route through a shell: %v", argv)
			}
		}
	}
	_ = ChatSummary(s)
}
