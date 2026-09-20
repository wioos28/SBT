// Package journal stores the SBT session workspace: the files a sandbox
// produced, the per-run manifests, the diffs between them and the export /
// discard operations.
//
// The store is the only place where sandbox output lives on the host. It has a
// strict layout so it can be inspected with ordinary tools:
//
//	<session>/workspace/...        current state, and the seed for the next run
//	<session>/runs/<id>/capture/...  what the helper copied out, plus manifest.json
//	<session>/runs/<id>/base/...     previous content of replaced or deleted files
//	<session>/runs/<id>/run.json     the run record as stored
//
// Every path that comes from a manifest or from the UI is validated with
// workspace.ValidPath and resolved through an os.Root, so neither a hostile
// sandbox nor a typo can read or write outside the session or the export
// destination.
package journal

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wioos28/sbt/internal/shared/workspace"
)

// ErrNoRuns is returned when an operation needs a finished run and none exists.
var ErrNoRuns = errors.New("no sandbox run has finished yet")

// Layout constants.
const (
	workspaceDirName = "workspace"
	runsDirName      = "runs"
	captureDirName   = "capture"
	baseDirName      = "base"
	manifestName     = "manifest.json"
	runFileName      = "run.json"
)

// Store is the on-disk session workspace and run history.
type Store struct {
	dir  string
	next int
}

// Open creates a fresh session store under the system temporary directory. The
// directory is created with mode 0700, owned by the user running SBT.
func Open() (*Store, error) {
	dir, err := os.MkdirTemp("", "sbt-session-")
	if err != nil {
		return nil, fmt.Errorf("cannot create the session workspace: %w", err)
	}
	return OpenAt(dir)
}

// OpenAt creates a fresh session store rooted at dir. It is used by tests and
// by callers that want the workspace somewhere specific.
func OpenAt(dir string) (*Store, error) {
	s := &Store{dir: dir}
	for _, sub := range []string{workspaceDirName, runsDirName} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			return nil, fmt.Errorf("cannot create the session workspace: %w", err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(dir, runsDirName))
	if err == nil {
		s.next = len(entries)
	}
	return s, nil
}

// Dir is the session root directory.
func (s *Store) Dir() string { return s.dir }

// WorkspaceDir is the directory the next sandbox run is seeded from and the
// export flow reads from. It is the single source of truth for "what my sandbox
// has right now".
func (s *Store) WorkspaceDir() string { return filepath.Join(s.dir, workspaceDirName) }

// CaptureDir returns the directory a helper must write a run's output into. It
// exists as soon as BeginRun returned.
func (s *Store) CaptureDir(runID string) string {
	return filepath.Join(s.runDir(runID), captureDirName)
}

// Remove deletes the whole session, including the workspace and every diff.
func (s *Store) Remove() error {
	if err := os.RemoveAll(s.dir); err != nil {
		return fmt.Errorf("cannot discard the session workspace: %w", err)
	}
	return nil
}

// Size returns the total bytes held in the session workspace.
func (s *Store) Size() (int64, error) {
	root, err := os.OpenRoot(s.WorkspaceDir())
	if err != nil {
		return 0, err
	}
	defer root.Close()
	var total int64
	err = fsWalkDir(root.FS(), func(p string, info os.FileInfo) {
		total += info.Size()
	})
	return total, err
}

func (s *Store) runDir(runID string) string { return filepath.Join(s.dir, runsDirName, runID) }

// workspaceRoot opens the current workspace through an os.Root so that no path
// derived from a manifest can escape it.
func (s *Store) workspaceRoot() (*os.Root, error) { return os.OpenRoot(s.WorkspaceDir()) }

// BeginRun registers a run that is about to start and returns its id.
func (s *Store) BeginRun(argv []string) (string, error) {
	s.next++
	id := "run-" + pad4(s.next)
	for {
		if _, err := os.Stat(s.runDir(id)); os.IsNotExist(err) {
			break
		}
		s.next++
		id = "run-" + pad4(s.next)
	}
	for _, sub := range []string{captureDirName, baseDirName} {
		if err := os.MkdirAll(filepath.Join(s.runDir(id), sub), 0o700); err != nil {
			return "", fmt.Errorf("cannot prepare the run directory: %w", err)
		}
	}
	// The argv is recorded up front so a run that dies before its manifest can
	// still be shown without pretending it produced anything.
	meta := map[string]any{"command": argv, "started": time.Now().UTC()}
	if err := writeJSON(filepath.Join(s.runDir(id), "begin.json"), meta, 0o600); err != nil {
		return "", err
	}
	return id, nil
}

func pad4(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 4 {
		s = "0" + s
	}
	return s
}

// ReadManifest reads the manifest a helper wrote into the capture directory.
func (s *Store) ReadManifest(runID string) (workspace.Run, error) {
	var run workspace.Run
	data, err := os.ReadFile(filepath.Join(s.CaptureDir(runID), manifestName))
	if err != nil {
		return run, fmt.Errorf("the sandbox did not report its changes: %w", err)
	}
	if err := json.Unmarshal(data, &run); err != nil {
		return run, fmt.Errorf("the sandbox change report is unreadable: %w", err)
	}
	run.Sort()
	return run, nil
}

func writeJSON(path string, v any, mode os.FileMode) error {
	data, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), mode)
}

// safeRel validates a path coming from a manifest or the UI.
//
// Anything that is not already a clean, relative, traversal-free path is
// rejected outright rather than normalised: silently rewriting "../x" into "x"
// would hide a hostile manifest from the user instead of reporting it.
func safeRel(p string) (string, bool) {
	q := strings.TrimSpace(p)
	if !workspace.ValidPath(q) {
		return "", false
	}
	return q, true
}

// fsWalkDir visits every regular file below fsys.
func fsWalkDir(fsys fs.FS, fn func(path string, info os.FileInfo)) error {
	return fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		fn(p, info)
		return nil
	})
}

// FinishRun records the manifest of a finished run, applies it to the session
// workspace and returns the stored run.
//
// fallback carries what the parent knows for certain (argv, exit code, timing);
// the manifest carries what the helper captured. When the manifest is missing
// the run is still recorded, with a note saying so - SBT never presents a
// missing change report as "no changes".
func (s *Store) FinishRun(runID string, fallback workspace.Run) (workspace.Run, error) {
	run := fallback
	manifest, err := s.ReadManifest(runID)
	if err != nil {
		run.Note = joinNote(run.Note, "the sandbox change report is missing; files it wrote were not captured")
		out, aerr := s.apply(runID, run)
		if aerr != nil {
			return out, aerr
		}
		return out, nil
	}
	run.Entries = manifest.Entries
	run.CapturedBytes = manifest.CapturedBytes
	run.Truncated = manifest.Truncated
	run.Seeded = manifest.Seeded
	run.Note = joinNote(run.Note, manifest.Note)
	if run.SandboxID == "" {
		run.SandboxID = manifest.SandboxID
	}
	if run.Started.IsZero() {
		run.Started = manifest.Started
	}
	if run.Finished.IsZero() {
		run.Finished = manifest.Finished
	}
	if len(run.Command) == 0 {
		run.Command = manifest.Command
	}
	run.Sort()
	return s.apply(runID, run)
}

// apply merges a run's captured files into the session workspace, keeping the
// previous content of every replaced or deleted file so Diff can show a real
// before/after.
func (s *Store) apply(runID string, run workspace.Run) (workspace.Run, error) {
	ws, err := s.workspaceRoot()
	if err != nil {
		return run, err
	}
	defer ws.Close()
	var capture *os.Root
	if root, cerr := os.OpenRoot(s.CaptureDir(runID)); cerr == nil {
		capture = root
		defer capture.Close()
	}
	baseRoot, err := os.OpenRoot(filepath.Join(s.runDir(runID), baseDirName))
	if err != nil {
		return run, err
	}
	defer baseRoot.Close()

	var notes []string
	for i := range run.Entries {
		e := run.Entries[i]
		rel, ok := safeRel(e.Path)
		if !ok {
			notes = append(notes, "dropped unsafe path: "+e.Path)
			continue
		}
		switch e.Kind {
		case workspace.Added, workspace.Modified:
			if !e.Captured || capture == nil {
				continue // the manifest already explains why it was not captured
			}
			if err := copyBetween(ws, rel, baseRoot, rel); err != nil && !os.IsNotExist(err) {
				notes = append(notes, "cannot keep the previous version of "+rel+": "+err.Error())
			}
			if err := copyWithin(capture, rel, ws, rel, e.FileMode()); err != nil {
				notes = append(notes, "cannot apply "+rel+": "+err.Error())
			}
		case workspace.Deleted:
			if err := copyBetween(ws, rel, baseRoot, rel); err != nil && !os.IsNotExist(err) {
				notes = append(notes, "cannot keep the deleted version of "+rel+": "+err.Error())
			}
			if err := ws.Remove(rel); err != nil && !os.IsNotExist(err) {
				notes = append(notes, "cannot remove "+rel+": "+err.Error())
			}
		}
	}
	run.Note = joinNote(run.Note, strings.Join(dedupeStrings(notes), "; "))
	run.Sort()
	if err := writeJSON(filepath.Join(s.runDir(runID), runFileName), run, 0o600); err != nil {
		return run, err
	}
	return run, nil
}

// Runs lists finished runs, oldest first.
func (s *Store) Runs() ([]workspace.Run, error) {
	stored, err := s.storedRuns()
	if err != nil {
		return nil, err
	}
	out := make([]workspace.Run, 0, len(stored))
	for _, sr := range stored {
		out = append(out, sr.Run)
	}
	return out, nil
}

// storedRun is a run record together with the id of the directory it lives in.
type storedRun struct {
	ID  string
	Run workspace.Run
}

// storedRuns reads every stored run, oldest first. A run directory whose record
// is missing or unreadable is skipped: an interrupted run must not make the
// whole history unreadable.
func (s *Store) storedRuns() ([]storedRun, error) {
	entries, err := os.ReadDir(filepath.Join(s.dir, runsDirName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	out := make([]storedRun, 0, len(names))
	for _, name := range names {
		data, rerr := os.ReadFile(filepath.Join(s.runDir(name), runFileName))
		if rerr != nil {
			continue
		}
		var run workspace.Run
		if jerr := json.Unmarshal(data, &run); jerr != nil {
			continue
		}
		out = append(out, storedRun{ID: name, Run: run})
	}
	return out, nil
}

// Latest returns the most recent finished run.
func (s *Store) Latest() (workspace.Run, error) {
	stored, err := s.storedRuns()
	if err != nil {
		return workspace.Run{}, err
	}
	if len(stored) == 0 {
		return workspace.Run{}, ErrNoRuns
	}
	return stored[len(stored)-1].Run, nil
}

// FileInfo describes one file in the current session workspace.
type FileInfo struct {
	// Path is relative to the workspace root, slash separated.
	Path string
	// Size is the file size in bytes.
	Size int64
	// Mode holds the unix permission bits.
	Mode uint32
	// SHA256 is the content hash recorded by the run that created the file.
	SHA256 string
	// Executable is true when any execute bit is set.
	Executable bool
	// Kind is the change that produced this file.
	Kind workspace.Kind
	// RunID is the run that last touched the file.
	RunID string
}

// Files lists the current workspace content, newest state only.
func (s *Store) Files() ([]FileInfo, error) {
	root, err := s.workspaceRoot()
	if err != nil {
		return nil, fmt.Errorf("cannot read the session workspace: %w", err)
	}
	defer root.Close()

	// The newest run that touched a path decides how it is labelled.
	kind := map[string]workspace.Kind{}
	runID := map[string]string{}
	hash := map[string]string{}
	stored, _ := s.storedRuns()
	for i := len(stored) - 1; i >= 0; i-- {
		sr := stored[i]
		for _, e := range sr.Run.Entries {
			if e.Kind == workspace.Deleted {
				continue
			}
			if _, seen := kind[e.Path]; seen {
				continue
			}
			kind[e.Path] = e.Kind
			runID[e.Path] = sr.ID
			hash[e.Path] = e.SHA256
		}
	}

	var out []FileInfo
	err = fsWalkDir(root.FS(), func(p string, info os.FileInfo) {
		k, ok := kind[p]
		if !ok {
			k = workspace.Added
		}
		out = append(out, FileInfo{
			Path:       p,
			Size:       info.Size(),
			Mode:       uint32(info.Mode().Perm()),
			SHA256:     hash[p],
			Executable: info.Mode().Perm()&0o111 != 0,
			Kind:       k,
			RunID:      runID[p],
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func joinNote(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "; " + b
	}
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
