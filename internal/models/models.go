// Package models manages local model files under ~/.sbt/models.
//
// Supported sources: Hugging Face repositories ("owner/name" or "hf://..."),
// direct model URLs and local GGUF files. SBT never executes a downloaded file;
// a model is data until an explicit runtime (llama.cpp) loads it, and the hash
// of every registered file is verified on use.
package models

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Info is the registry record of one model.
type Info struct {
	Name     string `json:"name"`
	Source   string `json:"source"`
	Path     string `json:"path,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Size     int64  `json:"size,omitempty"`
	Active   bool   `json:"active"`
	Verified string `json:"verified,omitempty"`
}

// Kind identifies how a source string should be interpreted.
type Kind string

// Source kinds.
const (
	KindHFRepo Kind = "hf-repo" // owner/name
	KindHFURI  Kind = "hf-uri"  // hf://owner/name/file
	KindURL    Kind = "url"     // https://...
	KindLocal  Kind = "local"   // existing local path
)

// Source is a parsed model source.
type Source struct {
	Kind Kind
	Raw  string
}

// ParseSource validates and classifies a model source string. It never executes
// anything.
func ParseSource(raw string) (Source, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Source{}, fmt.Errorf("empty model source")
	}
	if strings.HasPrefix(s, "hf://") {
		rest := strings.TrimPrefix(s, "hf://")
		parts := strings.Split(rest, "/")
		if len(parts) < 3 {
			return Source{}, fmt.Errorf("hf:// URI must be hf://owner/repo/file")
		}
		if err := validHFParts(parts[:2]); err != nil {
			return Source{}, err
		}
		return Source{Kind: KindHFURI, Raw: s}, nil
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		u, err := url.Parse(s)
		if err != nil || u.Host == "" {
			return Source{}, fmt.Errorf("invalid model URL %q", s)
		}
		if u.Scheme == "http" {
			return Source{}, fmt.Errorf("refusing insecure http model URL; use https")
		}
		if !strings.HasSuffix(strings.ToLower(u.Path), ".gguf") {
			return Source{}, fmt.Errorf("model URL must point at a .gguf file")
		}
		return Source{Kind: KindURL, Raw: s}, nil
	}
	if st, err := os.Stat(s); err == nil && !st.IsDir() {
		if !strings.HasSuffix(strings.ToLower(s), ".gguf") {
			return Source{}, fmt.Errorf("local model must be a .gguf file")
		}
		return Source{Kind: KindLocal, Raw: s}, nil
	}
	parts := strings.Split(s, "/")
	if len(parts) == 2 {
		if err := validHFParts(parts); err != nil {
			return Source{}, err
		}
		return Source{Kind: KindHFRepo, Raw: s}, nil
	}
	return Source{}, fmt.Errorf("unrecognised model source %q (want owner/repo, hf://..., https://....gguf or a local .gguf)", s)
}

func validHFParts(parts []string) error {
	for _, p := range parts {
		if p == "" || p == "." || p == ".." || len(p) > 96 {
			return fmt.Errorf("invalid Hugging Face component %q", p)
		}
		for _, r := range p {
			ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.'
			if !ok {
				return fmt.Errorf("invalid Hugging Face component %q", p)
			}
		}
	}
	return nil
}

// Root returns the model store directory.
func Root() string {
	if v := os.Getenv("SBT_HOME"); v != "" {
		return filepath.Join(v, "models")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".sbt", "models")
	}
	return filepath.Join(home, ".sbt", "models")
}

func registryPath() string { return filepath.Join(Root(), "registry.json") }

func loadRegistry() map[string]Info {
	out := map[string]Info{}
	data, err := os.ReadFile(registryPath())
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	return out
}

func saveRegistry(reg map[string]Info) error {
	if err := os.MkdirAll(Root(), 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(reg, "", "  ")
	tmp := registryPath() + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, registryPath())
}

// List returns every registered model, sorted by name.
func List() []Info {
	reg := loadRegistry()
	out := make([]Info, 0, len(reg))
	for _, m := range reg {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
