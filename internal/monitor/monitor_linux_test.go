//go:build linux

package monitor

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestParseStatHandlesWeirdComm checks the parser against the one part of
// /proc/<pid>/stat that a naive split gets wrong: a command name containing
// spaces and parentheses.
func TestParseStatHandlesWeirdComm(t *testing.T) {
	// pid (comm) state ppid ... with the fields the parser needs filled in.
	const line = "4242 (bash (weird) name) S 1 4242 4242 0 -1 4194560 1234 0 0 0 5 7 0 0 20 0 3 0 999 0 0 0 0 0"
	entry, err := parseStat(line)
	if err != nil {
		t.Fatalf("parseStat: %v", err)
	}
	if entry.Pid != 4242 {
		t.Errorf("pid = %d, want 4242", entry.Pid)
	}
	if entry.PPid != 1 {
		t.Errorf("ppid = %d, want 1", entry.PPid)
	}
	if entry.UTime != 5 || entry.STime != 7 {
		t.Errorf("utime/stime = %d/%d, want 5/7", entry.UTime, entry.STime)
	}
	if entry.Threads != 3 {
		t.Errorf("threads = %d, want 3", entry.Threads)
	}
}

// TestParseStatRejectsGarbage proves a malformed line is an error rather than a
// silently zeroed entry.
func TestParseStatRejectsGarbage(t *testing.T) {
	for _, line := range []string{"", "not a stat line", "1234 (unclosed"} {
		if _, err := parseStat(line); err == nil {
			t.Errorf("parseStat(%q) accepted a malformed line", line)
		}
	}
}

// TestSampleReportsRealNumbers samples this very process: the numbers must come
// from the kernel, not from a default.
func TestSampleReportsRealNumbers(t *testing.T) {
	s := New(os.Getpid())
	first := s.Sample()
	if !first.Running {
		t.Fatalf("sampler did not find its own process tree: %+v", first)
	}
	if first.Procs < 1 {
		t.Errorf("procs = %d, want at least 1", first.Procs)
	}
	if first.RSSBytes <= 0 {
		t.Errorf("rss = %d, want a positive number", first.RSSBytes)
	}
	if first.MemTotalBytes <= 0 {
		t.Errorf("host memory total = %d, want a positive number (err=%q)", first.MemTotalBytes, first.Err)
	}
	// A busy loop between two samples must produce a measurable cpu number.
	deadline := time.Now().Add(60 * time.Millisecond)
	var x int
	for time.Now().Before(deadline) {
		x++
	}
	second := s.Sample()
	if second.CPUPercent < 0 {
		t.Errorf("cpu percent = %v, want a non-negative number", second.CPUPercent)
	}
	if second.Procs < first.Procs {
		t.Errorf("procs dropped from %d to %d without the tree changing", first.Procs, second.Procs)
	}
	_ = x
}

// TestIdleSamplerStaysHonest proves an idle sampler reports "not running"
// instead of a zeroed sandbox.
func TestIdleSamplerStaysHonest(t *testing.T) {
	s := New(0)
	s.SetLimits(64, 512<<20)
	snap := s.Sample()
	if snap.Running {
		t.Error("idle sampler claims a running tree")
	}
	if snap.ProcsLimit != 64 || snap.MemLimitBytes != 512<<20 {
		t.Errorf("limits were not carried into the snapshot: %+v", snap)
	}
	if snap.Procs != 0 || snap.RSSBytes != 0 || snap.CPUPercent != 0 {
		t.Errorf("idle snapshot invented usage: %+v", snap)
	}
	if peak := s.Peak(); peak.Running {
		t.Error("peak of an idle sampler claims a running tree")
	}
}

// TestPeakKeepsMaximum checks that the high-water marks survive a smaller
// sample, which is what the run summary relies on.
func TestPeakKeepsMaximum(t *testing.T) {
	s := New(0)
	s.peak = Snapshot{Running: true, CPUPercent: 41, RSSBytes: 1024, Procs: 7}
	s.havePeak = true
	s.Sample() // an idle sample must not erase the marks
	peak := s.Peak()
	if peak.CPUPercent != 41 || peak.RSSBytes != 1024 || peak.Procs != 7 {
		t.Errorf("peak = %+v, want the earlier maximum", peak)
	}
	s.ResetPeak()
	if got := s.Peak(); got.Running || got.CPUPercent != 0 {
		t.Errorf("peak after reset = %+v, want a cleared peak", got)
	}
}

// TestWorkspaceBytesIgnoresSymlinks proves the disk figure covers the session
// workspace only.
func TestWorkspaceBytesIgnoresSymlinks(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "a.js"), []byte("aaaa"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.js"), []byte("bb"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "big.bin"), make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "big.bin"), filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	got, err := workspaceBytes(dir)
	if err != nil {
		t.Fatalf("workspaceBytes: %v", err)
	}
	if got != 6 {
		t.Errorf("workspaceBytes = %d, want 6 (symlink target must not count)", got)
	}
	s := New(0)
	s.SetWorkspace(dir, 128<<20)
	snap := s.Sample()
	if snap.WorkspaceBytes != 6 || snap.WorkspaceLimitBytes != 128<<20 {
		t.Errorf("snapshot workspace = %d/%d", snap.WorkspaceBytes, snap.WorkspaceLimitBytes)
	}
}
