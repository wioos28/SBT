//go:build linux

package process

import (
	"os"
	"strconv"
	"strings"
)

func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	_, err := os.Stat("/proc/" + strconv.Itoa(pid))
	return err == nil
}

// rss reads VmRSS from /proc/<pid>/status. A missing file means the process is
// gone; zero is reported, never an estimate.
func rss(pid int) int64 {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				n, _ := strconv.ParseInt(fields[1], 10, 64)
				return n * 1024
			}
		}
	}
	return 0
}
