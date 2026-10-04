// Package ai orchestrates local AI sessions on top of the model registry and
// the runtime abstraction. SBT never implements inference; it selects a
// registered model, resolves a detected runtime and (by default) constrains
// the run with the sandbox policy: no network, invisible host, isolated
// workspace, limited memory/processes and a filtered environment.
package ai

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/wioos28/sbt/internal/models"
	"github.com/wioos28/sbt/internal/runtime"
)

// Session describes one AI run request.
type Session struct {
	Model   string
	Runtime runtime.Name
	Sandbox bool
	Prompt  []string
}

// Defaults fills unset session fields from the registry and runtime detection.
func Defaults(s Session) (Session, error) {
	if s.Model == "" {
		active := ""
		for _, m := range models.List() {
			if m.Active {
				active = m.Name
			}
		}
		if active == "" {
			return Session{}, fmt.Errorf("no model selected; run `sbt model use <name>` first")
		}
		s.Model = active
	}
	if s.Runtime == "" {
		s.Runtime = runtime.LlamaCpp
	}
	if _, err := runtime.Resolve(string(s.Runtime)); err != nil {
		return Session{}, err
	}
	if _, err := models.ParseSource(s.Model); err == nil {
		// A literal source string is fine too.
		return s, nil
	}
	for _, m := range models.List() {
		if m.Name == s.Model {
			return s, nil
		}
	}
	return Session{}, fmt.Errorf("model %q is not registered", s.Model)
}

// Constraints returns the sandbox constraints an AI run is launched with. They
// are the fail-closed defaults and the caller must not weaken them silently.
func Constraints() []string {
	return []string{
		"network=OFF", "host=INVISIBLE", "workspace=ISOLATED",
		"memory=LIMITED", "processes=LIMITED", "environment=FILTERED",
	}
}

// LaunchCommand builds the argv for a runtime. It never shells out: the caller
// passes the result to exec.Command.
func LaunchCommand(s Session, modelPath string) ([]string, error) {
	rt, err := runtime.Resolve(string(s.Runtime))
	if err != nil {
		return nil, err
	}
	if !rt.Available {
		return nil, fmt.Errorf("AI runtime %s is not installed (%s)", rt.Name, rt.Note)
	}
	switch rt.Name {
	case runtime.Ollama:
		return append([]string{rt.Binary, "run", s.Model}, s.Prompt...), nil
	default:
		argv := []string{rt.Binary, "-m", modelPath}
		return append(argv, s.Prompt...), nil
	}
}

// ChatSummary renders what an AI run will do before it starts.
func ChatSummary(s Session) string {
	var b strings.Builder
	fmt.Fprintf(&b, "model:   %s\n", s.Model)
	fmt.Fprintf(&b, "runtime: %s\n", s.Runtime)
	fmt.Fprintf(&b, "sandbox: %v\n", s.Sandbox)
	if s.Sandbox {
		fmt.Fprintf(&b, "policy:  %s\n", strings.Join(Constraints(), " "))
	}
	return b.String()
}

var _ = exec.Command
