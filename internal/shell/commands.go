//go:build linux

package shell

import (
	"fmt"
	"strings"
)

func (s *Session) cmdHelp() {
	t := s.theme
	rows := [][2]string{
		{"start <cmd>", "run <cmd> inside a fresh verified sandbox"},
		{"status", "sandbox state, isolation summary and resource settings"},
		{"net on|off", "network isolation for the next sandbox"},
		{"mem <MB>", "memory limit (RLIMIT_AS) for the next sandbox"},
		{"cpu <sec>", "CPU seconds limit (RLIMIT_CPU) for the next sandbox"},
		{"stop", "stop the running sandbox immediately"},
		{"clear", "clear the screen"},
		{"exit", "leave SBT"},
	}
	s.line("")
	for _, r := range rows {
		s.line("  %s %s", t.Yellow(padTo(r[0], 12)), t.Gray(r[1]))
	}
	s.line("")
}

func (s *Session) cmdStatus() {
	t := s.theme
	s.line("")
	state := s.sbxState
	stateLvl := t.Gray
	switch {
	case strings.Contains(s.sbxState, "RUNNING"):
		stateLvl = t.Green
	case strings.Contains(s.sbxState, "FAIL"), strings.Contains(s.sbxState, "UNAVAILABLE"):
		stateLvl = t.Red
	}
	s.line("  %s %s   %s %s", t.Cyan("sandbox:"), stateLvl(state), t.Cyan("mode:"), t.Yellow(s.mode))
	if s.sbx != nil {
		s.line("  %s %s   %s %s", t.Cyan("id:"), t.Text(s.sbx.id), t.Cyan("helper pid:"), t.Text(fmt.Sprint(s.sbx.cmd.Process.Pid)))
	}
	if s.reason != "" {
		s.line("  %s %s", t.Cyan("note:"), t.Gray(s.reason))
	}
	s.line("  %s %d MB   %s %ds   %s %s",
		t.Cyan("memory:"), s.memoryMB, t.Cyan("cpu:"), s.cpuSeconds,
		t.Cyan("network:"), map[bool]string{true: "blocked", false: "allowed"}[s.networkOff])
	s.line("")
}

func (s *Session) cmdStart(args []string) {
	if len(args) == 0 {
		s.line("%s %s", s.theme.Red("usage:"), "start <command>")
		return
	}
	if s.sbx != nil {
		s.line("%s a sandbox is already running; use %s first", s.theme.Yellow("busy:"), s.theme.Text("stop"))
		return
	}
	if err := s.start(args); err != nil {
		s.line("%s %s", s.theme.Red("start failed:"), err.Error())
	}
}

func (s *Session) cmdStop() {
	if s.sbx == nil {
		s.line("%s no sandbox is running", s.theme.Gray("stop:"))
		return
	}
	s.stop()
	s.line("%s sandbox stopped", s.theme.Green("stop:"))
}

func (s *Session) cmdNet(args []string) {
	if len(args) == 0 {
		s.line("  network is %s", map[bool]string{true: "blocked", false: "allowed"}[s.networkOff])
		return
	}
	switch strings.ToLower(args[0]) {
	case "on", "block", "blocked":
		s.networkOff = true
		s.line("%s network will be blocked in the next sandbox", s.theme.Green("net:"))
	case "off", "allow":
		s.networkOff = false
		s.line("%s network will be allowed in the next sandbox", s.theme.Yellow("net:"))
	default:
		s.line("%s usage: net on|off", s.theme.Red("net:"))
	}
}

func (s *Session) cmdMem(args []string) {
	if len(args) == 0 {
		s.line("  memory limit is %d MB", s.memoryMB)
		return
	}
	n, ok := parseInt(args[0])
	if !ok || n < 32 || n > 8192 {
		s.line("%s usage: mem <32..8192> MB", s.theme.Red("mem:"))
		return
	}
	s.memoryMB = n
	s.line("%s memory limit set to %d MB", s.theme.Green("mem:"), n)
}

func (s *Session) cmdCPU(args []string) {
	if len(args) == 0 {
		s.line("  cpu limit is %d seconds", s.cpuSeconds)
		return
	}
	n, ok := parseInt(args[0])
	if !ok || n < 5 || n > 86400 {
		s.line("%s usage: cpu <5..86400> seconds", s.theme.Red("cpu:"))
		return
	}
	s.cpuSeconds = n
	s.line("%s cpu limit set to %d seconds", s.theme.Green("cpu:"), n)
}

func (s *Session) exitShell() {
	if s.sbx != nil {
		s.line("%s stopping sandbox…", s.theme.Gray("exit:"))
		s.stop()
	}
	s.line("%s", s.theme.Gray("bye."))
}

func padTo(s string, n int) string {
	for len(s) < n {
		s += " "
	}
	return s
}

func parseInt(v string) (int64, bool) {
	var n int64
	neg := false
	for i, c := range v {
		if i == 0 && c == '-' {
			neg = true
			continue
		}
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int64(c-'0')
	}
	if neg {
		n = -n
	}
	return n, true
}
