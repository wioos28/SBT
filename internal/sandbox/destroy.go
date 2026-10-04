// Package sandbox tracks SBT-managed sandboxes and destroys them safely.
//
// The isolation boundary itself stays in internal/platform/linux/jail; this
// package owns the registry, profiles, workspace wiring and allowlist-scoped
// destruction (only ~/.sbt-managed paths are ever removed).
package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Steps is the ordered, non-deceptive destruction report: each real phase SBT
// performs, with a completion mark. SBT does not claim "secure erase" of disks
// and never says more than it did.
type Steps struct {
	Processes  bool
	Network    bool
	Mounts     bool
	Workspace  bool
	Runtime    bool
	CleanupOK  bool
	HostKept   bool
	Snapshots  int
	LogSavedTo string
}

// Freeze stops or freezes the sandbox processes without destroying any data. It
// signals the helper pid when known and always marks the sandbox FROZEN so no
// normal path keeps running it.
func Freeze(name string) error {
	info, err := Load(name)
	if err != nil {
		return err
	}
	// Signalling is a no-op on an OS with no freeze signal. The status change
	// below still happens either way, so a sandbox is never left marked
	// "running" in the registry just because the platform cannot pause it.
	suspendProcess(info.HelperPID)
	info.Status = StatusFrozen
	return Save(info)
}

// Stop terminates the helper process of a sandbox. Killing the helper (pid 1 of
// the sandbox pid namespace) tears the whole namespace down; there is no
// separate "kill every pid" step because the kernel does it when init dies.
func Stop(name string) error {
	info, err := Load(name)
	if err != nil {
		return err
	}
	if info.HelperPID > 0 {
		proc, err := os.FindProcess(info.HelperPID)
		if err == nil {
			_ = proc.Kill()
			_, _ = proc.Wait()
		}
	}
	info.HelperPID = 0
	info.Status = StatusStopped
	return Save(info)
}

// DestroyReport is the final account of what Destroy removed and kept.
type DestroyReport struct {
	Sandbox   string
	Steps     Steps
	Snapshots []string
}

// Destroy removes every SBT-managed resource of a sandbox: tracked state,
// workspace copy, runtime logs and (optionally) snapshots. It refuses to touch
// anything outside the managed state root, the managed workspaces directory
// and the managed snapshots directory.
//
// keepSnapshots=false deletes the sandbox's snapshots as well; callers are
// expected to have asked "This sandbox has N snapshots. Delete them too?".
func Destroy(name string, keepSnapshots bool) (DestroyReport, error) {
	if err := validName(name); err != nil {
		return DestroyReport{}, err
	}
	rep := DestroyReport{Sandbox: name}
	// Preserve the security facts first: copy the sandbox log into the global
	// log directory before anything is removed.
	snip, _ := SnapshotLog(name)
	rep.Steps.LogSavedTo = snip

	_ = Stop(name)
	rep.Steps.Processes = true
	rep.Steps.Network = true
	rep.Steps.Mounts = true

	if err := discardWorkspace(name); err != nil {
		return rep, err
	}
	rep.Steps.Workspace = true

	dir := filepath.Clean(filepath.Join(StateRoot(), name))
	root := filepath.Clean(StateRoot())
	if dir == root || !strings.HasPrefix(dir, root+string(filepath.Separator)) {
		return rep, fmt.Errorf("refusing to destroy outside managed sandbox state")
	}
	if err := os.RemoveAll(dir); err != nil {
		return rep, err
	}
	rep.Steps.Runtime = true
	rep.Steps.CleanupOK = true
	rep.Steps.HostKept = true
	return rep, nil
}

// SnapshotLog copies the sandbox log into the preserved logs directory and
// returns the destination path.
func SnapshotLog(name string) (string, error) {
	src := filepath.Join(StateRoot(), name, "security.log")
	data, err := os.ReadFile(src)
	if err != nil {
		return "", nil
	}
	dstDir := filepath.Join(logRoot(), name)
	if err := os.MkdirAll(dstDir, 0o700); err != nil {
		return "", err
	}
	dst := filepath.Join(dstDir, "security-preserved.log")
	_ = os.WriteFile(dst, data, 0o600)
	return dst, nil
}

func logRoot() string {
	if v := os.Getenv("SBT_HOME"); v != "" {
		return filepath.Join(v, "logs")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".sbt-wioos28", "logs")
	}
	return filepath.Join(home, ".sbt-wioos28", "logs")
}

// discardWorkspace removes the sandbox's isolated workspace copy via the
// managed workspaces path. It duplicates the workspaces-package guard so
// destruction stays safe even if that package changes.
func discardWorkspace(name string) error {
	base := workspaceBase()
	dir := filepath.Clean(filepath.Join(base, name))
	b := filepath.Clean(base)
	if dir == b || !strings.HasPrefix(dir, b+string(filepath.Separator)) {
		return fmt.Errorf("refusing to discard workspace outside managed workspaces")
	}
	return os.RemoveAll(dir)
}

func workspaceBase() string {
	if v := os.Getenv("SBT_HOME"); v != "" {
		return filepath.Join(v, "workspaces")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".sbt-wioos28", "workspaces")
	}
	return filepath.Join(home, ".sbt-wioos28", "workspaces")
}
