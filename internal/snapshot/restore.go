package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Restore copies a snapshot back into a workspace directory after verifying
// its integrity.
func Restore(sandbox, id, workspaceDir string) (Meta, error) {
	if err := validSandbox(sandbox); err != nil {
		return Meta{}, err
	}
	if id == "" || id == "." || id == ".." || len(id) > 64 {
		return Meta{}, fmt.Errorf("invalid snapshot id %q", id)
	}
	src := filepath.Join(baseDir(), sandbox, id)
	data, err := os.ReadFile(filepath.Join(src, "meta.json"))
	if err != nil {
		return Meta{}, fmt.Errorf("snapshot not found: %w", err)
	}
	var meta Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		return Meta{}, fmt.Errorf("snapshot metadata is corrupt: %w", err)
	}
	hash := sha256.New()
	count := 0
	var total int64
	err = filepath.WalkDir(filepath.Join(src, "files"), func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		rel, rerr := filepath.Rel(filepath.Join(src, "files"), p)
		if rerr != nil || rel == "." {
			return rerr
		}
		if d.IsDir() {
			return nil
		}
		i, ierr := d.Info()
		if ierr != nil || !i.Mode().IsRegular() {
			return nil
		}
		f, oerr := os.Open(p)
		if oerr != nil {
			return oerr
		}
		defer f.Close()
		h := sha256.New()
		n, cerr := io.Copy(h, f)
		if cerr != nil {
			return cerr
		}
		var sum [32]byte
		copy(sum[:], h.Sum(nil))
		hash.Write([]byte(rel))
		hash.Write(sum[:])
		count++
		total += n
		return nil
	})
	if err != nil {
		return Meta{}, fmt.Errorf("cannot verify snapshot: %w", err)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != meta.RootHash {
		return Meta{}, fmt.Errorf("snapshot integrity check failed (expected %s, got %s)", meta.RootHash, got)
	}
	_ = count
	_ = total
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		return Meta{}, err
	}
	return meta, copyDir(filepath.Join(src, "files"), workspaceDir)
}

// Delete removes one snapshot. It stays inside the managed snapshots tree.
func Delete(sandbox, id string) error {
	if err := validSandbox(sandbox); err != nil {
		return err
	}
	if id == "" || id == "." || id == ".." || len(id) > 64 {
		return fmt.Errorf("invalid snapshot id %q", id)
	}
	dir := filepath.Clean(filepath.Join(baseDir(), sandbox, id))
	base := filepath.Clean(filepath.Join(baseDir(), sandbox))
	if dir == base || dir == filepath.Clean(baseDir()) {
		return fmt.Errorf("refusing to delete the snapshots root")
	}
	if dir != base && !hasPrefix(dir, base) {
		return fmt.Errorf("refusing to delete outside the snapshots tree")
	}
	return os.RemoveAll(dir)
}

// DeleteAll removes every snapshot of a sandbox.
func DeleteAll(sandbox string) error {
	if err := validSandbox(sandbox); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(baseDir(), sandbox))
}

// Count returns the number of snapshots of a sandbox.
func Count(sandbox string) (int, error) {
	list, err := List(sandbox)
	return len(list), err
}

func hasPrefix(p, base string) bool {
	return len(p) > len(base) && p[:len(base)] == base && p[len(base)] == filepath.Separator
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, p)
		if rerr != nil || rel == "." {
			return rerr
		}
		target := filepath.Join(dst, rel)
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
		return os.WriteFile(target, data, 0o644)
	})
}
