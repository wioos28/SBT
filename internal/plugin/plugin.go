// Package plugin is the future-ready plugin abstraction. A plugin declares the
// capabilities it needs (filesystem, network, process, model, sandbox) and is
// denied everything else. Nothing here executes plugin code yet; this package
// owns the manifest format, validation and the install/remove bookkeeping so
// later work cannot accidentally grant unrestricted privileges.
package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Capability is one privileged area a plugin may request.
type Capability string

// Declared capabilities.
const (
	CapFilesystem Capability = "filesystem"
	CapNetwork    Capability = "network"
	CapProcess    Capability = "process"
	CapModel      Capability = "model"
	CapSandbox    Capability = "sandbox"
)

// Manifest is the declared identity and privilege request of a plugin.
type Manifest struct {
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	Description  string       `json:"description,omitempty"`
	Capabilities []Capability `json:"capabilities"`
}

// Validate rejects manifests with bad names, versions or unknown capabilities.
// The default (no capabilities) is the only unrestricted-safe install.
func (m Manifest) Validate() error {
	if m.Name == "" || len(m.Name) > 64 {
		return fmt.Errorf("invalid plugin name %q", m.Name)
	}
	for _, r := range m.Name {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_'
		if !ok {
			return fmt.Errorf("invalid plugin name %q", m.Name)
		}
	}
	if m.Version == "" {
		return fmt.Errorf("plugin %q has no version", m.Name)
	}
	for _, c := range m.Capabilities {
		switch c {
		case CapFilesystem, CapNetwork, CapProcess, CapModel, CapSandbox:
		default:
			return fmt.Errorf("plugin %q requests unknown capability %q", m.Name, c)
		}
	}
	return nil
}

func root() string {
	if v := os.Getenv("SBT_HOME"); v != "" {
		return filepath.Join(v, "plugins")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".sbt", "plugins")
	}
	return filepath.Join(home, ".sbt", "plugins")
}

func manifestPath(name string) string { return filepath.Join(root(), name, "plugin.json") }

// Install records a plugin manifest from a local directory. Remote fetching,
// code execution and privilege grants are out of scope for this step; the
// manifest is validated and stored so the policy layer can reason about it.
func Install(dir string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		return Manifest{}, fmt.Errorf("cannot read plugin manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("invalid plugin manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	if strings.ContainsAny(m.Name, `/\`) {
		return Manifest{}, fmt.Errorf("invalid plugin name %q", m.Name)
	}
	dst := manifestPath(m.Name)
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return Manifest{}, err
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// Remove unregisters a plugin. It only removes the manifest SBT stored.
func Remove(name string) error {
	if name == "" || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid plugin name %q", name)
	}
	return os.RemoveAll(filepath.Join(root(), name))
}

// List returns every installed plugin manifest.
func List() []Manifest {
	entries, err := os.ReadDir(root())
	if err != nil {
		return nil
	}
	var out []Manifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, rerr := os.ReadFile(manifestPath(e.Name()))
		if rerr != nil {
			continue
		}
		var m Manifest
		if jerr := json.Unmarshal(data, &m); jerr != nil {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
