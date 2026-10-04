package filesystem

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Delete removes a file or directory inside a scope.
func (m *Manager) Delete(scope Scope, rel string) error {
	if err := m.guardWrite(scope); err != nil {
		return err
	}
	clean, err := SafeRel(rel)
	if err != nil {
		return err
	}
	if clean == "." || clean == "" {
		return fmt.Errorf("refusing to delete the scope root")
	}
	abs, err := m.Resolve(scope, clean)
	if err != nil {
		return err
	}
	return os.RemoveAll(abs)
}

// Copy copies a file within a scope.
func (m *Manager) Copy(scope Scope, src, dst string) error {
	if err := m.guardWrite(scope); err != nil {
		return err
	}
	sAbs, err := m.Resolve(scope, src)
	if err != nil {
		return err
	}
	dAbs, err := m.Resolve(scope, dst)
	if err != nil {
		return err
	}
	return copyFile(sAbs, dAbs)
}

// Move renames a path within a scope.
func (m *Manager) Move(scope Scope, src, dst string) error {
	if err := m.guardWrite(scope); err != nil {
		return err
	}
	sAbs, err := m.Resolve(scope, src)
	if err != nil {
		return err
	}
	dAbs, err := m.Resolve(scope, dst)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dAbs), 0o755); err != nil {
		return err
	}
	return os.Rename(sAbs, dAbs)
}

// Search walks a subtree looking for names containing query.
func (m *Manager) Search(scope Scope, rel, query string) ([]Entry, error) {
	abs, err := m.Resolve(scope, rel)
	if err != nil {
		return nil, err
	}
	root := m.roots[scope]
	q := strings.ToLower(query)
	var out []Entry
	_ = filepath.WalkDir(abs, func(p string, d os.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		if !strings.Contains(strings.ToLower(d.Name()), q) {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		relPath, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		out = append(out, Entry{
			Name: d.Name(), Path: filepath.ToSlash(relPath), Dir: d.IsDir(),
			Size: info.Size(), Mode: info.Mode().Perm().String(), Writable: !scope.ReadOnly(),
		})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// CompareSnapshots reports which files under a scope differ from a set of
// recorded SHA-256 hashes.
func (m *Manager) CompareSnapshots(scope Scope, recorded map[string]string) (changed []string, missing []string, err error) {
	abs, err := m.Resolve(scope, ".")
	if err != nil {
		return nil, nil, err
	}
	seen := map[string]bool{}
	_ = filepath.WalkDir(abs, func(p string, d os.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(abs, p)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		seen[rel] = true
		want, ok := recorded[rel]
		if !ok {
			changed = append(changed, rel)
			return nil
		}
		if got, herr := HashFile(p); herr == nil && got != want {
			changed = append(changed, rel)
		}
		return nil
	})
	for rel := range recorded {
		if !seen[rel] {
			missing = append(missing, rel)
		}
	}
	sort.Strings(changed)
	sort.Strings(missing)
	return changed, missing, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
