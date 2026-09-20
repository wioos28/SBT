package journal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wioos28/sbt/internal/shared/workspace"
)

// seedWorkspaceWith builds a session whose workspace holds the given files.
func seedWorkspaceWith(t *testing.T, files map[string]string, modes map[string]os.FileMode) (*Store, string) {
	t.Helper()
	s, err := OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Remove() })

	runID, err := s.BeginRun([]string{"seed"})
	if err != nil {
		t.Fatal(err)
	}
	var entries []workspace.Entry
	for rel := range files {
		mode := os.FileMode(0o644)
		if m, ok := modes[rel]; ok {
			mode = m
		}
		entries = append(entries, workspace.Entry{
			Path: rel, Kind: workspace.Added, Mode: uint32(mode.Perm()), Captured: true,
		})
	}
	writeCapture(t, s, runID, workspace.Run{Entries: entries}, files, modes)
	finish(t, s, runID, []string{"seed"})
	return s, runID
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestExportWritesSelectionPreservingMode is the happy path: selected files
// arrive with their content and their execute bit intact.
func TestExportWritesSelectionPreservingMode(t *testing.T) {
	s, _ := seedWorkspaceWith(t,
		map[string]string{"package.json": `{"name":"demo"}`, "bin/run.sh": "#!/bin/sh\n", "notes.txt": "skip me"},
		map[string]os.FileMode{"bin/run.sh": 0o755})
	dest := t.TempDir()

	report, err := s.Export([]string{"package.json", "bin/run.sh"}, ExportOptions{Destination: dest})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(report.Written) != 2 || len(report.Skipped) != 0 || len(report.Dropped) != 0 {
		t.Fatalf("report = %+v", report)
	}
	if report.Bytes == 0 {
		t.Error("report.Bytes = 0, want the exported size")
	}
	if got := readFile(t, filepath.Join(dest, "package.json")); got != `{"name":"demo"}` {
		t.Errorf("package.json = %q", got)
	}
	info, err := os.Stat(filepath.Join(dest, "bin", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("run.sh mode = %v, want 0755", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(dest, "notes.txt")); !os.IsNotExist(err) {
		t.Error("a file that was not selected was exported")
	}
}

// TestExportRefusesToOverwriteByDefault proves an existing file at the
// destination is never replaced without an explicit decision.
func TestExportRefusesToOverwriteByDefault(t *testing.T) {
	s, _ := seedWorkspaceWith(t, map[string]string{"package.json": `{"name":"demo"}`}, nil)
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, "package.json"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := s.Export([]string{"package.json"}, ExportOptions{Destination: dest})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Skipped) != 1 || len(again.Written) != 0 {
		t.Fatalf("export = %+v, want the file skipped", again)
	}
	if got := readFile(t, filepath.Join(dest, "package.json")); got != "mine" {
		t.Errorf("the existing file was overwritten without consent: %q", got)
	}
	over, err := s.Export([]string{"package.json"}, ExportOptions{Destination: dest, Overwrite: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(over.Written) != 1 {
		t.Fatalf("overwriting export = %+v", over)
	}
	if got := readFile(t, filepath.Join(dest, "package.json")); got != `{"name":"demo"}` {
		t.Errorf("package.json after overwrite = %q", got)
	}
}

// TestExportEmptySelectionExportsEverything proves the default of the export
// screen: no selection means the whole workspace.
func TestExportEmptySelectionExportsEverything(t *testing.T) {
	s, _ := seedWorkspaceWith(t, map[string]string{"a.txt": "a", "src/b.txt": "b"}, nil)
	dest := t.TempDir()
	report, err := s.Export(nil, ExportOptions{Destination: dest})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Written) != 2 {
		t.Fatalf("report = %+v, want both files", report)
	}
}

// TestExportRejectsBadDestinationAndPaths covers the failure modes the UI must
// show instead of a stack trace.
func TestExportRejectsBadDestinationAndPaths(t *testing.T) {
	s, _ := seedWorkspaceWith(t, map[string]string{"a.txt": "a"}, nil)

	if _, err := s.Export(nil, ExportOptions{}); err == nil {
		t.Error("Export accepted an empty destination")
	}
	if _, err := s.Export(nil, ExportOptions{Destination: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Error("Export accepted a destination that does not exist")
	}
	dest := t.TempDir()
	report, err := s.Export([]string{"../etc/passwd", "/absolute", "a.txt", "ghost.txt"}, ExportOptions{Destination: dest})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Dropped) != 2 {
		t.Errorf("dropped = %v, want the two unsafe paths", report.Dropped)
	}
	if len(report.Skipped) != 1 || !strings.Contains(report.Skipped[0], "ghost.txt") {
		t.Errorf("skipped = %v, want ghost.txt reported", report.Skipped)
	}
	if len(report.Written) != 1 {
		t.Errorf("written = %v, want a.txt only", report.Written)
	}
}

// TestExportDoesNotFollowSymlinks is the security test for the destination: a
// link planted where a file would go must be replaced, never followed.
func TestExportDoesNotFollowSymlinks(t *testing.T) {
	s, _ := seedWorkspaceWith(t, map[string]string{"config.txt": "from sandbox"}, nil)
	dest := t.TempDir()
	outside := filepath.Join(t.TempDir(), "real-secret.txt")
	if err := os.WriteFile(outside, []byte("must not change"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dest, "config.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	report, err := s.Export([]string{"config.txt"}, ExportOptions{Destination: dest, Overwrite: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Written) != 1 {
		t.Fatalf("report = %+v", report)
	}
	if got := readFile(t, outside); got != "must not change" {
		t.Fatalf("the export followed a symlink and rewrote %s: %q", outside, got)
	}
	info, err := os.Lstat(filepath.Join(dest, "config.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("the destination is still a symbolic link")
	}
	if got := readFile(t, filepath.Join(dest, "config.txt")); got != "from sandbox" {
		t.Errorf("exported content = %q", got)
	}
}

// TestExportSkipsDeletedPaths proves a deleted file can never be resurrected by
// an export.
func TestExportSkipsDeletedPaths(t *testing.T) {
	s, _ := seedWorkspaceWith(t, map[string]string{"keep.txt": "keep"}, nil)
	runID, err := s.BeginRun([]string{"rm"})
	if err != nil {
		t.Fatal(err)
	}
	writeCapture(t, s, runID, workspace.Run{
		Entries: []workspace.Entry{{Path: "keep.txt", Kind: workspace.Deleted, Mode: 0o644}},
	}, nil, nil)
	finish(t, s, runID, []string{"rm"})

	dest := t.TempDir()
	report, err := s.Export(nil, ExportOptions{Destination: dest})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Written) != 0 {
		t.Errorf("written = %v, want nothing", report.Written)
	}
	if _, err := os.Stat(filepath.Join(dest, "keep.txt")); !os.IsNotExist(err) {
		t.Error("a deleted file was exported")
	}
}

// TestExportedKindsWarnsAboutExecutables checks the pre-export warning data.
func TestExportedKindsWarnsAboutExecutables(t *testing.T) {
	files := []FileInfo{
		{Path: "a.txt", Kind: workspace.Added},
		{Path: "install.sh", Kind: workspace.Added, Executable: true},
		{Path: "b.txt", Kind: workspace.Modified},
	}
	counts := ExportedKinds(files, nil)
	if counts.Added != 2 || counts.Modified != 1 || counts.Warnings != 1 {
		t.Errorf("counts = %+v", counts)
	}
	only := ExportedKinds(files, []string{"install.sh"})
	if only.Added != 1 || only.Warnings != 1 || only.Modified != 0 {
		t.Errorf("selection counts = %+v", only)
	}
}
