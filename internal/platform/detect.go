package platform

import (
	"context"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Detect gathers the full platform report. It is cheap enough to run on every
// start (the probe forks one short-lived helper process).
func Detect() *Capabilities {
	c := &Capabilities{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Elevated: IsElevated(),
	}
	c.Kernel, c.Distribution = osRelease()
	c.InContainer = InContainer()
	features, backend, err := probePlatform()
	if err != nil {
		c.ProbeError = err.Error()
	}
	c.Features = orderFeatures(features)
	c.Backend = backend
	c.Warnings = platformWarnings(c)
	c.Dependencies = CheckDependencies(DefaultDependencies())
	return c
}

// DetectIsolation probes only the isolation backend (used by tests).
func DetectIsolation() []Feature {
	features, _, _ := probePlatform()
	return orderFeatures(features)
}

func orderFeatures(in map[string]Feature) []Feature {
	order := make([]string, 0, len(in))
	for k := range in {
		order = append(order, k)
	}
	sort.Slice(order, func(i, j int) bool {
		if in[order[i]].Level != in[order[j]].Level {
			return in[order[i]].Level < in[order[j]].Level
		}
		return order[i] < order[j]
	})
	out := make([]Feature, 0, len(in))
	for _, k := range order {
		f := in[k]
		f.Key = k
		if f.Label == "" {
			f.Label = FeatureLabels[k]
		}
		if f.Label == "" {
			f.Label = k
		}
		out = append(out, f)
	}
	return out
}

func platformWarnings(c *Capabilities) []string {
	var w []string
	if c.OS != "linux" {
		w = append(w, "This isolation feature is unavailable on this platform. SBT is running in compatibility mode.")
	}
	if c.Elevated {
		w = append(w, "SBT is running with elevated privileges (root). Running sandbox infrastructure as root changes the security model.")
	}
	if c.InContainer {
		w = append(w, "SBT appears to run inside a container; the container runtime may restrict namespace operations.")
	}
	if lvl := c.Feature("network_namespace").Level; lvl == Unavailable && c.OS == "linux" {
		w = append(w, "Network isolation cannot be enforced on this host; SBT reports the network policy as unenforced rather than blocked.")
	}
	if c.ProbeError != "" {
		w = append(w, "Platform probe failed: "+c.ProbeError+". SBT refuses to claim isolation it could not verify.")
	}
	return w
}

// DefaultDependencies is the dependency list checked by `sbt doctor` and before
// starting a sandbox.
func DefaultDependencies() []Dep {
	return []Dep{
		{Name: "bash", Required: true},
		{Name: "sh", Required: true},
		{Name: "git", Required: false},
		{Name: "node", Required: false},
		{Name: "npm", Required: false},
		{Name: "python3", Required: false},
		{Name: "pip3", Required: false},
		{Name: "curl", Required: false},
		{Name: "wget", Required: false},
		{Name: "tar", Required: false},
		{Name: "unzip", Required: false},
		{Name: "podman", Required: false, Note: "optional container runtime; SBT does not require it"},
		{Name: "docker", Required: false, Note: "optional container runtime; SBT does not require it"},
	}
}

// CheckDependencies resolves each dependency on the host and records the
// version when the tool can report one cheaply.
func CheckDependencies(list []Dep) []Dep {
	out := make([]Dep, 0, len(list))
	for _, d := range list {
		path, ok := LookPath(d.Name)
		d.Found = ok
		d.Path = path
		if ok {
			if v := versionOf(path); v != "" {
				d.Version = v
			}
		}
		out = append(out, d)
	}
	return out
}

func versionOf(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if len(line) > 60 {
		line = line[:60]
	}
	return line
}

// MissingDependencies returns the required dependencies that are not present.
func MissingDependencies(list []Dep) []Dep {
	var missing []Dep
	for _, d := range list {
		if d.Required && !d.Found {
			missing = append(missing, d)
		}
	}
	return missing
}
