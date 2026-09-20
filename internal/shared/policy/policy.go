// Package policy holds the SBT session policy presets.
//
// A preset is not a label: it is the exact set of knobs SBT writes into the
// jail spec. The UI colour follows the preset the user selected, so a "LOW"
// badge can never be shown for a relaxed policy - SBT does not decorate a
// sandbox with a security word it cannot back up.
package policy

import (
	"fmt"
	"strings"
)

// Mode is the trust level the session is currently granting to sandboxes.
type Mode string

// The three session modes.
const (
	// Normal is the idle state: no sandbox is running, so no trust is being
	// granted at all. It is neutral by definition and cannot be "relaxed".
	Normal Mode = "normal"
	// Low is the default sandbox policy: no network, hard limits, seccomp,
	// every capability dropped and the host filesystem read-only.
	Low Mode = "low"
	// High is the explicit relaxation of the default policy. Reaching it
	// requires the user to confirm, because SBT can no longer make the same
	// promises about the sandbox.
	High Mode = "high"
)

// Standard values of the default (Low) policy.
const (
	StdMemoryMB     = 512
	StdCPUSeconds   = 60
	StdWorkspaceMB  = 128
	StdProcesses    = 64
	StdOpenFiles    = 256
	HighMemoryMB    = 2048
	HighCPUSeconds  = 900
	HighWorkspaceMB = 512
	HighProcesses   = 256
)

// Preset is the complete policy applied to the next sandbox.
type Preset struct {
	// Mode is the trust level this preset represents.
	Mode Mode
	// NetworkBlocked puts the sandbox in a private network namespace with
	// only loopback, so it cannot reach the network at all.
	NetworkBlocked bool
	// MemoryMB is the RLIMIT_AS budget in mebibytes.
	MemoryMB int64
	// CPUSeconds is the RLIMIT_CPU budget.
	CPUSeconds int64
	// WorkspaceMB is the size of the writable /workspace tmpfs.
	WorkspaceMB int64
	// Processes is the RLIMIT_NPROC budget.
	Processes int64
	// OpenFiles is the RLIMIT_NOFILE budget.
	OpenFiles uint64
}

// Base returns the default policy. Every session starts here.
func Base() Preset {
	return Preset{
		Mode:           Low,
		NetworkBlocked: true,
		MemoryMB:       StdMemoryMB,
		CPUSeconds:     StdCPUSeconds,
		WorkspaceMB:    StdWorkspaceMB,
		Processes:      StdProcesses,
		OpenFiles:      StdOpenFiles,
	}
}

// Relaxed returns the explicitly weakened policy behind High risk mode.
func Relaxed() Preset {
	return Preset{
		Mode:           High,
		NetworkBlocked: false,
		MemoryMB:       HighMemoryMB,
		CPUSeconds:     HighCPUSeconds,
		WorkspaceMB:    HighWorkspaceMB,
		Processes:      HighProcesses,
		OpenFiles:      StdOpenFiles,
	}
}

// Derive recomputes the mode from the actual knobs. It returns Low while the
// policy stays at or below the standard preset and High as soon as any knob is
// relaxed, which is what keeps the UI badge tied to reality.
func (p Preset) Derive() Preset {
	std := Base()
	relaxed := p.NetworkBlocked == std.NetworkBlocked &&
		p.MemoryMB <= std.MemoryMB &&
		p.CPUSeconds <= std.CPUSeconds &&
		p.WorkspaceMB <= std.WorkspaceMB &&
		p.Processes <= std.Processes &&
		p.OpenFiles <= std.OpenFiles
	if relaxed {
		p.Mode = Low
	} else {
		p.Mode = High
	}
	return p
}

// Label is the badge text for the mode, e.g. "LOW MODE".
func (m Mode) Label() string {
	switch m {
	case Low:
		return "LOW MODE"
	case High:
		return "HIGH RISK"
	default:
		return "NORMAL"
	}
}

// Network renders the network policy as a word.
func (p Preset) Network() string {
	if p.NetworkBlocked {
		return "BLOCKED"
	}
	return "ALLOWED"
}

// Diff lists the knobs in which p relaxes the standard policy. It is the text
// shown in the confirmation dialog before entering High risk mode, so the user
// confirms facts rather than a label.
func (p Preset) Diff() []string {
	std := Base()
	var out []string
	if !p.NetworkBlocked {
		out = append(out, "network access is no longer blocked")
	}
	if p.MemoryMB > std.MemoryMB {
		out = append(out, fmt.Sprintf("memory limit raised to %d MB", p.MemoryMB))
	}
	if p.CPUSeconds > std.CPUSeconds {
		out = append(out, fmt.Sprintf("cpu limit raised to %d s", p.CPUSeconds))
	}
	if p.WorkspaceMB > std.WorkspaceMB {
		out = append(out, fmt.Sprintf("workspace raised to %d MB", p.WorkspaceMB))
	}
	if p.Processes > std.Processes {
		out = append(out, fmt.Sprintf("process limit raised to %d", p.Processes))
	}
	if p.OpenFiles > std.OpenFiles {
		out = append(out, fmt.Sprintf("open file limit raised to %d", p.OpenFiles))
	}
	return out
}

// Summary renders the active policy as a single reviewable line.
func (p Preset) Summary() string {
	return fmt.Sprintf("network %s · %d MB ram · %d s cpu · %d MB workspace · %d proc",
		strings.ToLower(p.Network()), p.MemoryMB, p.CPUSeconds, p.WorkspaceMB, p.Processes)
}
