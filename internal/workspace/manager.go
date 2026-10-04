// Package workspace manages isolated sandbox workspace copies under
// ~/.sbt/workspaces/<name>. A sandbox never mounts the user's project
// read-write; it works on a copy, and the original is only touched again at
// export time.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// InvalidNameError is returned for an unsafe sandbox/workspace name.
type InvalidNameError struct{ Name string }

func (e *InvalidNameError) Error() string { return fmt.Sprintf("invalid sandbox name %q", e.Name) }

// validWorkspaceName rules keep one sandbox name from referring to another
// sandbox's directory.
func validWorkspaceName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, `/\`+string(filepath.Separator)) {
		return false
	}
	return true
}

// Root returns the workspace copy directory for a sandbox, creating it with
// owner-only permissions.
func Root(name string) (string, error) {
	if !validWorkspaceName(name) {
		return "", &InvalidNameError{Name: name}
	}
	dir := filepath.Join(baseDir(), name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func baseDir() string {
	if v := os.Getenv("SBT_HOME"); v != "" {
		return filepath.Join(v, "workspaces")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".sbt", "workspaces")
	}
	return filepath.Join(home, ".sbt", "workspaces")
}

// BaseDir returns the directory that holds every workspace copy.
func BaseDir() string { return baseDir() }

// CreateFrom copies a host project into the isolated workspace for a sandbox.
// The source is only read; only the copy is ever written to by a sandbox.
func CreateFrom(name, src string) (string, error) {
	dst, err := Root(name)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(src)
	if err != nil {
		return "", fmt.Errorf("cannot read source project: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("source %s is not a directory", src)
	}
	if err := copyTree(src, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// ExportChanges copies the workspace back out to a destination SBT created.
func ExportChanges(name, dst string) error {
	src, err := Root(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return copyTree(src, dst)
}

// Discard removes the isolated workspace. It only ever touches the managed
// workspaces directory, never the original project.
func Discard(name string) error {
	if !validWorkspaceName(name) {
		return &InvalidNameError{Name: name}
	}
	dir := filepath.Clean(filepath.Join(baseDir(), name))
	base := filepath.Clean(baseDir())
	if dir == base || !strings.HasPrefix(dir, base+string(filepath.Separator)) {
		return fmt.Errorf("refusing to discard outside managed workspaces")
	}
	return os.RemoveAll(dir)
}

// List returns the sandbox workspace copies SBT manages.
func List() ([]string, error) {
	base := baseDir()
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, p)
		if rerr != nil {
			return rerr
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, ierr := d.Info()
		if ierr != nil || !info.Mode().IsRegular() {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		return os.WriteFile(target, data, 0o644)
	})
}
