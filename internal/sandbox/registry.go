// Package sandbox tracks SBT-managed sandboxes on the host.
//
// Design note: the actual isolation boundary is implemented by the Linux jail
// (internal/platform/linux/jail), driven for one-shot commands by
// internal/shell. This package owns everything around it: the registry of
// managed sandbox resources, per-sandbox preferences (language/theme/AI model),
// workspace copy wiring and safe destruction.
//
// Destruction is allowlist-scoped: it only removes directories SBT created
// under its own state root (~/.sbt). Refusing to delete an unmanaged path is a
// security property, covered by tests.
package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// StateRoot returns the directory that holds every SBT-managed sandbox resource.
func StateRoot() string {
	if v := os.Getenv("SBT_HOME"); v != "" {
		return filepath.Join(v, "sandboxes")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".sbt", "sandboxes")
	}
	return filepath.Join(home, ".sbt", "sandboxes")
}

// Status describes the lifecycle state of a tracked sandbox.
type Status string

// Lifecycle states.
const (
	StatusStopped Status = "STOPPED"
	StatusRunning Status = "RUNNING"
	StatusFrozen  Status = "FROZEN"
)

// Info is the tracked record of one sandbox.
type Info struct {
	Name      string            `json:"name"`
	CreatedAt time.Time         `json:"created_at"`
	Status    Status            `json:"status"`
	HelperPID int               `json:"helper_pid,omitempty"`
	Workspace string            `json:"workspace,omitempty"`
	Network   string            `json:"network"`
	Profile   Profile           `json:"profile"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// validName keeps one sandbox record from addressing another sandbox's state.
func validName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid sandbox name %q", name)
	}
	if len(name) > 64 {
		return fmt.Errorf("invalid sandbox name %q", name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid sandbox name %q", name)
	}
	for _, r := range name {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_'
		if !ok {
			return fmt.Errorf("invalid sandbox name %q", name)
		}
	}
	return nil
}

func recordPath(name string) string { return filepath.Join(StateRoot(), name, "sandbox.json") }

// Register creates the tracked record for a new sandbox.
func Register(name string) (Info, error) {
	if err := validName(name); err != nil {
		return Info{}, err
	}
	dir := filepath.Join(StateRoot(), name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Info{}, err
	}
	info := Info{
		Name: name, CreatedAt: time.Now().UTC(),
		Status: StatusStopped, Network: "OFF", Profile: DefaultProfile(),
	}
	data, _ := json.MarshalIndent(info, "", "  ")
	if err := os.WriteFile(recordPath(name), append(data, '\n'), 0o600); err != nil {
		return Info{}, err
	}
	return info, nil
}

// Load reads a sandbox record.
func Load(name string) (Info, error) {
	if err := validName(name); err != nil {
		return Info{}, err
	}
	data, err := os.ReadFile(recordPath(name))
	if err != nil {
		return Info{}, fmt.Errorf("sandbox %q not found: %w", name, err)
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return Info{}, fmt.Errorf("sandbox record is corrupt: %w", err)
	}
	return info, nil
}

// Save writes a sandbox record back (status transitions, profile edits).
func Save(info Info) error {
	if err := validName(info.Name); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(info, "", "  ")
	return os.WriteFile(recordPath(info.Name), append(data, '\n'), 0o600)
}

// List returns every tracked sandbox, sorted by name.
func List() ([]Info, error) {
	root := StateRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Info
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, lerr := Load(e.Name())
		if lerr != nil {
			continue
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// SandboxDir returns the managed state directory for a sandbox.
func SandboxDir(name string) (string, error) {
	if err := validName(name); err != nil {
		return "", err
	}
	return filepath.Join(StateRoot(), name), nil
}

// Profile is the per-sandbox preference set: language, theme, AI model,
// runtime, network policy and notification behaviour.
type Profile struct {
	Language      string `json:"language"`
	Theme         string `json:"theme"`
	Animations    bool   `json:"animations"`
	Notifications bool   `json:"notifications"`
	FileManager   string `json:"file_manager"`
	AIModel       string `json:"ai_model"`
	AIRuntime     string `json:"ai_runtime"`
	NetworkPolicy string `json:"network_policy"`
}

// DefaultProfile returns the profile defaults for a new sandbox.
func DefaultProfile() Profile {
	return Profile{
		Language: "en-US", Theme: "dark", Animations: true,
		Notifications: true, FileManager: "SANDBOX", AIModel: "qwen3-0.6b",
		AIRuntime: "llama.cpp", NetworkPolicy: "OFF",
	}
}
