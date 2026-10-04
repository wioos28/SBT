package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is one file or directory in a scoped listing.
type Entry struct {
	Name     string `json:"name"`
	Path     string `json:"path"` // scope-relative, slash separated
	Dir      bool   `json:"dir"`
	Size     int64  `json:"size"`
	Mode     string `json:"mode,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Symlink  bool   `json:"symlink,omitempty"`
	Writable bool   `json:"writable"`
}

// Manager performs scoped filesystem operations. It never follows a path outside
// the resolved root of the scope.
type Manager struct {
	roots      map[Scope]string
	hostWrites bool
}

// NewManager builds a manager with the default roots. The SANDBOX root is unset
// until the sandbox manager binds one.
func NewManager() *Manager {
	return &Manager{roots: map[Scope]string{
		Host:      "/",
		Workspace: filepath.Join(sbtHome(), "workspaces"),
		Model:     filepath.Join(sbtHome(), "models"),
	}}
}

func sbtHome() string {
	if v := os.Getenv("SBT_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".sbt")
	}
	return filepath.Join(home, ".sbt")
}

// SetRoot binds a scope to a directory.
func (m *Manager) SetRoot(scope Scope, dir string) {
	if m.roots == nil {
		m.roots = map[Scope]string{}
	}
	m.roots[scope] = dir
}

// Root returns the directory bound to a scope.
func (m *Manager) Root(scope Scope) string { return m.roots[scope] }

// AllowHostWrites enables/disables write access to the HOST scope. It is off by
// default and the file manager never turns it on by itself.
func (m *Manager) AllowHostWrites(v bool) { m.hostWrites = v }

// Resolve returns the absolute path for a scope-relative path, refusing any path
// that would escape the scope root.
func (m *Manager) Resolve(scope Scope, rel string) (string, error) {
	root := m.roots[scope]
	if root == "" {
		return "", fmt.Errorf("scope %s is not bound to a directory", scope)
	}
	clean, err := SafeRel(rel)
	if err != nil {
		return "", err
	}
	if clean == "." {
		return root, nil
	}
	return filepath.Join(root, filepath.FromSlash(clean)), nil
}

// SafeRel validates and normalises a scope-relative path. Absolute paths,
// backslashes and any ".." component are rejected rather than silently fixed.
func SafeRel(rel string) (string, error) {
	if rel == "" || rel == "." {
		return ".", nil
	}
	if strings.ContainsRune(rel, '\\') {
		return "", fmt.Errorf("path %q contains a backslash", rel)
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("path %q must be scope-relative", rel)
	}
	clean := filepath.ToSlash(filepath.Clean(rel))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path %q escapes the scope", rel)
	}
	return clean, nil
}

// List returns the entries directly below rel.
func (m *Manager) List(scope Scope, rel string) ([]Entry, error) {
	abs, err := m.Resolve(scope, rel)
	if err != nil {
		return nil, err
	}
	dirents, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(dirents))
	for _, d := range dirents {
		info, ierr := d.Info()
		if ierr != nil {
			continue
		}
		out = append(out, Entry{
			Name:     d.Name(),
			Path:     joinRel(rel, d.Name()),
			Dir:      d.IsDir(),
			Size:     info.Size(),
			Mode:     info.Mode().Perm().String(),
			Symlink:  info.Mode()&os.ModeSymlink != 0,
			Writable: !scope.ReadOnly(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func joinRel(base, name string) string {
	if base == "" || base == "." {
		return name
	}
	return base + "/" + name
}

// Stat returns metadata (including SHA-256 for regular files) for one path.
func (m *Manager) Stat(scope Scope, rel string) (Entry, error) {
	abs, err := m.Resolve(scope, rel)
	if err != nil {
		return Entry{}, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return Entry{}, err
	}
	e := Entry{
		Name:     info.Name(),
		Path:     rel,
		Dir:      info.IsDir(),
		Size:     info.Size(),
		Mode:     info.Mode().Perm().String(),
		Symlink:  info.Mode()&os.ModeSymlink != 0,
		Writable: !scope.ReadOnly(),
	}
	if info.Mode().IsRegular() {
		if h, herr := HashFile(abs); herr == nil {
			e.SHA256 = h
		}
	}
	return e, nil
}

// Read returns the contents of a regular file.
func (m *Manager) Read(scope Scope, rel string) ([]byte, error) {
	abs, err := m.Resolve(scope, rel)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(abs)
}

// Write writes data to a scope. It refuses a read-only scope.
func (m *Manager) Write(scope Scope, rel string, data []byte, mode os.FileMode) error {
	if err := m.guardWrite(scope); err != nil {
		return err
	}
	abs, err := m.Resolve(scope, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, data, mode)
}

func (m *Manager) guardWrite(scope Scope) error {
	if scope == Host && !m.hostWrites {
		return fmt.Errorf("the HOST scope is read-only; writes to the host are not allowed from the file manager")
	}
	return nil
}
