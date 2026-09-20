//go:build linux

package monitor

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	errBadStat    = errors.New("unexpected /proc/<pid>/stat layout")
	errBadMeminfo = errors.New("/proc/meminfo did not report MemTotal")
)

// procEntry is the subset of /proc/<pid>/stat SBT needs.
type procEntry struct {
	Pid     int
	PPid    int
	UTime   int64
	STime   int64
	Threads int
}

// parseStat decodes one line of /proc/<pid>/stat.
//
// The command name is wrapped in parentheses and may itself contain spaces and
// parentheses ("bash (weird) name"), so the line is split at the *last* ')'.
func parseStat(line string) (procEntry, error) {
	open := strings.IndexByte(line, '(')
	closeIdx := strings.LastIndexByte(line, ')')
	if open < 0 || closeIdx < open {
		return procEntry{}, errBadStat
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line[:open]))
	if err != nil {
		return procEntry{}, errBadStat
	}
	rest := strings.Fields(line[closeIdx+1:])
	// rest[0] is the state field, so man page field N lives at index N-3.
	const off = 3
	if len(rest) < 20-off {
		return procEntry{}, errBadStat
	}
	entry := procEntry{Pid: pid}
	entry.PPid, _ = strconv.Atoi(rest[4-off])
	entry.UTime, _ = strconv.ParseInt(rest[14-off], 10, 64)
	entry.STime, _ = strconv.ParseInt(rest[15-off], 10, 64)
	entry.Threads, _ = strconv.Atoi(rest[20-off])
	return entry, nil
}

// residentBytes reads /proc/<pid>/statm and converts pages to bytes.
func residentBytes(pid int) int64 {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/statm")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0
	}
	pages, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return pages * int64(os.Getpagesize())
}

// readStat reads and parses /proc/<pid>/stat.
func readStat(pid int) (procEntry, error) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return procEntry{}, err
	}
	return parseStat(strings.TrimRight(string(data), "\n"))
}

// TreePIDs returns the live pids of the process tree rooted at root, root
// included. The kernel exposes every process of a nested pid namespace in the
// host /proc, so following parent links is enough to find a sandbox tree.
func TreePIDs(root int) []int {
	if root <= 0 {
		return nil
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	children := map[int][]int{}
	for _, e := range entries {
		pid, cerr := strconv.Atoi(e.Name())
		if cerr != nil {
			continue
		}
		st, serr := readStat(pid)
		if serr != nil {
			continue
		}
		children[st.PPid] = append(children[st.PPid], pid)
	}
	var out []int
	queue := []int{root}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		out = append(out, pid)
		queue = append(queue, children[pid]...)
	}
	return out
}

// sample is the Linux implementation of Sampler.Sample.
func (s *Sampler) sample() Snapshot {
	snap := Snapshot{At: time.Now(), CPUCores: s.cores}
	snap.ProcsLimit = s.procs
	snap.MemLimitBytes = s.memLimit
	snap.WorkspaceLimitBytes = s.wsLimit
	if v, err := workspaceBytes(s.wsDir); err != nil {
		snap.Err = "workspace could not be measured: " + err.Error()
	} else {
		snap.WorkspaceBytes = v
	}
	if total, used, cores, load, err := Host(); err == nil {
		snap.MemTotalBytes, snap.MemUsedBytes, snap.CPUCores, snap.LoadAvg1 = total, used, cores, load
	} else {
		snap.Err = joinErr(snap.Err, "host memory could not be read: "+err.Error())
	}
	if s.root <= 0 {
		return snap
	}

	pids := TreePIDs(s.root)
	if len(pids) == 0 {
		return snap
	}
	snap.Running = true
	snap.Procs = len(pids)
	var ticks int64
	for _, pid := range pids {
		if st, err := readStat(pid); err == nil {
			ticks += st.UTime + st.STime
		}
		snap.RSSBytes += residentBytes(pid)
	}
	now := time.Now()
	if s.prevAt.IsZero() || !now.After(s.prevAt) {
		// First sample of a tree: there is no interval to divide by yet, so
		// cpu usage is reported as zero rather than invented.
		s.prevTicks, s.prevAt = ticks, now
		return snap
	}
	elapsed := now.Sub(s.prevAt).Seconds()
	delta := ticks - s.prevTicks
	s.prevTicks, s.prevAt = ticks, now
	if elapsed > 0 && delta >= 0 && s.tick > 0 {
		snap.CPUPercent = float64(delta) / float64(s.tick) / elapsed * 100
	}
	return snap
}

func hostCores() int { return runtime.NumCPU() }

// Host returns host memory totals, the cpu count and the one minute load
// average, all read from the kernel.
func Host() (total, used int64, cores int, load1 float64, err error) {
	cores = runtime.NumCPU()
	data, rerr := os.ReadFile("/proc/meminfo")
	if rerr != nil {
		return 0, 0, cores, 0, rerr
	}
	var available int64
	for _, line := range strings.Split(string(data), "\n") {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		kb, perr := strconv.ParseInt(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "kB")), 10, 64)
		if perr != nil {
			continue
		}
		switch strings.TrimSpace(name) {
		case "MemTotal":
			total = kb << 10
		case "MemAvailable":
			available = kb << 10
		case "MemFree":
			if available == 0 {
				available = kb << 10
			}
		}
	}
	if total > 0 {
		used = total - available
		if used < 0 {
			used = 0
		}
	}
	if loadData, lerr := os.ReadFile("/proc/loadavg"); lerr == nil {
		if fields := strings.Fields(string(loadData)); len(fields) > 0 {
			load1, _ = strconv.ParseFloat(fields[0], 64)
		}
	}
	if total == 0 {
		return 0, 0, cores, load1, errBadMeminfo
	}
	return total, used, cores, load1, nil
}

// workspaceBytes sums the regular files under dir without following symlinks.
func workspaceBytes(dir string) (int64, error) {
	if dir == "" {
		return 0, nil
	}
	var total int64
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}

func joinErr(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}
