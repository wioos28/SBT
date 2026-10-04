// Package snapshot stores point-in-time copies of sandbox workspaces under
// ~/.sbt/snapshots/<sandbox>/<id>. Snapshots always belong to a sandbox; they
// are the unit restored, compared and (optionally) destroyed with it.
package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Meta describes one snapshot.
type Meta struct {
	ID        string    `json:"id"`
	Sandbox   string    `json:"sandbox"`
	CreatedAt time.Time `json:"created_at"`
	Files     int       `json:"files"`
	Bytes     int64     `json:"bytes"`
	RootHash  string    `json:"root_sha256"`
	Note      string    `json:"note,omitempty"`
}

// Create copies the workspace directory into a new snapshot.
func Create(sandbox, workspaceDir, note string) (Meta, error) {
	if err := validSandbox(sandbox); err != nil {
		return Meta{}, err
	}
	info, err := os.Stat(workspaceDir)
	if err != nil {
		return Meta{}, fmt.Errorf("cannot read workspace: %w", err)
	}
	if !info.IsDir() {
		return Meta{}, fmt.Errorf("workspace %s is not a directory", workspaceDir)
	}
	id := time.Now().UTC().Format("20060102-150405")
	dst := filepath.Join(baseDir(), sandbox, id)
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return Meta{}, err
	}
	meta := Meta{ID: id, Sandbox: sandbox, CreatedAt: time.Now().UTC(), Note: note}
	hash := sha256.New()
	err = filepath.WalkDir(workspaceDir, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		rel, rerr := filepath.Rel(workspaceDir, p)
		if rerr != nil || rel == "." {
			return rerr
		}
		target := filepath.Join(dst, "files", rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		i, ierr := d.Info()
		if ierr != nil || !i.Mode().IsRegular() {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
		meta.Files++
		meta.Bytes += int64(len(data))
		h := sha256.Sum256(data)
		hash.Write([]byte(rel))
		hash.Write(h[:])
		return nil
	})
	if err != nil {
		_ = os.RemoveAll(dst)
		return Meta{}, err
	}
	meta.RootHash = hex.EncodeToString(hash.Sum(nil))
	data, _ := json.MarshalIndent(meta, "", "  ")
	_ = os.WriteFile(filepath.Join(dst, "meta.json"), append(data, '\n'), 0o600)
	return meta, nil
}

// validSandbox guards the sandbox portion of a snapshot path.
func validSandbox(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid sandbox name %q", name)
	}
	if len(name) > 64 {
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

func baseDir() string {
	if v := os.Getenv("SBT_HOME"); v != "" {
		return filepath.Join(v, "snapshots")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".sbt-wioos28", "snapshots")
	}
	return filepath.Join(home, ".sbt-wioos28", "snapshots")
}

// BaseDir returns the directory that holds every snapshot.
func BaseDir() string { return baseDir() }

// List returns the snapshots of one sandbox, oldest first.
func List(sandbox string) ([]Meta, error) {
	if err := validSandbox(sandbox); err != nil {
		return nil, err
	}
	dir := filepath.Join(baseDir(), sandbox)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Meta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(dir, e.Name(), "meta.json"))
		if rerr != nil {
			continue
		}
		var m Meta
		if jerr := json.Unmarshal(data, &m); jerr != nil {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
