// Package jailspec is the wire format used between the SBT parent process and
// its sandbox helper processes.
//
// The helper (stage 1) creates the namespaces and re-executes itself as stage 2
// inside them; stage 2 builds the jail filesystem, applies the security policy
// and runs the user command. The spec is passed as a JSON file so that it never
// travels through the environment (which is visible to the sandbox).
package jailspec

import "time"

// Bind is one mount instruction performed inside the sandbox mount namespace.
type Bind struct {
	// Source is a host path (or "tmpfs", "proc" for synthetic mounts).
	Source string `json:"source"`
	// Target is the absolute path inside the jail rootfs.
	Target string `json:"target"`
	// FSType is empty for bind mounts.
	FSType string `json:"fstype,omitempty"`
	// ReadOnly requests a read-only bind/mount.
	ReadOnly bool `json:"read_only,omitempty"`
	// Recursive uses MS_BIND|MS_REC (required for host trees with submounts).
	Recursive bool `json:"recursive,omitempty"`
	// SizeBytes is used for tmpfs mounts.
	SizeBytes int64 `json:"size_bytes,omitempty"`
	// ModeBits is used for tmpfs mounts (e.g. 0o1777 for /tmp).
	ModeBits uint32 `json:"mode_bits,omitempty"`
	// Optional binds do not fail the sandbox when they cannot be mounted.
	Optional bool `json:"optional,omitempty"`
	// NoExec/NoSuid/NoDev harden the mount flags.
	NoExec bool `json:"no_exec,omitempty"`
	NoSuid bool `json:"no_suid,omitempty"`
	NoDev  bool `json:"no_dev,omitempty"`
}

// Limits are the resource limits enforced for the sandbox command. Values are
// enforced by the kernel (rlimits) or by the SBT monitor where noted.
type Limits struct {
	// MemoryBytes is enforced per process with RLIMIT_AS.
	MemoryBytes int64 `json:"memory_bytes,omitempty"`
	// CPUQuotaSeconds is enforced per process with RLIMIT_CPU.
	CPUQuotaSeconds int64 `json:"cpu_seconds,omitempty"`
	// FileBytes is the maximum size of a single file (RLIMIT_FSIZE).
	FileBytes int64 `json:"file_bytes,omitempty"`
	// OpenFiles is RLIMIT_NOFILE.
	OpenFiles uint64 `json:"open_files,omitempty"`
	// Processes is RLIMIT_NPROC (per uid, i.e. effectively per sandbox).
	Processes uint64 `json:"processes,omitempty"`
	// CoreDumpBytes 0 disables core dumps.
	CoreDumpBytes int64 `json:"core_dump_bytes,omitempty"`
}

// Spec is the complete description of one sandboxed command execution.
type Spec struct {
	SandboxID string   `json:"sandbox_id"`
	Rootfs    string   `json:"rootfs"`
	Command   []string `json:"command"`
	// Cwd is the working directory *inside* the jail.
	Cwd string `json:"cwd"`
	// Env is the environment for the command (already scrubbed).
	Env []string `json:"env"`
	// Binds are performed in order before chroot.
	Binds []Bind `json:"binds"`
	// NetworkBlocked uses a private network namespace with only loopback.
	NetworkBlocked bool `json:"network_blocked"`
	// Hostname is set inside the UTS namespace.
	Hostname string `json:"hostname"`
	// DropAllCaps removes every capability from the bounding/effective sets.
	DropAllCaps bool `json:"drop_all_caps"`
	// Seccomp installs the SBT syscall deny list.
	Seccomp bool `json:"seccomp"`
	// NoNewPrivs sets PR_SET_NO_NEW_PRIVS (required before seccomp anyway).
	NoNewPrivs bool `json:"no_new_privs"`
	// Limits are applied with setrlimit(2) inside the jail.
	Limits Limits `json:"limits"`
	// Interactive marks commands that need the terminal as-is.
	Interactive bool `json:"interactive,omitempty"`
	// LogPath receives the helper's own diagnostics (never command output).
	LogPath string `json:"log_path,omitempty"`

	// WorkspaceDir is the writable workspace path inside the jail. It defaults
	// to /workspace and must be one of the tmpfs binds in Binds.
	WorkspaceDir string `json:"workspace_dir,omitempty"`
	// WorkspaceIn is a host directory whose contents are copied into
	// WorkspaceDir before the command starts, giving the session a workspace
	// that survives across runs. The helper only ever reads it.
	WorkspaceIn string `json:"workspace_in,omitempty"`
	// WorkspaceOut is a host directory that receives the files the command
	// changed inside WorkspaceDir, plus manifest.json. The helper opens it
	// before entering the jail and the sandboxed command never gets a handle
	// to it, so nothing the command writes can reach the host directly.
	WorkspaceOut string `json:"workspace_out,omitempty"`
	// WorkspaceLimitBytes is the size of the workspace tmpfs. It is reported so
	// the UI can show real capacity without parsing Binds.
	WorkspaceLimitBytes int64 `json:"workspace_limit_bytes,omitempty"`
	// CaptureLimitBytes bounds how much content the helper copies out.
	CaptureLimitBytes int64 `json:"capture_limit_bytes,omitempty"`
	// CaptureMaxFiles bounds how many files the helper copies out. Both limits
	// are reported to the user when they are hit; SBT never silently drops a
	// change.
	CaptureMaxFiles int `json:"capture_max_files,omitempty"`
	// CreatedAt is informative only.
	CreatedAt time.Time `json:"created_at"`
}

// Result is reported by the helper over the control pipe.
type Result struct {
	// Stage is "stage1" or "stage2".
	Stage string `json:"stage"`
	// OK is true once the jail is fully set up and the command was started.
	OK bool `json:"ok"`
	// Reason explains a setup failure in user-facing terms.
	Reason string `json:"reason,omitempty"`
	// Feature is the capability that failed (e.g. "mount_namespace").
	Feature string `json:"feature,omitempty"`
	// Errno carries the raw error for `sbt doctor --verbose`.
	Errno string `json:"errno,omitempty"`
	// ChildPid is the pid of the command inside its pid namespace.
	ChildPid int `json:"child_pid,omitempty"`
}
