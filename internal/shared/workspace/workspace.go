// Package workspace describes the files a sandbox run produced.
//
// A sandboxed command writes into a tmpfs mounted at /workspace, so nothing it
// creates can reach the host while it runs. When the command has finished, the
// SBT helper - trusted code, pid 1 of the sandbox - scans that tmpfs, compares
// it with the snapshot the run started from and copies the changed files into a
// host directory the parent created. The sandboxed process never holds a handle
// to that directory, so the isolation model is unchanged: files leave the
// sandbox only after the command that wrote them has exited, and only through
// SBT.
//
// This package holds the wire format shared by the helper and the parent. It
// deliberately contains no filesystem code so both sides can import it without
// pulling in each other's dependencies.
package workspace

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

// Kind is how a path changed during one run.
type Kind string

// Change kinds. They are the three symbols the review UI shows.
const (
	// Added marks a path that did not exist when the run started.
	Added Kind = "added"
	// Modified marks a path whose contents or mode differ from the snapshot.
	Modified Kind = "modified"
	// Deleted marks a snapshot path that is gone after the run.
	Deleted Kind = "deleted"
)

// Symbol returns the single character the review UI uses for this kind.
func (k Kind) Symbol() string {
	switch k {
	case Added:
		return "+"
	case Modified:
		return "~"
	case Deleted:
		return "-"
	default:
		return "?"
	}
}

// Entry is one changed path inside the sandbox workspace.
type Entry struct {
	// Path is relative to the sandbox workspace root, slash separated.
	Path string `json:"path"`
	// Kind is the change this entry records.
	Kind Kind `json:"kind"`
	// Size is the file size after the run (zero for deletions).
	Size int64 `json:"size,omitempty"`
	// Mode holds the unix permission bits after the run.
	Mode uint32 `json:"mode,omitempty"`
	// SHA256 is the content hash after the run, empty when not hashed.
	SHA256 string `json:"sha256,omitempty"`
	// Previous is the content hash from the snapshot, for modified/deleted.
	Previous string `json:"previous,omitempty"`
	// Captured reports whether this entry's content was copied to the host.
	Captured bool `json:"captured,omitempty"`
	// Warning explains why an entry needs review, e.g. "symlink skipped".
	Warning string `json:"warning,omitempty"`
}

// Executable reports whether the entry is a regular file with an execute bit.
// SBT surfaces this because exporting an executable is a decision the user has
// to make knowingly, not by accident.
func (e Entry) Executable() bool {
	if e.Kind == Deleted {
		return false
	}
	return e.Mode&0o111 != 0
}

// FileMode returns the permission bits as an os.FileMode.
func (e Entry) FileMode() os.FileMode { return os.FileMode(e.Mode & 0o777) }

// Risk reports the human readable reason an entry needs a second look. It is
// empty for ordinary files.
func (e Entry) Risk() string {
	if e.Warning != "" {
		return e.Warning
	}
	if e.Executable() {
		return "executable file"
	}
	return ""
}

// ValidPath reports whether p is a safe workspace-relative path. The helper
// refuses to capture anything else and the exporter refuses to write anything
// else, so a malicious sandbox cannot steer either operation outside its roots.
func ValidPath(p string) bool {
	if p == "" || p == "." || strings.ContainsRune(p, 0) {
		return false
	}
	if strings.HasPrefix(p, "/") || strings.Contains(p, `\`) {
		return false
	}
	if path.Clean(p) != p {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

// kindRank orders kinds so review output is stable across runs.
func kindRank(k Kind) int {
	switch k {
	case Added:
		return 0
	case Modified:
		return 1
	default:
		return 2
	}
}

// Run is the record of one sandboxed command.
type Run struct {
	// SandboxID is the sandbox the command ran in.
	SandboxID string `json:"sandbox_id"`
	// Command is the argv that ran inside the sandbox.
	Command []string `json:"command"`
	// ExitCode is the command's exit status (128+signal when signalled).
	ExitCode int `json:"exit_code"`
	// Started and Finished bracket the command execution.
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	// Seeded counts the files copied into the workspace before the run.
	Seeded int `json:"seeded,omitempty"`
	// Entries are the changed paths, sorted by kind then path.
	Entries []Entry `json:"entries,omitempty"`
	// CapturedBytes is how much content was copied out to the host.
	CapturedBytes int64 `json:"captured_bytes,omitempty"`
	// Truncated is set when the capture hit a size or count limit.
	Truncated bool `json:"truncated,omitempty"`
	// Note carries an operational fact the user must see (never a substitute
	// for a missing result).
	Note string `json:"note,omitempty"`
}

// Duration is how long the command ran.
func (r Run) Duration() time.Duration {
	if r.Finished.Before(r.Started) {
		return 0
	}
	return r.Finished.Sub(r.Started)
}

// Counts summarises a run's entries for the review header.
type Counts struct {
	Added    int
	Modified int
	Deleted  int
	Warnings int
}

// Total is the number of changed paths.
func (c Counts) Total() int { return c.Added + c.Modified + c.Deleted }

// Clean reports whether the run changed nothing.
func (c Counts) Clean() bool { return c.Total() == 0 }

// Counts returns the summary of a run.
func (r Run) Counts() Counts {
	var c Counts
	for _, e := range r.Entries {
		switch e.Kind {
		case Added:
			c.Added++
		case Modified:
			c.Modified++
		case Deleted:
			c.Deleted++
		}
		if e.Risk() != "" {
			c.Warnings++
		}
	}
	return c
}

// Summary renders the counts as "124 added · 7 modified · 2 deleted".
func (c Counts) Summary() string {
	parts := []string{}
	if c.Added > 0 {
		parts = append(parts, fmt.Sprintf("%d added", c.Added))
	}
	if c.Modified > 0 {
		parts = append(parts, fmt.Sprintf("%d modified", c.Modified))
	}
	if c.Deleted > 0 {
		parts = append(parts, fmt.Sprintf("%d deleted", c.Deleted))
	}
	if len(parts) == 0 {
		return "no changes"
	}
	return strings.Join(parts, " · ")
}

// Sort orders entries by kind then path so the review UI is stable.
func (r *Run) Sort() {
	sort.SliceStable(r.Entries, func(i, j int) bool {
		a, b := r.Entries[i], r.Entries[j]
		if a.Kind != b.Kind {
			return kindRank(a.Kind) < kindRank(b.Kind)
		}
		return a.Path < b.Path
	})
}

// Has reports whether the run contains an entry for path.
func (r Run) Has(p string) bool {
	for _, e := range r.Entries {
		if e.Path == p {
			return true
		}
	}
	return false
}
