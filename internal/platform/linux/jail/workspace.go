//go:build linux

package jail

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wioos28/sbt/internal/shared/jailspec"
	"github.com/wioos28/sbt/internal/shared/workspace"
)

// The workspace handoff is the only path by which files leave a sandbox. It has
// two halves, both executed by this helper (trusted SBT code):
//
//   - seedWorkspace copies the session workspace from the host into the tmpfs
//     the sandbox command will write to. The source is only ever read.
//   - captureWorkspace runs after the command has exited, compares the tmpfs
//     with the baseline recorded during seeding and copies the changed files
//     into a host directory whose descriptor this helper opened before it
//     entered the jail.
//
// The sandboxed command never holds a descriptor to either host directory
// (SBT's own file descriptors are opened with O_CLOEXEC and only the control
// pipe is passed to the helper), so the isolation model is unchanged: a command
// can only write into its own tmpfs.

// Default capture limits. They bound how much a single run can copy onto the
// host disk; hitting them marks the run as truncated instead of silently
// dropping content.
const (
	defaultCaptureBytes     = 64 << 20
	defaultCaptureFileBytes = 32 << 20
	defaultCaptureFiles     = 4096
)

// captureLimits bounds one capture so a sandbox cannot fill the host disk.
type captureLimits struct {
	MaxBytes     int64
	MaxFiles     int
	MaxFileBytes int64
}

func (l captureLimits) normalized() captureLimits {
	if l.MaxBytes <= 0 {
		l.MaxBytes = defaultCaptureBytes
	}
	if l.MaxFiles <= 0 {
		l.MaxFiles = defaultCaptureFiles
	}
	if l.MaxFileBytes <= 0 || l.MaxFileBytes > l.MaxBytes {
		l.MaxFileBytes = defaultCaptureFileBytes
	}
	if l.MaxFileBytes > l.MaxBytes {
		l.MaxFileBytes = l.MaxBytes
	}
	return l
}

// fileState is the snapshot of one workspace file: enough to decide whether a
// later run added, modified or left it alone.
type fileState struct {
	SHA256 string
	Size   int64
	Mode   uint32
}

// seedWorkspace copies the session workspace (src, a host directory opened
// through os.Root) into dst, the workspace path the sandbox will see. It
// returns the baseline map used to classify the run's changes, the number of
// files copied and any operational note the user must see.
func seedWorkspace(src fs.FS, dst string, limits captureLimits) (map[string]fileState, int, []string, error) {
	limits = limits.normalized()
	baseline := map[string]fileState{}
	var notes []string
	var seeded int
	var total int64

	err := fs.WalkDir(src, ".", func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return fmt.Errorf("cannot read the session workspace: %w", werr)
		}
		if p == "." {
			return nil
		}
		if !workspace.ValidPath(p) {
			notes = append(notes, "skipped unsafe workspace path: "+p)
			return nil
		}
		target := filepath.Join(dst, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, ierr := d.Info()
		if ierr != nil {
			notes = append(notes, "skipped "+p+": "+ierr.Error())
			return nil
		}
		if !info.Mode().IsRegular() {
			notes = append(notes, "skipped "+p+": not a regular file")
			return nil
		}
		if seeded >= limits.MaxFiles {
			notes = append(notes, fmt.Sprintf("workspace seed stopped at %d files", limits.MaxFiles))
			return fs.SkipAll
		}
		if total+info.Size() > limits.MaxBytes {
			notes = append(notes, fmt.Sprintf("workspace seed stopped at %s", humanBytes(total)))
			return fs.SkipAll
		}
		f, oerr := src.Open(p)
		if oerr != nil {
			notes = append(notes, "skipped "+p+": "+oerr.Error())
			return nil
		}
		defer f.Close()
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		sum, n, cerr := copyHash(f, target, info.Mode().Perm())
		if cerr != nil {
			notes = append(notes, "skipped "+p+": "+cerr.Error())
			return nil
		}
		baseline[p] = fileState{SHA256: sum, Size: n, Mode: uint32(info.Mode().Perm())}
		total += n
		seeded++
		return nil
	})
	if err != nil {
		return baseline, seeded, notes, err
	}
	return baseline, seeded, notes, nil
}

// copyHash streams r into path with mode, returning the content hash and size.
// It is used for both halves of the handoff so a file is hashed exactly once on
// the way in or out.
func copyHash(r io.Reader, path string, mode os.FileMode) (string, int64, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return "", 0, err
	}
	// os.OpenFile applies the process umask, which would silently drop an
	// execute bit and make the restored workspace differ from the session state.
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		return "", 0, err
	}
	h := sha256.New()
	n, cerr := io.Copy(io.MultiWriter(f, h), r)
	serr := f.Close()
	if cerr != nil {
		return "", n, cerr
	}
	if serr != nil {
		return "", n, serr
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// hashFile reads path and returns its content hash.
func hashFile(f fs.File) (string, int64, error) {
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", n, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	v := float64(n)
	for _, u := range units {
		v /= unit
		if v < unit {
			return fmt.Sprintf("%.1f %s", v, u)
		}
	}
	return fmt.Sprintf("%.1f PB", v/unit)
}

// captureWorkspace scans the sandbox workspace after the command exited,
// compares it with the baseline recorded by seedWorkspace and copies every
// added or modified regular file into out (a host directory opened through
// os.Root before the jail was entered). It returns the completed run record and
// writes manifest.json next to the captured content.
//
// Non-regular files are never read: a FIFO would block the helper forever and a
// symlink could point anywhere, so both are recorded as warnings instead.
func captureWorkspace(src fs.FS, baseline map[string]fileState, out *os.Root, run workspace.Run, limits captureLimits) (workspace.Run, error) {
	limits = limits.normalized()
	run.Entries = nil
	run.CapturedBytes = 0

	seen := map[string]bool{}
	var notes []string
	var capturedFiles int

	walkErr := fs.WalkDir(src, ".", func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			// A file that vanished mid-scan is not an error: report and go on.
			notes = append(notes, "cannot read "+p+": "+werr.Error())
			return nil
		}
		if p == "." {
			return nil
		}
		if !workspace.ValidPath(p) {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			notes = append(notes, "cannot stat "+p+": "+ierr.Error())
			return nil
		}
		if d.IsDir() {
			return nil
		}
		seen[p] = true
		if !d.Type().IsRegular() {
			run.Entries = append(run.Entries, workspace.Entry{
				Path:    p,
				Kind:    classify(baseline, p),
				Mode:    uint32(info.Mode().Perm()),
				Warning: "not captured: " + describeMode(info.Mode()) + " (only regular files can be exported)",
			})
			return nil
		}

		state := fileState{SHA256: "", Size: info.Size(), Mode: uint32(info.Mode().Perm())}
		prev, existed := baseline[p]
		sum, sumErr := hashPath(src, p)
		if sumErr != nil {
			run.Entries = append(run.Entries, workspace.Entry{
				Path: p, Kind: workspace.Added, Size: info.Size(),
				Mode: state.Mode, Warning: "not captured: " + sumErr.Error(),
			})
			return nil
		}
		state.SHA256 = sum

		kind := workspace.Added
		if existed {
			if prev.SHA256 == sum && prev.Mode == state.Mode {
				return nil // unchanged
			}
			kind = workspace.Modified
		}

		entry := workspace.Entry{
			Path: p, Kind: kind, Size: state.Size, Mode: state.Mode, SHA256: sum,
			Previous: prev.SHA256,
		}
		switch {
		case capturedFiles >= limits.MaxFiles:
			entry.Warning = fmt.Sprintf("not captured: run reached the %d file capture limit", limits.MaxFiles)
			run.Truncated = true
		case run.CapturedBytes+state.Size > limits.MaxBytes:
			entry.Warning = fmt.Sprintf("not captured: run reached the %s capture limit", humanBytes(limits.MaxBytes))
			run.Truncated = true
		case state.Size > limits.MaxFileBytes:
			entry.Warning = fmt.Sprintf("not captured: file is larger than %s", humanBytes(limits.MaxFileBytes))
			run.Truncated = true
		default:
			n, cerr := copyOut(src, p, out, state.Mode)
			if cerr != nil {
				entry.Warning = "not captured: " + cerr.Error()
				run.Truncated = true
			} else {
				entry.Captured = true
				run.CapturedBytes += n
				capturedFiles++
			}
		}
		run.Entries = append(run.Entries, entry)
		return nil
	})
	if walkErr != nil {
		return run, fmt.Errorf("cannot scan the sandbox workspace: %w", walkErr)
	}

	// Anything the baseline knew about and the scan did not see was deleted.
	for p, prev := range baseline {
		if seen[p] {
			continue
		}
		run.Entries = append(run.Entries, workspace.Entry{
			Path: p, Kind: workspace.Deleted, Previous: prev.SHA256, Mode: prev.Mode,
		})
	}

	run.Sort()
	if len(notes) > 0 {
		run.Note = joinNotes(run.Note, notes)
	}
	if err := writeManifest(out, run); err != nil {
		return run, err
	}
	return run, nil
}

// classify decides whether a non-regular file is news to this run.
func classify(baseline map[string]fileState, p string) workspace.Kind {
	if _, ok := baseline[p]; ok {
		return workspace.Modified
	}
	return workspace.Added
}

func describeMode(m os.FileMode) string {
	switch {
	case m&os.ModeSymlink != 0:
		return "symbolic link"
	case m&os.ModeNamedPipe != 0:
		return "named pipe"
	case m&os.ModeSocket != 0:
		return "socket"
	case m&os.ModeDevice != 0:
		return "device node"
	case m&os.ModeDir != 0:
		return "directory"
	default:
		return "special file"
	}
}

// hashPath reads one workspace file without copying it.
func hashPath(src fs.FS, p string) (string, error) {
	f, err := src.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sum, _, err := hashFile(f)
	return sum, err
}

// copyOut writes one workspace file into the capture root, creating parents.
func copyOut(src fs.FS, p string, out *os.Root, mode uint32) (int64, error) {
	if err := out.MkdirAll(filepath.ToSlash(filepath.Dir(p)), 0o755); err != nil {
		return 0, err
	}
	f, err := src.Open(p)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	dst, err := out.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(mode&0o777))
	if err != nil {
		return 0, err
	}
	// The umask of the helper must not decide whether an executable stays
	// executable: the capture reports the mode the sandbox command set.
	if err := dst.Chmod(os.FileMode(mode & 0o777)); err != nil {
		_ = dst.Close()
		return 0, err
	}
	n, cerr := io.Copy(dst, f)
	if serr := dst.Close(); cerr == nil {
		cerr = serr
	}
	return n, cerr
}

// manifestName is the file the parent reads to learn what a run produced.
const manifestName = "manifest.json"

func writeManifest(out *os.Root, run workspace.Run) error {
	data, err := json.MarshalIndent(run, "", " ")
	if err != nil {
		return err
	}
	f, err := out.OpenFile(manifestName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, werr := f.Write(append(data, '\n')); werr != nil {
		_ = f.Close()
		return werr
	}
	return f.Close()
}

func joinNotes(existing string, notes []string) string {
	if len(notes) == 0 {
		return existing
	}
	sort.Strings(notes)
	added := strings.Join(dedupe(notes), "; ")
	if existing == "" {
		return added
	}
	return existing + "; " + added
}

func dedupe(in []string) []string {
	out := in[:0]
	var last string
	for i, s := range in {
		if i > 0 && s == last {
			continue
		}
		last = s
		out = append(out, s)
	}
	return out
}

// handoff owns the two host directories of one sandbox run: the workspace the
// command starts from and the directory its changes are copied into.
//
// Both descriptors are opened by the helper *before* it enters the jail and are
// created with O_CLOEXEC, so the sandboxed command can reach neither: the only
// descriptor SBT passes to a sandboxed process is stdout/stderr/stdin plus the
// control pipe. This is what keeps "files leave the sandbox" an operation the
// helper performs on its own behalf after the command is gone.
type handoff struct {
	in       *os.Root
	out      *os.Root
	limits   captureLimits
	baseline map[string]fileState
	seeded   int
	notes    []string
	result   workspace.Run
}

// workspacePath returns the workspace directory inside the jail.
func workspacePath(spec *jailspec.Spec) string {
	if spec.WorkspaceDir != "" {
		return spec.WorkspaceDir
	}
	return "/workspace"
}

// openHandoff prepares the workspace handoff described by the spec. A spec
// without WorkspaceIn and WorkspaceOut returns (nil, nil): the run then behaves
// exactly like a sandbox with no workspace review, which is the default for
// callers that do not need it.
func openHandoff(spec *jailspec.Spec) (*handoff, error) {
	if spec.WorkspaceIn == "" && spec.WorkspaceOut == "" {
		return nil, nil
	}
	h := &handoff{
		limits: captureLimits{
			MaxBytes: spec.CaptureLimitBytes,
			MaxFiles: spec.CaptureMaxFiles,
		},
	}
	if spec.WorkspaceIn != "" {
		root, err := os.OpenRoot(spec.WorkspaceIn)
		if err != nil {
			return nil, fmt.Errorf("cannot open the session workspace %s: %w", spec.WorkspaceIn, err)
		}
		h.in = root
	}
	if spec.WorkspaceOut != "" {
		root, err := os.OpenRoot(spec.WorkspaceOut)
		if err != nil {
			h.close()
			return nil, fmt.Errorf("cannot open the capture directory %s: %w", spec.WorkspaceOut, err)
		}
		h.out = root
	}
	return h, nil
}

func (h *handoff) close() {
	if h == nil {
		return
	}
	if h.in != nil {
		_ = h.in.Close()
	}
	if h.out != nil {
		_ = h.out.Close()
	}
}

// seedInto copies the session workspace into the sandbox tmpfs and records the
// baseline the capture step compares against. It is called before the security
// policy is applied, while no command is running yet.
func (h *handoff) seedInto(dst string) error {
	if h == nil || h.in == nil {
		return nil
	}
	baseline, n, notes, err := seedWorkspace(h.in.FS(), dst, h.limits)
	h.baseline = baseline
	h.seeded = n
	h.notes = notes
	return err
}

// captureRun copies the changed files out of the sandbox workspace and writes
// the run manifest into the capture directory. It is called after the command
// has exited, from the helper only: the sandboxed process is gone by then.
func (h *handoff) captureRun(spec *jailspec.Spec, code int, started time.Time) error {
	if h == nil || h.out == nil {
		return nil
	}
	run := workspace.Run{
		SandboxID: spec.SandboxID,
		Command:   spec.Command,
		ExitCode:  code,
		Started:   started.UTC(),
		Finished:  time.Now().UTC(),
		Seeded:    h.seeded,
		Note:      strings.Join(h.notes, "; "),
	}
	baseline := h.baseline
	if baseline == nil {
		baseline = map[string]fileState{}
	}
	out, err := captureWorkspace(os.DirFS(workspacePath(spec)), baseline, h.out, run, h.limits)
	if err != nil {
		return err
	}
	h.result = out
	return nil
}

// Result is the manifest of the captured run, empty when nothing was captured.
func (h *handoff) Result() workspace.Run {
	if h == nil {
		return workspace.Run{}
	}
	return h.result
}
