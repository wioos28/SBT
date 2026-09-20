// Package platform reports what the current operating system really supports.
//
// Every capability in this package is verified at runtime: SBT never assumes
// that a kernel feature exists because a build tag said so. When a feature
// cannot be verified it is reported as Unavailable or Unknown and SBT disables
// the corresponding isolation instead of pretending it is active.
package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Level is the capability level of an isolation feature.
type Level int

// Capability levels.
const (
	// Full means the feature was verified as available and SBT can enforce it.
	Full Level = iota
	// Partial means the feature works with documented restrictions.
	Partial
	// Unavailable means the platform cannot provide the feature.
	Unavailable
	// Unknown means SBT could not verify the feature.
	Unknown
)

// Label renders the level as a word.
func (l Level) Label() string {
	switch l {
	case Full:
		return "Full"
	case Partial:
		return "Limited"
	case Unavailable:
		return "Unavailable"
	default:
		return "Unknown"
	}
}

// Feature describes one isolation primitive and its verification evidence.
type Feature struct {
	// Key is a stable identifier, e.g. "user_namespace".
	Key string `json:"key"`
	// Label is the human readable name.
	Label string `json:"label"`
	// Level is the verified capability level.
	Level Level `json:"level"`
	// Reason explains why the level is what it is (evidence or error text).
	Reason string `json:"reason,omitempty"`
}

// Dep describes a host dependency used by SBT or by commands run inside it.
type Dep struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Found    bool   `json:"found"`
	Path     string `json:"path,omitempty"`
	Version  string `json:"version,omitempty"`
	Note     string `json:"note,omitempty"`
}

// Capabilities is the full platform report used by `sbt doctor`, the startup
// banner and the sandbox backend selection.
type Capabilities struct {
	OS           string    `json:"os"`
	Arch         string    `json:"arch"`
	Kernel       string    `json:"kernel,omitempty"`
	Distribution string    `json:"distribution,omitempty"`
	Backend      string    `json:"backend"`
	Elevated     bool      `json:"elevated"`
	InContainer  bool      `json:"in_container"`
	Features     []Feature `json:"features"`
	Dependencies []Dep     `json:"dependencies,omitempty"`
	Warnings     []string  `json:"warnings,omitempty"`
	// ProbeError is set when the platform probe itself failed.
	ProbeError string `json:"probe_error,omitempty"`
}

// Feature returns the feature with the given key.
func (c *Capabilities) Feature(key string) Feature {
	for _, f := range c.Features {
		if f.Key == key {
			return f
		}
	}
	return Feature{Key: key, Label: key, Level: Unknown, Reason: "not probed"}
}

// OverallLevel aggregates the isolation features into a single level used for
// the platform capability bar.
func (c *Capabilities) OverallLevel() Level {
	if len(c.Features) == 0 {
		return Unknown
	}
	worst := Full
	for _, f := range c.Features {
		if f.Level > worst {
			worst = f.Level
		}
	}
	return worst
}

// FeatureLabels maps feature keys to display labels.
var FeatureLabels = map[string]string{
	"user_namespace":    "User namespace",
	"mount_namespace":   "Mount namespace",
	"pid_namespace":     "Process isolation",
	"network_namespace": "Network isolation",
	"filesystem_jail":   "Filesystem jail",
	"capability_drop":   "Privilege drop",
	"seccomp":           "Syscall filter",
	"resource_limits":   "Resource limits",
	"cgroup_v2":         "cgroup v2 limits",
}

// IsElevated reports whether the process runs as root.
func IsElevated() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	return os.Geteuid() == 0
}

// InContainer performs a best-effort detection of container runtimes.
func InContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return true
	}
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		s := string(data)
		for _, marker := range []string{"docker", "containerd", "kubepods", "podman", "lxc"} {
			if strings.Contains(s, marker) {
				return true
			}
		}
	}
	return os.Getenv("container") != ""
}

func osRelease() (kernel, distro string) {
	if runtime.GOOS != "linux" {
		return "", ""
	}
	if data, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		kernel = strings.TrimSpace(string(data))
	}
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				distro = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
				break
			}
		}
	}
	return kernel, distro
}

// LookPath resolves a binary and returns its absolute path.
func LookPath(name string) (string, bool) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}
	if abs, aerr := filepath.Abs(p); aerr == nil {
		p = abs
	}
	if resolved, rerr := filepath.EvalSymlinks(p); rerr == nil {
		p = resolved
	}
	return p, true
}
