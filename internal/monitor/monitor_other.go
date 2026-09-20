//go:build !linux

package monitor

import "time"

// sample is the fallback used on platforms where SBT cannot read process
// accounting. It reports an unavailable sample instead of zeros that would look
// like a measurement.
func (s *Sampler) sample() Snapshot {
	return Snapshot{
		At:                  time.Now(),
		Err:                 ErrUnsupported.Error(),
		ProcsLimit:          s.procs,
		MemLimitBytes:       s.memLimit,
		WorkspaceLimitBytes: s.wsLimit,
	}
}

// TreePIDs is unavailable outside Linux.
func TreePIDs(int) []int { return nil }

// Host is unavailable outside Linux.
func Host() (int64, int64, int, float64, error) { return 0, 0, 0, 0, ErrUnsupported }

func hostCores() int { return 0 }
