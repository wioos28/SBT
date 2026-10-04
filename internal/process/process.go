// Package process tracks the processes SBT started: sandbox helpers, AI
// runtimes and adapter commands. It reads /proc on Linux and reports honestly
// on other platforms (no invented CPU numbers).
package process

import (
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

// Kind identifies what a tracked process is.
type Kind string

// Process kinds.
const (
	KindSandbox Kind = "sandbox"
	KindAI      Kind = "ai"
	KindCLI     Kind = "cli"
	KindWeb     Kind = "web"
)

// Entry is one tracked process.
type Entry struct {
	PID       int       `json:"pid"`
	Name      string    `json:"name"`
	Kind      Kind      `json:"kind"`
	Model     string    `json:"model,omitempty"`
	Runtime   string    `json:"runtime,omitempty"`
	Sandbox   string    `json:"sandbox,omitempty"`
	Network   string    `json:"network,omitempty"`
	Security  string    `json:"security,omitempty"`
	StartedAt time.Time `json:"started_at"`
	Command   []string  `json:"command,omitempty"`
}

// Info decorates an entry with a live sample.
type Info struct {
	Entry
	Alive      bool
	MemoryRSS  int64
	CPUPercent float64
}

var (
	mu      sync.Mutex
	tracked = map[int]Entry{}
)

// Register records a process SBT started.
func Register(e Entry) {
	if e.StartedAt.IsZero() {
		e.StartedAt = time.Now()
	}
	mu.Lock()
	tracked[e.PID] = e
	mu.Unlock()
}

// Unregister forgets a process.
func Unregister(pid int) {
	mu.Lock()
	delete(tracked, pid)
	mu.Unlock()
}

// List returns every tracked process with a live sample, sorted by pid.
func List() []Info {
	mu.Lock()
	entries := make([]Entry, 0, len(tracked))
	for _, e := range tracked {
		entries = append(entries, e)
	}
	mu.Unlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i].PID < entries[j].PID })
	out := make([]Info, 0, len(entries))
	for _, e := range entries {
		out = append(out, Info{Entry: e, Alive: alive(e.PID), MemoryRSS: rss(e.PID)})
	}
	return out
}

// Stop terminates a tracked process. As a safety rule it only signals PIDs that
// SBT registered; anything else is refused so `sbt process stop` can never
// become a generic kill switch for the host.
func Stop(pid int) error {
	mu.Lock()
	e, ok := tracked[pid]
	mu.Unlock()
	if !ok {
		return fmt.Errorf("pid %d is not a process SBT started; refusing to signal it", pid)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := proc.Kill(); err != nil {
		return err
	}
	Unregister(e.PID)
	return nil
}
