// Package runtime abstracts the local inference runtimes SBT drives. SBT does
// not implement an inference engine; it detects and manages existing ones
// (llama.cpp first, Ollama as an option) and launches them with argument
// arrays, never shell strings.
package runtime

import (
	"fmt"
	"os/exec"
	"strings"
)

// Name identifies a runtime.
type Name string

// Supported runtime names.
const (
	LlamaCpp Name = "llama.cpp"
	Ollama   Name = "ollama"
)

// Runtime describes one detected inference runtime.
type Runtime struct {
	Name      Name
	Binary    string
	Version   string
	Available bool
	Note      string
}

// Detect probes the host for a runtime binary without executing any model.
func Detect(name Name) Runtime {
	binary := ""
	for _, cand := range candidates(name) {
		if p, err := exec.LookPath(cand); err == nil {
			binary = p
			break
		}
	}
	if binary == "" {
		return Runtime{Name: name, Note: "not installed"}
	}
	version := probeVersion(binary)
	return Runtime{Name: name, Binary: binary, Version: version, Available: version != "" || binary != ""}
}

func candidates(name Name) []string {
	switch name {
	case Ollama:
		return []string{"ollama"}
	default:
		return []string{"llama-server", "llama.cpp", "llamacpp-server"}
	}
}

func probeVersion(binary string) string {
	out, err := exec.Command(binary, "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if len(line) > 80 {
		line = line[:80]
	}
	return line
}

// All returns the status of every known runtime.
func All() []Runtime { return []Runtime{Detect(LlamaCpp), Detect(Ollama)} }

// Resolve maps a configured runtime name to a detected runtime.
func Resolve(name string) (Runtime, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "llama.cpp", "llamacpp", "llama-cpp":
		return Detect(LlamaCpp), nil
	case "ollama":
		return Detect(Ollama), nil
	default:
		return Runtime{}, fmt.Errorf("unknown AI runtime %q", name)
	}
}
