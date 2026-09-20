// Package monitor samples real resource usage for the host and for the sandbox
// process tree.
//
// SBT shows these numbers next to the sandbox status, so they have to be
// measured, never estimated: every value in a Snapshot comes from /proc and a
// field that could not be read stays zero with Snapshot.Err explaining why.
package monitor

import (
	"errors"
	"time"
)

// ErrUnsupported is returned on platforms where SBT cannot sample process
// resource usage.
var ErrUnsupported = errors.New("resource sampling is not implemented on this platform")

// Snapshot is one observation of the host and of the sandbox process tree.
type Snapshot struct {
	// At is when the sample was taken.
	At time.Time
	// Running reports whether a sandbox process tree was found.
	Running bool
	// CPUPercent is the busy percentage of the tree, where 100 means one core
	// fully busy. It is measured against the previous Sample of the tree.
	CPUPercent float64
	// RSSBytes is the resident memory of the whole sandbox tree.
	RSSBytes int64
	// Procs is the number of live processes in the tree.
	Procs int
	// ProcsLimit is the RLIMIT_NPROC budget configured for the sandbox.
	ProcsLimit int64
	// WorkspaceBytes is how much the session workspace currently holds.
	WorkspaceBytes int64
	// WorkspaceLimitBytes is the workspace capacity configured for the sandbox.
	WorkspaceLimitBytes int64
	// MemUsedBytes and MemTotalBytes describe host memory.
	MemUsedBytes  int64
	MemTotalBytes int64
	// MemLimitBytes is the RLIMIT_AS budget configured for the sandbox.
	MemLimitBytes int64
	// CPUCores is the host cpu count.
	CPUCores int
	// LoadAvg1 is the host one minute load average.
	LoadAvg1 float64
	// Err explains why this sample is incomplete, empty when it is complete.
	Err string
}

// Idle reports whether nothing measurable is running.
func (s Snapshot) Idle() bool { return !s.Running }

// Sampler samples one sandbox process tree over time. Its methods are not safe
// for concurrent use; the owner is expected to be a single UI or session loop.
type Sampler struct {
	root      int
	wsDir     string
	wsLimit   int64
	procs     int64
	memLimit  int64
	tick      int64
	cores     int
	prevTicks int64
	prevAt    time.Time
	peak      Snapshot
	havePeak  bool
}

// New returns a Sampler. A root pid <= 0 means "no sandbox yet": use SetRoot
// when one starts.
func New(root int) *Sampler {
	s := &Sampler{root: root, tick: 100}
	s.cores = hostCores()
	return s
}

// SetRoot points the sampler at the sandbox tree root, which is the SBT helper
// process (pid 1 of the sandbox pid namespace). Pass 0 when the sandbox stops.
func (s *Sampler) SetRoot(pid int) {
	if s.root != pid {
		s.prevTicks, s.prevAt = 0, time.Time{}
	}
	s.root = pid
}

// SetWorkspace tells the sampler where the session workspace lives on the host
// and how large the workspace tmpfs is allowed to become.
func (s *Sampler) SetWorkspace(dir string, limitBytes int64) {
	s.wsDir = dir
	s.wsLimit = limitBytes
}

// SetLimits records the sandbox limits so the UI can render "17 / 50" honestly.
func (s *Sampler) SetLimits(procs, memBytes int64) {
	s.procs = procs
	s.memLimit = memBytes
}

// Sample takes one observation. CPUPercent is measured against the previous
// Sample of the same tree, so the first Sample after a sandbox starts reports 0
// instead of inventing a number.
func (s *Sampler) Sample() Snapshot {
	snap := s.sample()
	if snap.Running && s.havePeak {
		if snap.CPUPercent > s.peak.CPUPercent {
			s.peak.CPUPercent = snap.CPUPercent
		}
		if snap.RSSBytes > s.peak.RSSBytes {
			s.peak.RSSBytes = snap.RSSBytes
		}
		if snap.Procs > s.peak.Procs {
			s.peak.Procs = snap.Procs
		}
	}
	if snap.Running && !s.havePeak {
		s.peak = snap
		s.havePeak = true
	}
	s.peak.WorkspaceBytes = snap.WorkspaceBytes
	return snap
}

// Peak returns the high-water marks seen so far, which is what the run summary
// shows once a sandbox has finished.
func (s *Sampler) Peak() Snapshot {
	if !s.havePeak {
		return Snapshot{WorkspaceLimitBytes: s.wsLimit, ProcsLimit: s.procs, MemLimitBytes: s.memLimit}
	}
	peak := s.peak
	peak.ProcsLimit = s.procs
	peak.MemLimitBytes = s.memLimit
	peak.WorkspaceLimitBytes = s.wsLimit
	return peak
}

// ResetPeak clears the high-water marks for a new run.
func (s *Sampler) ResetPeak() {
	s.peak, s.havePeak = Snapshot{}, false
}
