package journal

import (
	"fmt"
	"io"
	"os"
	"path"
	"sort"

	"github.com/wioos28/sbt/internal/shared/workspace"
)

// ExportOptions controls an export.
type ExportOptions struct {
	// Destination is the host directory that receives the files. It must
	// already exist: SBT never creates the tree the user pointed at.
	Destination string
	// Overwrite allows replacing files that already exist there.
	Overwrite bool
}

// ExportReport is what the export confirmation shows once it is done.
type ExportReport struct {
	// Written lists the paths copied to the destination.
	Written []string
	// Skipped lists the paths that were left alone, with the reason appended.
	Skipped []string
	// Dropped lists entries rejected by the path validator.
	Dropped []string
	// Bytes is how much content was written.
	Bytes int64
}

// Export copies the selected workspace paths into opts.Destination. An empty
// selection means "everything in the workspace".
//
// Every write goes through an os.Root opened on the destination, existing files
// are removed before being replaced (so a symbolic link in the destination is
// replaced, never followed), and non-regular sources are refused. Executable
// bits are preserved and nothing is ever executed.
func (s *Store) Export(paths []string, opts ExportOptions) (ExportReport, error) {
	report := ExportReport{}
	if opts.Destination == "" {
		return report, fmt.Errorf("choose a destination directory first")
	}
	dest, err := os.OpenRoot(opts.Destination)
	if err != nil {
		return report, fmt.Errorf("the destination directory %s cannot be used: %w", opts.Destination, err)
	}
	defer dest.Close()
	ws, err := s.workspaceRoot()
	if err != nil {
		return report, err
	}
	defer ws.Close()

	selected := paths
	if len(selected) == 0 {
		files, ferr := s.Files()
		if ferr != nil {
			return report, ferr
		}
		for _, f := range files {
			selected = append(selected, f.Path)
		}
	}

	seen := map[string]bool{}
	for _, p := range selected {
		rel, ok := safeRel(p)
		if !ok {
			report.Dropped = append(report.Dropped, p)
			continue
		}
		if seen[rel] {
			continue
		}
		seen[rel] = true

		in, oerr := ws.Open(rel)
		if oerr != nil {
			report.Skipped = append(report.Skipped, rel+" (not in the session workspace)")
			continue
		}
		info, serr := in.Stat()
		if serr != nil || !info.Mode().IsRegular() {
			_ = in.Close()
			report.Skipped = append(report.Skipped, rel+" (only regular files are exported)")
			continue
		}

		if existing, lerr := dest.Lstat(rel); lerr == nil {
			switch {
			case !opts.Overwrite:
				_ = in.Close()
				report.Skipped = append(report.Skipped, rel+" (already exists at the destination)")
				continue
			case existing.Mode()&os.ModeSymlink != 0:
				// Remove deletes the link itself, never what it points at.
				if rerr := dest.Remove(rel); rerr != nil {
					_ = in.Close()
					report.Skipped = append(report.Skipped, rel+" (a symbolic link is in the way: "+rerr.Error()+")")
					continue
				}
			default:
				if rerr := dest.Remove(rel); rerr != nil {
					_ = in.Close()
					report.Skipped = append(report.Skipped, rel+" (cannot be replaced: "+rerr.Error()+")")
					continue
				}
			}
		}
		if merr := dest.MkdirAll(path.Dir(rel), 0o755); merr != nil {
			_ = in.Close()
			report.Skipped = append(report.Skipped, rel+" (cannot create the directory: "+merr.Error()+")")
			continue
		}
		n, werr := writeExport(dest, rel, in, info.Mode().Perm())
		_ = in.Close()
		if werr != nil {
			report.Skipped = append(report.Skipped, rel+" ("+werr.Error()+")")
			continue
		}
		report.Written = append(report.Written, rel)
		report.Bytes += n
	}
	sort.Strings(report.Written)
	sort.Strings(report.Skipped)
	sort.Strings(report.Dropped)
	return report, nil
}

// writeExport creates the file with O_EXCL so a link that appeared between the
// check and the write can never be followed.
func writeExport(dest *os.Root, rel string, in *os.File, mode os.FileMode) (int64, error) {
	out, err := dest.OpenFile(rel, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return 0, err
	}
	if err := out.Chmod(mode); err != nil {
		_ = out.Close()
		return 0, err
	}
	n, cerr := io.Copy(out, in)
	if serr := out.Close(); cerr == nil {
		cerr = serr
	}
	return n, cerr
}

// ExportedKinds reports which change kinds an export selection contains, so the
// confirmation can warn about executables before anything is written.
func ExportedKinds(files []FileInfo, selected []string) workspace.Counts {
	want := map[string]bool{}
	for _, p := range selected {
		if rel, ok := safeRel(p); ok {
			want[rel] = true
		}
	}
	var c workspace.Counts
	for _, f := range files {
		if len(want) > 0 && !want[f.Path] {
			continue
		}
		switch f.Kind {
		case workspace.Added:
			c.Added++
		case workspace.Modified:
			c.Modified++
		case workspace.Deleted:
			c.Deleted++
		}
		if f.Executable {
			c.Warnings++
		}
	}
	return c
}
