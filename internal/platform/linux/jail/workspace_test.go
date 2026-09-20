//go:build linux

package jail

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/wioos28/sbt/internal/shared/jailspec"
	"github.com/wioos28/sbt/internal/shared/workspace"
)

// writeFile creates a file with content below root.
func writeFile(t *testing.T, root, rel, content string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	// The host umask must not decide what the test sees: set the mode exactly.
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func testRoot(t *testing.T, dir string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

// TestSeedWorkspaceCopiesAndRecordsBaseline checks the copy-in half of the
// handoff: files arrive in the sandbox workspace and the baseline can tell a
// later change apart from an untouched file.
func TestSeedWorkspaceCopiesAndRecordsBaseline(t *testing.T) {
	session := t.TempDir()
	ws := t.TempDir()
	writeFile(t, session, "package.json", `{"name":"demo"}`, 0o644)
	writeFile(t, session, "src/main.js", "console.log(1)\n", 0o644)
	writeFile(t, session, "run.sh", "#!/bin/sh\n", 0o755)

	baseline, n, notes, err := seedWorkspace(os.DirFS(session), ws, captureLimits{})
	if err != nil {
		t.Fatalf("seedWorkspace: %v", err)
	}
	if n != 3 {
		t.Fatalf("seeded %d files, want 3 (notes=%v)", n, notes)
	}
	if len(baseline) != 3 {
		t.Fatalf("baseline has %d entries, want 3", len(baseline))
	}
	if got := baseline["run.sh"].Mode; got != 0o755 {
		t.Errorf("baseline mode for run.sh = %o, want 755", got)
	}
	body, err := os.ReadFile(filepath.Join(ws, "src", "main.js"))
	if err != nil {
		t.Fatalf("seeded file missing: %v", err)
	}
	if string(body) != "console.log(1)\n" {
		t.Errorf("seeded body = %q", body)
	}
}

// TestSeedWorkspaceSkipsUnsafeEntries proves nothing but a safe regular file is
// copied into the sandbox.
func TestSeedWorkspaceSkipsUnsafeEntries(t *testing.T) {
	session := t.TempDir()
	ws := t.TempDir()
	writeFile(t, session, "keep.txt", "ok", 0o644)
	if err := os.Symlink("/etc/passwd", filepath.Join(session, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := syscall.Mkfifo(filepath.Join(session, "pipe"), 0o600); err != nil {
		t.Skipf("fifos unavailable: %v", err)
	}

	baseline, n, notes, err := seedWorkspace(os.DirFS(session), ws, captureLimits{})
	if err != nil {
		t.Fatalf("seedWorkspace: %v", err)
	}
	if n != 1 {
		t.Fatalf("seeded %d files, want 1", n)
	}
	if _, ok := baseline["link"]; ok {
		t.Error("baseline contains a symlink")
	}
	if _, err := os.Lstat(filepath.Join(ws, "link")); !os.IsNotExist(err) {
		t.Error("symlink was copied into the sandbox workspace")
	}
	if len(notes) == 0 {
		t.Error("skipping a symlink must be reported to the user")
	}
}

// TestSeedWorkspaceHonoursLimits checks the size ceiling that keeps a seeded
// session from filling the sandbox tmpfs in one go.
func TestSeedWorkspaceHonoursLimits(t *testing.T) {
	session := t.TempDir()
	ws := t.TempDir()
	writeFile(t, session, "a.bin", strings.Repeat("x", 4096), 0o644)
	writeFile(t, session, "b.bin", strings.Repeat("x", 4096), 0o644)
	writeFile(t, session, "c.bin", strings.Repeat("x", 4096), 0o644)

	_, n, notes, err := seedWorkspace(os.DirFS(session), ws, captureLimits{MaxBytes: 5000})
	if err != nil {
		t.Fatalf("seedWorkspace: %v", err)
	}
	if n >= 3 {
		t.Fatalf("seeded %d files past the byte limit", n)
	}
	if len(notes) == 0 {
		t.Error("hitting the seed limit must be reported")
	}
}

func readManifest(t *testing.T, dir string) (workspace.Run, error) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		return workspace.Run{}, err
	}
	var run workspace.Run
	if err := json.Unmarshal(data, &run); err != nil {
		return workspace.Run{}, err
	}
	return run, nil
}

// TestCaptureWorkspaceClassifiesChanges is the core behaviour test: added,
// modified and deleted files must be recognised against the baseline.
func TestCaptureWorkspaceClassifiesChanges(t *testing.T) {
	ws := t.TempDir()
	session := t.TempDir()
	writeFile(t, session, "package.json", `{"lodash":"4.17.20"}`, 0o644)
	writeFile(t, session, "keep.txt", "same", 0o644)
	writeFile(t, session, "gone.txt", "delete me", 0o644)
	base, _, _, err := seedWorkspace(os.DirFS(session), ws, captureLimits{})
	if err != nil {
		t.Fatal(err)
	}

	// The command runs: it edits one file, adds another, removes a third.
	writeFile(t, ws, "package.json", `{"lodash":"4.17.21"}`, 0o644)
	writeFile(t, ws, "install.sh", "#!/bin/sh\n", 0o755)
	if err := os.Remove(filepath.Join(ws, "gone.txt")); err != nil {
		t.Fatal(err)
	}

	out := t.TempDir()
	outRoot := testRoot(t, out)

	run := workspace.Run{SandboxID: "sbx-test", Command: []string{"npm", "install"}, ExitCode: 0}
	got, err := captureWorkspace(os.DirFS(ws), base, outRoot, run, captureLimits{})
	if err != nil {
		t.Fatalf("captureWorkspace: %v", err)
	}
	counts := got.Counts()
	if counts.Added != 1 || counts.Modified != 1 || counts.Deleted != 1 {
		t.Fatalf("counts = %+v, want 1 added / 1 modified / 1 deleted (entries=%+v)", counts, got.Entries)
	}
	if got.Has("keep.txt") {
		t.Error("unchanged file was reported as a change")
	}
	for _, e := range got.Entries {
		switch e.Path {
		case "package.json":
			if e.Kind != workspace.Modified || e.Previous == "" || e.SHA256 == e.Previous {
				t.Errorf("package.json entry = %+v", e)
			}
		case "install.sh":
			if e.Kind != workspace.Added || !e.Executable() {
				t.Errorf("install.sh entry = %+v", e)
			}
		case "gone.txt":
			if e.Kind != workspace.Deleted || e.Captured {
				t.Errorf("gone.txt entry = %+v", e)
			}
		}
	}
	// The changed content must be readable on the host, and the manifest must
	// describe the same run.
	body, err := os.ReadFile(filepath.Join(out, "package.json"))
	if err != nil {
		t.Fatalf("captured file missing: %v", err)
	}
	if !strings.Contains(string(body), "4.17.21") {
		t.Errorf("captured body = %q", body)
	}
	manifest, err := readManifest(t, out)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SandboxID != "sbx-test" || manifest.Counts().Total() != 3 {
		t.Errorf("manifest = %+v", manifest)
	}
	if _, err := os.Stat(filepath.Join(out, "keep.txt")); !os.IsNotExist(err) {
		t.Error("unchanged file was copied to the host")
	}
	if _, err := os.Stat(filepath.Join(out, "gone.txt")); !os.IsNotExist(err) {
		t.Error("deleted file was copied to the host")
	}
}

// TestCaptureWorkspaceNeverReadsSpecialFiles proves a FIFO cannot hang the
// helper and a symlink cannot be followed out of the workspace.
func TestCaptureWorkspaceNeverReadsSpecialFiles(t *testing.T) {
	ws := t.TempDir()
	out := t.TempDir()
	outRoot := testRoot(t, out)

	secret := writeFile(t, t.TempDir(), "secret.txt", "top secret", 0o600)
	if err := os.Symlink(secret, filepath.Join(ws, "leak")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := syscall.Mkfifo(filepath.Join(ws, "pipe"), 0o600); err != nil {
		t.Skipf("fifos unavailable: %v", err)
	}

	done := make(chan struct{})
	var run workspace.Run
	go func() {
		defer close(done)
		run, _ = captureWorkspace(os.DirFS(ws), nil, outRoot, workspace.Run{}, captureLimits{})
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("capture blocked on a special file")
	}

	if len(run.Entries) != 2 {
		t.Fatalf("entries = %+v, want one entry per skipped special file", run.Entries)
	}
	for _, e := range run.Entries {
		if e.Captured {
			t.Errorf("%s was captured, want a warning only", e.Path)
		}
		if e.Risk() == "" {
			t.Errorf("%s has no warning: %+v", e.Path, e)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "leak")); !os.IsNotExist(err) {
		t.Error("symlink was copied to the host")
	}
}

// TestCaptureWorkspaceMarksTruncation checks that exceeding the capture budget
// is reported instead of silently dropping files.
func TestCaptureWorkspaceMarksTruncation(t *testing.T) {
	ws := t.TempDir()
	out := t.TempDir()
	outRoot := testRoot(t, out)

	writeFile(t, ws, "big.bin", strings.Repeat("a", 8192), 0o644)
	run, err := captureWorkspace(os.DirFS(ws), nil, outRoot, workspace.Run{}, captureLimits{MaxFileBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if !run.Truncated {
		t.Error("run was not marked truncated")
	}
	if len(run.Entries) != 1 || run.Entries[0].Captured || run.Entries[0].Risk() == "" {
		t.Errorf("entries = %+v", run.Entries)
	}
	if _, err := os.Stat(filepath.Join(out, "big.bin")); !os.IsNotExist(err) {
		t.Error("oversized file was copied to the host")
	}
}

// TestCaptureWorkspaceEmptyRunIsClean proves a run that changes nothing stays
// quiet: no entries, no false "added" noise.
func TestCaptureWorkspaceEmptyRunIsClean(t *testing.T) {
	ws := t.TempDir()
	session := t.TempDir()
	writeFile(t, session, "a.txt", "a", 0o644)
	base, _, _, err := seedWorkspace(os.DirFS(session), ws, captureLimits{})
	if err != nil {
		t.Fatal(err)
	}
	outRoot := testRoot(t, t.TempDir())

	run, err := captureWorkspace(os.DirFS(ws), base, outRoot, workspace.Run{}, captureLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if c := run.Counts(); !c.Clean() {
		t.Fatalf("counts = %+v, want a clean run", c)
	}
}

// TestHandoffDisabledKeepsLegacyBehaviour proves a spec without workspace
// directories captures nothing at all: a caller that does not use the review
// flow keeps exactly the behaviour of a sandbox without a workspace handoff.
func TestHandoffDisabledKeepsLegacyBehaviour(t *testing.T) {
	spec := &jailspec.Spec{SandboxID: "sbx-legacy", Command: []string{"true"}}
	h, err := openHandoff(spec)
	if err != nil {
		t.Fatal(err)
	}
	if h != nil {
		t.Fatal("openHandoff returned a handoff for a spec without workspace paths")
	}
	if err := h.seedInto("/workspace"); err != nil {
		t.Fatalf("seedInto on a disabled handoff: %v", err)
	}
	if err := h.captureRun(spec, 0, time.Now()); err != nil {
		t.Fatalf("captureRun on a disabled handoff: %v", err)
	}
	if got := h.Result(); got.SandboxID != "" {
		t.Errorf("disabled handoff produced a manifest: %+v", got)
	}
}
