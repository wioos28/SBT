package journal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wioos28/sbt/internal/shared/workspace"
)

// Diff budget. A review view must stay responsive even when a sandbox wrote a
// large file, so both the bytes read and the number of rendered lines are
// bounded and the result says when it was cut short.
const (
	maxDiffBytes = 16 << 20
	maxDiffLines = 1500
)

// FileDiff is the before/after view of one path in one run. Deleted files have
// no After, added files have no Before.
type FileDiff struct {
	Path      string
	Kind      workspace.Kind
	Before    string
	After     string
	Binary    bool
	Truncated bool
	Note      string
}

// Diff returns the before/after content of one path in one run.
func (s *Store) Diff(runID, rel string) (FileDiff, error) {
	clean, ok := safeRel(rel)
	if !ok {
		return FileDiff{}, fmt.Errorf("cannot review %q: not a workspace path", rel)
	}
	out := FileDiff{Path: clean}

	var run workspace.Run
	if stored, err := s.storedRuns(); err == nil {
		for _, sr := range stored {
			if sr.ID == runID {
				run = sr.Run
			}
		}
	}
	found := false
	for _, e := range run.Entries {
		if e.Path == clean {
			out.Kind, found = e.Kind, true
			break
		}
	}
	if !found {
		out.Kind = workspace.Added
		out.Note = "this path is not part of the selected run"
	}

	base := filepath.Join(s.runDir(runID), baseDirName)
	if body, err := readCapped(filepath.Join(base, filepath.FromSlash(clean))); err == nil {
		out.Before, out.Truncated = body.text, out.Truncated || body.truncated
	} else if !errors.Is(err, os.ErrNotExist) {
		out.Note = joinNote(out.Note, "the previous version could not be read: "+err.Error())
	}
	capture := filepath.Join(s.CaptureDir(runID), filepath.FromSlash(clean))
	if body, err := readCapped(capture); err == nil {
		out.After, out.Truncated = body.text, out.Truncated || body.truncated
	} else if !errors.Is(err, os.ErrNotExist) {
		out.Note = joinNote(out.Note, "the captured version could not be read: "+err.Error())
	}
	out.Binary = isBinary(out.Before) || isBinary(out.After)
	return out, nil
}

type cappedBody struct {
	text      string
	truncated bool
}

// readCapped reads a file up to maxDiffBytes and reports whether it was cut.
func readCapped(p string) (cappedBody, error) {
	info, err := os.Stat(p)
	if err != nil {
		return cappedBody{}, err
	}
	if !info.Mode().IsRegular() {
		return cappedBody{}, fmt.Errorf("%s is not a regular file", p)
	}
	f, err := os.Open(p)
	if err != nil {
		return cappedBody{}, err
	}
	defer f.Close()
	buf := make([]byte, 0, 64<<10)
	tmp := make([]byte, 64<<10)
	for int64(len(buf)) < maxDiffBytes {
		n, rerr := f.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if rerr != nil {
			break
		}
	}
	return cappedBody{text: string(buf), truncated: info.Size() > int64(len(buf))}, nil
}

// isBinary reports whether s looks like binary data. Reviewing binary content
// as text is noise, so the UI says "binary" instead.
func isBinary(s string) bool {
	limit := len(s)
	if limit > 8000 {
		limit = 8000
	}
	return strings.IndexByte(s[:limit], 0) >= 0
}

// Unified renders the diff as prefixed context lines (" "... , "-"..., "+"...),
// capped at maxLines. A cut diff says so instead of silently ending.
func (d FileDiff) Unified(maxLines int) []string {
	if maxLines <= 0 {
		maxLines = 200
	}
	switch {
	case d.Binary:
		return []string{d.NoteLine("binary file; SBT does not render binary content as a diff")}
	case d.Before == "" && d.After == "":
		return []string{d.NoteLine("no content available for this path")}
	}
	ops := diffLines(splitLines(d.Before), splitLines(d.After))
	if len(ops) > maxLines {
		hidden := len(ops) - maxLines
		ops = append(ops[:maxLines], lineOp{kind: ' ', text: fmt.Sprintf("... %d more lines", hidden)})
	}
	out := make([]string, 0, len(ops)+1)
	if d.Truncated {
		out = append(out, d.NoteLine("file is larger than the review limit; showing the beginning"))
	}
	if d.Note != "" {
		out = append(out, d.NoteLine(d.Note))
	}
	for _, op := range ops {
		out = append(out, string(op.kind)+" "+op.text)
	}
	return out
}

// NoteLine renders an informational line inside a diff view.
func (d FileDiff) NoteLine(text string) string { return "  " + text }

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > maxDiffLines {
		lines = lines[:maxDiffLines]
	}
	return lines
}

// lineOp is one rendered diff line.
type lineOp struct {
	kind byte // ' ' context, '-' removed, '+' added
	text string
}

// diffLines produces a line diff. The common prefix and suffix are trimmed
// first, which keeps the dynamic programming table small for the common case of
// a file with a few changed lines, and the table itself is bounded: beyond the
// budget the changed block is reported as a replacement instead of guessing.
func diffLines(a, b []string) []lineOp {
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}

	var out []lineOp
	for _, l := range a[:prefix] {
		out = append(out, lineOp{kind: ' ', text: l})
	}
	am, bm := a[prefix:len(a)-suffix], b[prefix:len(b)-suffix]
	const budget = 4_000_000
	switch {
	case len(am) == 0 && len(bm) == 0:
	case len(am)*len(bm) > budget:
		for _, l := range am {
			out = append(out, lineOp{kind: '-', text: l})
		}
		for _, l := range bm {
			out = append(out, lineOp{kind: '+', text: l})
		}
	default:
		out = append(out, lcsOps(am, bm)...)
	}
	for _, l := range a[len(a)-suffix:] {
		out = append(out, lineOp{kind: ' ', text: l})
	}
	return out
}

// lcsOps walks the longest common subsequence table and emits the operations.
func lcsOps(a, b []string) []lineOp {
	n, m := len(a), len(b)
	table := make([]int32, (n+1)*(m+1))
	at := func(i, j int) int32 { return table[i*(m+1)+j] }
	set := func(i, j int, v int32) { table[i*(m+1)+j] = v }
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				set(i, j, at(i+1, j+1)+1)
				continue
			}
			if at(i+1, j) >= at(i, j+1) {
				set(i, j, at(i+1, j))
			} else {
				set(i, j, at(i, j+1))
			}
		}
	}
	var out []lineOp
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out = append(out, lineOp{kind: ' ', text: a[i]})
			i, j = i+1, j+1
		case at(i+1, j) >= at(i, j+1):
			out = append(out, lineOp{kind: '-', text: a[i]})
			i++
		default:
			out = append(out, lineOp{kind: '+', text: b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, lineOp{kind: '-', text: a[i]})
	}
	for ; j < m; j++ {
		out = append(out, lineOp{kind: '+', text: b[j]})
	}
	return out
}
