package journal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wioos28/sbt/internal/shared/workspace"
)

// writeCapture writes a helper manifest plus its captured files, exactly like
// the sandbox helper would.
func writeCapture(t *testing.T, s *Store, runID string, run workspace.Run, files map[string]string, modes map[string]os.FileMode) {
	t.Helper()
	dir := s.CaptureDir(runID)
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o600)
		if m, ok := modes[rel]; ok {
			mode = m
		}
		if err := os.WriteFile(p, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, manifestName), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// finish records a run the way the session does.
func finish(t *testing.T, s *Store, runID string, argv []string) workspace.Run {
	t.Helper()
	fallback := workspace.Run{
		Command:  argv,
		ExitCode: 0,
		Started:  time.Now().Add(-time.Second).UTC(),
		Finished: time.Now().UTC(),
	}
	run, err := s.FinishRun(runID, fallback)
	if err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	return run
}

func readWorkspace(t *testing.T, s *Store, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.WorkspaceDir(), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestSessionLifecycle walks the whole review flow: a run adds files, a second
// run modifies one and deletes another, and the workspace, the history and the
// diff must all agree.
func TestSessionLifecycle(t *testing.T) {
	s, err := OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Remove() }()

	run1, err := s.BeginRun([]string{"npm", "init", "-y"})
	if err != nil {
		t.Fatal(err)
	}
	writeCapture(t, s, run1, workspace.Run{
		SandboxID: "sbx-1",
		Entries: []workspace.Entry{
			{Path: "package.json", Kind: workspace.Added, Size: 21, Mode: 0o644, SHA256: "h1", Captured: true},
			{Path: "install.sh", Kind: workspace.Added, Size: 10, Mode: 0o755, SHA256: "h2", Captured: true},
		},
	}, map[string]string{
		"package.json": `{"lodash":"4.17.20"}`,
		"install.sh":   "#!/bin/sh\n",
	}, map[string]os.FileMode{"install.sh": 0o755})
	got1 := finish(t, s, run1, []string{"npm", "init", "-y"})
	if c := got1.Counts(); c.Added != 2 || c.Warnings != 1 {
		t.Fatalf("run 1 counts = %+v, want 2 added and 1 warning", c)
	}

	run2, err := s.BeginRun([]string{"npm", "install"})
	if err != nil {
		t.Fatal(err)
	}
	writeCapture(t, s, run2, workspace.Run{
		SandboxID: "sbx-2",
		Entries: []workspace.Entry{
			{Path: "package.json", Kind: workspace.Modified, Size: 21, Mode: 0o644, SHA256: "h3", Previous: "h1", Captured: true},
			{Path: "install.sh", Kind: workspace.Deleted, Mode: 0o755, Previous: "h2"},
			{Path: "src/app.js", Kind: workspace.Added, Size: 5, Mode: 0o644, SHA256: "h4", Captured: true},
		},
	}, map[string]string{
		"package.json": `{"lodash":"4.17.21"}`,
		"src/app.js":   "hi()\n",
	}, nil)
	got2 := finish(t, s, run2, []string{"npm", "install"})
	if c := got2.Counts(); c.Added != 1 || c.Modified != 1 || c.Deleted != 1 {
		t.Fatalf("run 2 counts = %+v", c)
	}

	files, err := s.Files()
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]FileInfo{}
	for _, f := range files {
		paths[f.Path] = f
	}
	if len(paths) != 2 {
		t.Fatalf("workspace holds %d files, want 2 (%+v)", len(paths), paths)
	}
	if _, gone := paths["install.sh"]; gone {
		t.Error("the deleted file is still in the workspace")
	}
	if f := paths["package.json"]; f.Kind != workspace.Modified || f.Size != 20 {
		t.Errorf("package.json = %+v", f)
	}
	if !strings.Contains(readWorkspace(t, s, "package.json"), "4.17.21") {
		t.Error("the workspace still holds the previous content")
	}
	if info, serr := os.Stat(filepath.Join(s.WorkspaceDir(), "src", "app.js")); serr != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("src/app.js mode = %v (%v)", info, serr)
	}

	runs, err := s.Runs()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("history has %d runs, want 2", len(runs))
	}
	if latest, lerr := s.Latest(); lerr != nil || latest.SandboxID != "sbx-2" {
		t.Errorf("latest = %+v (%v)", latest, lerr)
	}

	diff, err := s.Diff(run2, "package.json")
	if err != nil {
		t.Fatal(err)
	}
	if diff.Before != `{"lodash":"4.17.20"}` || diff.After != `{"lodash":"4.17.21"}` {
		t.Errorf("diff = %+v", diff)
	}
	unified := strings.Join(diff.Unified(50), "\n")
	if !strings.Contains(unified, `- {"lodash":"4.17.20"}`) || !strings.Contains(unified, `+ {"lodash":"4.17.21"}`) {
		t.Errorf("unified diff:\n%s", unified)
	}
	delDiff, err := s.Diff(run2, "install.sh")
	if err != nil {
		t.Fatal(err)
	}
	if delDiff.Kind != workspace.Deleted || delDiff.Before == "" || delDiff.After != "" {
		t.Errorf("deleted diff = %+v", delDiff)
	}

	if err := s.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Dir()); !os.IsNotExist(err) {
		t.Error("the session directory survived Remove")
	}
}

// TestUnreadableManifestIsReportedNotHidden proves a run whose capture failed is
// still recorded and marked, never presented as "no changes".
func TestUnreadableManifestIsReportedNotHidden(t *testing.T) {
	s, err := OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Remove() }()

	runID, err := s.BeginRun([]string{"true"})
	if err != nil {
		t.Fatal(err)
	}
	run := finish(t, s, runID, []string{"true"})
	if !strings.Contains(run.Note, "change report is missing") {
		t.Errorf("note = %q, want an explicit missing-report note", run.Note)
	}
	if c := run.Counts(); !c.Clean() {
		t.Errorf("counts = %+v, want no invented changes", c)
	}
}

// TestManifestPathTraversalIsDropped is the security test: a manifest that tries
// to escape the session must be rejected, and the file it aimed at must be
// untouched.
func TestManifestPathTraversalIsDropped(t *testing.T) {
	session := t.TempDir()
	s, err := OpenAt(filepath.Join(session, "store"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Remove() }()
	outside := filepath.Join(session, "outside.txt")
	if err := os.WriteFile(outside, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}

	runID, err := s.BeginRun([]string{"evil"})
	if err != nil {
		t.Fatal(err)
	}
	writeCapture(t, s, runID, workspace.Run{
		Entries: []workspace.Entry{
			{Path: "../outside.txt", Kind: workspace.Modified, Size: 9, Mode: 0o600, Captured: true},
			{Path: "/etc/passwd", Kind: workspace.Added, Size: 9, Mode: 0o600, Captured: true},
			{Path: "a/../../b", Kind: workspace.Added, Size: 9, Mode: 0o600, Captured: true},
			{Path: `a\b`, Kind: workspace.Added, Size: 9, Mode: 0o600, Captured: true},
			{Path: ".", Kind: workspace.Added, Size: 9, Mode: 0o600, Captured: true},
		},
	}, map[string]string{"../outside.txt": "owned", "a/../../b": "owned"}, nil)
	run := finish(t, s, runID, []string{"evil"})
	if !strings.Contains(run.Note, "dropped unsafe path") {
		t.Errorf("note = %q, want the rejected paths reported", run.Note)
	}
	body, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "untouched" {
		t.Fatalf("a manifest escaped the session: %s = %q", outside, body)
	}
}

// TestDiffShapesCoversAddedBinaryAndTraversal checks the review renderer on the
// shapes a run can produce, and that it refuses paths outside the workspace.
func TestDiffShapesCoversAddedBinaryAndTraversal(t *testing.T) {
	s, err := OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Remove() }()

	runID, err := s.BeginRun([]string{"run"})
	if err != nil {
		t.Fatal(err)
	}
	writeCapture(t, s, runID, workspace.Run{
		Entries: []workspace.Entry{
			{Path: "new.txt", Kind: workspace.Added, Captured: true},
			{Path: "blob.bin", Kind: workspace.Added, Captured: true},
		},
	}, map[string]string{"new.txt": "hello\nworld\n", "blob.bin": "PG\x00\x01\x02"}, nil)
	finish(t, s, runID, []string{"run"})

	added, err := s.Diff(runID, "new.txt")
	if err != nil {
		t.Fatal(err)
	}
	lines := added.Unified(20)
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "+ hello") {
		t.Errorf("added diff = %#v", lines)
	}
	bin, err := s.Diff(runID, "blob.bin")
	if err != nil {
		t.Fatal(err)
	}
	if !bin.Binary {
		t.Error("binary file was not detected")
	}
	if got := strings.Join(bin.Unified(20), "\n"); !strings.Contains(got, "binary file") {
		t.Errorf("binary diff = %q", got)
	}
	if _, err := s.Diff(runID, "../escape"); err == nil {
		t.Error("Diff accepted a path outside the workspace")
	}
	if _, err := s.Diff(runID, "a/../../b"); err == nil {
		t.Error("Diff accepted a traversal path")
	}
}

// TestLineDiffIsMinimal checks the diff algorithm reports a single change
// instead of replacing a whole file.
func TestLineDiffIsMinimal(t *testing.T) {
	a := splitLines("one\ntwo\nthree\nfour\n")
	b := splitLines("one\ntwo\nTHREE\nfour\n")
	ops := diffLines(a, b)
	var removed, added int
	for _, op := range ops {
		switch op.kind {
		case '-':
			removed++
		case '+':
			added++
		}
	}
	if removed != 1 || added != 1 {
		t.Errorf("diff = %+v, want exactly one -/+ pair", ops)
	}
	if len(ops) != 5 {
		t.Errorf("diff has %d ops, want 5 including context", len(ops))
	}
	if got := strings.Join(FileDiff{Before: "a\n", After: "b\n"}.Unified(10), "\n"); !strings.Contains(got, "- a") {
		t.Errorf("unified = %q", got)
	}
}
